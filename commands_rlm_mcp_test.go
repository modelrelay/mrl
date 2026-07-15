package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelrelay/modelrelay/platform/rlmrunner"
)

func writeMCPConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "mcp.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadLocalMCPMounts_BearerSecretStaysInHostBroker(t *testing.T) {
	t.Setenv("DOCS_MCP_TOKEN", "top-secret-value")
	path := writeMCPConfig(t, `{
  "name": "docs",
  "config": {
    "endpoint": "https://mcp.example.com/v1",
    "allowed_endpoints": ["https://mcp.example.com/v1"],
    "tenant_id": "project-123",
    "auth": {"type": "bearer", "token_ref": "docs-access"},
    "allowed_tools": ["search"],
    "bindings": {"search": "search"},
    "effects": {"search": "read"},
    "budget_classes": {"search": "data.read"}
  },
  "secrets": {"docs-access": "DOCS_MCP_TOKEN"}
}`)

	mounts, err := loadLocalMCPMounts([]string{path})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if len(mounts.Sources) != 1 || mounts.Sources[0].Type != "mcp_http" || mounts.Sources[0].Name != "docs" {
		t.Fatalf("unexpected sources: %#v", mounts.Sources)
	}
	if len(mounts.SecretEnvNames) != 1 || mounts.SecretEnvNames[0] != "DOCS_MCP_TOKEN" {
		t.Fatalf("unexpected secret env names: %#v", mounts.SecretEnvNames)
	}
	encoded, err := json.Marshal(mounts.Sources[0])
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte("top-secret-value")) || bytes.Contains(encoded, []byte("DOCS_MCP_TOKEN")) {
		t.Fatalf("runner spec leaked secret material or environment locator: %s", encoded)
	}
	if !bytes.Contains(encoded, []byte(`"token_ref":"docs-access"`)) {
		t.Fatalf("runner spec lost logical secret reference: %s", encoded)
	}
	key := localMCPSecretKey{TenantID: "project-123", SourceID: "docs", Ref: "docs-access"}
	if mounts.Secrets[key] != "top-secret-value" {
		t.Fatal("trusted host did not retain the scoped secret")
	}
	runnerEnv := []string{"PATH=/usr/bin", "DOCS_MCP_TOKEN=top-secret-value", "UNRELATED=value"}
	for _, envName := range mounts.SecretEnvNames {
		runnerEnv = environmentWithoutVariable(runnerEnv, envName)
	}
	joined := strings.Join(runnerEnv, "\n")
	if strings.Contains(joined, "top-secret-value") || !strings.Contains(joined, "UNRELATED=value") {
		t.Fatalf("runner environment secret filtering failed: %q", joined)
	}
}

func TestLoadLocalMCPMounts_RequiresExactSecretReferenceMappings(t *testing.T) {
	t.Setenv("DOCS_MCP_TOKEN", "secret")
	path := writeMCPConfig(t, `{
  "name":"docs",
  "config":{"tenant_id":"p1","auth":{"type":"bearer","token_ref":"wanted"}},
  "secrets":{"other":"DOCS_MCP_TOKEN"}
}`)
	_, err := loadLocalMCPMounts([]string{path})
	if err == nil || !strings.Contains(err.Error(), "missing environment mapping") {
		t.Fatalf("expected exact-reference error, got %v", err)
	}
}

func TestLoadLocalMCPMounts_RejectsInlineOrUnknownEnvelopeFields(t *testing.T) {
	path := writeMCPConfig(t, `{"name":"docs","config":{"tenant_id":"p1"},"token":"inline-secret"}`)
	_, err := loadLocalMCPMounts([]string{path})
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("expected unknown field rejection, got %v", err)
	}
}

