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
		cfg.Addr = ":4317"
	}

	return &OTLPGRPCReceiver{
		addr:    cfg.Addr,
		handler: cfg.Handler,
		logger:  cfg.Logger,
	}
}

// Start begins accepting gRPC connections.
func (r *OTLPGRPCReceiver) Start(ctx context.Context) error {
	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return errors.New("receiver already running")
	}

	ln, err := net.Listen("tcp", r.addr)
	if err != nil {
		r.mu.Unlock()
		return fmt.Errorf("failed to listen: %w", err)
	}
	r.listener = ln

	r.server = grpc.NewServer(
		grpc.MaxRecvMsgSize(64 * 1024 * 1024), // 64MB
		grpc.MaxSendMsgSize(64 * 1024 * 1024),
	)

	collectorpb.RegisterTraceServiceServer(r.server, r)

	// Register health service
	healthServer := health.NewServer()
	healthpb.RegisterHealthServer(r.server, healthServer)
	healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)

	r.running = true
	r.mu.Unlock()

	r.logger.Info("OTLP gRPC receiver started", "addr", ln.Addr().String())

	go func() {
		<-ctx.Done()
		r.Stop()
	}()

	if err := r.server.Serve(ln); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
		return fmt.Errorf("server error: %w", err)
	}

	return nil
}

// Stop gracefully shuts down the receiver.
func (r *OTLPGRPCReceiver) Stop() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.running {
		return nil
	}

	r.running = false
	r.logger.Info("stopping OTLP gRPC receiver")

	stopped := make(chan struct{})
	go func() {
		r.server.GracefulStop()
		close(stopped)
	}()

	select {
	case <-stopped:
		return nil
	case <-time.After(5 * time.Second):
		r.server.Stop()
		return nil
	}
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
