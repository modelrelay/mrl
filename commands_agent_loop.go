package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	sdk "github.com/modelrelay/modelrelay/sdk/go"
	"github.com/modelrelay/modelrelay/sdk/go/llm"
	"github.com/spf13/cobra"
)

const toolNameTasksWrite sdk.ToolName = "tasks_write"

type agentLoopFlags struct {
	inputText       string
	inputFile       string
	systemPrompt    string
	model           string
	maxTurns        int
	noTurnLimit     bool
	customerID      string
	toolsFile       string
	tools           []string
	toolRoot        string
	bashAllow       []string
	bashDeny        []string
	bashAllowAll    bool
	bashTimeout     time.Duration
	bashMaxOutBytes uint64
	stateID         string
	stateTTLSeconds int64
	outputPath      string
	trace           bool
	tasksOutputPath string
	printTasks      bool
}

type agentLoopStep struct {
	Turn       int      `json:"turn"`
	ToolCalls  int      `json:"tool_calls"`
	ToolErrors int      `json:"tool_errors"`
	Tools      []string `json:"tools,omitempty"`
}

type agentLoopResult struct {
	Output   string          `json:"output,omitempty"`
	Usage    sdk.AgentUsage  `json:"usage"`
	StateID  string          `json:"state_id,omitempty"`
	Steps    []agentLoopStep `json:"steps,omitempty"`
	Tasks    []runTask       `json:"tasks,omitempty"`
	Response *sdk.Response   `json:"response,omitempty"`
}

func newAgentLoopCmd() *cobra.Command {
	flags := &agentLoopFlags{}
	cmd := &cobra.Command{
		Use:   "loop",
		Short: "Run an agentic tool loop with local tools",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAgentLoop(cmd, args, flags)
		},
	}
	bindAgentLoopFlags(cmd, flags)
	return cmd
}

func bindAgentLoopFlags(cmd *cobra.Command, flags *agentLoopFlags) {
	cmd.Flags().StringVar(&flags.inputText, "input", "", "Inline user input")
	cmd.Flags().StringVar(&flags.inputFile, "input-file", "", "Path to JSON array of input items")
	cmd.Flags().StringVar(&flags.systemPrompt, "system", "", "System prompt")
	cmd.Flags().StringVar(&flags.model, "model", "", "Model ID")
	cmd.Flags().IntVar(&flags.maxTurns, "max-turns", sdk.DefaultMaxTurns, "Max tool loop turns (0 uses default)")
	cmd.Flags().BoolVar(&flags.noTurnLimit, "no-turn-limit", false, "Disable turn limit")
	cmd.Flags().StringVar(&flags.customerID, "customer", "", "Customer ID (allows omitting model)")
	cmd.Flags().StringVar(&flags.toolsFile, "tools-file", "", "Tool manifest file (.toml or .json)")
	cmd.Flags().StringSliceVar(&flags.tools, "tool", nil, "Tool to enable (bash, tasks.write)")
	cmd.Flags().StringVar(&flags.toolRoot, "tool-root", ".", "Root directory for local tools")
	cmd.Flags().StringSliceVar(&flags.bashAllow, "bash-allow", nil, "Allow bash command prefix (repeatable)")
	cmd.Flags().StringSliceVar(&flags.bashDeny, "bash-deny", nil, "Deny bash command prefix (repeatable)")
	cmd.Flags().BoolVar(&flags.bashAllowAll, "bash-allow-all", false, "Allow all bash commands (use with care)")
	cmd.Flags().DurationVar(&flags.bashTimeout, "bash-timeout", 10*time.Second, "Bash tool timeout")
	cmd.Flags().Uint64Var(&flags.bashMaxOutBytes, "bash-max-output-bytes", 32_000, "Bash tool max output bytes")
	cmd.Flags().StringVar(&flags.stateID, "state-id", "", "State handle UUID for stateful tools")
	cmd.Flags().Int64Var(&flags.stateTTLSeconds, "state-ttl-sec", 0, "Create state handle with TTL seconds")
	cmd.Flags().StringVar(&flags.outputPath, "output", "", "Write JSON output to file")
	cmd.Flags().BoolVar(&flags.trace, "trace", false, "Print per-turn tool activity")
	cmd.Flags().StringVar(&flags.tasksOutputPath, "tasks-output", "", "Write tasks list to file (JSON)")
	cmd.Flags().BoolVar(&flags.printTasks, "print-tasks", false, "Print tasks at end")
}

