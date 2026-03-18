package collector

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/petal-labs/petaltrace/store"
)

// Correlator groups spans into runs and manages run lifecycle.
type Correlator struct {
	store      store.TraceStore
	classifier *Classifier
	logger     *slog.Logger

	mu         sync.RWMutex
	activeRuns map[string]*runState
}

type runState struct {
	run       *store.Run
	lastSeen  time.Time
	rootFound bool
}

// CorrelatorConfig configures the correlator.
type CorrelatorConfig struct {
	Store      store.TraceStore
	Classifier *Classifier
	Logger     *slog.Logger
}

// NewCorrelator creates a new run correlator.
func NewCorrelator(cfg CorrelatorConfig) *Correlator {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Classifier == nil {
		cfg.Classifier = NewClassifier()
	}

	return &Correlator{
		store:      cfg.Store,
		classifier: cfg.Classifier,
		logger:     cfg.Logger,
		activeRuns: make(map[string]*runState),
	}
}

// CorrelatedSpan holds a classified span with its associated run.
type CorrelatedSpan struct {
	Span   *store.Span
	Run    *store.Run
	IsRoot bool
}

// ProcessSpans classifies spans and correlates them to runs.
func (c *Correlator) ProcessSpans(ctx context.Context, inSpans []*IngestedSpan) ([]*CorrelatedSpan, error) {
	var result []*CorrelatedSpan

	for _, in := range inSpans {
		correlated, err := c.processSpan(ctx, in)
		if err != nil {
			c.logger.Error("failed to process span", "error", err, "span_id", in.SpanID)
			continue
		}
		result = append(result, correlated)
	}

	return result, nil
}

func (c *Correlator) processSpan(ctx context.Context, in *IngestedSpan) (*CorrelatedSpan, error) {
	// Classify the span
	span := c.classifier.ClassifySpan(in)
	span.ID = store.NewID()

	// Extract run context
	runID := c.extractRunID(in)
	span.RunID = runID

	// Set parent span ID if present
	if in.ParentSpanID != "" {
		span.ParentID = &in.ParentSpanID
	}

	// Determine if this is a root span
	isRoot := c.isRootSpan(in)

	// Get or create run
	run, err := c.getOrCreateRun(ctx, runID, in, isRoot)
	if err != nil {
		return nil, err
	}

	return &CorrelatedSpan{
		Span:   span,
		Run:    run,
		IsRoot: isRoot,
	}, nil
}

// extractRunID gets the run ID from span attributes or falls back to trace ID.
func (c *Correlator) extractRunID(in *IngestedSpan) string {
	// Check for explicit run ID
	if runID, ok := getStringAttr(in.Attributes, "petalflow.run.id"); ok && runID != "" {
		return runID
	}

	// Check resource attributes
	if runID, ok := getStringAttr(in.Resource, "petalflow.run.id"); ok && runID != "" {
		return runID
	}

	// Fall back to trace ID
	if in.TraceID != "" {
		return "trace-" + in.TraceID
	}

	// Generate a new run ID for orphan spans
	return "orphan-" + store.NewID()
}

// isRootSpan determines if this span is the root of a workflow execution.
func (c *Correlator) isRootSpan(in *IngestedSpan) bool {
	// No parent span ID
	if in.ParentSpanID == "" {
		return true
	}

	// Explicit root marker
	if isRoot, ok := in.Attributes["petalflow.run.root"]; ok {
		if b, ok := isRoot.(bool); ok && b {
			return true
		}
	}

	return false
}

// getOrCreateRun retrieves an existing run or creates a new one.
func (c *Correlator) getOrCreateRun(ctx context.Context, runID string, in *IngestedSpan, isRoot bool) (*store.Run, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check active runs cache
	if state, ok := c.activeRuns[runID]; ok {
		state.lastSeen = time.Now()

		// Update root info if this is the root span
		if isRoot && !state.rootFound {
			state.rootFound = true
			c.updateRunFromRoot(state.run, in)
		}

		return state.run, nil
	}

	// Check if run exists in store
	existingRun, err := c.store.GetRun(ctx, runID)
	if err == nil && existingRun != nil {
		// Cache it
		c.activeRuns[runID] = &runState{
			run:       existingRun,
			lastSeen:  time.Now(),
			rootFound: true,
		}
		return existingRun, nil
	}

	// Create new run
	run := c.createRun(runID, in, isRoot)

	// Save to store
	if err := c.store.CreateRun(ctx, run); err != nil {
		return nil, err
	}

	// Cache it
	c.activeRuns[runID] = &runState{
		run:       run,
		lastSeen:  time.Now(),
		rootFound: isRoot,
	}

	c.logger.Info("created new run", "run_id", runID, "workflow", run.WorkflowName)

	return run, nil
}

