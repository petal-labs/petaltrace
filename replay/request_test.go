package replay

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/petal-labs/petaltrace/store"
)

// mockStore implements a minimal TraceStore for testing
type mockStore struct {
	runs  map[string]*store.Run
	spans map[string][]*store.Span
}

func newMockStore() *mockStore {
	return &mockStore{
		runs:  make(map[string]*store.Run),
		spans: make(map[string][]*store.Span),
	}
}

func (m *mockStore) GetRun(ctx context.Context, id string) (*store.Run, error) {
	return m.runs[id], nil
}

func (m *mockStore) GetSpanTree(ctx context.Context, runID string) ([]*store.Span, error) {
	return m.spans[runID], nil
}

func (m *mockStore) GetSpansByKind(ctx context.Context, runID string, kind store.SpanKind) ([]*store.Span, error) {
	var result []*store.Span
	for _, span := range m.spans[runID] {
		if span.Kind == kind {
			result = append(result, span)
		}
	}
	return result, nil
}

// Implement other required interface methods with no-ops
func (m *mockStore) CreateRun(ctx context.Context, run *store.Run) error { return nil }
func (m *mockStore) UpdateRun(ctx context.Context, run *store.Run) error { return nil }
func (m *mockStore) DeleteRun(ctx context.Context, id string) error      { return nil }
func (m *mockStore) ListRuns(ctx context.Context, opts store.ListRunsOptions) ([]store.Run, string, error) {
	return nil, "", nil
}
func (m *mockStore) CreateSpan(ctx context.Context, span *store.Span) error         { return nil }
func (m *mockStore) CreateSpanBatch(ctx context.Context, spans []*store.Span) error { return nil }
func (m *mockStore) GetSpan(ctx context.Context, id string) (*store.Span, error)    { return nil, nil }
func (m *mockStore) UpdateSpan(ctx context.Context, span *store.Span) error         { return nil }
func (m *mockStore) UpdateRunAggregates(ctx context.Context, runID string) error    { return nil }
func (m *mockStore) IndexSpanText(ctx context.Context, spanID, promptText, completionText string) error {
	return nil
}
func (m *mockStore) SearchSpans(ctx context.Context, query string, limit int) ([]*store.Span, error) {
	return nil, nil
}
func (m *mockStore) CreateDiff(ctx context.Context, diff *store.RunDiff) error { return nil }
func (m *mockStore) GetDiff(ctx context.Context, id string) (*store.RunDiff, error) {
	return nil, nil
}
func (m *mockStore) GetDiffByRuns(ctx context.Context, baseRunID, compareRunID string) (*store.RunDiff, error) {
	return nil, nil
}
func (m *mockStore) GetPricing(ctx context.Context, provider, model string) (*store.PricingEntry, error) {
	return nil, nil
}
func (m *mockStore) UpsertPricing(ctx context.Context, entry *store.PricingEntry) error { return nil }
func (m *mockStore) ListPricing(ctx context.Context) ([]store.PricingEntry, error)      { return nil, nil }
func (m *mockStore) GetStats(ctx context.Context) (*store.StoreStats, error)            { return nil, nil }
func (m *mockStore) GarbageCollect(ctx context.Context, retentionDays int, dryRun bool) (int, error) {
	return 0, nil
}
func (m *mockStore) Close() error { return nil }

func TestReplayModes(t *testing.T) {
	tests := []struct {
		mode ReplayMode
		want string
	}{
		{ReplayModeLive, "live"},
		{ReplayModeMocked, "mocked"},
		{ReplayModeHybrid, "hybrid"},
	}

	for _, tt := range tests {
		if string(tt.mode) != tt.want {
			t.Errorf("ReplayMode %v = %q, want %q", tt.mode, string(tt.mode), tt.want)
		}
	}
}

func TestReplayStatus(t *testing.T) {
	tests := []struct {
		status ReplayStatus
		want   string
	}{
		{ReplayStatusPending, "pending"},
		{ReplayStatusRunning, "running"},
		{ReplayStatusCompleted, "completed"},
		{ReplayStatusFailed, "failed"},
	}

	for _, tt := range tests {
		if string(tt.status) != tt.want {
			t.Errorf("ReplayStatus %v = %q, want %q", tt.status, string(tt.status), tt.want)
		}
	}
}

func TestNewRequestValidator(t *testing.T) {
	ms := newMockStore()
	v := NewRequestValidator(ms)

	if v == nil {
		t.Fatal("NewRequestValidator() returned nil")
	}
	if v.store == nil {
		t.Error("validator.store is nil")
	}
}

