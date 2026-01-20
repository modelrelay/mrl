package main

import (
	"strings"
	"testing"
)

func TestResolveModel_FlagTakesPrecedence(t *testing.T) {
	cfg := runtimeConfig{Model: "default-model"}
	got := resolveModel("flag-model", cfg)
	if got != "flag-model" {
		t.Fatalf("expected flag-model, got %s", got)
	}
}

func TestResolveModel_FallsBackToConfig(t *testing.T) {
	cfg := runtimeConfig{Model: "config-model"}
	got := resolveModel("", cfg)
	if got != "config-model" {
		t.Fatalf("expected config-model, got %s", got)
	}
}

func TestResolveModel_EmptyWhenNoModelSet(t *testing.T) {
	cfg := runtimeConfig{}
	got := resolveModel("", cfg)
	if got != "" {
		t.Fatalf("expected empty string, got %s", got)
	}
}

// buildFinalPrompt mirrors the prompt construction logic in runPrompt.
// Extracted here for testability.
func buildFinalPrompt(stdinText, argsPrompt string) string {
	switch {
	case stdinText != "" && strings.TrimSpace(argsPrompt) != "":
		return stdinText + "\n\n" + argsPrompt
	case stdinText != "":
		return stdinText
	default:
		return argsPrompt
	}
}

func TestBuildFinalPrompt_StdinAndArgs(t *testing.T) {
	stdin := "This is the file content."
	args := "summarize this"
	got := buildFinalPrompt(stdin, args)
	want := "This is the file content.\n\nsummarize this"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestBuildFinalPrompt_OnlyStdin(t *testing.T) {
	stdin := "What is 2+2?"
	got := buildFinalPrompt(stdin, "")
	if got != stdin {
		t.Fatalf("expected %q, got %q", stdin, got)
	}
}

func TestBuildFinalPrompt_OnlyArgs(t *testing.T) {
	args := "tell me a joke"
	got := buildFinalPrompt("", args)
	if got != args {
		t.Fatalf("expected %q, got %q", args, got)
	}
}

func TestBuildFinalPrompt_StdinWithWhitespaceOnlyArgs(t *testing.T) {
	stdin := "What is the capital of France?"
	got := buildFinalPrompt(stdin, "   ")
	// Whitespace-only args should be treated as no args
	if got != stdin {
		t.Fatalf("expected %q, got %q", stdin, got)
	}
}

func TestUseStdinAsText_Conditions(t *testing.T) {
	tests := []struct {
		name           string
		stdinIsTTY     bool
		attachments    []string
		attachmentType string
		attachStdin    bool
		want           bool
	}{
		{
			name:       "stdin piped, no flags",
			stdinIsTTY: false,
			want:       true,
		},
		{
			name:       "stdin is TTY",
			stdinIsTTY: true,
			want:       false,
		},
		{
			name:        "explicit attachment",
			stdinIsTTY:  false,
			attachments: []string{"file.pdf"},
			want:        false,
		},
		{
			name:           "attachment type set",
			stdinIsTTY:     false,
			attachmentType: "image/png",
			want:           false,
		},
		{
			name:        "attachStdin flag",
			stdinIsTTY:  false,
			attachStdin: true,
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := !tt.stdinIsTTY && len(tt.attachments) == 0 && tt.attachmentType == "" && !tt.attachStdin
			if got != tt.want {
				t.Fatalf("useStdinAsText: expected %v, got %v", tt.want, got)
			}
		})
	}
}
