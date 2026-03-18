package replay

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/petal-labs/petaltrace/store"
)

// ReplayMode defines how the replay should be executed
type ReplayMode string

const (
	// ReplayModeLive re-executes against real LLM providers
	ReplayModeLive ReplayMode = "live"
	// ReplayModeMocked uses captured responses from the source run
	ReplayModeMocked ReplayMode = "mocked"
	// ReplayModeHybrid mocks tools but makes live LLM calls
	ReplayModeHybrid ReplayMode = "hybrid"
)

// ReplayRequest defines a replay operation
type ReplayRequest struct {
	// SourceRunID is the run to replay
	SourceRunID string `json:"source_run_id"`

	// Mode determines how the replay executes
	Mode ReplayMode `json:"mode"`

	// Overrides for replay execution
	Overrides ReplayOverrides `json:"overrides,omitempty"`

	// Tags to add to the new run
	Tags map[string]string `json:"tags,omitempty"`

	// AutoDiff triggers a diff after completion
	AutoDiff bool `json:"auto_diff,omitempty"`
}

// ReplayOverrides contains configuration overrides for replay
type ReplayOverrides struct {
	// Model overrides the LLM model
	Model string `json:"model,omitempty"`

	// Temperature overrides the sampling temperature
	Temperature *float64 `json:"temperature,omitempty"`

	// MaxTokens overrides the max tokens
	MaxTokens *int `json:"max_tokens,omitempty"`

	// Provider overrides the LLM provider
	Provider string `json:"provider,omitempty"`

	// GraphOverrides contains modifications to the graph
	GraphOverrides json.RawMessage `json:"graph_overrides,omitempty"`

	// InputOverrides contains modifications to the input
	InputOverrides json.RawMessage `json:"input_overrides,omitempty"`
}

// ReplayStatus represents the current state of a replay
type ReplayStatus string

const (
	ReplayStatusPending   ReplayStatus = "pending"
	ReplayStatusRunning   ReplayStatus = "running"
	ReplayStatusCompleted ReplayStatus = "completed"
	ReplayStatusFailed    ReplayStatus = "failed"
)

// Replay represents an in-progress or completed replay
type Replay struct {
	ID          string       `json:"id"`
	SourceRunID string       `json:"source_run_id"`
	NewRunID    string       `json:"new_run_id,omitempty"`
	DiffID      string       `json:"diff_id,omitempty"`
	Mode        ReplayMode   `json:"mode"`
	Status      ReplayStatus `json:"status"`
	Error       string       `json:"error,omitempty"`
	StartedAt   time.Time    `json:"started_at"`
	CompletedAt *time.Time   `json:"completed_at,omitempty"`
}

// RequestValidator validates replay requests
type RequestValidator struct {
	store store.TraceStore
}

// NewRequestValidator creates a new request validator
func NewRequestValidator(s store.TraceStore) *RequestValidator {
	return &RequestValidator{store: s}
}

// Validate validates a replay request
func (v *RequestValidator) Validate(ctx context.Context, req *ReplayRequest) (*store.Run, error) {
	if req.SourceRunID == "" {
		return nil, fmt.Errorf("source_run_id is required")
	}

	// Validate mode
	switch req.Mode {
	case ReplayModeLive, ReplayModeMocked, ReplayModeHybrid:
		// Valid modes
	case "":
		req.Mode = ReplayModeLive // Default to live
	default:
		return nil, fmt.Errorf("invalid replay mode: %s", req.Mode)
	}

	// Load source run
	sourceRun, err := v.store.GetRun(ctx, req.SourceRunID)
	if err != nil {
		return nil, fmt.Errorf("loading source run: %w", err)
	}
	if sourceRun == nil {
		return nil, fmt.Errorf("source run not found: %s", req.SourceRunID)
	}

	// Validate run is replayable
	if err := v.validateReplayable(sourceRun, req.Mode); err != nil {
		return nil, err
	}

	return sourceRun, nil
}

