package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	sdk "github.com/modelrelay/modelrelay/sdk/go"
	"github.com/modelrelay/modelrelay/sdk/go/llm"
)

const (
	customToolDefaultTimeout        = 10 * time.Second
	customToolDefaultMaxOutputBytes = 32_000
)

type customExecTool struct {
	name        sdk.ToolName
	description string
	schema      json.RawMessage
	command     []string
	workDir     string
	timeout     time.Duration
	env         map[string]string
	maxOutput   int
}

type execToolResult struct {
	Stdout          string `json:"stdout"`
	Stderr          string `json:"stderr"`
	ExitCode        int    `json:"exit_code"`
	TimedOut        bool   `json:"timed_out,omitempty"`
	OutputTruncated bool   `json:"output_truncated,omitempty"`
	Error           string `json:"error,omitempty"`
}

func registerCustomTools(registry *sdk.ToolRegistry, toolRoot string, manifest *toolManifest, seen map[sdk.ToolName]struct{}) ([]llm.Tool, error) {
	if manifest == nil || len(manifest.Custom) == 0 {
		return nil, nil
	}
	if registry == nil {
		return nil, errors.New("tool registry required")
	}
	var defs []llm.Tool
	for _, entry := range manifest.Custom {
		tool, def, err := buildCustomExecTool(toolRoot, manifest.sourceDir, entry)
		if err != nil {
			return nil, err
		}
		var errDef error
		defs, errDef = appendToolDefs(defs, seen, def)
		if errDef != nil {
			return nil, errDef
		}
		registry.Register(tool.name, tool.handle)
	}
	return defs, nil
}

func buildCustomExecTool(toolRoot, manifestDir string, entry toolManifestCustom) (*customExecTool, llm.Tool, error) {
	nameRaw := strings.TrimSpace(entry.Name)
	if nameRaw == "" {
		return nil, llm.Tool{}, errors.New("custom tool name is required")
	}
	toolName, err := sdk.ParseToolName(nameRaw)
	if err != nil {
		return nil, llm.Tool{}, fmt.Errorf("invalid custom tool name %q: %w", nameRaw, err)
	}
	if len(entry.Command) == 0 {
		return nil, llm.Tool{}, fmt.Errorf("custom tool %q requires a command", toolName)
	}

	schema, err := resolveCustomSchema(entry, manifestDir)
	if err != nil {
		return nil, llm.Tool{}, fmt.Errorf("custom tool %q schema: %w", toolName, err)
	}

	desc := strings.TrimSpace(entry.Description)
	if desc == "" {
		desc = "Custom tool " + toolName.String()
	}

	def, err := sdk.NewFunctionTool(toolName, desc, schema)
	if err != nil {
		return nil, llm.Tool{}, fmt.Errorf("custom tool %q definition: %w", toolName, err)
	}

	workDir := resolveWorkDir(toolRoot, entry.WorkDir)
	timeout := customToolDefaultTimeout
	if strings.TrimSpace(entry.Timeout) != "" {
		timeout, err = time.ParseDuration(strings.TrimSpace(entry.Timeout))
		if err != nil {
			return nil, llm.Tool{}, fmt.Errorf("custom tool %q timeout: %w", toolName, err)
		}
	}
	maxOutput := customToolDefaultMaxOutputBytes
	if entry.MaxOutputBytes > 0 {
		maxOutput = int(entry.MaxOutputBytes)
	}

	return &customExecTool{
		name:        toolName,
		description: desc,
		schema:      schema,
		command:     append([]string(nil), entry.Command...),
		workDir:     workDir,
		timeout:     timeout,
		env:         entry.Env,
		maxOutput:   maxOutput,
	}, def, nil
}

