package collector

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	collectorpb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/proto"

	"github.com/petal-labs/petaltrace/internal/netutil"
)

// OTLPHTTPReceiver receives OTLP traces over HTTP.
type OTLPHTTPReceiver struct {
	addr     string
	handler  SpanHandler
	server   *http.Server
	logger   *slog.Logger
	mu       sync.Mutex
	running  bool
	listener net.Listener

	// Metrics
	spansReceived int64
	requestCount  int64
	errorCount    int64
}

// OTLPHTTPConfig configures the OTLP HTTP receiver.
type OTLPHTTPConfig struct {
	Addr    string
	Handler SpanHandler
	Logger  *slog.Logger
}

// NewOTLPHTTPReceiver creates a new OTLP HTTP receiver.
func NewOTLPHTTPReceiver(cfg OTLPHTTPConfig) *OTLPHTTPReceiver {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Addr == "" {
		cfg.Addr = "127.0.0.1:4318"
	}

	return &OTLPHTTPReceiver{
		addr:    cfg.Addr,
		handler: cfg.Handler,
		logger:  cfg.Logger,
	}
}

// Start begins accepting HTTP connections.
func (r *OTLPHTTPReceiver) Start(ctx context.Context) error {
	listener, err := r.listen()
	if err != nil {
		return err
	}
	if err := r.prepare(listener); err != nil {
		_ = listener.Close()
		return err
	}
	return r.serve(ctx)
}

func (r *OTLPHTTPReceiver) listen() (net.Listener, error) {
	if !netutil.IsLoopbackAddr(r.addr) {
		return nil, fmt.Errorf("OTLP HTTP address must use a loopback host because network authentication is not implemented")
	}

	listener, err := net.Listen("tcp", r.addr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen: %w", err)
	}
	return listener, nil
}

func (r *OTLPHTTPReceiver) prepare(listener net.Listener) error {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/traces", r.handleTraces)
	mux.HandleFunc("GET /health", r.handleHealth)

	server := &http.Server{
		Addr:         r.addr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return errors.New("receiver already running")
	}
	r.listener = listener
	r.server = server
	r.running = true
	r.mu.Unlock()

	r.logger.Info("OTLP HTTP receiver started", "addr", listener.Addr().String())
	return nil
}

func (r *OTLPHTTPReceiver) serve(ctx context.Context) error {
	r.mu.Lock()
	if !r.running || r.server == nil || r.listener == nil {
		r.mu.Unlock()
		return nil
	}
	server, listener := r.server, r.listener
	r.mu.Unlock()

	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = r.Stop()
		case <-done:
		}
	}()
	defer close(done)

	err := server.Serve(listener)
	r.mu.Lock()
	if r.server == server {
		r.running = false
	}
	r.mu.Unlock()

	if err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, net.ErrClosed) {
		return fmt.Errorf("server error: %w", err)
	}
	return nil
}

// Stop gracefully shuts down the receiver.
func (r *OTLPHTTPReceiver) Stop() error {
	r.mu.Lock()
	if r.server == nil || r.listener == nil {
		r.running = false
		r.mu.Unlock()
		return nil
	}
	server, listener := r.server, r.listener
	r.running = false
	r.server = nil
	r.listener = nil
	r.mu.Unlock()

	r.logger.Info("stopping OTLP HTTP receiver")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		_ = server.Close()
		_ = listener.Close()
		return fmt.Errorf("shutdown error: %w", err)
	}
	if err := listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
		return fmt.Errorf("closing listener: %w", err)
	}
	return nil
}

// Addr returns the listener address, useful when using port 0.
func (r *OTLPHTTPReceiver) Addr() string {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.listener != nil {
		return r.listener.Addr().String()
	}
	return r.addr
}

// Stats returns receiver statistics.
func (r *OTLPHTTPReceiver) Stats() (requests, spans, errors int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.requestCount, r.spansReceived, r.errorCount
}

func (r *OTLPHTTPReceiver) handleTraces(w http.ResponseWriter, req *http.Request) {
	r.mu.Lock()
	r.requestCount++
	r.mu.Unlock()

	contentType := req.Header.Get("Content-Type")
	if contentType != "application/x-protobuf" && contentType != "" {
		r.recordError()
		http.Error(w, "unsupported content type, expected application/x-protobuf", http.StatusUnsupportedMediaType)
		return
	}

	body, err := io.ReadAll(io.LimitReader(req.Body, 64*1024*1024)) // 64MB limit
	if err != nil {
		r.recordError()
		r.logger.Error("failed to read request body", "error", err)
		http.Error(w, "failed to read request", http.StatusBadRequest)
		return
	}
	defer req.Body.Close()

	var traceReq collectorpb.ExportTraceServiceRequest
	if err := proto.Unmarshal(body, &traceReq); err != nil {
		r.recordError()
		r.logger.Error("failed to unmarshal protobuf", "error", err)
		http.Error(w, "invalid protobuf", http.StatusBadRequest)
		return
	}

	spans := ConvertOTLPSpans(traceReq.ResourceSpans)
	if len(spans) == 0 {
		w.Header().Set("Content-Type", "application/x-protobuf")
		resp := &collectorpb.ExportTraceServiceResponse{}
		respBody, _ := proto.Marshal(resp)
		w.Write(respBody)
		return
	}

	r.mu.Lock()
	r.spansReceived += int64(len(spans))
	r.mu.Unlock()

	r.logger.Debug("received spans", "count", len(spans))

	if r.handler != nil {
		if err := r.handler.HandleSpans(spans); err != nil {
			r.recordError()
			r.logger.Error("handler failed", "error", err)
			http.Error(w, "failed to process spans", http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/x-protobuf")
	resp := &collectorpb.ExportTraceServiceResponse{}
	respBody, _ := proto.Marshal(resp)
	w.Write(respBody)
}

func (r *OTLPHTTPReceiver) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}

func (r *OTLPHTTPReceiver) recordError() {
	r.mu.Lock()
	r.errorCount++
	r.mu.Unlock()
}