// validateReplayable checks if a run can be replayed in the given mode
func (v *RequestValidator) validateReplayable(run *store.Run, mode ReplayMode) error {
	// Check run status - only completed runs can be replayed
	if run.Status == store.RunStatusRunning {
		return fmt.Errorf("cannot replay a running run")
	}

	// For mocked mode, we need captured LLM responses
	if mode == ReplayModeMocked || mode == ReplayModeHybrid {
		// We need the graph snapshot for mocked replay
		if len(run.GraphSnapshot) == 0 {
			return fmt.Errorf("source run missing graph snapshot, required for %s mode", mode)
		}
	}

	// For live mode, we need graph and input snapshots
	if mode == ReplayModeLive {
		if len(run.GraphSnapshot) == 0 {
			return fmt.Errorf("source run missing graph snapshot, required for live replay")
		}
		if len(run.InputSnapshot) == 0 {
			return fmt.Errorf("source run missing input snapshot, required for live replay")
		}
	}

	return nil
}

// CapturedData holds data captured from a source run for replay
type CapturedData struct {
	Run       *store.Run
	Spans     []*store.Span
	LLMSpans  []*store.Span
	ToolSpans []*store.Span
}

// LoadCapturedData loads all necessary data from a source run
func (v *RequestValidator) LoadCapturedData(ctx context.Context, runID string) (*CapturedData, error) {
	run, err := v.store.GetRun(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("loading run: %w", err)
	}
	if run == nil {
		return nil, fmt.Errorf("run not found: %s", runID)
	}

	spans, err := v.store.GetSpanTree(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("loading spans: %w", err)
	}

	llmSpans, err := v.store.GetSpansByKind(ctx, runID, store.SpanKindLLM)
	if err != nil {
		return nil, fmt.Errorf("loading LLM spans: %w", err)
	}

	toolSpans, err := v.store.GetSpansByKind(ctx, runID, store.SpanKindTool)
	if err != nil {
		return nil, fmt.Errorf("loading tool spans: %w", err)
	}

	return &CapturedData{
		Run:       run,
		Spans:     spans,
		LLMSpans:  llmSpans,
		ToolSpans: toolSpans,
	}, nil
}

// ApplyOverrides applies configuration overrides to the replay
func ApplyOverrides(captured *CapturedData, overrides ReplayOverrides) error {
	// Apply graph overrides if provided
	if len(overrides.GraphOverrides) > 0 {
		// Merge graph overrides into graph snapshot
		var graph map[string]any
		if err := json.Unmarshal(captured.Run.GraphSnapshot, &graph); err != nil {
			return fmt.Errorf("parsing graph snapshot: %w", err)
		}

		var graphOverrides map[string]any
		if err := json.Unmarshal(overrides.GraphOverrides, &graphOverrides); err != nil {
			return fmt.Errorf("parsing graph overrides: %w", err)
		}

		// Merge overrides
		for k, v := range graphOverrides {
			graph[k] = v
		}

		newGraph, err := json.Marshal(graph)
		if err != nil {
			return fmt.Errorf("encoding merged graph: %w", err)
		}
		captured.Run.GraphSnapshot = newGraph
	}

	// Apply input overrides if provided
	if len(overrides.InputOverrides) > 0 {
		var input map[string]any
		if err := json.Unmarshal(captured.Run.InputSnapshot, &input); err != nil {
			// If no existing input, start fresh
			input = make(map[string]any)
		}

		var inputOverrides map[string]any
		if err := json.Unmarshal(overrides.InputOverrides, &inputOverrides); err != nil {
			return fmt.Errorf("parsing input overrides: %w", err)
		}

		// Merge overrides
		for k, v := range inputOverrides {
			input[k] = v
		}

		newInput, err := json.Marshal(input)
		if err != nil {
			return fmt.Errorf("encoding merged input: %w", err)
		}
		captured.Run.InputSnapshot = newInput
	}

	return nil
}