func runAgentLoop(cmd *cobra.Command, args []string, flags *agentLoopFlags) error {
	cfg, err := runtimeConfigFrom(cmd)
	if err != nil {
		return err
	}
	var manifest *toolManifest
	if strings.TrimSpace(flags.toolsFile) != "" {
		loaded, err := loadToolManifest(flags.toolsFile)
		if err != nil {
			return err
		}
		if err := applyToolManifest(flags, loaded, cmd.Flags()); err != nil {
			return err
		}
		manifest = &loaded
	}
	if strings.TrimSpace(flags.model) == "" && strings.TrimSpace(flags.customerID) == "" {
		return errors.New("model is required unless --customer is set")
	}

	client, err := newAgentClient(cfg)
	if err != nil {
		return err
	}

	input, err := resolveInput(flags.inputText, flags.inputFile, args)
	if err != nil {
		return err
	}
	if sys := strings.TrimSpace(flags.systemPrompt); sys != "" {
		input = append([]llm.InputItem{llm.NewSystemText(sys)}, input...)
	}

	tools, registry, taskState, err := buildAgentLoopTools(flags, manifest)
	if err != nil {
		return err
	}

	ctx, cancel := contextWithTimeout(cfg.Timeout)
	defer cancel()

	stateID, stateCreated, err := resolveLoopStateID(ctx, client, flags)
	if err != nil {
		return err
	}

	maxTurns := flags.maxTurns
	if flags.noTurnLimit {
		maxTurns = sdk.NoTurnLimit
	}
	if maxTurns == 0 {
		maxTurns = sdk.DefaultMaxTurns
	}
	if maxTurns < 0 {
		maxTurns = int(^uint(0) >> 1)
	}

	var (
		usage    sdk.AgentUsage
		steps    []agentLoopStep
		lastResp *sdk.Response
		messages = input
		toolDefs = tools
	)

	for turn := 0; turn < maxTurns; turn++ {
		builder := client.Responses.New().
			Input(messages).
			Tools(toolDefs)

		if strings.TrimSpace(flags.customerID) != "" {
			builder = builder.CustomerID(flags.customerID)
		} else {
			builder = builder.Model(sdk.NewModelID(flags.model))
		}
		if stateID != nil {
			builder = builder.StateID(*stateID)
		}

		req, callOpts, err := builder.Build()
		if err != nil {
			return err
		}
		resp, err := client.Responses.Create(ctx, req, callOpts...)
		if err != nil {
			return err
		}

		lastResp = resp
		usage.LLMCalls++
		usage.InputTokens += resp.Usage.InputTokens
		usage.OutputTokens += resp.Usage.OutputTokens
		usage.TotalTokens += resp.Usage.TotalTokens

		toolCalls := resp.ToolCalls()
		if len(toolCalls) == 0 {
			return handleAgentLoopOutput(cfg, resp, usage, steps, taskState, stateID, stateCreated, flags)
		}

		usage.ToolCalls += len(toolCalls)

		step := agentLoopStep{
			Turn:      turn,
			ToolCalls: len(toolCalls),
		}
		for _, call := range toolCalls {
			if call.Function != nil {
				step.Tools = append(step.Tools, call.Function.Name.String())
			}
		}

		messages = append(messages, sdk.AssistantMessageWithToolCalls(resp.AssistantText(), toolCalls))
		results := registry.ExecuteAll(toolCalls)
		for _, res := range results {
			if res.Error != nil {
				step.ToolErrors++
			}
		}
		messages = append(messages, registry.ResultsToMessages(results)...)

		if cfg.Output == outputFormatTable && flags.trace {
			printAgentLoopTrace(step, results)
		}
		if cfg.Output == outputFormatJSON || flags.trace {
			steps = append(steps, step)
		}
	}

	return sdk.AgentMaxTurnsError{
		MaxTurns:     maxTurns,
		LastResponse: lastResp,
		Usage:        usage,
	}
}

