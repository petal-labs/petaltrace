package collector

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/petal-labs/petaltrace/pricing"
	"github.com/petal-labs/petaltrace/store"
)

// Collector orchestrates OTLP receivers, span processing, and storage.
type Collector struct {
	cfg          CollectorConfig
	logger       *slog.Logger
	store        store.TraceStore
	httpReceiver *OTLPHTTPReceiver
	grpcReceiver *OTLPGRPCReceiver
	correlator   *Correlator
	enricher     *Enricher
	writer       *BatchWriter

	mu      sync.Mutex
	running bool
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup

	// Metrics
	spansReceived  int64
	spansProcessed int64
}

// CollectorConfig configures the collector.
type CollectorConfig struct {
	Store        store.TraceStore
	PricingTable *pricing.PricingTable
	Logger       *slog.Logger

	// Receiver configuration
	HTTPAddr string
	GRPCAddr string

	// Processing configuration
	BatchSize     int
	FlushInterval time.Duration

	// Retention configuration
	InactiveRunAge time.Duration
}

// NewCollector creates a new collector.
func NewCollector(cfg CollectorConfig) (*Collector, error) {
	if cfg.Store == nil {
		return nil, errors.New("store is required")
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.HTTPAddr == "" {
		cfg.HTTPAddr = ":4318"
	}
	if cfg.GRPCAddr == "" {
		cfg.GRPCAddr = ":4317"
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 100
	}
	if cfg.FlushInterval <= 0 {
		cfg.FlushInterval = time.Second
	}
	if cfg.InactiveRunAge <= 0 {
		cfg.InactiveRunAge = 5 * time.Minute
	}

	classifier := NewClassifier()

	correlator := NewCorrelator(CorrelatorConfig{
		Store:      cfg.Store,
		Classifier: classifier,
		Logger:     cfg.Logger,
	})

	enricher := NewEnricher(EnricherConfig{
		PricingTable: cfg.PricingTable,
		Logger:       cfg.Logger,
	})

	textExtractor := NewTextExtractor(TextExtractorConfig{})

	writer := NewBatchWriter(BatchWriterConfig{
		Store:         cfg.Store,
		TextExtractor: textExtractor,
		Logger:        cfg.Logger,
		BatchSize:     cfg.BatchSize,
		FlushInterval: cfg.FlushInterval,
	})

	c := &Collector{
		cfg:        cfg,
		logger:     cfg.Logger,
		store:      cfg.Store,
		correlator: correlator,
		enricher:   enricher,
		writer:     writer,
	}

	// Create span handler that processes spans through the pipeline
	handler := SpanHandlerFunc(c.handleSpans)

	c.httpReceiver = NewOTLPHTTPReceiver(OTLPHTTPConfig{
		Addr:    cfg.HTTPAddr,
		Handler: handler,
		Logger:  cfg.Logger,
	})

	c.grpcReceiver = NewOTLPGRPCReceiver(OTLPGRPCConfig{
		Addr:    cfg.GRPCAddr,
		Handler: handler,
		Logger:  cfg.Logger,
	})

	return c, nil
}

// Start begins accepting traces.
func (c *Collector) Start(ctx context.Context) error {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return errors.New("collector already running")
	}

	c.ctx, c.cancel = context.WithCancel(ctx)
	c.running = true
	c.mu.Unlock()

	// Start writer
	c.writer.Start()

	// Start HTTP receiver
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		if err := c.httpReceiver.Start(c.ctx); err != nil {
			c.logger.Error("HTTP receiver error", "error", err)
		}
	}()

	// Start gRPC receiver
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		if err := c.grpcReceiver.Start(c.ctx); err != nil {
			c.logger.Error("gRPC receiver error", "error", err)
		}
	}()

	// Start inactive run cleanup
	c.wg.Add(1)
	go c.cleanupLoop()

	c.logger.Info("collector started",
		"http_addr", c.cfg.HTTPAddr,
		"grpc_addr", c.cfg.GRPCAddr,
	)

	return nil
}

