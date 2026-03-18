package api

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/petal-labs/petaltrace/store"
)

// handleRunStream handles GET /api/runs/{id}/stream - SSE for single run
func (s *Server) handleRunStream(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("id")

	if runID == "" {
		s.writeError(w, http.StatusBadRequest, "run ID is required")
		return
	}

	// Verify run exists
	run, err := s.store.GetRun(r.Context(), runID)
	if err != nil {
		s.logger.Error("failed to get run", "error", err, "run_id", runID)
		s.writeError(w, http.StatusInternalServerError, "failed to get run")
		return
	}
	if run == nil {
		s.writeError(w, http.StatusNotFound, "run not found")
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	// Send initial run state
	s.writeSSE(w, &SSEEvent{
		Event: "run",
		Data:  run,
	})

	// Send initial spans
	spans, err := s.store.GetSpanTree(r.Context(), runID)
	if err == nil {
		s.writeSSE(w, &SSEEvent{
			Event: "spans",
			Data:  spans,
		})
	}

	// If run is already completed, send done event and close
	if run.Status != store.RunStatusRunning {
		s.writeSSE(w, &SSEEvent{
			Event: "done",
			Data:  map[string]string{"status": string(run.Status)},
		})
		return
	}

	// Poll for updates while run is active
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	ctx := r.Context()
	lastSpanCount := len(spans)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Check run status
			run, err := s.store.GetRun(ctx, runID)
			if err != nil {
				s.logger.Error("failed to poll run", "error", err, "run_id", runID)
				continue
			}
			if run == nil {
				s.writeSSE(w, &SSEEvent{
					Event: "error",
					Data:  map[string]string{"error": "run deleted"},
				})
				return
			}

			// Send run update
			s.writeSSE(w, &SSEEvent{
				Event: "run",
				Data:  run,
			})

			// Check for new spans
			spans, err := s.store.GetSpanTree(ctx, runID)
			if err != nil {
				s.logger.Error("failed to poll spans", "error", err, "run_id", runID)
				continue
			}

			if len(spans) > lastSpanCount {
				// Send new spans
				newSpans := spans[lastSpanCount:]
				for _, span := range newSpans {
					s.writeSSE(w, &SSEEvent{
						Event: "span",
						Data:  span,
					})
				}
				lastSpanCount = len(spans)
			}

			// If run completed, send done event and close
			if run.Status != store.RunStatusRunning {
				s.writeSSE(w, &SSEEvent{
					Event: "done",
					Data:  map[string]string{"status": string(run.Status)},
				})
				return
			}
		}
	}
}

// handleLiveStream handles GET /api/live - SSE for all active runs
func (s *Server) handleLiveStream(w http.ResponseWriter, r *http.Request) {
	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	// Send initial list of active runs
	activeRuns, err := s.getActiveRuns(r.Context())
	if err != nil {
		s.logger.Error("failed to get active runs", "error", err)
		s.writeSSE(w, &SSEEvent{
			Event: "error",
			Data:  map[string]string{"error": "failed to get active runs"},
		})
		return
	}

	s.writeSSE(w, &SSEEvent{
		Event: "init",
		Data:  map[string]any{"active_runs": activeRuns},
	})

	// Poll for updates
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	ctx := r.Context()
	knownRuns := make(map[string]store.RunStatus)
	for _, run := range activeRuns {
		knownRuns[run.ID] = run.Status
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Get current active runs
			currentRuns, err := s.getActiveRuns(ctx)
			if err != nil {
				s.logger.Error("failed to poll active runs", "error", err)
				continue
			}

			// Check for new runs
			currentRunMap := make(map[string]*store.Run)
			for _, run := range currentRuns {
				currentRunMap[run.ID] = run

				if _, known := knownRuns[run.ID]; !known {
					// New run started
					s.writeSSE(w, &SSEEvent{
						Event: "run_started",
						Data:  run,
					})
					knownRuns[run.ID] = run.Status
				} else if knownRuns[run.ID] != run.Status {
					// Run status changed
					s.writeSSE(w, &SSEEvent{
						Event: "run_updated",
						Data:  run,
					})
					knownRuns[run.ID] = run.Status
				}
			}

			// Check for completed/removed runs
			for runID, status := range knownRuns {
				if _, exists := currentRunMap[runID]; !exists {
					if status == store.RunStatusRunning {
						// Run completed
						s.writeSSE(w, &SSEEvent{
							Event: "run_completed",
							Data:  map[string]string{"run_id": runID},
						})
					}
					delete(knownRuns, runID)
				}
			}

			// Send heartbeat
			s.writeSSE(w, &SSEEvent{
				Event: "heartbeat",
				Data:  map[string]any{"timestamp": time.Now().Unix(), "active_count": len(currentRuns)},
			})
		}
	}
}

func (s *Server) getActiveRuns(ctx context.Context) ([]*store.Run, error) {
	opts := store.ListRunsOptions{
		Status: store.RunStatusRunning,
		Limit:  100,
	}

	runs, _, err := s.store.ListRuns(ctx, opts)
	if err != nil {
		return nil, err
	}

	// Convert to pointers
	result := make([]*store.Run, len(runs))
	for i := range runs {
		result[i] = &runs[i]
	}

	return result, nil
}

// LiveEventBroadcaster manages live event broadcasting to SSE clients
type LiveEventBroadcaster struct {
	mu          sync.RWMutex
	subscribers map[chan *SSEEvent]struct{}
}

// NewLiveEventBroadcaster creates a new broadcaster
func NewLiveEventBroadcaster() *LiveEventBroadcaster {
	return &LiveEventBroadcaster{
		subscribers: make(map[chan *SSEEvent]struct{}),
	}
}

// Subscribe adds a new subscriber
func (b *LiveEventBroadcaster) Subscribe() chan *SSEEvent {
	ch := make(chan *SSEEvent, 100)
	b.mu.Lock()
	b.subscribers[ch] = struct{}{}
	b.mu.Unlock()
	return ch
}

// Unsubscribe removes a subscriber
func (b *LiveEventBroadcaster) Unsubscribe(ch chan *SSEEvent) {
	b.mu.Lock()
	delete(b.subscribers, ch)
	close(ch)
	b.mu.Unlock()
}

// Broadcast sends an event to all subscribers
func (b *LiveEventBroadcaster) Broadcast(event *SSEEvent) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for ch := range b.subscribers {
		select {
		case ch <- event:
		default:
			// Skip slow consumers
		}
	}
}
