package replay

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/petal-labs/petaltrace/diff"
	"github.com/petal-labs/petaltrace/store"
)

// Engine orchestrates replay operations
type Engine struct {
	store          store.TraceStore
	validator      *RequestValidator
	liveReplayer   *LiveReplayer
	mockedReplayer *MockedReplayer
	hybridReplayer *HybridReplayer
	diffEngine     *diff.Engine
	logger         *slog.Logger
}

// EngineConfig configures the replay engine
type EngineConfig struct {
	Store          store.TraceStore
	PetalFlowURL   string
	PetalFlowToken string
	Logger         *slog.Logger
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
			Store:          cfg.Store,
			PetalFlowURL:   cfg.PetalFlowURL,
			PetalFlowToken: cfg.PetalFlowToken,
		}),
		mockedReplayer: NewMockedReplayer(MockedReplayerConfig{
			Store:        cfg.Store,
			PetalFlowURL: cfg.PetalFlowURL,
		}),
		hybridReplayer: NewHybridReplayer(HybridReplayerConfig{
			Store:          cfg.Store,
			PetalFlowURL:   cfg.PetalFlowURL,
			PetalFlowToken: cfg.PetalFlowToken,
		}),
		diffEngine: diff.NewEngine(cfg.Store),
		logger:     cfg.Logger,
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

	if replay != nil {
		if persistErr := e.store.CreateReplay(ctx, replayToRecord(replay)); persistErr != nil {
			return replay, fmt.Errorf("persisting replay: %w", persistErr)
		}
	}
	if err != nil {
		e.logger.Error("replay failed",
			"source_run_id", req.SourceRunID,
			"mode", req.Mode,
			"error", err,
		)
		return replay, err
	}

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

	if err := e.UpdateReplay(ctx, replay); err != nil {
		return replay, fmt.Errorf("persisting replay status: %w", err)
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

	// Update replay with diff ID.
	replay.DiffID = runDiff.ID
	if err := e.UpdateReplay(ctx, replay); err != nil {
		e.logger.Error("persisting replay diff ID failed",
			"replay_id", replay.ID,
			"diff_id", runDiff.ID,
			"error", err,
		)
		return
	}

	e.logger.Info("auto-diff completed",
		"replay_id", replay.ID,
		"diff_id", runDiff.ID,
	)
}

// GetReplay retrieves a replay by ID
func (e *Engine) GetReplay(replayID string) (*Replay, bool) {
	replay, err := e.LoadReplay(context.Background(), replayID)
	return replay, err == nil
}

// ListReplays returns all tracked replays
func (e *Engine) ListReplays() []*Replay {
	replays, err := e.LoadReplays(context.Background(), "")
	if err != nil {
		return nil
	}
	return replays
}

// GetReplaysBySourceRun returns replays for a source run
func (e *Engine) GetReplaysBySourceRun(sourceRunID string) []*Replay {
	replays, err := e.LoadReplays(context.Background(), sourceRunID)
	if err != nil {
		return nil
	}
	return replays
}

// LoadReplay retrieves a persisted replay by ID.
func (e *Engine) LoadReplay(ctx context.Context, replayID string) (*Replay, error) {
	record, err := e.store.GetReplay(ctx, replayID)
	if err != nil {
		return nil, fmt.Errorf("getting replay: %w", err)
	}
	if record == nil {
		return nil, fmt.Errorf("replay not found: %s", replayID)
	}
	return replayFromRecord(record), nil
}

// LoadReplays returns persisted replays, optionally filtered by source run.
func (e *Engine) LoadReplays(ctx context.Context, sourceRunID string) ([]*Replay, error) {
	records, err := e.store.ListReplays(ctx, sourceRunID)
	if err != nil {
		return nil, fmt.Errorf("listing replays: %w", err)
	}
	replays := make([]*Replay, len(records))
	for i := range records {
		replays[i] = replayFromRecord(&records[i])
	}
	return replays, nil
}

// UpdateReplay persists a replay's current status and result identifiers.
func (e *Engine) UpdateReplay(ctx context.Context, replay *Replay) error {
	if replay == nil {
		return fmt.Errorf("replay is required")
	}
	if err := e.store.UpdateReplay(ctx, replayToRecord(replay)); err != nil {
		return fmt.Errorf("updating replay: %w", err)
	}
	return nil
}

// RefreshReplayStatus updates replay status from the store
func (e *Engine) RefreshReplayStatus(ctx context.Context, replayID string) (*Replay, error) {
	replay, err := e.LoadReplay(ctx, replayID)
	if err != nil {
		return nil, err
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

	updated := false
	switch run.Status {
	case store.RunStatusCompleted:
		replay.Status = ReplayStatusCompleted
		now := time.Now()
		replay.CompletedAt = &now
		updated = true
	case store.RunStatusFailed:
		replay.Status = ReplayStatusFailed
		now := time.Now()
		replay.CompletedAt = &now
		updated = true
	}

	if updated {
		if err := e.UpdateReplay(ctx, replay); err != nil {
			return nil, fmt.Errorf("persisting replay status: %w", err)
		}
	}
	return replay, nil
}

func replayToRecord(replay *Replay) *store.ReplayRecord {
	record := &store.ReplayRecord{
		ID:          replay.ID,
		SourceRunID: replay.SourceRunID,
		NewRunID:    replay.NewRunID,
		DiffID:      replay.DiffID,
		Mode:        string(replay.Mode),
		Status:      string(replay.Status),
		Error:       replay.Error,
		StartedAt:   replay.StartedAt,
	}
	if replay.CompletedAt != nil {
		completedAt := *replay.CompletedAt
		record.CompletedAt = &completedAt
	}
	return record
}

func replayFromRecord(record *store.ReplayRecord) *Replay {
	replay := &Replay{
		ID:          record.ID,
		SourceRunID: record.SourceRunID,
		NewRunID:    record.NewRunID,
		DiffID:      record.DiffID,
		Mode:        ReplayMode(record.Mode),
		Status:      ReplayStatus(record.Status),
		Error:       record.Error,
		StartedAt:   record.StartedAt,
	}
	if record.CompletedAt != nil {
		completedAt := *record.CompletedAt
		replay.CompletedAt = &completedAt
	}
	return replay
}
