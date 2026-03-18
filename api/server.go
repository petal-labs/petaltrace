package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/petal-labs/petaltrace/pricing"
	"github.com/petal-labs/petaltrace/store"
)

// Server is the HTTP API server for PetalTrace
type Server struct {
	store        store.TraceStore
	pricingTable *pricing.PricingTable
	logger       *slog.Logger
	httpServer   *http.Server
	addr         string

	// SSE subscribers
	liveSubscribers map[chan *SSEEvent]struct{}
}

// ServerConfig holds configuration for the API server
type ServerConfig struct {
	Store        store.TraceStore
	PricingTable *pricing.PricingTable
	Logger       *slog.Logger
	Addr         string
}

// NewServer creates a new API server
func NewServer(cfg ServerConfig) *Server {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Addr == "" {
		cfg.Addr = ":8090"
	}

	s := &Server{
		store:           cfg.Store,
		pricingTable:    cfg.PricingTable,
		logger:          cfg.Logger,
		addr:            cfg.Addr,
		liveSubscribers: make(map[chan *SSEEvent]struct{}),
	}

	return s
}

// Start starts the HTTP server
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	// Register routes
	s.registerRoutes(mux)

	// Apply middleware
	handler := s.loggingMiddleware(mux)
	handler = s.corsMiddleware(handler)
	handler = s.recoveryMiddleware(handler)

	s.httpServer = &http.Server{
		Addr:         s.addr,
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	s.logger.Info("starting API server", "addr", s.addr)

	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("API server error", "error", err)
		}
	}()

	return nil
}

// Stop gracefully stops the HTTP server
func (s *Server) Stop() error {
	if s.httpServer == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	return s.httpServer.Shutdown(ctx)
}

// Addr returns the server address
func (s *Server) Addr() string {
	return s.addr
}

func (s *Server) registerRoutes(mux *http.ServeMux) {
	// Health
	mux.HandleFunc("GET /api/health", s.handleHealth)

	// Runs
	mux.HandleFunc("GET /api/runs", s.handleListRuns)
	mux.HandleFunc("GET /api/runs/{id}", s.handleGetRun)
	mux.HandleFunc("DELETE /api/runs/{id}", s.handleDeleteRun)
	mux.HandleFunc("GET /api/runs/{id}/spans", s.handleGetSpans)
	mux.HandleFunc("GET /api/runs/{id}/spans/{spanId}", s.handleGetSpan)
	mux.HandleFunc("GET /api/runs/{id}/graph", s.handleGetGraph)

	// Prompts
	mux.HandleFunc("GET /api/runs/{id}/prompts/{nodeId}", s.handleGetPrompt)

	// Cost
	mux.HandleFunc("GET /api/cost/summary", s.handleCostSummary)
	mux.HandleFunc("GET /api/cost/runs/{id}", s.handleCostRun)
	mux.HandleFunc("GET /api/cost/timeseries", s.handleCostTimeseries)

	// SSE Live streaming
	mux.HandleFunc("GET /api/runs/{id}/stream", s.handleRunStream)
	mux.HandleFunc("GET /api/live", s.handleLiveStream)

	// Diff
	mux.HandleFunc("POST /api/diff", s.handleComputeDiff)
	mux.HandleFunc("GET /api/diff/{id}", s.handleGetDiff)
	mux.HandleFunc("GET /api/diff/runs", s.handleGetDiffByRuns)
	mux.HandleFunc("GET /api/diffs", s.handleListDiffs)

	// Replay
	mux.HandleFunc("POST /api/replay", s.handleTriggerReplay)
	mux.HandleFunc("GET /api/replay/{id}", s.handleGetReplayStatus)
	mux.HandleFunc("GET /api/replays", s.handleListReplays)

	// System
	mux.HandleFunc("GET /api/stats", s.handleStats)
	mux.HandleFunc("GET /api/pricing", s.handleGetPricing)
	mux.HandleFunc("PUT /api/pricing", s.handleUpdatePricing)
}

// Middleware

func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap response writer to capture status
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(wrapped, r)

		s.logger.Debug("http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", wrapped.statusCode,
			"duration", time.Since(start),
		)
	})
}

func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Server) recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				s.logger.Error("panic recovered", "error", err, "path", r.URL.Path)
				s.writeError(w, http.StatusInternalServerError, "internal server error")
			}
		}()

		next.ServeHTTP(w, r)
	})
}

// Response helpers

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		s.logger.Error("failed to encode JSON response", "error", err)
	}
}

func (s *Server) writeError(w http.ResponseWriter, status int, message string) {
	s.writeJSON(w, status, map[string]string{
		"error": message,
	})
}

// APIError represents an error response
type APIError struct {
	Error   string `json:"error"`
	Code    string `json:"code,omitempty"`
	Details any    `json:"details,omitempty"`
}

// Pagination response wrapper
type PaginatedResponse struct {
	Data       any    `json:"data"`
	Cursor     string `json:"cursor,omitempty"`
	HasMore    bool   `json:"has_more"`
	TotalCount int    `json:"total_count,omitempty"`
}

// SSE Event
type SSEEvent struct {
	Event string `json:"event"`
	Data  any    `json:"data"`
}

func (s *Server) writeSSE(w http.ResponseWriter, event *SSEEvent) error {
	data, err := json.Marshal(event.Data)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Event, data)
	if err != nil {
		return err
	}

	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	return nil
}