func TestRequestValidator_Validate(t *testing.T) {
	ms := newMockStore()
	v := NewRequestValidator(ms)
	ctx := context.Background()

	tests := []struct {
		name    string
		setup   func()
		req     *ReplayRequest
		wantErr bool
		errMsg  string
	}{
		{
			name:    "empty source run ID",
			setup:   func() {},
			req:     &ReplayRequest{SourceRunID: ""},
			wantErr: true,
			errMsg:  "source_run_id is required",
		},
		{
			name:  "invalid replay mode",
			setup: func() {},
			req: &ReplayRequest{
				SourceRunID: "run-1",
				Mode:        ReplayMode("invalid"),
			},
			wantErr: true,
			errMsg:  "invalid replay mode",
		},
		{
			name:  "source run not found",
			setup: func() {},
			req: &ReplayRequest{
				SourceRunID: "run-not-found",
				Mode:        ReplayModeLive,
			},
			wantErr: true,
			errMsg:  "source run not found",
		},
		{
			name: "cannot replay running run",
			setup: func() {
				ms.runs["run-running"] = &store.Run{
					ID:            "run-running",
					Status:        store.RunStatusRunning,
					GraphSnapshot: json.RawMessage(`{}`),
					InputSnapshot: json.RawMessage(`{}`),
				}
			},
			req: &ReplayRequest{
				SourceRunID: "run-running",
				Mode:        ReplayModeLive,
			},
			wantErr: true,
			errMsg:  "cannot replay a running run",
		},
		{
			name: "missing graph snapshot for live mode",
			setup: func() {
				ms.runs["run-no-graph"] = &store.Run{
					ID:            "run-no-graph",
					Status:        store.RunStatusCompleted,
					GraphSnapshot: nil,
					InputSnapshot: json.RawMessage(`{}`),
				}
			},
			req: &ReplayRequest{
				SourceRunID: "run-no-graph",
				Mode:        ReplayModeLive,
			},
			wantErr: true,
			errMsg:  "missing graph snapshot",
		},
		{
			name: "missing input snapshot for live mode",
			setup: func() {
				ms.runs["run-no-input"] = &store.Run{
					ID:            "run-no-input",
					Status:        store.RunStatusCompleted,
					GraphSnapshot: json.RawMessage(`{}`),
					InputSnapshot: nil,
				}
			},
			req: &ReplayRequest{
				SourceRunID: "run-no-input",
				Mode:        ReplayModeLive,
			},
			wantErr: true,
			errMsg:  "missing input snapshot",
		},
		{
			name: "valid live replay request",
			setup: func() {
				ms.runs["run-valid"] = &store.Run{
					ID:            "run-valid",
					Status:        store.RunStatusCompleted,
					GraphSnapshot: json.RawMessage(`{"nodes": []}`),
					InputSnapshot: json.RawMessage(`{"query": "hello"}`),
				}
			},
			req: &ReplayRequest{
				SourceRunID: "run-valid",
				Mode:        ReplayModeLive,
			},
			wantErr: false,
		},
		{
			name: "valid mocked replay request",
			setup: func() {
				ms.runs["run-mocked"] = &store.Run{
					ID:            "run-mocked",
					Status:        store.RunStatusCompleted,
					GraphSnapshot: json.RawMessage(`{"nodes": []}`),
				}
			},
			req: &ReplayRequest{
				SourceRunID: "run-mocked",
				Mode:        ReplayModeMocked,
			},
			wantErr: false,
		},
		{
			name: "defaults to live mode when mode is empty",
			setup: func() {
				ms.runs["run-default"] = &store.Run{
					ID:            "run-default",
					Status:        store.RunStatusCompleted,
					GraphSnapshot: json.RawMessage(`{}`),
					InputSnapshot: json.RawMessage(`{}`),
				}
			},
			req: &ReplayRequest{
				SourceRunID: "run-default",
				Mode:        "",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()
			_, err := v.Validate(ctx, tt.req)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				} else if tt.errMsg != "" && !containsSubstring(err.Error(), tt.errMsg) {
					t.Errorf("error = %q, want to contain %q", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestRequestValidator_LoadCapturedData(t *testing.T) {
	ms := newMockStore()
	v := NewRequestValidator(ms)
	ctx := context.Background()

	// Setup test data
	ms.runs["run-1"] = &store.Run{
		ID:           "run-1",
		WorkflowName: "test-workflow",
	}
	ms.spans["run-1"] = []*store.Span{
		{ID: "span-1", RunID: "run-1", Kind: store.SpanKindNode},
		{ID: "span-2", RunID: "run-1", Kind: store.SpanKindLLM, LLM: &store.LLMSpanData{Model: "gpt-4"}},
		{ID: "span-3", RunID: "run-1", Kind: store.SpanKindTool, Tool: &store.ToolSpanData{ToolName: "calculator"}},
	}

	tests := []struct {
		name      string
		runID     string
		wantErr   bool
		wantSpans int
		wantLLM   int
		wantTool  int
	}{
		{
			name:    "run not found",
			runID:   "not-found",
			wantErr: true,
		},
		{
			name:      "loads all captured data",
			runID:     "run-1",
			wantErr:   false,
			wantSpans: 3,
			wantLLM:   1,
			wantTool:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			captured, err := v.LoadCapturedData(ctx, tt.runID)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if captured.Run == nil {
				t.Error("captured.Run is nil")
			}

			if len(captured.Spans) != tt.wantSpans {
				t.Errorf("len(Spans) = %d, want %d", len(captured.Spans), tt.wantSpans)
			}

			if len(captured.LLMSpans) != tt.wantLLM {
				t.Errorf("len(LLMSpans) = %d, want %d", len(captured.LLMSpans), tt.wantLLM)
			}

			if len(captured.ToolSpans) != tt.wantTool {
				t.Errorf("len(ToolSpans) = %d, want %d", len(captured.ToolSpans), tt.wantTool)
			}
		})
	}
}

func TestApplyOverrides(t *testing.T) {
	tests := []struct {
		name      string
		captured  *CapturedData
		overrides ReplayOverrides
		wantErr   bool
		check     func(*CapturedData) error
	}{
		{
			name: "no overrides",
			captured: &CapturedData{
				Run: &store.Run{
					GraphSnapshot: json.RawMessage(`{"key": "value"}`),
					InputSnapshot: json.RawMessage(`{"input": "data"}`),
				},
			},
			overrides: ReplayOverrides{},
			wantErr:   false,
		},
		{
			name: "graph overrides merge",
			captured: &CapturedData{
				Run: &store.Run{
					GraphSnapshot: json.RawMessage(`{"key1": "value1"}`),
				},
			},
			overrides: ReplayOverrides{
				GraphOverrides: json.RawMessage(`{"key2": "value2"}`),
			},
			wantErr: false,
			check: func(c *CapturedData) error {
				var graph map[string]any
				if err := json.Unmarshal(c.Run.GraphSnapshot, &graph); err != nil {
					return err
				}
				if graph["key1"] != "value1" {
					return fmt.Errorf("key1 not preserved")
				}
				if graph["key2"] != "value2" {
					return fmt.Errorf("key2 not merged")
				}
				return nil
			},
		},
		{
			name: "input overrides merge",
			captured: &CapturedData{
				Run: &store.Run{
					InputSnapshot: json.RawMessage(`{"query": "hello"}`),
				},
			},
			overrides: ReplayOverrides{
				InputOverrides: json.RawMessage(`{"lang": "en"}`),
			},
			wantErr: false,
			check: func(c *CapturedData) error {
				var input map[string]any
				if err := json.Unmarshal(c.Run.InputSnapshot, &input); err != nil {
					return err
				}
				if input["query"] != "hello" {
					return fmt.Errorf("query not preserved")
				}
				if input["lang"] != "en" {
					return fmt.Errorf("lang not merged")
				}
				return nil
			},
		},
		{
			name: "invalid graph JSON",
			captured: &CapturedData{
				Run: &store.Run{
					GraphSnapshot: json.RawMessage(`invalid json`),
				},
			},
			overrides: ReplayOverrides{
				GraphOverrides: json.RawMessage(`{"key": "value"}`),
			},
			wantErr: true,
		},
		{
			name: "invalid override JSON",
			captured: &CapturedData{
				Run: &store.Run{
					GraphSnapshot: json.RawMessage(`{}`),
				},
			},
			overrides: ReplayOverrides{
				GraphOverrides: json.RawMessage(`invalid`),
			},
			wantErr: true,
		},
		{
			name: "input override with nil input",
			captured: &CapturedData{
				Run: &store.Run{
					InputSnapshot: nil,
				},
			},
			overrides: ReplayOverrides{
				InputOverrides: json.RawMessage(`{"key": "value"}`),
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ApplyOverrides(tt.captured, tt.overrides)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.check != nil {
				if err := tt.check(tt.captured); err != nil {
					t.Errorf("check failed: %v", err)
				}
			}
		})
	}
}

func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstringHelper(s, substr))
}

func containsSubstringHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
