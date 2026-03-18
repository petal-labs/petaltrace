package replay

import (
	"testing"
	"time"
)

func TestNewLiveReplayer(t *testing.T) {
	ms := newMockStore()

	// Test with defaults
	r := NewLiveReplayer(LiveReplayerConfig{
		Store: ms,
	})

	if r == nil {
		t.Fatal("NewLiveReplayer() returned nil")
	}

	if r.petalflowURL != "http://localhost:8080" {
		t.Errorf("default petalflowURL = %q, want %q", r.petalflowURL, "http://localhost:8080")
	}

	if r.requestTimeout != 5*time.Minute {
		t.Errorf("default requestTimeout = %v, want %v", r.requestTimeout, 5*time.Minute)
	}

	if r.httpClient == nil {
		t.Error("httpClient is nil")
	}

	// Test with custom values
	r2 := NewLiveReplayer(LiveReplayerConfig{
		Store:          ms,
		PetalFlowURL:   "http://custom:9000",
		RequestTimeout: 30 * time.Second,
	})

	if r2.petalflowURL != "http://custom:9000" {
		t.Errorf("custom petalflowURL = %q, want %q", r2.petalflowURL, "http://custom:9000")
	}

	if r2.requestTimeout != 30*time.Second {
		t.Errorf("custom requestTimeout = %v, want %v", r2.requestTimeout, 30*time.Second)
	}
}

func TestExecuteRequest_Struct(t *testing.T) {
	temp := 0.7
	maxTokens := 1000

	req := ExecuteRequest{
		WorkflowID:          "wf-123",
		WorkflowName:        "test-workflow",
		Graph:               []byte(`{"nodes": []}`),
		Input:               []byte(`{"query": "hello"}`),
		Config:              []byte(`{}`),
		ParentRunID:         "parent-run",
		Tags:                map[string]string{"env": "test"},
		TriggerSource:       "petaltrace-replay",
		ModelOverride:       "gpt-4o",
		ProviderOverride:    "openai",
		TemperatureOverride: &temp,
		MaxTokensOverride:   &maxTokens,
	}

	if req.WorkflowID != "wf-123" {
		t.Errorf("WorkflowID = %q, want %q", req.WorkflowID, "wf-123")
	}

	if req.ModelOverride != "gpt-4o" {
		t.Errorf("ModelOverride = %q, want %q", req.ModelOverride, "gpt-4o")
	}

	if *req.TemperatureOverride != 0.7 {
		t.Errorf("TemperatureOverride = %f, want %f", *req.TemperatureOverride, 0.7)
	}

	if *req.MaxTokensOverride != 1000 {
		t.Errorf("MaxTokensOverride = %d, want %d", *req.MaxTokensOverride, 1000)
	}
}

func TestExecuteResponse_Struct(t *testing.T) {
	resp := ExecuteResponse{
		RunID:   "run-abc",
		Status:  "completed",
		Error:   "",
		Message: "Workflow executed successfully",
	}

	if resp.RunID != "run-abc" {
		t.Errorf("RunID = %q, want %q", resp.RunID, "run-abc")
	}

	if resp.Status != "completed" {
		t.Errorf("Status = %q, want %q", resp.Status, "completed")
	}

	// Test with error
	errResp := ExecuteResponse{
		RunID:  "",
		Status: "failed",
		Error:  "execution failed",
	}

	if errResp.Error != "execution failed" {
		t.Errorf("Error = %q, want %q", errResp.Error, "execution failed")
	}
}
