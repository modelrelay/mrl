package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/spf13/pflag"
)

type toolManifest struct {
	ToolRoot        string               `json:"tool_root" toml:"tool_root"`
	Tools           []string             `json:"tools" toml:"tools"`
	StateID         string               `json:"state_id" toml:"state_id"`
	StateTTLSeconds *int64               `json:"state_ttl_sec" toml:"state_ttl_sec"`
	Bash            *toolManifestBash    `json:"bash" toml:"bash"`
	TasksWrite      *toolManifestTasks   `json:"tasks_write" toml:"tasks_write"`
	FS              *toolManifestFS      `json:"fs" toml:"fs"`
	Custom          []toolManifestCustom `json:"custom" toml:"custom"`

	sourceDir string `json:"-" toml:"-"`
}

type toolManifestBash struct {
	Allow          []string `json:"allow" toml:"allow"`
	Deny           []string `json:"deny" toml:"deny"`
	AllowAll       *bool    `json:"allow_all" toml:"allow_all"`
	Timeout        string   `json:"timeout" toml:"timeout"`
	MaxOutputBytes *uint64  `json:"max_output_bytes" toml:"max_output_bytes"`
}

type toolManifestTasks struct {
	Output string `json:"output" toml:"output"`
	Print  *bool  `json:"print" toml:"print"`
}

type toolManifestFS struct {
	IgnoreDirs       []string `json:"ignore_dirs" toml:"ignore_dirs"`
	MaxReadBytes     *uint64  `json:"max_read_bytes" toml:"max_read_bytes"`
	MaxListEntries   *uint64  `json:"max_list_entries" toml:"max_list_entries"`
	MaxSearchBytes   *uint64  `json:"max_search_bytes" toml:"max_search_bytes"`
	MaxSearchMatches *uint64  `json:"max_search_matches" toml:"max_search_matches"`
	SearchTimeout    string   `json:"search_timeout" toml:"search_timeout"`
}

type toolManifestCustom struct {
	Name           string            `json:"name" toml:"name"`
	Description    string            `json:"description" toml:"description"`
	Command        []string          `json:"command" toml:"command"`
	WorkDir        string            `json:"work_dir" toml:"work_dir"`
	Timeout        string            `json:"timeout" toml:"timeout"`
	MaxOutputBytes uint64            `json:"max_output_bytes" toml:"max_output_bytes"`
	Env            map[string]string `json:"env" toml:"env"`
	Schema         any               `json:"schema" toml:"schema"`
	SchemaFile     string            `json:"schema_file" toml:"schema_file"`
}

func loadToolManifest(path string) (toolManifest, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return toolManifest{}, errors.New("tool manifest path required")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return toolManifest{}, err
	}
	if len(raw) == 0 {
		return toolManifest{}, errors.New("tool manifest is empty")
	}
	ext := strings.ToLower(filepath.Ext(path))
	var manifest toolManifest
	switch ext {
	case ".toml":
		fallthrough
	case ".tml":
		if err := toml.Unmarshal(raw, &manifest); err != nil {
			return toolManifest{}, fmt.Errorf("failed to parse toml manifest: %w", err)
		}
	case ".json":
		if err := json.Unmarshal(raw, &manifest); err != nil {
			return toolManifest{}, fmt.Errorf("failed to parse json manifest: %w", err)
		}
	default:
		return toolManifest{}, fmt.Errorf("unsupported manifest extension %q (use .toml or .json)", ext)
	}
	manifest.sourceDir = filepath.Dir(path)
	return manifest, nil
}

func applyToolManifest(flags *agentLoopFlags, manifest toolManifest, flagset *pflag.FlagSet) error {
	if flagset == nil {
		return errors.New("flagset required")
	}
	if !flagset.Changed("tool") {
		switch {
		case len(manifest.Tools) > 0:
			flags.tools = append([]string(nil), manifest.Tools...)
		default:
			inferred := manifest.inferredTools()
			if len(inferred) > 0 {
				flags.tools = inferred
			}
		}
	}
	if !flagset.Changed("tool-root") && strings.TrimSpace(manifest.ToolRoot) != "" {
		flags.toolRoot = strings.TrimSpace(manifest.ToolRoot)
	}
	if !flagset.Changed("state-id") && strings.TrimSpace(manifest.StateID) != "" {
		flags.stateID = strings.TrimSpace(manifest.StateID)
	}
	if !flagset.Changed("state-ttl-sec") && manifest.StateTTLSeconds != nil && *manifest.StateTTLSeconds > 0 {
		flags.stateTTLSeconds = *manifest.StateTTLSeconds
	}

	if manifest.Bash != nil {
		if !flagset.Changed("bash-allow") && len(manifest.Bash.Allow) > 0 {
			flags.bashAllow = append([]string(nil), manifest.Bash.Allow...)
		}
		if !flagset.Changed("bash-deny") && len(manifest.Bash.Deny) > 0 {
			flags.bashDeny = append([]string(nil), manifest.Bash.Deny...)
		}
		if !flagset.Changed("bash-allow-all") && manifest.Bash.AllowAll != nil {
			flags.bashAllowAll = *manifest.Bash.AllowAll
		}
		if !flagset.Changed("bash-timeout") && strings.TrimSpace(manifest.Bash.Timeout) != "" {
			dur, err := time.ParseDuration(strings.TrimSpace(manifest.Bash.Timeout))
			if err != nil {
				return fmt.Errorf("invalid bash timeout %q: %w", manifest.Bash.Timeout, err)
			}
			flags.bashTimeout = dur
		}
		if !flagset.Changed("bash-max-output-bytes") && manifest.Bash.MaxOutputBytes != nil && *manifest.Bash.MaxOutputBytes > 0 {
			flags.bashMaxOutBytes = *manifest.Bash.MaxOutputBytes
		}
	}

	if manifest.TasksWrite != nil {
		if !flagset.Changed("tasks-output") && strings.TrimSpace(manifest.TasksWrite.Output) != "" {
			flags.tasksOutputPath = strings.TrimSpace(manifest.TasksWrite.Output)
		}
		if !flagset.Changed("print-tasks") && manifest.TasksWrite.Print != nil {
			flags.printTasks = *manifest.TasksWrite.Print
		}
	}

	return nil
}

func (m toolManifest) inferredTools() []string {
	var out []string
	if m.Bash != nil {
		out = append(out, "bash")
	}
	if m.TasksWrite != nil {
		out = append(out, "tasks_write")
	}
	if m.FS != nil {
		out = append(out, "fs")
	}
	return out
}