func handleAgentLoopOutput(
	cfg runtimeConfig,
	resp *sdk.Response,
	usage sdk.AgentUsage,
	steps []agentLoopStep,
	taskState *tasksState,
	stateID *uuid.UUID,
	stateCreated bool,
	flags *agentLoopFlags,
) error {
	result := agentLoopResult{
		Output:   resp.AssistantText(),
		Usage:    usage,
		Steps:    steps,
		Response: resp,
	}
	if stateID != nil {
		result.StateID = stateID.String()
	}
	if taskState != nil {
		result.Tasks = taskState.Snapshot()
	}

	jsonPayload, _ := json.MarshalIndent(result, "", "  ")
	if flags.outputPath != "" {
		if err := os.MkdirAll(filepath.Dir(flags.outputPath), 0o755); err != nil {
			return fmt.Errorf("failed to create output directory: %w", err)
		}
		if err := os.WriteFile(flags.outputPath, jsonPayload, 0o600); err != nil {
			return fmt.Errorf("failed to write output: %w", err)
		}
	}

	if cfg.Output == outputFormatJSON {
		fmt.Println(string(jsonPayload))
		return nil
	}

	if stateCreated && stateID != nil {
		fmt.Printf("State ID: %s\n", stateID.String())
	}
	if outputText := strings.TrimSpace(result.Output); outputText != "" {
		fmt.Println("Output:\n" + outputText)
	}
	fmt.Printf("Usage: %d LLM calls | %d tool calls | %d tokens\n",
		usage.LLMCalls,
		usage.ToolCalls,
		usage.TotalTokens,
	)

	if flags.printTasks && taskState != nil {
		printTasks(taskState.Snapshot())
	}

	return nil
}

func printAgentLoopTrace(step agentLoopStep, results []sdk.ToolExecutionResult) {
	fmt.Printf("Turn %d: %d tool calls, %d errors\n", step.Turn, step.ToolCalls, step.ToolErrors)
	for i, res := range results {
		status := "ok"
		if res.Error != nil {
			status = "error"
		}
		toolName := res.ToolName.String()
		if toolName == "" && i < len(step.Tools) {
			toolName = step.Tools[i]
		}
		fmt.Printf("- %s (%s)\n", toolName, status)
	}
}

func printTasks(tasks []runTask) {
	if len(tasks) == 0 {
		fmt.Println("Tasks: (none)")
		return
	}
	fmt.Println("Tasks:")
	for _, task := range tasks {
		line := fmt.Sprintf("- [%s] %s", task.Status, task.Content)
		if strings.TrimSpace(task.ActiveForm) != "" {
			line += " (" + strings.TrimSpace(task.ActiveForm) + ")"
		}
		fmt.Println(line)
	}
}

type loopToolSelection struct {
	enableBash  bool
	enableTasks bool
	enableFS    bool
}

func parseLoopTools(values []string, allowEmpty bool) (loopToolSelection, error) {
	flat := splitCSVValues(values)
	if len(flat) == 0 && !allowEmpty {
		return loopToolSelection{}, errors.New("at least one --tool is required (bash, tasks.write, fs)")
	}
	var sel loopToolSelection
	for _, raw := range flat {
		switch strings.ToLower(strings.TrimSpace(raw)) {
		case "bash":
			sel.enableBash = true
		case "tasks_write", "tasks.write", "task.write", "tasks":
			sel.enableTasks = true
		case "fs":
			sel.enableFS = true
		case "":
			continue
		default:
			return loopToolSelection{}, fmt.Errorf("unknown tool %q (supported: bash, tasks_write, fs)", raw)
		}
	}
	return sel, nil
}

func splitCSVValues(values []string) []string {
	var out []string
	for _, raw := range values {
		for _, part := range strings.Split(raw, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			out = append(out, part)
		}
	}
	return out
}

