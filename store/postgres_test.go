package store

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"testing"
	"time"
)

func TestPostgresStoreRunSpanLifecycle(t *testing.T) {
	traceStore := newPostgresTestStore(t)
	ctx := context.Background()
	prefix := "postgres-" + NewID()
	now := time.Now().UTC().Truncate(time.Microsecond)

	if err := runPostgresMigrations(ctx, traceStore.db); err != nil {
		t.Fatalf("repeat PostgreSQL migrations: %v", err)
	}

	runA := &Run{
		ID:           prefix + "-a",
		WorkflowID:   prefix,
		WorkflowName: "postgres integration",
		SourceKind:   "test",
		Status:       RunStatusRunning,
		StartedAt:    now.Add(-2 * time.Minute),
	}
	runB := &Run{
		ID:           prefix + "-b",
		WorkflowID:   prefix,
		WorkflowName: "postgres integration",
		SourceKind:   "test",
		Status:       RunStatusCompleted,
		StartedAt:    now.Add(-time.Minute),
	}
	for _, run := range []*Run{runA, runB} {
		if err := traceStore.CreateRun(ctx, run); err != nil {
			t.Fatalf("CreateRun(%s): %v", run.ID, err)
		}
	}

	replayStartedAt := now.Add(-30 * time.Second)
	replay := &ReplayRecord{
		ID:          prefix + "-replay",
		SourceRunID: runA.ID,
		Mode:        "mocked",
		Status:      "running",
		StartedAt:   replayStartedAt,
	}
	if err := traceStore.CreateReplay(ctx, replay); err != nil {
		t.Fatalf("CreateReplay(): %v", err)
	}
	replay.NewRunID = runB.ID
	replay.DiffID = prefix + "-diff"
	replay.Status = "completed"
	replayCompletedAt := replayStartedAt.Add(time.Second)
	replay.CompletedAt = &replayCompletedAt
	if err := traceStore.UpdateReplay(ctx, replay); err != nil {
		t.Fatalf("UpdateReplay(): %v", err)
	}
	persistedReplay, err := traceStore.GetReplay(ctx, replay.ID)
	if err != nil {
		t.Fatalf("GetReplay(): %v", err)
	}
	if persistedReplay == nil || persistedReplay.SourceRunID != runA.ID ||
		persistedReplay.NewRunID != runB.ID || persistedReplay.DiffID != replay.DiffID ||
		persistedReplay.Mode != replay.Mode || persistedReplay.Status != replay.Status ||
		persistedReplay.CompletedAt == nil || !persistedReplay.CompletedAt.Equal(*replay.CompletedAt) {
		t.Fatalf("persisted replay = %#v, want %#v", persistedReplay, replay)
	}
	replaysForSource, err := traceStore.ListReplays(ctx, runA.ID)
	if err != nil {
		t.Fatalf("ListReplays(source): %v", err)
	}
	if len(replaysForSource) != 1 || replaysForSource[0].ID != replay.ID {
		t.Fatalf("ListReplays(source) = %#v, want replay %q", replaysForSource, replay.ID)
	}

	updatedRun, err := traceStore.GetRun(ctx, runA.ID)
	if err != nil {
		t.Fatalf("GetRun(): %v", err)
	}
	if updatedRun == nil || updatedRun.WorkflowID != prefix {
		t.Fatalf("GetRun() = %#v, want workflow ID %q", updatedRun, prefix)
	}

	runB.Tags = map[string]string{"environment": "test"}
	runB.Starred = true
	if err := traceStore.UpdateRun(ctx, runB); err != nil {
		t.Fatalf("UpdateRun(): %v", err)
	}
	updatedRunB, err := traceStore.GetRun(ctx, runB.ID)
	if err != nil {
		t.Fatalf("GetRun() after update: %v", err)
	}
	if updatedRunB == nil || !updatedRunB.Starred || updatedRunB.Tags["environment"] != "test" {
		t.Fatalf("updated run = %#v, want starred run with environment tag", updatedRunB)
	}

	firstPage, cursor, err := traceStore.ListRuns(ctx, ListRunsOptions{
		WorkflowID: prefix,
		Limit:      1,
	})
	if err != nil {
		t.Fatalf("ListRuns(first page): %v", err)
	}
	if len(firstPage) != 1 || firstPage[0].ID != runB.ID || cursor != runB.ID {
		t.Fatalf("ListRuns(first page) = (%v, %q), want run %q and cursor %q", runIDs(firstPage), cursor, runB.ID, runB.ID)
	}
	secondPage, nextCursor, err := traceStore.ListRuns(ctx, ListRunsOptions{
		WorkflowID: prefix,
		Cursor:     cursor,
		Limit:      1,
	})
	if err != nil {
		t.Fatalf("ListRuns(second page): %v", err)
	}
	if len(secondPage) != 1 || secondPage[0].ID != runA.ID || nextCursor != "" {
		t.Fatalf("ListRuns(second page) = (%v, %q), want run %q and no cursor", runIDs(secondPage), nextCursor, runA.ID)
	}

	llm := &LLMSpanData{
		Provider: "anthropic",
		Model:    "test-model",
		Tokens: TokenDetail{
			InputTokens:  3,
			OutputTokens: 4,
			TotalTokens:  7,
			CostEstimate: 0.15,
		},
	}
	nodeSpan := &Span{
		ID:        prefix + "-node",
		RunID:     runA.ID,
		TraceID:   prefix,
		Kind:      SpanKindNode,
		Name:      "node",
		Status:    SpanStatusOK,
		StartedAt: now.Add(-90 * time.Second),
	}
	llmSpan := &Span{
		ID:        prefix + "-llm",
		RunID:     runA.ID,
		TraceID:   prefix,
		Kind:      SpanKindLLM,
		Name:      "llm",
		Status:    SpanStatusError,
		StartedAt: now.Add(-80 * time.Second),
		LLM:       llm,
	}
	if err := traceStore.CreateSpanBatch(ctx, []*Span{nodeSpan, llmSpan}); err != nil {
		t.Fatalf("CreateSpanBatch(): %v", err)
	}

	singleSpan := &Span{
		ID:        prefix + "-single",
		RunID:     runB.ID,
		TraceID:   prefix,
		Kind:      SpanKindTool,
		Name:      "tool",
		Status:    SpanStatusOK,
		StartedAt: now,
	}
	if err := traceStore.CreateSpan(ctx, singleSpan); err != nil {
		t.Fatalf("CreateSpan(): %v", err)
	}

	gotSpan, err := traceStore.GetSpan(ctx, singleSpan.ID)
	if err != nil {
		t.Fatalf("GetSpan(): %v", err)
	}
	if gotSpan == nil || gotSpan.Name != singleSpan.Name {
		t.Fatalf("GetSpan() = %#v, want span %q", gotSpan, singleSpan.ID)
	}
	singleSpan.Name = "updated tool"
	if err := traceStore.UpdateSpan(ctx, singleSpan); err != nil {
		t.Fatalf("UpdateSpan(): %v", err)
	}
	gotSpan, err = traceStore.GetSpan(ctx, singleSpan.ID)
	if err != nil {
		t.Fatalf("GetSpan() after update: %v", err)
	}
	if gotSpan == nil || gotSpan.Name != "updated tool" {
		t.Fatalf("updated span = %#v, want name %q", gotSpan, "updated tool")
	}
	spansByKind, err := traceStore.GetSpansByKind(ctx, runA.ID, SpanKindLLM)
	if err != nil {
		t.Fatalf("GetSpansByKind(): %v", err)
	}
	if len(spansByKind) != 1 || spansByKind[0].ID != llmSpan.ID {
		t.Fatalf("GetSpansByKind() IDs = %v, want [%s]", spanIDs(spansByKind), llmSpan.ID)
	}
	spanTree, err := traceStore.GetSpanTree(ctx, runA.ID)
	if err != nil {
		t.Fatalf("GetSpanTree(): %v", err)
	}
	if len(spanTree) != 2 {
		t.Fatalf("GetSpanTree() returned %d spans, want 2", len(spanTree))
	}

	if err := traceStore.UpdateRunAggregates(ctx, runA.ID); err != nil {
		t.Fatalf("UpdateRunAggregates(): %v", err)
	}
	updatedRun, err = traceStore.GetRun(ctx, runA.ID)
	if err != nil {
		t.Fatalf("GetRun() after aggregates: %v", err)
	}
	if updatedRun.TotalTokens.InputTokens != 3 ||
		updatedRun.TotalTokens.OutputTokens != 4 ||
		updatedRun.TotalTokens.TotalTokens != 7 ||
		updatedRun.EstimatedCost.Total != 0.15 ||
		updatedRun.NodeCount != 1 ||
		updatedRun.ErrorCount != 1 {
		t.Fatalf("run aggregates = %#v, want token counts (3, 4, 7), cost 0.15, one node, one error", updatedRun)
	}

	stats, err := traceStore.GetStats(ctx)
	if err != nil {
		t.Fatalf("GetStats(): %v", err)
	}
	if stats.RunCount != 2 || stats.SpanCount != 3 {
		t.Fatalf("stats counts = (%d, %d), want (2, 3)", stats.RunCount, stats.SpanCount)
	}

	if _, err := traceStore.SearchSpans(ctx, "query", 10); !errors.Is(err, ErrUnsupportedOperation) {
		t.Fatalf("SearchSpans() error = %v, want ErrUnsupportedOperation", err)
	}

	if err := traceStore.DeleteRun(ctx, runB.ID); err != nil {
		t.Fatalf("DeleteRun(): %v", err)
	}
	deletedRun, err := traceStore.GetRun(ctx, runB.ID)
	if err != nil {
		t.Fatalf("GetRun() after delete: %v", err)
	}
	if deletedRun != nil {
		t.Fatalf("GetRun() after delete = %#v, want nil", deletedRun)
	}
	deletedSpan, err := traceStore.GetSpan(ctx, singleSpan.ID)
	if err != nil {
		t.Fatalf("GetSpan() after run delete: %v", err)
	}
	if deletedSpan != nil {
		t.Fatalf("GetSpan() after run delete = %#v, want nil", deletedSpan)
	}
	if err := traceStore.DeleteRun(ctx, runA.ID); err != nil {
		t.Fatalf("DeleteRun(source): %v", err)
	}
	replaysForSource, err = traceStore.ListReplays(ctx, runA.ID)
	if err != nil {
		t.Fatalf("ListReplays() after source delete: %v", err)
	}
	if len(replaysForSource) != 0 {
		t.Fatalf("replays after source delete = %#v, want none", replaysForSource)
	}
}

