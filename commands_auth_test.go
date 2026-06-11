package main

import (
	"net/http"
	"testing"

	"github.com/spf13/cobra"
)

func TestApplyAuth_Bearer(t *testing.T) {
	cfg := runtimeConfig{Token: "acct_jwt"}
	req, _ := http.NewRequest(http.MethodPost, "https://example.com", nil)
	if err := applyAuth(req, cfg, authModeBearer); err != nil {
		t.Fatalf("apply bearer: %v", err)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer acct_jwt" {
		t.Fatalf("expected bearer header, got %q", got)
	}
}

func TestApplyAuth_BearerMissingToken(t *testing.T) {
	req, _ := http.NewRequest(http.MethodPost, "https://example.com", nil)
	if err := applyAuth(req, runtimeConfig{}, authModeBearer); err == nil {
		t.Fatalf("expected error when token is missing")
	}
}

func TestResolveLoginPassword(t *testing.T) {
	// Flag takes precedence over the environment.
	t.Setenv("MODELRELAY_PASSWORD", "envpass")
	if got, err := resolveLoginPassword("flagpass", false); err != nil || got != "flagpass" {
		t.Fatalf("flag password: got %q err %v", got, err)
	}
	// Falls back to the environment when no flag is set.
	if got, err := resolveLoginPassword("", false); err != nil || got != "envpass" {
		t.Fatalf("env password: got %q err %v", got, err)
	}
	// Errors when neither is available.
	t.Setenv("MODELRELAY_PASSWORD", "")
	if _, err := resolveLoginPassword("", false); err == nil {
		t.Fatalf("expected error when no password provided")
	}
}

func TestResolveRuntimeConfig_Token(t *testing.T) {
	t.Setenv("MODELRELAY_TOKEN", "")
	cmd := &cobra.Command{}
	cmd.Flags().String("profile", "", "")
	cmd.Flags().String("base-url", "", "")
	cmd.Flags().String("project", "", "")
	cmd.Flags().String("api-key", "", "")
	cmd.Flags().String("token", "", "")
	cmd.Flags().Bool("json", false, "")
	cmd.Flags().Duration("timeout", 30, "")

	cfgFile := cliConfig{
		CurrentProfile: "dev",
		Profiles:       map[string]cliProfile{"dev": {BaseURL: "https://example.com", Token: "profile_jwt"}},
	}
	cfg, err := resolveRuntimeConfig(cmd, cfgFile)
	if err != nil {
		t.Fatalf("resolveRuntimeConfig error: %v", err)
	}
	if cfg.Token != "profile_jwt" {
		t.Fatalf("expected token from profile, got %q", cfg.Token)
	}
}
