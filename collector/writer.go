package collector

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/petal-labs/petaltrace/store"
)

// BatchWriter buffers spans and writes them in batches to the store.
type BatchWriter struct {
	store         store.TraceStore
	textExtractor *TextExtractor
	logger        *slog.Logger

	batchSize     int
	flushInterval time.Duration

	mu      sync.Mutex
	buffer  []*CorrelatedSpan
	runUpdates map[string]*store.Run

	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	flushChan  chan struct{}
	closedChan chan struct{}

	// Metrics
	spansWritten   int64
	spansDropped   int64
	batchesWritten int64
}

// BatchWriterConfig configures the batch writer.
type BatchWriterConfig struct {
	Store         store.TraceStore
	TextExtractor *TextExtractor
	Logger        *slog.Logger
	BatchSize     int
	FlushInterval time.Duration
}

// NewBatchWriter creates a new batch writer.
func NewBatchWriter(cfg BatchWriterConfig) *BatchWriter {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 100
	}
	if cfg.FlushInterval <= 0 {
		cfg.FlushInterval = time.Second
	}
	if cfg.TextExtractor == nil {
		cfg.TextExtractor = NewTextExtractor(TextExtractorConfig{})
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &BatchWriter{
		store:         cfg.Store,
		textExtractor: cfg.TextExtractor,
		logger:        cfg.Logger,
		batchSize:     cfg.BatchSize,
		flushInterval: cfg.FlushInterval,
		buffer:        make([]*CorrelatedSpan, 0, cfg.BatchSize),
		runUpdates:    make(map[string]*store.Run),
		ctx:           ctx,
		cancel:        cancel,
		flushChan:     make(chan struct{}, 1),
		closedChan:    make(chan struct{}),
	}
}

// Start begins the background flush loop.
func (w *BatchWriter) Start() {
	w.wg.Add(1)
	go w.flushLoop()
}

// Stop gracefully shuts down the writer, flushing any remaining spans.
func (w *BatchWriter) Stop() error {
	// Signal the flush loop to stop
	close(w.closedChan)
	w.wg.Wait()

	// Final flush with background context (not the canceled one)
	w.mu.Lock()
	defer w.mu.Unlock()

	if len(w.buffer) == 0 {
		w.cancel()
		return nil
	}

	spans := w.buffer
	runUpdates := w.runUpdates
	w.buffer = make([]*CorrelatedSpan, 0, w.batchSize)
	w.runUpdates = make(map[string]*store.Run)

	// Use background context for final flush
	ctx := context.Background()

	// Convert to store spans
	storeSpans := make([]*store.Span, 0, len(spans))
	for _, cs := range spans {
		storeSpans = append(storeSpans, cs.Span)
	}

	// Write spans in batch
	if err := w.store.CreateSpanBatch(ctx, storeSpans); err != nil {
		w.spansDropped += int64(len(spans))
		w.logger.Error("final batch write failed", "error", err, "count", len(spans))
		w.cancel()
		return err
	}

	w.spansWritten += int64(len(spans))
	w.batchesWritten++

	// Index text for FTS
	extractedTexts := w.textExtractor.ExtractBatch(spans)
	for _, et := range extractedTexts {
		if err := w.store.IndexSpanText(ctx, et.SpanID, et.PromptText, et.Completion); err != nil {
			w.logger.Warn("failed to index span text on shutdown", "error", err, "span_id", et.SpanID)
		}
	}

	// Update runs
	for _, run := range runUpdates {
		if err := w.store.UpdateRun(ctx, run); err != nil {
			w.logger.Warn("failed to update run on shutdown", "error", err, "run_id", run.ID)
		}
	}

	w.logger.Debug("final flush on shutdown", "spans", len(spans), "runs", len(runUpdates))
	w.cancel()

	return nil
}

// Write adds spans to the buffer, flushing if the batch size is reached.
func (w *BatchWriter) Write(spans []*CorrelatedSpan) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.buffer = append(w.buffer, spans...)

	// Track run updates
	for _, cs := range spans {
		if cs.Run != nil {
			w.runUpdates[cs.Run.ID] = cs.Run
		}
	}

	if len(w.buffer) >= w.batchSize {
		return w.flushLocked()
	}

	return nil
}

// Flush writes all buffered spans to the store.
func (w *BatchWriter) Flush() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.flushLocked()
}

func (w *BatchWriter) flushLocked() error {
	if len(w.buffer) == 0 {
		return nil
	}

	spans := w.buffer
	runUpdates := w.runUpdates
	w.buffer = make([]*CorrelatedSpan, 0, w.batchSize)
	w.runUpdates = make(map[string]*store.Run)

	// Convert to store spans
	storeSpans := make([]*store.Span, 0, len(spans))
	for _, cs := range spans {
		storeSpans = append(storeSpans, cs.Span)
	}

	// Write spans in batch
	if err := w.store.CreateSpanBatch(w.ctx, storeSpans); err != nil {
		w.spansDropped += int64(len(spans))
		w.logger.Error("batch write failed", "error", err, "count", len(spans))
		return err
	}

	w.spansWritten += int64(len(spans))
	w.batchesWritten++

	// Index text for FTS
	extractedTexts := w.textExtractor.ExtractBatch(spans)
	for _, et := range extractedTexts {
		if err := w.store.IndexSpanText(w.ctx, et.SpanID, et.PromptText, et.Completion); err != nil {
			w.logger.Warn("failed to index span text", "error", err, "span_id", et.SpanID)
		}
	}

	// Update runs
	for _, run := range runUpdates {
		if err := w.store.UpdateRun(w.ctx, run); err != nil {
			w.logger.Warn("failed to update run", "error", err, "run_id", run.ID)
		}
	}

	w.logger.Debug("flushed batch", "spans", len(spans), "runs", len(runUpdates))

	return nil
}

func (w *BatchWriter) flushLoop() {
	defer w.wg.Done()

	ticker := time.NewTicker(w.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-w.closedChan:
			return
		case <-ticker.C:
			if err := w.Flush(); err != nil {
				w.logger.Error("periodic flush failed", "error", err)
			}
		case <-w.flushChan:
			if err := w.Flush(); err != nil {
				w.logger.Error("triggered flush failed", "error", err)
			}
		}
	}
}

// TriggerFlush requests an immediate flush.
func (w *BatchWriter) TriggerFlush() {
	select {
	case w.flushChan <- struct{}{}:
	default:
		// Flush already pending
	}
}

// Stats returns writer statistics.
func (w *BatchWriter) Stats() (written, dropped, batches int64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.spansWritten, w.spansDropped, w.batchesWritten
}

// BufferLen returns the current buffer length.
func (w *BatchWriter) BufferLen() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.buffer)
}
