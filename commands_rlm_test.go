package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/modelrelay/modelrelay/platform/rlm"
	"github.com/modelrelay/modelrelay/platform/rlmrunner"
	generated "github.com/modelrelay/modelrelay/sdk/go/generated"
)

func TestBuildRLMSystemAdditions_IncludesLimits(t *testing.T) {
	prompt := rlm.BuildRunnerSystemAdditions("", 2, 10)
	if !strings.Contains(prompt, "max_depth=2") {
		t.Fatalf("prompt missing max_depth: %s", prompt)
	}
	if !strings.Contains(prompt, "max_subcalls=10") {
		t.Fatalf("prompt missing max_subcalls: %s", prompt)
	}
	if !strings.Contains(prompt, "answer['content']") {
		t.Fatalf("prompt missing answer guidance: %s", prompt)
	}
}

func TestBuildRLMSystemAdditions_PrependsCustom(t *testing.T) {
	prompt := rlm.BuildRunnerSystemAdditions("Custom", 1, 1)
	if !strings.HasPrefix(prompt, "Custom") {
		t.Fatalf("prompt missing custom prefix: %s", prompt)
	}
}

func TestBuildRLMJSONResult_Success(t *testing.T) {
	dataSourceRequests := 4
	resp := rlmrunner.RunnerResponse{
		Answer:             "42",
		Ready:              true,
		Iterations:         2,
		Subcalls:           3,
		DataSourceRequests: &dataSourceRequests,
		Trajectory: []rlmrunner.RunnerTrajectoryEntry{
			{Iteration: 1, CodeExecuted: "print(1)", ExecutionResult: "1"},
		},
	}
	result, err := buildRLMJSONResult(nil, resp, nil)
	if err != nil {
		t.Fatalf("buildRLMJSONResult: %v", err)
	}
	if result.Error != nil {
		t.Fatalf("expected no error block, got %+v", result.Error)
	}
	if !result.Ready || result.Iterations != 2 || result.Subcalls != 3 {
		t.Fatalf("unexpected result meta: %+v", result)
	}
	if result.DataSourceRequests == nil || *result.DataSourceRequests != dataSourceRequests {
		t.Fatalf("data-source request attribution = %v", result.DataSourceRequests)
	}
	if result.Trajectory.Availability != "unavailable" || result.Trajectory.Reason != "default_no_content_retention" {
		t.Fatalf("trajectory fact = %+v", result.Trajectory)
	}
	var answer string
	if err := json.Unmarshal(result.Answer, &answer); err != nil || answer != "42" {
		t.Fatalf("answer = %s (%v), want 42", result.Answer, err)
	}
}

func TestBuildRLMJSONResult_EmptyAnswerStillPresent(t *testing.T) {
	result, err := buildRLMJSONResult(nil, rlmrunner.RunnerResponse{Answer: "", Ready: true}, nil)
	if err != nil {
		t.Fatalf("buildRLMJSONResult: %v", err)
	}
	if result.Answer == nil {
		t.Fatal("answer field must be present even when empty")
	}
	var answer string
	if err := json.Unmarshal(result.Answer, &answer); err != nil || answer != "" {
		t.Fatalf("answer = %q (%v), want empty string", answer, err)
	}
}

func TestBuildRLMJSONResult_FailureDoesNotLeakTrajectory(t *testing.T) {
	resp := rlmrunner.RunnerResponse{
		Answer:     "draft",
		Ready:      false,
		Iterations: 5,
		Trajectory: []rlmrunner.RunnerTrajectoryEntry{
			{Iteration: 1, CodeExecuted: "x=1", ExecutionResult: ""},
			{Iteration: 2, CodeExecuted: "print(x)", ExecutionResult: "1"},
		},
	}
	result, err := buildRLMJSONResult(nil, resp, errors.New("max iterations exceeded"))
	if err != nil {
		t.Fatalf("buildRLMJSONResult: %v", err)
	}
	if result.Error == nil || result.Error.Message != "max iterations exceeded" {
		t.Fatalf("error = %+v, want max iterations exceeded", result.Error)
	}
	if result.Ready {
		t.Fatal("ready must be false on failure")
	}
	if result.Trajectory.Availability != "unavailable" {
		t.Fatalf("trajectory fact = %+v", result.Trajectory)
	}
	var answer string
	if err := json.Unmarshal(result.Answer, &answer); err != nil || answer != "draft" {
		t.Fatalf("partial answer = %s (%v)", result.Answer, err)
	}
}

