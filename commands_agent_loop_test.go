package main

import (
	"testing"

	sdk "github.com/modelrelay/modelrelay/sdk/go"
)

func TestParseLoopTools(t *testing.T) {
	selection, err := parseLoopTools([]string{"bash,tasks.write"}, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !selection.enableBash || !selection.enableTasks {
		t.Fatalf("expected both bash and tasks.write enabled, got %+v", selection)
	}
}

func TestParseLoopToolsUnknown(t *testing.T) {
	_, err := parseLoopTools([]string{"unknown"}, false)
	if err == nil {
		t.Fatal("expected error for unknown tool")
	}
}

func TestParseLoopToolsFS(t *testing.T) {
	selection, err := parseLoopTools([]string{"fs"}, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !selection.enableFS {
		t.Fatalf("expected fs enabled, got %+v", selection)
	}
}

func TestParseLoopToolsAllowEmpty(t *testing.T) {
	selection, err := parseLoopTools(nil, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if selection.enableBash || selection.enableTasks || selection.enableFS {
		t.Fatalf("expected no tools enabled, got %+v", selection)
	}
}

func TestParseBashRules(t *testing.T) {
	rules, err := parseBashRules([]string{"exact:git status", "prefix:go ", "regexp:^ls"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := rules[0].(sdk.BashCommandExact); !ok {
		t.Fatalf("expected exact rule, got %T", rules[0])
	}
	if _, ok := rules[1].(sdk.BashCommandPrefix); !ok {
		t.Fatalf("expected prefix rule, got %T", rules[1])
	}
	if _, ok := rules[2].(sdk.BashCommandRegexp); !ok {
		t.Fatalf("expected regexp rule, got %T", rules[2])
	}
}

func TestTasksWriteArgsValidate(t *testing.T) {
	args := tasksWriteArgs{
		Tasks: []runTask{{Content: "one", Status: runTaskStatusPending}},
	}
	if err := args.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	args.Tasks[0].Status = "nope"
	if err := args.Validate(); err == nil {
		t.Fatal("expected validation error")
	}
}
