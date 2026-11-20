package store

import (
	"context"
	"database/sql"
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

	if _, err := traceStore.SearchSpans(ctx, "query", 10); err != nil {
		t.Fatalf("SearchSpans(): %v", err)
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

func TestPostgresStoreSearchPricingDiffsAndRetention(t *testing.T) {
	traceStore := newPostgresTestStore(t)
	ctx := context.Background()
	prefix := "pg-parity-" + NewID()
	now := time.Now().UTC().Truncate(time.Microsecond)

	// Retention fixture: two old unstarred runs, one old starred run, and
	// one recent run.
	oldRuns := []*Run{
		{ID: prefix + "-old-1", WorkflowID: prefix, WorkflowName: "old workflow one", SourceKind: "test", Status: RunStatusCompleted, StartedAt: now.AddDate(0, 0, -40)},
		{ID: prefix + "-old-2", WorkflowID: prefix, WorkflowName: "old workflow two", SourceKind: "test", Status: RunStatusCompleted, StartedAt: now.AddDate(0, 0, -35)},
		{ID: prefix + "-old-starred", WorkflowID: prefix, WorkflowName: "old starred workflow", SourceKind: "test", Status: RunStatusCompleted, StartedAt: now.AddDate(0, 0, -30), Starred: true},
		{ID: prefix + "-recent", WorkflowID: prefix, WorkflowName: "recent workflow", SourceKind: "test", Status: RunStatusCompleted, StartedAt: now.Add(-time.Minute)},
	}
	for _, run := range oldRuns {
		if err := traceStore.CreateRun(ctx, run); err != nil {
			t.Fatalf("CreateRun(%s): %v", run.ID, err)
		}
	}

	// Indexed spans: three indexed spans on distinct runs, newest first.
	indexed := []*Span{
		{ID: prefix + "-span-1", RunID: oldRuns[3].ID, TraceID: prefix, Kind: SpanKindLLM, Name: "llm newest", Status: SpanStatusOK, StartedAt: now.Add(-time.Minute)},
		{ID: prefix + "-span-2", RunID: oldRuns[0].ID, TraceID: prefix, Kind: SpanKindLLM, Name: "llm older", Status: SpanStatusOK, StartedAt: now.AddDate(0, 0, -40)},
		{ID: prefix + "-span-3", RunID: oldRuns[1].ID, TraceID: prefix, Kind: SpanKindLLM, Name: "llm oldest", Status: SpanStatusOK, StartedAt: now.AddDate(0, 0, -35)},
	}
	texts := map[string][2]string{
		prefix + "-span-1": {"retry payment invoice processing", "charge the customer card"},
		prefix + "-span-2": {"retry payment invoice processing", "charge the customer card"},
		prefix + "-span-3": {"totally unrelated content", "sunny day weather report"},
	}
	for _, span := range indexed {
		if err := traceStore.CreateSpan(ctx, span); err != nil {
			t.Fatalf("CreateSpan(%s): %v", span.ID, err)
		}
		text := texts[span.ID]
		if err := traceStore.IndexSpanText(ctx, span.ID, text[0], text[1]); err != nil {
			t.Fatalf("IndexSpanText(%s): %v", span.ID, err)
		}
	}

	// Search: token matches hit both prompt and completion text.
	results, err := traceStore.SearchSpans(ctx, "payment", 10)
	if err != nil {
		t.Fatalf("SearchSpans(payment): %v", err)
	}
	if len(results) != 2 || spanIDs(results)[0] != prefix+"-span-1" || spanIDs(results)[1] != prefix+"-span-2" {
		t.Fatalf("SearchSpans(payment) = %v, want [%s %s] ordered newest first", spanIDs(results), prefix+"-span-1", prefix+"-span-2")
	}
	results, err = traceStore.SearchSpans(ctx, "sunny weather", 10)
	if err != nil {
		t.Fatalf("SearchSpans(sunny weather): %v", err)
	}
	if len(results) != 1 || results[0].ID != prefix+"-span-3" {
		t.Fatalf("SearchSpans(sunny weather) = %v, want [%s]", spanIDs(results), prefix+"-span-3")
	}
	results, err = traceStore.SearchSpans(ctx, "zzznotfound", 10)
	if err != nil {
		t.Fatalf("SearchSpans(zzznotfound): %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("SearchSpans(zzznotfound) = %v, want no results", spanIDs(results))
	}
	results, err = traceStore.SearchSpans(ctx, "payment", 1)
	if err != nil {
		t.Fatalf("SearchSpans(payment, limit 1): %v", err)
	}
	if len(results) != 1 || results[0].ID != prefix+"-span-1" {
		t.Fatalf("SearchSpans(payment, limit 1) = %v, want only the newest match %q", spanIDs(results), prefix+"-span-1")
	}

	// Re-indexing a span replaces its indexed text.
	if err := traceStore.IndexSpanText(ctx, prefix+"-span-2", "replaced content", "nothing relevant here"); err != nil {
		t.Fatalf("re-index %s: %v", prefix+"-span-2", err)
	}
	results, err = traceStore.SearchSpans(ctx, "payment", 10)
	if err != nil {
		t.Fatalf("SearchSpans(payment) after re-index: %v", err)
	}
	if len(results) != 1 || results[0].ID != prefix+"-span-1" {
		t.Fatalf("SearchSpans(payment) after re-index = %v, want only [%s]", spanIDs(results), prefix+"-span-1")
	}
	results, err = traceStore.SearchSpans(ctx, "replaced", 10)
	if err != nil {
		t.Fatalf("SearchSpans(replaced): %v", err)
	}
	if len(results) != 1 || results[0].ID != prefix+"-span-2" {
		t.Fatalf("SearchSpans(replaced) = %v, want [%s]", spanIDs(results), prefix+"-span-2")
	}

	// Pricing: latest effective_from wins for a lookup; unknown lookups
	// return nil without error.
	base := now.Truncate(time.Microsecond)
	if err := traceStore.UpsertPricing(ctx, &PricingEntry{
		Provider: "anthropic", Model: "parity-model",
		InputPer1M: 3, OutputPer1M: 15,
		CacheReadPer1M: 0.3, CacheWritePer1M: 3.75,
		EffectiveFrom: base,
	}); err != nil {
		t.Fatalf("UpsertPricing(current): %v", err)
	}
	if err := traceStore.UpsertPricing(ctx, &PricingEntry{
		Provider: "anthropic", Model: "parity-model",
		InputPer1M: 1.5, OutputPer1M: 7.5,
		EffectiveFrom: base.AddDate(0, 0, -30),
	}); err != nil {
		t.Fatalf("UpsertPricing(older): %v", err)
	}
	if err := traceStore.UpsertPricing(ctx, &PricingEntry{
		Provider: "openai", Model: "parity-model",
		InputPer1M: 2, OutputPer1M: 8,
		EffectiveFrom: base,
	}); err != nil {
		t.Fatalf("UpsertPricing(openai): %v", err)
	}

	entry, err := traceStore.GetPricing(ctx, "anthropic", "parity-model")
	if err != nil {
		t.Fatalf("GetPricing(): %v", err)
	}
	if entry == nil || entry.InputPer1M != 3 || entry.OutputPer1M != 15 ||
		entry.CacheReadPer1M != 0.3 || entry.CacheWritePer1M != 3.75 ||
		!entry.EffectiveFrom.Equal(base) {
		t.Fatalf("GetPricing() = %#v, want the newest entry with cache rates", entry)
	}

	entry, err = traceStore.GetPricing(ctx, "nobody", "parity-model")
	if err != nil {
		t.Fatalf("GetPricing(unknown): %v", err)
	}
	if entry != nil {
		t.Fatalf("GetPricing(unknown) = %#v, want nil", entry)
	}

	entries, err := traceStore.ListPricing(ctx)
	if err != nil {
		t.Fatalf("ListPricing(): %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("ListPricing() returned %d entries, want 3", len(entries))
	}
	if entries[0].Provider != "anthropic" || entries[0].InputPer1M != 3 {
		t.Fatalf("ListPricing()[0] = %#v, want newest anthropic entry first", entries[0])
	}
	if entries[1].Provider != "anthropic" || entries[1].InputPer1M != 1.5 {
		t.Fatalf("ListPricing()[1] = %#v, want older anthropic entry second", entries[1])
	}
	if entries[2].Provider != "openai" {
		t.Fatalf("ListPricing()[2] = %#v, want openai entry last", entries[2])
	}

	// Diffs: stored with auto ID and retrievable both by ID and by run pair.
	created := &RunDiff{BaseRunID: oldRuns[0].ID, CompareRunID: oldRuns[3].ID}
	if err := traceStore.CreateDiff(ctx, created); err != nil {
		t.Fatalf("CreateDiff(): %v", err)
	}
	if created.ID == "" || created.CreatedAt.IsZero() {
		t.Fatalf("CreateDiff() did not populate ID and CreatedAt: %#v", created)
	}

	gotDiff, err := traceStore.GetDiff(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetDiff(): %v", err)
	}
	if gotDiff == nil || gotDiff.BaseRunID != created.BaseRunID || gotDiff.CompareRunID != created.CompareRunID {
		t.Fatalf("GetDiff() = %#v, want the created diff", gotDiff)
	}

	gotDiff, err = traceStore.GetDiffByRuns(ctx, created.BaseRunID, created.CompareRunID)
	if err != nil {
		t.Fatalf("GetDiffByRuns(): %v", err)
	}
	if gotDiff == nil || gotDiff.ID != created.ID {
		t.Fatalf("GetDiffByRuns() = %#v, want the created diff %q", gotDiff, created.ID)
	}

	gotDiff, err = traceStore.GetDiff(ctx, prefix+"-missing")
	if err != nil {
		t.Fatalf("GetDiff(missing): %v", err)
	}
	if gotDiff != nil {
		t.Fatalf("GetDiff(missing) = %#v, want nil", gotDiff)
	}

	// Retention: dry run counts without deleting; the real run deletes only
	// unstarred old runs and cascades their spans (and indexed text) away.
	deletedCount, err := traceStore.GarbageCollect(ctx, 30, true)
	if err != nil {
		t.Fatalf("GarbageCollect(dry run): %v", err)
	}
	if deletedCount != 2 {
		t.Fatalf("GarbageCollect(dry run) = %d, want 2", deletedCount)
	}
	for _, run := range oldRuns {
		got, err := traceStore.GetRun(ctx, run.ID)
		if err != nil {
			t.Fatalf("GetRun(%s) after dry run: %v", run.ID, err)
		}
		if got == nil {
			t.Fatalf("GetRun(%s) after dry run = nil, dry run must not delete", run.ID)
		}
	}

	deletedCount, err = traceStore.GarbageCollect(ctx, 30, false)
	if err != nil {
		t.Fatalf("GarbageCollect(): %v", err)
	}
	if deletedCount != 2 {
		t.Fatalf("GarbageCollect() = %d, want 2", deletedCount)
	}

	for _, id := range []string{prefix + "-old-1", prefix + "-old-2"} {
		got, err := traceStore.GetRun(ctx, id)
		if err != nil {
			t.Fatalf("GetRun(%s) after GC: %v", id, err)
		}
		if got != nil {
			t.Fatalf("GetRun(%s) after GC = %#v, want nil", id, got)
		}
	}
	for _, id := range []string{prefix + "-old-starred", prefix + "-recent"} {
		got, err := traceStore.GetRun(ctx, id)
		if err != nil {
			t.Fatalf("GetRun(%s) after GC: %v", id, err)
		}
		if got == nil {
			t.Fatalf("GetRun(%s) after GC = nil, want it preserved", id)
		}
	}

	// Cascade: the deleted run's spans (and indexed text) are gone.
	deletedSpan, err := traceStore.GetSpan(ctx, prefix+"-span-2")
	if err != nil {
		t.Fatalf("GetSpan(%s) after GC: %v", prefix+"-span-2", err)
	}
	if deletedSpan != nil {
		t.Fatalf("GetSpan(%s) after GC = %#v, want nil", prefix+"-span-2", deletedSpan)
	}
	results, err = traceStore.SearchSpans(ctx, "payment", 10)
	if err != nil {
		t.Fatalf("SearchSpans(payment) after GC: %v", err)
	}
	if len(results) != 1 || results[0].ID != prefix+"-span-1" {
		t.Fatalf("SearchSpans(payment) after GC = %v, want only [%s]", spanIDs(results), prefix+"-span-1")
	}

	stats, err := traceStore.GetStats(ctx)
	if err != nil {
		t.Fatalf("GetStats(): %v", err)
	}
	if stats.DatabaseSize <= 0 {
		t.Fatalf("GetStats().DatabaseSize = %d, want > 0", stats.DatabaseSize)
	}
	if stats.RunCount != 2 {
		t.Fatalf("GetStats().RunCount = %d, want 2 after GC", stats.RunCount)
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
