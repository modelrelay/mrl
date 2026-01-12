package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

func TestLoadToolManifestTOML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tools.toml")
	content := `
tool_root = "."
	tools = ["bash", "tasks.write", "fs"]
state_id = "550e8400-e29b-41d4-a716-446655440000"
state_ttl_sec = 3600

[bash]
allow = ["git ", "rg "]
deny = ["rm "]
allow_all = false
timeout = "15s"
max_output_bytes = 64000

[tasks_write]
output = "tasks.json"
print = true

[fs]
ignore_dirs = ["node_modules"]
search_timeout = "3s"

[[custom]]
name = "custom.echo"
description = "Echo args"
command = ["cat"]
schema = { type = "object", properties = { message = { type = "string" } }, required = ["message"] }
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	manifest, err := loadToolManifest(path)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}
	if manifest.ToolRoot != "." {
		t.Fatalf("expected tool_root '.', got %q", manifest.ToolRoot)
	}
	if len(manifest.Tools) != 3 {
		t.Fatalf("expected 3 tools, got %d", len(manifest.Tools))
	}
	if manifest.Bash == nil || manifest.Bash.Timeout != "15s" {
		t.Fatalf("expected bash timeout")
	}
	if manifest.TasksWrite == nil || manifest.TasksWrite.Output != "tasks.json" {
		t.Fatalf("expected tasks_write output")
	}
	if manifest.FS == nil || len(manifest.FS.IgnoreDirs) != 1 {
		t.Fatalf("expected fs config")
	}
	if len(manifest.Custom) != 1 {
		t.Fatalf("expected custom tool")
	}
}

func TestLoadToolManifestJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tools.json")
	content := `{
  "tool_root": ".",
  "tools": ["bash"],
  "bash": {
    "allow": ["git "],
    "timeout": "5s"
  }
}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	manifest, err := loadToolManifest(path)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}
	if manifest.Bash == nil || manifest.Bash.Timeout != "5s" {
		t.Fatalf("expected bash timeout")
	}
}

func TestApplyToolManifest(t *testing.T) {
	flags := &agentLoopFlags{}
	cmd := &cobra.Command{}
	bindAgentLoopFlags(cmd, flags)

	if err := cmd.Flags().Set("tool", "bash"); err != nil {
		t.Fatalf("set flag: %v", err)
	}

	allowAll := true
	timeout := "20s"
	maxBytes := uint64(128000)
	ttl := int64(7200)
	manifest := toolManifest{
		ToolRoot:        "/tmp",
		Tools:           []string{"bash", "tasks.write"},
		StateTTLSeconds: &ttl,
		Bash: &toolManifestBash{
			Allow:          []string{"git "},
			AllowAll:       &allowAll,
			Timeout:        timeout,
			MaxOutputBytes: &maxBytes,
		},
		TasksWrite: &toolManifestTasks{
			Output: "tasks.json",
		},
	}

	if err := applyToolManifest(flags, manifest, cmd.Flags()); err != nil {
		t.Fatalf("apply manifest: %v", err)
	}

	if len(flags.tools) != 1 || flags.tools[0] != "bash" {
		t.Fatalf("expected tools from flag to win, got %#v", flags.tools)
	}
	if flags.toolRoot != "/tmp" {
		t.Fatalf("expected tool root to come from manifest")
	}
	if flags.stateTTLSeconds != ttl {
		t.Fatalf("expected state ttl from manifest")
	}
	if flags.bashTimeout != 20*time.Second {
		t.Fatalf("expected bash timeout to be parsed")
	}
	if flags.bashMaxOutBytes != maxBytes {
		t.Fatalf("expected bash max output bytes")
	}
	if flags.tasksOutputPath != "tasks.json" {
		t.Fatalf("expected tasks output path")
	}
}

func TestApplyToolManifestInference(t *testing.T) {
	flags := &agentLoopFlags{}
	cmd := &cobra.Command{}
	bindAgentLoopFlags(cmd, flags)

	manifest := toolManifest{
		Bash:       &toolManifestBash{Allow: []string{"git "}},
		TasksWrite: &toolManifestTasks{Output: "tasks.json"},
		FS:         &toolManifestFS{IgnoreDirs: []string{"node_modules"}},
	}

	if err := applyToolManifest(flags, manifest, cmd.Flags()); err != nil {
		t.Fatalf("apply manifest: %v", err)
	}
	if len(flags.tools) != 3 {
		t.Fatalf("expected inferred tools, got %v", flags.tools)
	}
}

func TestLoadToolManifestUnsupportedExt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tools.yaml")
	if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	_, err := loadToolManifest(path)
	if err == nil {
		t.Fatal("expected error for unsupported extension")
	}
}