func buildAgentLoopTools(flags *agentLoopFlags, manifest *toolManifest) ([]llm.Tool, *sdk.ToolRegistry, *tasksState, error) {
	allowEmpty := manifest != nil && len(manifest.Custom) > 0
	selection, err := parseLoopTools(flags.tools, allowEmpty)
	if err != nil {
		return nil, nil, nil, err
	}
	if !selection.enableBash {
		if flags.bashAllowAll || len(flags.bashAllow) > 0 || len(flags.bashDeny) > 0 {
			return nil, nil, nil, errors.New("bash flags set but bash tool not enabled (add --tool bash)")
		}
	}
	if !selection.enableTasks {
		if flags.printTasks || strings.TrimSpace(flags.tasksOutputPath) != "" {
			return nil, nil, nil, errors.New("tasks output requested but tasks.write tool not enabled (add --tool tasks.write)")
		}
	}
	if !selection.enableFS && manifest != nil && manifest.FS != nil {
		return nil, nil, nil, errors.New("fs tool config provided but fs tool not enabled (add --tool fs)")
	}

	registry := sdk.NewToolRegistry()
	var defs []llm.Tool
	var taskState *tasksState
	seen := make(map[sdk.ToolName]struct{})

	if selection.enableBash {
		allowRules, err := parseBashRules(flags.bashAllow)
		if err != nil {
			return nil, nil, nil, err
		}
		denyRules, err := parseBashRules(flags.bashDeny)
		if err != nil {
			return nil, nil, nil, err
		}
		if !flags.bashAllowAll && len(allowRules) == 0 {
			return nil, nil, nil, errors.New("bash tool requires --bash-allow or --bash-allow-all")
		}

		opts := []sdk.LocalBashOption{
			sdk.WithLocalBashTimeout(flags.bashTimeout),
			sdk.WithLocalBashMaxOutputBytes(flags.bashMaxOutBytes),
		}
		if flags.bashAllowAll {
			opts = append(opts, sdk.WithLocalBashAllowAllCommands())
		}
		if len(allowRules) > 0 {
			opts = append(opts, sdk.WithLocalBashAllowRules(allowRules...))
		}
		if len(denyRules) > 0 {
			opts = append(opts, sdk.WithLocalBashDenyRules(denyRules...))
		}

		sdk.NewLocalBashToolPack(flags.toolRoot, opts...).RegisterInto(registry)
		defs, err = appendToolDefs(defs, seen, bashToolDefinition())
		if err != nil {
			return nil, nil, nil, err
		}
	}

	if selection.enableTasks {
		taskState = newTasksState(flags.tasksOutputPath)
		registry.Register(toolNameTasksWrite, taskState.handleToolCall)
		defs, err = appendToolDefs(defs, seen, tasksWriteToolDefinition())
		if err != nil {
			return nil, nil, nil, err
		}
	}

	if selection.enableFS {
		fsOptions, err := buildFSToolOptions(manifest)
		if err != nil {
			return nil, nil, nil, err
		}
		sdk.NewLocalFSToolPack(flags.toolRoot, fsOptions...).RegisterInto(registry)
		defs, err = appendToolDefs(defs, seen, fsToolDefinitions()...)
		if err != nil {
			return nil, nil, nil, err
		}
	}

	customDefs, err := registerCustomTools(registry, flags.toolRoot, manifest, seen)
	if err != nil {
		return nil, nil, nil, err
	}
	defs = append(defs, customDefs...)

	if len(defs) == 0 {
		return nil, nil, nil, errors.New("no tools configured")
	}

	return defs, registry, taskState, nil
}

func parseBashRules(values []string) ([]sdk.BashCommandRule, error) {
	flat := splitCSVValues(values)
	rules := make([]sdk.BashCommandRule, 0, len(flat))
	for _, raw := range flat {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		switch {
		case strings.HasPrefix(raw, "exact:"):
			rules = append(rules, sdk.BashCommandExact(strings.TrimPrefix(raw, "exact:")))
		case strings.HasPrefix(raw, "regexp:"):
			expr := strings.TrimPrefix(raw, "regexp:")
			re, err := compileBashRegexp(expr)
			if err != nil {
				return nil, err
			}
			rules = append(rules, sdk.BashCommandRegexp{Re: re})
		case strings.HasPrefix(raw, "prefix:"):
			rules = append(rules, sdk.BashCommandPrefix(strings.TrimPrefix(raw, "prefix:")))
		default:
			rules = append(rules, sdk.BashCommandPrefix(raw))
		}
	}
	return rules, nil
}

func compileBashRegexp(expr string) (*regexp.Regexp, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil, errors.New("regexp rule is empty")
	}
	re, err := regexp.Compile(expr)
	if err != nil {
		return nil, fmt.Errorf("invalid regexp %q: %w", expr, err)
	}
	return re, nil
}

