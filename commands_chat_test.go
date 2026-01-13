package main

import "testing"

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
