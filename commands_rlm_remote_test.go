package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelrelay/modelrelay/platform/rlm"
	sdk "github.com/modelrelay/modelrelay/sdk/go"
)

func TestRLMExecuteRemoteRequest_HasNoLegacyMaxIterationsControl(t *testing.T) {
	payload, err := json.Marshal(rlmExecuteRemoteRequest{Model: "demo", Query: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(payload, []byte("max_iterations")) {
		t.Fatalf("remote request leaked removed control: %s", payload)
	}
	if !bytes.Contains(payload, []byte(`"seed":null`)) {
		t.Fatalf("remote request omitted explicit unavailable seed: %s", payload)
	}
}

func TestRunRLMRemote_RejectsLocalExecutionTimeout(t *testing.T) {
	err := runRLMRemote(
		context.Background(), runtimeConfig{}, sdk.SecretKey("mr_sk_test"), "demo", "hi",
		nil, rlm.ContextPlan{}, &rlmFlags{execTimeoutMS: 1000}, false,
	)
	if err == nil || !strings.Contains(err.Error(), "local-mode only") {
		t.Fatalf("error = %v, want local-only timeout rejection", err)
	}
}

func TestExecuteRLMRemote_Progress(t *testing.T) {
	var (
		gotPath   string
		gotKey    string
		gotClient string
		gotModel  string
		gotQuery  string
		gotSeed   *int64
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotKey = r.Header.Get("X-ModelRelay-Api-Key")
		gotClient = r.Header.Get("X-ModelRelay-Client")

		var req rlmExecuteRemoteRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		gotModel = req.Model
		gotQuery = req.Query
		gotSeed = req.Seed

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"demo","answer":"ok","iterations":1,"subcalls":1,"usage":{},"trajectory":[],"progress":[{"status":"step 1"}]}`))
	}))
	t.Cleanup(server.Close)

	seed := int64(42)
	req := rlmExecuteRemoteRequest{Model: "demo", Query: "hi", Seed: &seed}
	result, err := executeRLMRemote(context.Background(), server.Client(), server.URL, sdk.SecretKey("mr_sk_test"), req)
	if err != nil {
		t.Fatalf("executeRLMRemote error: %v", err)
	}

	if gotPath != "/rlm/execute" {
		t.Fatalf("path = %q, want %q", gotPath, "/rlm/execute")
	}
	if gotKey != "mr_sk_test" {
		t.Fatalf("api key = %q, want %q", gotKey, "mr_sk_test")
	}
	if gotClient == "" {
		t.Fatalf("expected X-ModelRelay-Client header")
	}
	if gotModel != "demo" || gotQuery != "hi" {
		t.Fatalf("request model/query = %q/%q, want demo/hi", gotModel, gotQuery)
	}
	if gotSeed == nil || *gotSeed != 42 {
		t.Fatalf("request seed = %v, want 42", gotSeed)
	}
	if len(result.Progress) != 1 || result.Progress[0].Status != "step 1" {
		t.Fatalf("progress = %+v, want status 'step 1'", result.Progress)
	}
}

func TestValidateRLMRemoteAttachments_RejectsMissingText(t *testing.T) {
	err := validateRLMRemoteAttachments([]rlmFileAttachment{{Name: "data.csv"}})
	if err == nil {
		t.Fatalf("expected error for missing inline text")
	}
}

func TestDoRLMLeaseJSONUsesOneCustomerAuthority(t *testing.T) {
	t.Parallel()

	var gotPath, gotKey, gotClient, gotCustomer string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotKey = r.Header.Get("X-ModelRelay-Api-Key")
		gotClient = r.Header.Get("X-ModelRelay-Client")
		gotCustomer = r.Header.Get("X-ModelRelay-Customer-Id")
		var request rlmLeaseResolutionRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if request.Model != "preset:test" {
			t.Errorf("model = %q", request.Model)
		}
		_, _ = w.Write([]byte(`{"profile":{"selector":"preset:test"}}`))
	}))
	t.Cleanup(server.Close)

	var response struct {
		Profile struct {
			Selector string `json:"selector"`
		} `json:"profile"`
	}
	if err := doRLMLeaseJSON(t.Context(), server.Client(), server.URL, rlmTestProjectAuthority("customer-123"), http.MethodPost, "/rlm/executions/resolve", rlmLeaseResolutionRequest{Model: "preset:test"}, &response); err != nil {
		t.Fatalf("doRLMLeaseJSON: %v", err)
	}
	if gotPath != "/rlm/executions/resolve" || gotKey != "mr_sk_test" || gotClient == "" || gotCustomer != "customer-123" {
		t.Fatalf("request path/key/client/customer = %q/%q/%q/%q", gotPath, gotKey, gotClient, gotCustomer)
	}
	if response.Profile.Selector != "preset:test" {
		t.Fatalf("response selector = %q", response.Profile.Selector)
	}
}

func TestNewRLMLeaseAuthority_RequiresProjectKeyAndCustomerScope(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		config   runtimeConfig
		customer string
		wantErr  string
	}{
		{name: "project key with explicit scope", config: runtimeConfig{APIKey: "mr_sk_test"}, customer: " customer-123 "},
		{name: "project key missing scope", config: runtimeConfig{APIKey: "mr_sk_test"}, wantErr: "--customer is required"},
		{name: "account token is not a customer token", config: runtimeConfig{Token: "account-token"}, wantErr: "project API key required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authority, err := newRLMLeaseAuthority(tt.config, tt.customer)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("newRLMLeaseAuthority: %v", err)
			}
			if authority.apiKey == nil || authority.apiKey.String() != "mr_sk_test" {
				t.Fatalf("api key = %v", authority.apiKey)
			}
			if authority.customerExternalID != "customer-123" {
				t.Fatalf("customer scope = %q", authority.customerExternalID)
			}
		})
	}
}
