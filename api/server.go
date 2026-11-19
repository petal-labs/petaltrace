package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/petal-labs/petaltrace/internal/netutil"
	"github.com/petal-labs/petaltrace/pricing"
	"github.com/petal-labs/petaltrace/replay"
	"github.com/petal-labs/petaltrace/store"
)

// Server is the HTTP API server for PetalTrace
type Server struct {
	store          store.TraceStore
	petalFlowURL   string
	petalFlowToken string
	pricingTable   *pricing.PricingTable
	logger         *slog.Logger
	httpServer     *http.Server
	listener       net.Listener
	serveDone      chan struct{}
	serverMu       sync.Mutex
	addr           string
	replayMu       sync.Mutex
	replayEngine   *replay.Engine

	// SSE subscribers
	liveSubscribers map[chan *SSEEvent]struct{}
}

// ServerConfig holds configuration for the API server
type ServerConfig struct {
	Store          store.TraceStore
	PricingTable   *pricing.PricingTable
	Logger         *slog.Logger
	Addr           string
	PetalFlowURL   string
	PetalFlowToken string
}

// NewServer creates a new API server
func NewServer(cfg ServerConfig) *Server {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Addr == "" {
		cfg.Addr = "127.0.0.1:8090"
	}

	s := &Server{
		store:           cfg.Store,
		petalFlowURL:    cfg.PetalFlowURL,
		petalFlowToken:  cfg.PetalFlowToken,
		pricingTable:    cfg.PricingTable,
		logger:          cfg.Logger,
		addr:            cfg.Addr,
		liveSubscribers: make(map[chan *SSEEvent]struct{}),
	}

	return s
}

// Start starts the HTTP server
func (s *Server) Start(ctx context.Context) error {
	if !netutil.IsLoopbackAddr(s.addr) {
		return fmt.Errorf("API server address must use a loopback host because network authentication is not implemented")
	}

	listener, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("listening for API: %w", err)
	}

	server := &http.Server{
		Addr:         s.addr,
		Handler:      s.handler(),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	serveDone := make(chan struct{})

	s.serverMu.Lock()
	if s.httpServer != nil {
		s.serverMu.Unlock()
		_ = listener.Close()
		return errors.New("API server already started")
	}
	s.httpServer = server
	s.listener = listener
	s.serveDone = serveDone
	s.addr = listener.Addr().String()
	s.serverMu.Unlock()

	s.logger.Info("starting API server", "addr", s.addr)

	go func() {
		defer close(serveDone)
		if err := server.Serve(listener); err != nil &&
			!errors.Is(err, http.ErrServerClosed) &&
			!errors.Is(err, net.ErrClosed) {
			s.logger.Error("API server error", "error", err)
		}
	}()
	go func() {
		select {
		case <-ctx.Done():
			_ = s.Stop()
		case <-serveDone:
		}
	}()

	return nil
}

func (s *Server) handler() http.Handler {
	mux := http.NewServeMux()
	s.registerRoutes(mux)

	handler := s.loggingMiddleware(mux)
	return s.recoveryMiddleware(handler)
}

// Stop gracefully stops the HTTP server
func (s *Server) Stop() error {
	s.serverMu.Lock()
	server, listener, serveDone := s.httpServer, s.listener, s.serveDone
	s.httpServer = nil
	s.listener = nil
	s.serveDone = nil
	s.serverMu.Unlock()
	if server == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	shutdownErr := server.Shutdown(ctx)
	if shutdownErr != nil {
		_ = server.Close()
	}
	if listener != nil {
		if err := listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) && shutdownErr == nil {
			shutdownErr = fmt.Errorf("closing API listener: %w", err)
		}
	}
	if serveDone != nil {
		<-serveDone
	}
	if shutdownErr != nil {
		return fmt.Errorf("shutting down API server: %w", shutdownErr)
	}
	return nil
}

// Addr returns the server address
func (s *Server) Addr() string {
	s.serverMu.Lock()
	defer s.serverMu.Unlock()
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
