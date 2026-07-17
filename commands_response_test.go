package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequestResponseResolution_UsesAPIKeyAndStrictPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/responses/resolve" {
			t.Fatalf("request = %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("X-ModelRelay-Api-Key") != "mr_sk_test" || request.Header.Get("Authorization") != "" {
			t.Fatalf("unexpected auth headers: %+v", request.Header)
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if len(body) != 2 || body["model"] != "gpt-5.6-sol" || body["provider"] != "openai" {
			t.Fatalf("unexpected request body: %+v", body)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{
          "resolved_model":"gpt-5.6-sol",
          "provider":"openai",
          "pricing":{"availability":"available","content_hash":"sha256:test"}
        }`))
	}))
	defer server.Close()

	response, err := requestResponseResolution(t.Context(), runtimeConfig{
		BaseURL: server.URL,
		APIKey:  "mr_sk_test",
	}, "gpt-5.6-sol", "openai")
	if err != nil {
		t.Fatal(err)
	}
	if response.ResolvedModel != "gpt-5.6-sol" || response.Provider != "openai" || response.Pricing.ContentHash != "sha256:test" {
		t.Fatalf("unexpected response: %+v", response)
	}
}

func TestRequestResponseResolution_UsesBearerWhenAPIKeyAbsent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer account-token" || request.Header.Get("X-ModelRelay-Api-Key") != "" {
			t.Fatalf("unexpected auth headers: %+v", request.Header)
		}
		_, _ = writer.Write([]byte(`{
          "resolved_model":"gpt-5.6-sol",
          "provider":"openai",
          "pricing":{"availability":"available","content_hash":"sha256:test"}
        }`))
	}))
	defer server.Close()

	if _, err := requestResponseResolution(t.Context(), runtimeConfig{
		BaseURL: server.URL,
		Token:   "account-token",
	}, "gpt-5.6-sol", ""); err != nil {
		t.Fatal(err)
	}
}

func TestDataPlaneAuthMode_RequiresCredential(t *testing.T) {
	if _, err := dataPlaneAuthMode(runtimeConfig{}); err == nil {
		t.Fatal("expected missing credential error")
	}
}
