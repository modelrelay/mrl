package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/modelrelay/modelrelay/platform/rlmrunner"
)

const (
	maxLocalMCPConfigs    = 16
	maxLocalMCPConfigSize = 1 << 20
)

var mcpEnvironmentName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// localMCPConfigFile is ModelRelay's small product envelope around the
// Droste-owned mcp_http configuration. The config object is passed through
// losslessly; secrets map logical Droste references to trusted host environment
// names and is consumed before the runner starts.
type localMCPConfigFile struct {
	Name            string            `json:"name"`
	Config          json.RawMessage   `json:"config"`
	Secrets         map[string]string `json:"secrets,omitempty"`
	AllowedNetworks []string          `json:"allowed_networks,omitempty"`
}

type localMCPSecretKey struct {
	TenantID string
	SourceID string
	Ref      string
}

type localMCPMounts struct {
	Sources         []rlmrunner.DataSourceSpec
	Secrets         map[localMCPSecretKey]string
	SecretEnvNames  []string
	AllowedNetworks map[string][]string
}

func resolveLocalDefaultSource(sources []rlmrunner.DataSourceSpec, existing, override string) (string, error) {
	requested := strings.TrimSpace(override)
	if requested == "" {
		requested = strings.TrimSpace(existing)
	}
	if requested == "" {
		switch len(sources) {
		case 0:
			return "", nil
		case 1:
			return sources[0].Name, nil
		default:
			return "", errors.New("--default-source is required when multiple data sources are mounted")
		}
	}
	for _, source := range sources {
		if source.Name == requested {
			return requested, nil
		}
	}
	return "", fmt.Errorf("--default-source %q does not name a mounted data source", requested)
}

func loadLocalMCPMounts(paths []string) (localMCPMounts, error) {
	if len(paths) == 0 {
		return localMCPMounts{}, nil
	}
	if len(paths) > maxLocalMCPConfigs {
		return localMCPMounts{}, fmt.Errorf("--mcp-config may be repeated at most %d times", maxLocalMCPConfigs)
	}

	result := localMCPMounts{
		Secrets:         make(map[localMCPSecretKey]string),
		AllowedNetworks: make(map[string][]string),
	}
	seenSources := make(map[string]struct{}, len(paths))
	seenEnvNames := make(map[string]struct{})
	for _, rawPath := range paths {
		path := strings.TrimSpace(rawPath)
		if path == "" {
			return localMCPMounts{}, errors.New("--mcp-config requires a file path")
		}
		file, err := os.Open(path)
		if err != nil {
			return localMCPMounts{}, fmt.Errorf("--mcp-config %q: %w", path, err)
		}
		raw, readErr := io.ReadAll(io.LimitReader(file, maxLocalMCPConfigSize+1))
		closeErr := file.Close()
		if readErr != nil {
			return localMCPMounts{}, fmt.Errorf("--mcp-config %q: %w", path, readErr)
		}
		if closeErr != nil {
			return localMCPMounts{}, fmt.Errorf("--mcp-config %q: %w", path, closeErr)
		}
		if len(raw) > maxLocalMCPConfigSize {
			return localMCPMounts{}, fmt.Errorf("--mcp-config %q exceeds %d bytes", path, maxLocalMCPConfigSize)
		}
		var envelope localMCPConfigFile
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&envelope); err != nil {
			return localMCPMounts{}, fmt.Errorf("--mcp-config %q: %w", path, err)
		}
		if err := requireJSONEOF(decoder); err != nil {
			return localMCPMounts{}, fmt.Errorf("--mcp-config %q: %w", path, err)
		}
		name := strings.TrimSpace(envelope.Name)
		if !validMCPSourceName(name) {
			return localMCPMounts{}, fmt.Errorf("--mcp-config %q: name must be a public ASCII Python identifier", path)
		}
		if _, exists := seenSources[name]; exists {
			return localMCPMounts{}, fmt.Errorf("--mcp-config: duplicate source name %q", name)
		}
		seenSources[name] = struct{}{}
		for _, rawNetwork := range envelope.AllowedNetworks {
			prefix, parseErr := netip.ParsePrefix(strings.TrimSpace(rawNetwork))
			if parseErr != nil || prefix != prefix.Masked() || prefix.String() != strings.TrimSpace(rawNetwork) {
				return localMCPMounts{}, fmt.Errorf("--mcp-config %q: allowed_networks must contain canonical CIDRs", path)
			}
			result.AllowedNetworks[name] = append(result.AllowedNetworks[name], prefix.String())
		}

		config, tenantID, refs, err := validateLocalMCPConfig(envelope.Config)
		if err != nil {
			return localMCPMounts{}, fmt.Errorf("--mcp-config %q: %w", path, err)
		}
		if len(refs) != len(envelope.Secrets) {
			return localMCPMounts{}, fmt.Errorf("--mcp-config %q: secrets must map every auth reference exactly", path)
		}
		for _, ref := range refs {
			envName, ok := envelope.Secrets[ref]
			if !ok {
				return localMCPMounts{}, fmt.Errorf("--mcp-config %q: missing environment mapping for secret reference %q", path, ref)
			}
			if !mcpEnvironmentName.MatchString(envName) {
				return localMCPMounts{}, fmt.Errorf("--mcp-config %q: secret reference %q has an invalid environment name", path, ref)
			}
			value, ok := os.LookupEnv(envName)
			if !ok || value == "" {
				return localMCPMounts{}, fmt.Errorf("--mcp-config %q: environment variable %q is unset or empty", path, envName)
			}
			if len(value) > 65536 {
				return localMCPMounts{}, fmt.Errorf("--mcp-config %q: environment variable %q exceeds 64 KiB", path, envName)
			}
			if strings.ContainsAny(value, "\r\n") {
				return localMCPMounts{}, fmt.Errorf("--mcp-config %q: environment variable %q contains a newline", path, envName)
			}
			key := localMCPSecretKey{TenantID: tenantID, SourceID: name, Ref: ref}
			result.Secrets[key] = value
			seenEnvNames[envName] = struct{}{}
		}
		for ref := range envelope.Secrets {
			if !containsString(refs, ref) {
				return localMCPMounts{}, fmt.Errorf("--mcp-config %q: secret mapping %q is not referenced by config.auth", path, ref)
			}
		}
		result.Sources = append(result.Sources, rlmrunner.DataSourceSpec{
			Type:      "mcp_http",
			Name:      name,
			MCPConfig: config,
		})
	}
	for name := range seenEnvNames {
		result.SecretEnvNames = append(result.SecretEnvNames, name)
	}
	sort.Strings(result.SecretEnvNames)
	return result, nil
}

func requireJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("configuration must contain one JSON object")
		}
		return err
	}
	return nil
}

func validMCPSourceName(name string) bool {
	// Droste remains authoritative for its full generated-binding vocabulary.
	// This host-side check only rejects values that can never be public Python
	// identifiers, before any network/session acquisition happens.
	return mcpEnvironmentName.MatchString(name) && !strings.HasPrefix(name, "_") && len(name) <= 64
}

func validateLocalMCPConfig(raw json.RawMessage) (json.RawMessage, string, []string, error) {
	var config map[string]json.RawMessage
	if len(raw) == 0 || json.Unmarshal(raw, &config) != nil || config == nil {
		return nil, "", nil, errors.New("config must be a JSON object")
	}
	var tenantID string
	if value, ok := config["tenant_id"]; !ok || json.Unmarshal(value, &tenantID) != nil || strings.TrimSpace(tenantID) == "" {
		return nil, "", nil, errors.New("config.tenant_id must be a non-empty string")
	}
	tenantID = strings.TrimSpace(tenantID)
	if len(tenantID) > 256 {
		return nil, "", nil, errors.New("config.tenant_id exceeds 256 bytes")
	}
	if rawEffects, ok := config["effects"]; ok {
		var effects map[string]string
		if json.Unmarshal(rawEffects, &effects) != nil || effects == nil {
			return nil, "", nil, errors.New("config.effects must classify tools as strings")
		}
		for tool, effect := range effects {
			if effect != "read" {
				return nil, "", nil, fmt.Errorf("config.effects.%s must be read; mrl has no effectful-tool confirmation policy", tool)
			}
		}
	}

	refs, err := mcpAuthSecretRefs(config["auth"])
	if err != nil {
		return nil, "", nil, err
	}
	cloned := append(json.RawMessage(nil), raw...)
	return cloned, tenantID, refs, nil
}

func mcpAuthSecretRefs(raw json.RawMessage) ([]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var auth map[string]json.RawMessage
	if json.Unmarshal(raw, &auth) != nil || auth == nil {
		return nil, errors.New("config.auth must be an object")
	}
	var authType string
	if value, ok := auth["type"]; !ok || json.Unmarshal(value, &authType) != nil {
		return nil, errors.New("config.auth.type must be a string")
	}
	var fields []string
	switch authType {
	case "none":
		return nil, nil
	case "bearer":
		fields = []string{"token_ref"}
	case "oauth_client_credentials":
		fields = []string{"client_id_ref", "client_secret_ref"}
	default:
		return nil, errors.New("config.auth.type must be none, bearer, or oauth_client_credentials")
	}
	refs := make([]string, 0, len(fields))
	seen := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		var ref string
		if value, ok := auth[field]; !ok || json.Unmarshal(value, &ref) != nil || strings.TrimSpace(ref) == "" {
			return nil, fmt.Errorf("config.auth.%s must be a non-empty secret reference", field)
		}
		ref = strings.TrimSpace(ref)
		if len(ref) > 512 {
			return nil, fmt.Errorf("config.auth.%s exceeds 512 bytes", field)
		}
		if _, exists := seen[ref]; exists {
			return nil, errors.New("config.auth secret references must be distinct")
		}
		seen[ref] = struct{}{}
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	return refs, nil
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

type localMCPSecretBrokerHandler struct {
	token   string
	secrets map[localMCPSecretKey]string
}

func (h *localMCPSecretBrokerHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !validBearerToken(r, h.token) {
		writeSQLBrokerError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var request struct {
		TenantID string `json:"tenant_id"`
		SourceID string `json:"source_id"`
		Ref      string `json:"reference"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeSQLBrokerError(w, http.StatusBadRequest, "invalid MCP secret request")
		return
	}
	if err := requireJSONEOF(decoder); err != nil {
		writeSQLBrokerError(w, http.StatusBadRequest, "invalid MCP secret request")
		return
	}
	secret, ok := h.secrets[localMCPSecretKey{
		TenantID: request.TenantID,
		SourceID: request.SourceID,
		Ref:      request.Ref,
	}]
	if !ok {
		writeSQLBrokerError(w, http.StatusNotFound, "MCP secret reference not found")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	_ = json.NewEncoder(w).Encode(map[string]string{"secret": secret})
}