func resolveLoopStateID(ctx context.Context, client *sdk.Client, flags *agentLoopFlags) (*uuid.UUID, bool, error) {
	stateIDRaw := strings.TrimSpace(flags.stateID)
	if stateIDRaw != "" && flags.stateTTLSeconds > 0 {
		return nil, false, errors.New("use either --state-id or --state-ttl-sec, not both")
	}
	if stateIDRaw != "" {
		parsed, err := uuid.Parse(stateIDRaw)
		if err != nil {
			return nil, false, fmt.Errorf("invalid state id: %w", err)
		}
		return &parsed, false, nil
	}
	if flags.stateTTLSeconds <= 0 {
		return nil, false, nil
	}
	req := sdk.StateHandleCreateRequest{TtlSeconds: &flags.stateTTLSeconds}
	resp, err := client.StateHandles.Create(ctx, req)
	if err != nil {
		return nil, false, err
	}
	created := uuid.UUID(resp.Id)
	return &created, true, nil
}

type runTaskStatus string

const (
	runTaskStatusPending    runTaskStatus = "pending"
	runTaskStatusInProgress runTaskStatus = "in_progress"
	runTaskStatusCompleted  runTaskStatus = "completed"
)

type runTask struct {
	Content    string        `json:"content" description:"Task description"`
	Status     runTaskStatus `json:"status" enum:"pending,in_progress,completed" description:"Task status"`
	ActiveForm string        `json:"active_form,omitempty" description:"Present-progress phrasing"`
}

type tasksWriteArgs struct {
	Tasks []runTask `json:"tasks" description:"Complete task list"`
}

func (t tasksWriteArgs) Validate() error {
	for _, task := range t.Tasks {
		if strings.TrimSpace(task.Content) == "" {
			return errors.New("task content is required")
		}
		switch task.Status {
		case runTaskStatusPending, runTaskStatusInProgress, runTaskStatusCompleted:
		default:
			return fmt.Errorf("invalid task status: %s", task.Status)
		}
	}
	return nil
}

type tasksState struct {
	mu         sync.Mutex
	tasks      []runTask
	outputPath string
}

func newTasksState(outputPath string) *tasksState {
	return &tasksState{outputPath: strings.TrimSpace(outputPath)}
}

func (s *tasksState) Snapshot() []runTask {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]runTask(nil), s.tasks...)
}

func (s *tasksState) handleToolCall(args map[string]any, _ llm.ToolCall) (any, error) {
	if s == nil {
		return nil, errors.New("tasks state unavailable")
	}
	payload, err := parseTasksWriteArgs(args)
	if err != nil {
		return nil, err
	}
	if err := payload.Validate(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.tasks = append([]runTask(nil), payload.Tasks...)
	s.mu.Unlock()
	if strings.TrimSpace(s.outputPath) != "" {
		if err := s.writeToFile(); err != nil {
			return nil, err
		}
	}
	return map[string]any{"ok": true}, nil
}

func parseTasksWriteArgs(args map[string]any) (tasksWriteArgs, error) {
	data, err := json.Marshal(args)
	if err != nil {
		return tasksWriteArgs{}, err
	}
	var payload tasksWriteArgs
	if err := json.Unmarshal(data, &payload); err != nil {
		return tasksWriteArgs{}, err
	}
	return payload, nil
}

func (s *tasksState) writeToFile() error {
	if s == nil || strings.TrimSpace(s.outputPath) == "" {
		return nil
	}
	s.mu.Lock()
	payload := struct {
		Tasks []runTask `json:"tasks"`
	}{Tasks: append([]runTask(nil), s.tasks...)}
	s.mu.Unlock()

	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.outputPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(s.outputPath, data, 0o600)
}

type bashToolArgs struct {
	Command string `json:"command" description:"Shell command to execute"`
}

func bashToolDefinition() llm.Tool {
	return sdk.MustFunctionToolFromType[bashToolArgs](sdk.ToolNameBash, "Execute a shell command")
}

func tasksWriteToolDefinition() llm.Tool {
	return sdk.MustFunctionToolFromType[tasksWriteArgs](toolNameTasksWrite, "Update task list")
}