func resolveCustomSchema(entry toolManifestCustom, manifestDir string) (json.RawMessage, error) {
	if strings.TrimSpace(entry.SchemaFile) != "" {
		path := entry.SchemaFile
		if !filepath.IsAbs(path) {
			path = filepath.Join(manifestDir, path)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		if len(raw) == 0 {
			return nil, errors.New("schema file is empty")
		}
		if !json.Valid(raw) {
			return nil, errors.New("schema file is not valid JSON")
		}
		return json.RawMessage(raw), nil
	}

	if entry.Schema != nil {
		raw, err := json.Marshal(entry.Schema)
		if err != nil {
			return nil, err
		}
		return json.RawMessage(raw), nil
	}

	return json.RawMessage(`{"type":"object"}`), nil
}

func resolveWorkDir(toolRoot, override string) string {
	root := strings.TrimSpace(toolRoot)
	if root == "" {
		root = "."
	}
	if strings.TrimSpace(override) == "" {
		return root
	}
	if filepath.IsAbs(override) {
		return override
	}
	return filepath.Join(root, override)
}

func (t *customExecTool) handle(args map[string]any, _ llm.ToolCall) (any, error) {
	if t == nil {
		return nil, errors.New("custom tool is nil")
	}
	if len(t.command) == 0 {
		return nil, errors.New("custom tool command is empty")
	}

	ctx, cancel := context.WithTimeout(context.Background(), t.timeout)
	defer cancel()

	stdoutBuf := newLimitedBuffer(t.maxOutput, cancel)
	stderrBuf := newLimitedBuffer(t.maxOutput, cancel)

	input, err := json.Marshal(args)
	if err != nil {
		return nil, fmt.Errorf("encode args: %w", err)
	}

	cmd := exec.CommandContext(ctx, t.command[0], t.command[1:]...) //nolint:gosec // tool execution is explicit and user-configured
	cmd.Dir = t.workDir
	cmd.Env = mergeEnv(t.env)
	cmd.Stdin = bytes.NewReader(input)
	cmd.Stdout = stdoutBuf
	cmd.Stderr = stderrBuf

	runErr := cmd.Run()

	res := execToolResult{
		Stdout:          stdoutBuf.String(),
		Stderr:          stderrBuf.String(),
		ExitCode:        0,
		TimedOut:        ctx.Err() == context.DeadlineExceeded,
		OutputTruncated: stdoutBuf.Truncated() || stderrBuf.Truncated(),
	}
	if runErr != nil {
		res.Error = runErr.Error()
		if exitErr := (*exec.ExitError)(nil); errors.As(runErr, &exitErr) {
			res.ExitCode = exitErr.ExitCode()
		} else {
			res.ExitCode = -1
		}
	}

	return res, nil
}

type limitedBuffer struct {
	buf       bytes.Buffer
	maxBytes  int
	truncated bool
	cancel    context.CancelFunc
}

func newLimitedBuffer(maxBytes int, cancel context.CancelFunc) *limitedBuffer {
	if maxBytes <= 0 {
		maxBytes = customToolDefaultMaxOutputBytes
	}
	return &limitedBuffer{maxBytes: maxBytes, cancel: cancel}
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.maxBytes <= 0 {
		return b.buf.Write(p)
	}
	remaining := b.maxBytes - b.buf.Len()
	if remaining <= 0 {
		b.truncated = true
		if b.cancel != nil {
			b.cancel()
		}
		return len(p), nil
	}
	if len(p) > remaining {
		_, _ = b.buf.Write(p[:remaining])
		b.truncated = true
		if b.cancel != nil {
			b.cancel()
		}
		return len(p), nil
	}
	return b.buf.Write(p)
}

func (b *limitedBuffer) String() string {
	return b.buf.String()
}

func (b *limitedBuffer) Truncated() bool {
	return b.truncated
}

func mergeEnv(extra map[string]string) []string {
	if len(extra) == 0 {
		return os.Environ()
	}
	out := os.Environ()
	for k, v := range extra {
		if strings.TrimSpace(k) == "" {
			continue
		}
		out = append(out, k+"="+v)
	}
	return out
}