func newPostgresTestStore(t *testing.T) *PostgresStore {
	t.Helper()

	dsn := os.Getenv("PETALTRACE_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("PETALTRACE_TEST_POSTGRES_DSN is not set")
	}
	parsedDSN, err := url.Parse(dsn)
	if err != nil || parsedDSN.Scheme == "" {
		t.Fatalf("PETALTRACE_TEST_POSTGRES_DSN must be a PostgreSQL URL")
	}

	adminDB, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open PostgreSQL test database: %v", err)
	}
	adminDB.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := adminDB.PingContext(ctx); err != nil {
		_ = adminDB.Close()
		t.Fatalf("connect to PostgreSQL test database: %v", err)
	}

	schema := "petaltrace_test_" + NewID()
	if _, err := adminDB.ExecContext(ctx, `CREATE SCHEMA "`+schema+`"`); err != nil {
		_ = adminDB.Close()
		t.Fatalf("create PostgreSQL test schema: %v", err)
	}
	t.Cleanup(func() {
		_, _ = adminDB.ExecContext(context.Background(), `DROP SCHEMA "`+schema+`" CASCADE`)
		_ = adminDB.Close()
	})

	query := parsedDSN.Query()
	query.Set("search_path", schema)
	parsedDSN.RawQuery = query.Encode()
	configuredStore, err := NewStore(ctx, StoreOptions{
		Driver:   "postgres",
		Postgres: PostgresOptions{DSN: parsedDSN.String()},
	})
	if err != nil {
		t.Fatalf("create store from PostgreSQL configuration: %v", err)
	}
	traceStore, ok := configuredStore.(*PostgresStore)
	if !ok {
		_ = configuredStore.Close()
		t.Fatalf("configured store type = %T, want *PostgresStore", configuredStore)
	}
	t.Cleanup(func() { _ = traceStore.Close() })
	return traceStore
}

func runIDs(runs []Run) []string {
	ids := make([]string, len(runs))
	for i := range runs {
		ids[i] = runs[i].ID
	}
	return ids
}

func spanIDs(spans []*Span) []string {
	ids := make([]string, len(spans))
	for i := range spans {
		ids[i] = spans[i].ID
	}
	return ids
}
