package store

import (
	"context"
	"time"
)

type TraceStore interface {
	Close() error

	CreateRun(ctx context.Context, run *Run) error
	GetRun(ctx context.Context, id string) (*Run, error)
	UpdateRun(ctx context.Context, run *Run) error
	DeleteRun(ctx context.Context, id string) error
	ListRuns(ctx context.Context, opts ListRunsOptions) ([]Run, string, error)
	UpdateRunAggregates(ctx context.Context, runID string) error

	CreateSpan(ctx context.Context, span *Span) error
	CreateSpanBatch(ctx context.Context, spans []*Span) error
	GetSpan(ctx context.Context, id string) (*Span, error)
	GetSpanTree(ctx context.Context, runID string) ([]*Span, error)
	GetSpansByKind(ctx context.Context, runID string, kind SpanKind) ([]*Span, error)
	UpdateSpan(ctx context.Context, span *Span) error

	IndexSpanText(ctx context.Context, spanID, promptText, completionText string) error
	SearchSpans(ctx context.Context, query string, limit int) ([]*Span, error)

	CreateDiff(ctx context.Context, diff *RunDiff) error
	GetDiff(ctx context.Context, id string) (*RunDiff, error)
	GetDiffByRuns(ctx context.Context, baseRunID, compareRunID string) (*RunDiff, error)

	GetPricing(ctx context.Context, provider, model string) (*PricingEntry, error)
	UpsertPricing(ctx context.Context, entry *PricingEntry) error
	ListPricing(ctx context.Context) ([]PricingEntry, error)

	GetStats(ctx context.Context) (*StoreStats, error)
	GarbageCollect(ctx context.Context, retentionDays int, dryRun bool) (int, error)
}

type StoreStats struct {
	DatabaseSize    int64     `json:"database_size_bytes"`
	RunCount        int64     `json:"run_count"`
	SpanCount       int64     `json:"span_count"`
	DiffCount       int64     `json:"diff_count"`
	OldestRun       time.Time `json:"oldest_run,omitempty"`
	NewestRun       time.Time `json:"newest_run,omitempty"`
	TopWorkflows    []WorkflowStats `json:"top_workflows"`
}

type WorkflowStats struct {
	WorkflowName string `json:"workflow_name"`
	RunCount     int64  `json:"run_count"`
}