// Stop gracefully shuts down the collector.
func (c *Collector) Stop() error {
	c.mu.Lock()
	if !c.running {
		c.mu.Unlock()
		return nil
	}
	c.running = false
	c.mu.Unlock()

	c.logger.Info("stopping collector")

	// Cancel context to stop receivers
	c.cancel()

	// Stop receivers
	c.httpReceiver.Stop()
	c.grpcReceiver.Stop()

	// Wait for background goroutines
	c.wg.Wait()

	// Stop writer (flushes remaining spans)
	if err := c.writer.Stop(); err != nil {
		c.logger.Error("writer stop error", "error", err)
	}

	c.logger.Info("collector stopped")
	return nil
}

// handleSpans processes incoming spans through the pipeline.
func (c *Collector) handleSpans(spans []*IngestedSpan) error {
	c.mu.Lock()
	c.spansReceived += int64(len(spans))
	c.mu.Unlock()

	// Correlate spans to runs
	correlated, err := c.correlator.ProcessSpans(c.ctx, spans)
	if err != nil {
		return fmt.Errorf("correlating spans: %w", err)
	}

	// Enrich spans with cost data
	c.enricher.EnrichSpans(c.ctx, correlated)

	// Write to store
	if err := c.writer.Write(correlated); err != nil {
		return fmt.Errorf("writing spans: %w", err)
	}

	c.mu.Lock()
	c.spansProcessed += int64(len(correlated))
	c.mu.Unlock()

	return nil
}

// cleanupLoop periodically cleans up inactive runs from the correlator cache.
func (c *Collector) cleanupLoop() {
	defer c.wg.Done()

	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			flushed := c.correlator.FlushInactiveRuns(c.cfg.InactiveRunAge)
			if len(flushed) > 0 {
				c.logger.Debug("flushed inactive runs", "count", len(flushed))
			}
		}
	}
}

// Stats returns collector statistics.
func (c *Collector) Stats() CollectorStats {
	c.mu.Lock()
	received := c.spansReceived
	processed := c.spansProcessed
	c.mu.Unlock()

	httpRequests, httpSpans, httpErrors := c.httpReceiver.Stats()
	grpcRequests, grpcSpans, grpcErrors := c.grpcReceiver.Stats()
	written, dropped, batches := c.writer.Stats()

	return CollectorStats{
		SpansReceived:  received,
		SpansProcessed: processed,
		SpansWritten:   written,
		SpansDropped:   dropped,
		BatchesWritten: batches,
		ActiveRuns:     c.correlator.ActiveRunCount(),
		BufferLen:      c.writer.BufferLen(),
		HTTP: ReceiverStats{
			Requests: httpRequests,
			Spans:    httpSpans,
			Errors:   httpErrors,
		},
		GRPC: ReceiverStats{
			Requests: grpcRequests,
			Spans:    grpcSpans,
			Errors:   grpcErrors,
		},
	}
}

// Health returns the health status of the collector.
func (c *Collector) Health() HealthStatus {
	c.mu.Lock()
	running := c.running
	c.mu.Unlock()

	status := HealthStatus{
		Healthy: running,
		Status:  "ok",
	}

	if !running {
		status.Status = "stopped"
	}

	return status
}

// HTTPAddr returns the HTTP receiver address.
func (c *Collector) HTTPAddr() string {
	return c.httpReceiver.Addr()
}

// GRPCAddr returns the gRPC receiver address.
func (c *Collector) GRPCAddr() string {
	return c.grpcReceiver.Addr()
}

// CollectorStats holds collector statistics.
type CollectorStats struct {
	SpansReceived  int64         `json:"spans_received"`
	SpansProcessed int64         `json:"spans_processed"`
	SpansWritten   int64         `json:"spans_written"`
	SpansDropped   int64         `json:"spans_dropped"`
	BatchesWritten int64         `json:"batches_written"`
	ActiveRuns     int           `json:"active_runs"`
	BufferLen      int           `json:"buffer_len"`
	HTTP           ReceiverStats `json:"http"`
	GRPC           ReceiverStats `json:"grpc"`
}

// ReceiverStats holds receiver statistics.
type ReceiverStats struct {
	Requests int64 `json:"requests"`
	Spans    int64 `json:"spans"`
	Errors   int64 `json:"errors"`
}

// HealthStatus represents the collector health.
type HealthStatus struct {
	Healthy bool   `json:"healthy"`
	Status  string `json:"status"`
}