// createRun creates a new Run from span context.
func (c *Correlator) createRun(runID string, in *IngestedSpan, isRoot bool) *store.Run {
	run := &store.Run{
		ID:          runID,
		Status:      store.RunStatusRunning,
		StartedAt:   in.StartTime,
		CreatedAt:   time.Now(),
		SourceKind:  "otlp",
		Tags:        make(map[string]string),
		TotalTokens: store.TokenSummary{},
		EstimatedCost: store.CostEstimate{
			Currency:   "USD",
			ByProvider: make(map[string]float64),
			ByModel:    make(map[string]float64),
			ByNode:     make(map[string]float64),
		},
	}

	// Extract workflow info from attributes
	if wfID, ok := getStringAttr(in.Attributes, "petalflow.workflow.id"); ok {
		run.WorkflowID = wfID
	}
	if wfName, ok := getStringAttr(in.Attributes, "petalflow.workflow.name"); ok {
		run.WorkflowName = wfName
	} else if wfName, ok := getStringAttr(in.Resource, "service.name"); ok {
		run.WorkflowName = wfName
	} else {
		run.WorkflowName = "unknown"
	}
	if wfVersion, ok := getStringAttr(in.Attributes, "petalflow.workflow.version"); ok {
		run.WorkflowVersion = wfVersion
	}

	// Extract source kind
	if sk, ok := getStringAttr(in.Attributes, "petalflow.source_kind"); ok {
		run.SourceKind = sk
	}

	// Extract trigger source
	if ts, ok := getStringAttr(in.Attributes, "petalflow.trigger_source"); ok {
		run.TriggerSource = ts
	}

	// Extract parent run ID for replays
	if parentID, ok := getStringAttr(in.Attributes, "petalflow.parent_run_id"); ok {
		run.ParentRunID = &parentID
	}

	// Extract tags
	for k, v := range in.Attributes {
		if len(k) > 4 && k[:4] == "tag." {
			if s, ok := v.(string); ok {
				run.Tags[k[4:]] = s
			}
		}
	}

	// If this is the root span, capture snapshots
	if isRoot {
		c.updateRunFromRoot(run, in)
	}

	return run
}

// updateRunFromRoot updates run with information from the root span.
func (c *Correlator) updateRunFromRoot(run *store.Run, in *IngestedSpan) {
	// Capture graph snapshot
	if graph, ok := in.Attributes["petalflow.graph"]; ok {
		run.GraphSnapshot, _ = json.Marshal(graph)
	}

	// Capture input snapshot
	if input, ok := in.Attributes["petalflow.input"]; ok {
		run.InputSnapshot, _ = json.Marshal(input)
	}

	// Capture config snapshot
	if config, ok := in.Attributes["petalflow.config"]; ok {
		run.ConfigSnapshot, _ = json.Marshal(config)
	}
}

// UpdateRunStatus updates the status of a run based on span status.
func (c *Correlator) UpdateRunStatus(ctx context.Context, runID string, status store.SpanStatus, completedAt time.Time) error {
	c.mu.Lock()
	state, ok := c.activeRuns[runID]
	if !ok {
		c.mu.Unlock()
		return nil
	}

	run := state.run
	c.mu.Unlock()

	// Determine run status from span status
	switch status {
	case store.SpanStatusError:
		run.Status = store.RunStatusFailed
	default:
		run.Status = store.RunStatusCompleted
	}

	run.CompletedAt = &completedAt
	run.DurationMs = completedAt.Sub(run.StartedAt).Milliseconds()

	return c.store.UpdateRun(ctx, run)
}

// CompleteRun marks a run as completed.
func (c *Correlator) CompleteRun(ctx context.Context, runID string, status store.RunStatus, completedAt time.Time) error {
	c.mu.Lock()
	state, ok := c.activeRuns[runID]
	if !ok {
		c.mu.Unlock()
		// Try to load from store
		run, err := c.store.GetRun(ctx, runID)
		if err != nil {
			return err
		}
		run.Status = status
		run.CompletedAt = &completedAt
		run.DurationMs = completedAt.Sub(run.StartedAt).Milliseconds()
		return c.store.UpdateRun(ctx, run)
	}

	run := state.run
	c.mu.Unlock()

	run.Status = status
	run.CompletedAt = &completedAt
	run.DurationMs = completedAt.Sub(run.StartedAt).Milliseconds()

	return c.store.UpdateRun(ctx, run)
}

// FlushInactiveRuns removes runs that haven't received spans recently.
func (c *Correlator) FlushInactiveRuns(maxAge time.Duration) []string {
	c.mu.Lock()
	defer c.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	var flushed []string

	for runID, state := range c.activeRuns {
		if state.lastSeen.Before(cutoff) {
			delete(c.activeRuns, runID)
			flushed = append(flushed, runID)
		}
	}

	return flushed
}

// ActiveRunCount returns the number of active runs in cache.
func (c *Correlator) ActiveRunCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.activeRuns)
}
