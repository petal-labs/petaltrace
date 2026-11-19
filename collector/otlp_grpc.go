package collector

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	collectorpb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"

	"github.com/petal-labs/petaltrace/internal/netutil"
)

// OTLPGRPCReceiver receives OTLP traces over gRPC.
type OTLPGRPCReceiver struct {
	collectorpb.UnimplementedTraceServiceServer

	addr     string
	handler  SpanHandler
	server   *grpc.Server
	logger   *slog.Logger
	mu       sync.Mutex
	running  bool
	listener net.Listener

	// Metrics
	spansReceived int64
	requestCount  int64
	errorCount    int64
}

// OTLPGRPCConfig configures the OTLP gRPC receiver.
type OTLPGRPCConfig struct {
	Addr    string
	Handler SpanHandler
	Logger  *slog.Logger
}

// NewOTLPGRPCReceiver creates a new OTLP gRPC receiver.
func NewOTLPGRPCReceiver(cfg OTLPGRPCConfig) *OTLPGRPCReceiver {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Addr == "" {
		cfg.Addr = "127.0.0.1:4317"
	}

	return &OTLPGRPCReceiver{
		addr:    cfg.Addr,
		handler: cfg.Handler,
		logger:  cfg.Logger,
	}
}

// Start begins accepting gRPC connections.
func (r *OTLPGRPCReceiver) Start(ctx context.Context) error {
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

func (r *OTLPGRPCReceiver) listen() (net.Listener, error) {
	if !netutil.IsLoopbackAddr(r.addr) {
		return nil, fmt.Errorf("OTLP gRPC address must use a loopback host because network authentication is not implemented")
	}

	listener, err := net.Listen("tcp", r.addr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen: %w", err)
	}
	return listener, nil
}

func (r *OTLPGRPCReceiver) prepare(listener net.Listener) error {
	server := grpc.NewServer(
		grpc.MaxRecvMsgSize(64*1024*1024), // 64MB
		grpc.MaxSendMsgSize(64*1024*1024),
	)

	collectorpb.RegisterTraceServiceServer(server, r)

	// Register health service
	healthServer := health.NewServer()
	healthpb.RegisterHealthServer(server, healthServer)
	healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)

	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return errors.New("receiver already running")
	}
	r.listener = listener
	r.server = server
	r.running = true
	r.mu.Unlock()

	r.logger.Info("OTLP gRPC receiver started", "addr", listener.Addr().String())
	return nil
}

func (r *OTLPGRPCReceiver) serve(ctx context.Context) error {
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

	if err != nil && !errors.Is(err, grpc.ErrServerStopped) && !errors.Is(err, net.ErrClosed) {
		return fmt.Errorf("server error: %w", err)
	}
	return nil
}

// Stop gracefully shuts down the receiver.
func (r *OTLPGRPCReceiver) Stop() error {
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

	r.logger.Info("stopping OTLP gRPC receiver")

	stopped := make(chan struct{})
	go func() {
		server.GracefulStop()
		close(stopped)
	}()

	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		server.Stop()
	}

	if err := listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
		return fmt.Errorf("closing listener: %w", err)
	}
	return nil
}

// Addr returns the listener address.
func (r *OTLPGRPCReceiver) Addr() string {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.listener != nil {
		return r.listener.Addr().String()
	}
	return r.addr
}

// Stats returns receiver statistics.
func (r *OTLPGRPCReceiver) Stats() (requests, spans, errors int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.requestCount, r.spansReceived, r.errorCount
}

// Export implements the TraceService Export RPC.
func (r *OTLPGRPCReceiver) Export(ctx context.Context, req *collectorpb.ExportTraceServiceRequest) (*collectorpb.ExportTraceServiceResponse, error) {
	r.mu.Lock()
	r.requestCount++
	r.mu.Unlock()

	spans := ConvertOTLPSpans(req.ResourceSpans)
	if len(spans) == 0 {
		return &collectorpb.ExportTraceServiceResponse{}, nil
	}

	r.mu.Lock()
	r.spansReceived += int64(len(spans))
	r.mu.Unlock()

	r.logger.Debug("received spans via gRPC", "count", len(spans))

	if r.handler != nil {
		if err := r.handler.HandleSpans(spans); err != nil {
			r.recordError()
			r.logger.Error("handler failed", "error", err)
			return nil, status.Errorf(codes.Internal, "failed to process spans: %v", err)
		}
	}

	return &collectorpb.ExportTraceServiceResponse{}, nil
}

func (r *OTLPGRPCReceiver) recordError() {
	r.mu.Lock()
	r.errorCount++
	r.mu.Unlock()
}