func TestLoadLocalMCPMounts_OAuthRequiresBothScopedSecretMappings(t *testing.T) {
	t.Setenv("DOCS_CLIENT_ID", "client-id")
	t.Setenv("DOCS_CLIENT_SECRET", "client-secret")
	path := writeMCPConfig(t, `{
  "name":"docs",
  "config":{
    "tenant_id":"project-1",
    "auth":{
      "type":"oauth_client_credentials",
      "client_id_ref":"oauth-id",
      "client_secret_ref":"oauth-secret"
    }
  },
  "secrets":{"oauth-id":"DOCS_CLIENT_ID","oauth-secret":"DOCS_CLIENT_SECRET"},
  "allowed_networks":["10.40.8.0/24"]
}`)
	mounts, err := loadLocalMCPMounts([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	if len(mounts.Secrets) != 2 || len(mounts.AllowedNetworks["docs"]) != 1 || mounts.AllowedNetworks["docs"][0] != "10.40.8.0/24" {
		t.Fatalf("unexpected OAuth mount: %#v", mounts)
	}
}

func TestLoadLocalMCPMounts_RejectsNonCanonicalPrivateNetwork(t *testing.T) {
	path := writeMCPConfig(t, `{
  "name":"docs",
  "config":{"tenant_id":"project-1","auth":{"type":"none"}},
  "allowed_networks":["10.40.8.1/24"]
}`)
	_, err := loadLocalMCPMounts([]string{path})
	if err == nil || !strings.Contains(err.Error(), "canonical CIDRs") {
		t.Fatalf("expected canonical CIDR rejection, got %v", err)
	}
}

func TestLoadLocalMCPMounts_UnauthenticatedMountKeepsBrokerEnabled(t *testing.T) {
	path := writeMCPConfig(t, `{
  "name":"docs",
  "config":{"tenant_id":"project-1","auth":{"type":"none"}}
}`)
	mounts, err := loadLocalMCPMounts([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	if mounts.Secrets == nil || len(mounts.Secrets) != 0 {
		t.Fatalf("unauthenticated MCP mount lost enabled broker state: %#v", mounts.Secrets)
	}
}

func TestLoadLocalMCPMounts_RejectsEffectfulToolsWithoutConfirmationPolicy(t *testing.T) {
	path := writeMCPConfig(t, `{
  "name":"tickets",
  "config":{
    "tenant_id":"project-1",
    "auth":{"type":"none"},
    "effects":{"close_ticket":"effectful"}
  }
}`)
	_, err := loadLocalMCPMounts([]string{path})
	if err == nil || !strings.Contains(err.Error(), "no effectful-tool confirmation policy") {
		t.Fatalf("expected effectful tool rejection, got %v", err)
	}
}

func TestLocalMCPSecretBroker_IsolatesTenantSourceAndReference(t *testing.T) {
	handler := &localMCPSecretBrokerHandler{
		token: "runner-capability",
		secrets: map[localMCPSecretKey]string{
			{TenantID: "tenant-a", SourceID: "docs", Ref: "access"}: "secret-a",
			{TenantID: "tenant-b", SourceID: "docs", Ref: "access"}: "secret-b",
		},
	}
	request := func(token, tenant string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/mcp/secret", strings.NewReader(`{"tenant_id":"`+tenant+`","source_id":"docs","reference":"access"}`))
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}
	if rec := request("wrong", "tenant-a"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong token status = %d", rec.Code)
	}
	if rec := request("runner-capability", "tenant-c"); rec.Code != http.StatusNotFound || strings.Contains(rec.Body.String(), "secret-") {
		t.Fatalf("cross-tenant lookup leaked: %d %s", rec.Code, rec.Body.String())
	}
	rec := request("runner-capability", "tenant-b")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "secret-b") || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("authorized lookup failed: %d %s", rec.Code, rec.Body.String())
	}
}

func TestResolveLocalDefaultSource_MultipleMCPRequiresExplicitDefault(t *testing.T) {
	_, err := resolveLocalDefaultSource(
		[]rlmrunner.DataSourceSpec{{Name: "docs"}, {Name: "tickets"}}, "", "",
	)
	if err == nil || !strings.Contains(err.Error(), "--default-source") {
		t.Fatalf("expected explicit default error, got %v", err)
	}
	got, err := resolveLocalDefaultSource(
		[]rlmrunner.DataSourceSpec{{Name: "docs"}, {Name: "tickets"}}, "", "tickets",
	)
	if err != nil || got != "tickets" {
		t.Fatalf("explicit default = %q, %v", got, err)
	}
}
