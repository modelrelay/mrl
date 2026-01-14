package main

import (
	"net/http"
	"testing"

	"github.com/spf13/cobra"
)

func TestResolveOutputFormat(t *testing.T) {
	format, err := resolveOutputFormat(false, "")
	if err != nil {
		t.Fatalf("resolveOutputFormat error: %v", err)
	}
	if format != outputFormatTable {
		t.Fatalf("expected table, got %s", format)
	}

	format, err = resolveOutputFormat(true, "table")
	if err != nil {
		t.Fatalf("resolveOutputFormat error: %v", err)
	}
	if format != outputFormatJSON {
		t.Fatalf("expected json, got %s", format)
	}

	format, err = resolveOutputFormat(false, "json")
	if err != nil {
		t.Fatalf("resolveOutputFormat error: %v", err)
	}
	if format != outputFormatJSON {
		t.Fatalf("expected json, got %s", format)
	}

	if _, err := resolveOutputFormat(false, "bogus"); err == nil {
		t.Fatalf("expected error for invalid format")
	}
}

func TestJoinBaseURL(t *testing.T) {
	tests := []struct {
		base string
		path string
		want string
	}{
		{
			base: "https://api.modelrelay.ai/api/v1",
			path: "/projects",
			want: "https://api.modelrelay.ai/api/v1/projects",
		},
		{
			base: "https://api.modelrelay.ai/api/v1",
			path: "/models?capability=text_generation",
			want: "https://api.modelrelay.ai/api/v1/models?capability=text_generation",
		},
		{
			base: "https://api.modelrelay.ai/api/v1/",
			path: "/models?provider=openai&capability=tools",
			want: "https://api.modelrelay.ai/api/v1/models?provider=openai&capability=tools",
		},
	}
	for _, tc := range tests {
		got, err := joinBaseURL(tc.base, tc.path)
		if err != nil {
			t.Fatalf("joinBaseURL(%q, %q) error: %v", tc.base, tc.path, err)
		}
		if got != tc.want {
			t.Errorf("joinBaseURL(%q, %q) = %q, want %q", tc.base, tc.path, got, tc.want)
		}
	}
}

func TestApplyAuth_APIKey(t *testing.T) {
	cfg := runtimeConfig{APIKey: "mr_sk_test"}
	req, _ := http.NewRequest(http.MethodGet, "https://example.com", nil)
	if err := applyAuth(req, cfg, authModeAPIKey); err != nil {
		t.Fatalf("apply api key: %v", err)
	}
	if got := req.Header.Get("X-ModelRelay-Api-Key"); got != "mr_sk_test" {
		t.Fatalf("expected api key header, got %s", got)
	}
}

func TestResolveRuntimeConfig_Profile(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("profile", "", "")
	cmd.Flags().String("base-url", "", "")
	cmd.Flags().String("project", "", "")
	cmd.Flags().String("api-key", "", "")
	cmd.Flags().Bool("json", false, "")
	cmd.Flags().Duration("timeout", 30, "")

	cfgFile := cliConfig{CurrentProfile: "dev", Profiles: map[string]cliProfile{"dev": {BaseURL: "https://example.com", Output: "json"}}}
	cfg, err := resolveRuntimeConfig(cmd, cfgFile)
	if err != nil {
		t.Fatalf("resolveRuntimeConfig error: %v", err)
	}
	if cfg.BaseURL != "https://example.com" {
		t.Fatalf("expected base url, got %s", cfg.BaseURL)
	}
	if cfg.Output != outputFormatJSON {
		t.Fatalf("expected json output, got %s", cfg.Output)
	}
}
