package main

import (
	"strings"
	"testing"

	"github.com/modelrelay/modelrelay/platform/rlm"
)

func TestRLMSytemPromptIncludesLimits(t *testing.T) {
	prompt := rlm.BuildSystemPrompt(rlm.SystemPromptOptions{
		MaxDepth:    2,
		MaxSubcalls: 10,
	})
	if !strings.Contains(prompt, "max_depth=2") {
		t.Fatalf("prompt missing max_depth: %s", prompt)
	}
	if !strings.Contains(prompt, "max_subcalls=10") {
		t.Fatalf("prompt missing max_subcalls: %s", prompt)
	}
}
