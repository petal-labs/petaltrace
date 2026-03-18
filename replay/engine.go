package replay

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/petal-labs/petaltrace/diff"
	"github.com/petal-labs/petaltrace/store"
)

// Engine orchestrates replay operations
type Engine struct {
	store        store.TraceStore
	validator    *RequestValidator
	liveReplayer *LiveReplayer
	mockedReplayer *MockedReplayer
	hybridReplayer *HybridReplayer
	diffEngine   *diff.Engine
	logger       *slog.Logger

	// Track active replays
	mu      sync.RWMutex
	replays map[string]*Replay
}

// EngineConfig configures the replay engine
type EngineConfig struct {
	Store        store.TraceStore
	PetalFlowURL string
	Logger       *slog.Logger
}

// NewEngine creates a new replay engine
func NewEngine(cfg EngineConfig) *Engine {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	return &Engine{
		store:     cfg.Store,
		validator: NewRequestValidator(cfg.Store),
		liveReplayer: NewLiveReplayer(LiveReplayerConfig{
			Store:        cfg.Store,
			PetalFlowURL: cfg.PetalFlowURL,
		}),
		mockedReplayer: NewMockedReplayer(MockedReplayerConfig{
			Store:        cfg.Store,
			PetalFlowURL: cfg.PetalFlowURL,
		}),
		hybridReplayer: NewHybridReplayer(HybridReplayerConfig{
			Store:        cfg.Store,
			PetalFlowURL: cfg.PetalFlowURL,
		}),
		diffEngine: diff.NewEngine(cfg.Store),
		logger:     cfg.Logger,
		replays:    make(map[string]*Replay),
	}
}

// Execute runs a replay operation
func (e *Engine) Execute(ctx context.Context, req *ReplayRequest) (*Replay, error) {
	// Validate request
	sourceRun, err := e.validator.Validate(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Load captured data
	captured, err := e.validator.LoadCapturedData(ctx, req.SourceRunID)
	if err != nil {
		return nil, fmt.Errorf("loading captured data: %w", err)
	}

	// Apply overrides
	if err := ApplyOverrides(captured, req.Overrides); err != nil {
		return nil, fmt.Errorf("applying overrides: %w", err)
	}

	e.logger.Info("starting replay",
		"source_run_id", req.SourceRunID,
		"mode", req.Mode,
		"workflow", sourceRun.WorkflowName,
	)

	// Execute based on mode
	var replay *Replay
	switch req.Mode {
	case ReplayModeLive:
		replay, err = e.liveReplayer.Replay(ctx, req, captured)
	case ReplayModeMocked:
		replay, err = e.mockedReplayer.Replay(ctx, req, captured)
	case ReplayModeHybrid:
		replay, err = e.hybridReplayer.Replay(ctx, req, captured)
	default:
		return nil, fmt.Errorf("unsupported replay mode: %s", req.Mode)
	}

	if err != nil {
		e.logger.Error("replay failed",
			"source_run_id", req.SourceRunID,
			"mode", req.Mode,
			"error", err,
		)
		return replay, err
	}

	// Track replay
	e.mu.Lock()
	e.replays[replay.ID] = replay
	e.mu.Unlock()

	e.logger.Info("replay started",
		"replay_id", replay.ID,
		"new_run_id", replay.NewRunID,
		"mode", req.Mode,
	)

	// Auto-diff if requested and replay completed
	if req.AutoDiff && replay.Status == ReplayStatusCompleted && replay.NewRunID != "" {
		go e.performAutoDiff(context.Background(), replay, req.SourceRunID)
	}

	return replay, nil
}

// ExecuteSync runs a replay and waits for completion
func (e *Engine) ExecuteSync(ctx context.Context, req *ReplayRequest) (*Replay, error) {
	replay, err := e.Execute(ctx, req)
	if err != nil {
		return replay, err
	}

	// If already completed (mocked mode), return
	if replay.Status == ReplayStatusCompleted || replay.Status == ReplayStatusFailed {
		return replay, nil
	}

	// Wait for completion
	pollInterval := 2 * time.Second
	switch req.Mode {
	case ReplayModeLive:
		err = e.liveReplayer.WaitForCompletion(ctx, replay, pollInterval)
	case ReplayModeHybrid:
		err = e.hybridReplayer.WaitForCompletion(ctx, replay, pollInterval)
	}

	if err != nil {
		e.logger.Error("waiting for replay completion failed",
			"replay_id", replay.ID,
			"error", err,
		)
		return replay, err
	}

	// Auto-diff if requested
	if req.AutoDiff && replay.NewRunID != "" {
		e.performAutoDiff(ctx, replay, req.SourceRunID)
	}

	return replay, nil
}

// performAutoDiff computes diff between source and new run
func (e *Engine) performAutoDiff(ctx context.Context, replay *Replay, sourceRunID string) {
	if replay.NewRunID == "" {
		return
	}

	opts := diff.DiffOptions{
		IncludeContent:    true,
		IncludeSimilarity: true,
		CacheResult:       true,
	}

	runDiff, err := e.diffEngine.ComputeDiff(ctx, sourceRunID, replay.NewRunID, opts)
	if err != nil {
		e.logger.Error("auto-diff failed",
			"replay_id", replay.ID,
			"source_run_id", sourceRunID,
			"new_run_id", replay.NewRunID,
			"error", err,
		)
		return
	}

	// Update replay with diff ID
	e.mu.Lock()
	if r, ok := e.replays[replay.ID]; ok {
		r.DiffID = runDiff.ID
	}
	replay.DiffID = runDiff.ID
	e.mu.Unlock()

	e.logger.Info("auto-diff completed",
		"replay_id", replay.ID,
		"diff_id", runDiff.ID,
	)
}

// GetReplay retrieves a replay by ID
func (e *Engine) GetReplay(replayID string) (*Replay, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	replay, ok := e.replays[replayID]
	return replay, ok
}

// ListReplays returns all tracked replays
func (e *Engine) ListReplays() []*Replay {
	e.mu.RLock()
	defer e.mu.RUnlock()

	replays := make([]*Replay, 0, len(e.replays))
	for _, r := range e.replays {
		replays = append(replays, r)
	}
	return replays
}

// GetReplaysBySourceRun returns replays for a source run
func (e *Engine) GetReplaysBySourceRun(sourceRunID string) []*Replay {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var replays []*Replay
	for _, r := range e.replays {
		if r.SourceRunID == sourceRunID {
			replays = append(replays, r)
		}
	}
	return replays
}

// RefreshReplayStatus updates replay status from the store
func (e *Engine) RefreshReplayStatus(ctx context.Context, replayID string) (*Replay, error) {
	e.mu.Lock()
	replay, ok := e.replays[replayID]
	e.mu.Unlock()

	if !ok {
		return nil, fmt.Errorf("replay not found: %s", replayID)
	}

	if replay.NewRunID == "" || replay.Status == ReplayStatusCompleted || replay.Status == ReplayStatusFailed {
		return replay, nil
	}

	run, err := e.store.GetRun(ctx, replay.NewRunID)
	if err != nil {
		return nil, fmt.Errorf("getting run: %w", err)
	}
	if run == nil {
		return replay, nil
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	switch run.Status {
	case store.RunStatusCompleted:
		replay.Status = ReplayStatusCompleted
		now := time.Now()
		replay.CompletedAt = &now
	case store.RunStatusFailed:
		replay.Status = ReplayStatusFailed
		now := time.Now()
		replay.CompletedAt = &now
	}

	return replay, nil
}