func TestBuildRLMJSONResult_RunnerStructuredError(t *testing.T) {
	resp := rlmrunner.RunnerResponse{
		Ready: false,
		Error: &rlmrunner.RunnerError{Type: "SandboxError", Message: "execution timed out"},
	}
	result, err := buildRLMJSONResult(nil, resp, errors.New("runner failed"))
	if err != nil {
		t.Fatalf("buildRLMJSONResult: %v", err)
	}
	if result.Error == nil {
		t.Fatal("expected error block")
	}
	if result.Error.Type != "SandboxError" || result.Error.Message != "execution timed out" {
		t.Fatalf("error = %+v, want SandboxError/execution timed out", result.Error)
	}
}

func TestWriteRLMLocalOutcomeTo_JSONEmitsOnFailure(t *testing.T) {
	var buf bytes.Buffer
	cfg := runtimeConfig{Output: outputFormatJSON}
	resp := rlmrunner.RunnerResponse{
		Answer:     "partial",
		Ready:      false,
		Iterations: 3,
		Trajectory: []rlmrunner.RunnerTrajectoryEntry{
			{Iteration: 1, CodeExecuted: "print('hi')", ExecutionResult: "hi"},
		},
	}
	err := writeRLMLocalOutcomeTo(&buf, cfg, nil, resp, errors.New("max iterations exceeded"))
	if err == nil || err.Error() != "max iterations exceeded" {
		t.Fatalf("return err = %v, want max iterations exceeded", err)
	}
	if buf.Len() == 0 {
		t.Fatal("expected JSON on stdout for --json failure; got empty buffer")
	}
	var decoded rlmJSONResult
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("decode JSON: %v\nraw: %s", err, buf.String())
	}
	if decoded.Error == nil || decoded.Error.Message != "max iterations exceeded" {
		t.Fatalf("decoded error = %+v", decoded.Error)
	}
	if decoded.Trajectory.Availability != "unavailable" {
		t.Fatalf("decoded trajectory = %+v", decoded.Trajectory)
	}
}

func TestWriteRLMLocalOutcomeWithEvidenceTo_JSONIncludesReturnedModelFacts(t *testing.T) {
	t.Parallel()

	rootValue := "provider/root-v1"
	missing := generated.RLMReturnedModelFactReason("missing_observation")
	evidence := &generated.RLMRetrievedExecutionEvidence{
		Version: generated.ModelrelayRlmExecutionEvidenceViewV3,
		RootReturnedModel: generated.RLMReturnedModelFact{
			Availability: generated.RLMReturnedModelFactAvailability("available"), Value: &rootValue,
		},
		SubcallReturnedModel: generated.RLMReturnedModelFact{
			Availability: generated.RLMReturnedModelFactAvailability("unavailable"), Reason: &missing,
		},
	}
	var buf bytes.Buffer
	if err := writeRLMLocalOutcomeWithEvidenceTo(&buf, runtimeConfig{Output: outputFormatJSON}, nil,
		rlmrunner.RunnerResponse{Answer: "answer", Ready: true}, nil, evidence); err != nil {
		t.Fatal(err)
	}
	var result rlmJSONResult
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.ExecutionEvidence == nil || result.ExecutionEvidence.Version != generated.ModelrelayRlmExecutionEvidenceViewV3 ||
		result.ExecutionEvidence.RootReturnedModel.Value == nil || *result.ExecutionEvidence.RootReturnedModel.Value != rootValue ||
		result.ExecutionEvidence.SubcallReturnedModel.Reason == nil || *result.ExecutionEvidence.SubcallReturnedModel.Reason != missing {
		t.Fatalf("execution evidence lost returned-model facts: %+v", result.ExecutionEvidence)
	}
}

func TestWriteRLMLocalOutcomeTo_TextFailureWritesNothing(t *testing.T) {
	var buf bytes.Buffer
	cfg := runtimeConfig{Output: ""}
	err := writeRLMLocalOutcomeTo(&buf, cfg, nil, rlmrunner.RunnerResponse{Answer: "x"}, errors.New("boom"))
	if err == nil || err.Error() != "boom" {
		t.Fatalf("return err = %v, want boom", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("text mode failure should not write answer, got %q", buf.String())
	}
}
