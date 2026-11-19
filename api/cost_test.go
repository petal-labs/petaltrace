package api

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/petal-labs/petaltrace/store"
)

// costFixture holds the deterministic fixture data shared by the cost
// integration tests, along with the expected aggregate values computed from
// it.
type costFixture struct {
	since    time.Time
	until    time.Time
	runIDs   []string
	started  map[string]time.Time
	workflow map[string]string
	provider map[string]string
	model    map[string]string
	input    map[string]int
	output   map[string]int
	cacheR   map[string]int
	cacheW   map[string]int
	total    map[string]int
	cost     map[string]float64

	// Expected aggregates over the in-window runs.
	wantTotalRuns  int
	wantTotalCost  float64
	wantByWorkflow map[string]costGroupExpect
	wantByProvider map[string]costGroupExpect
	wantByModel    map[string]costGroupExpect
}

type costGroupExpect struct {
	Runs   int
	Tokens int64
	Cost   float64
}

// newCostFixture builds a fixture with 25 in-window runs plus one run exactly
// on the since boundary and two runs before the window. Run ids are ordered
// so that, with the test page size of 10, matching runs span three pages.
func newCostFixture(now time.Time) *costFixture {
	f := &costFixture{
		since:    now.UTC().Truncate(time.Second).Add(-48 * time.Hour),
		until:    now.UTC().Truncate(time.Second),
		runIDs:   []string{},
		started:  map[string]time.Time{},
		workflow: map[string]string{},
		provider: map[string]string{},
		model:    map[string]string{},
		input:    map[string]int{},
		output:   map[string]int{},
		cacheR:   map[string]int{},
		cacheW:   map[string]int{},
		total:    map[string]int{},
		cost:     map[string]float64{},
	}

	addRun := func(id string, startedAt time.Time, n int, wf, provider, model string) {
		f.runIDs = append(f.runIDs, id)
		f.started[id] = startedAt
		f.workflow[id] = wf
		f.provider[id] = provider
		f.model[id] = model
		f.input[id] = 100 * n
		f.output[id] = 10 * n
		f.cacheR[id] = 2 * n
		f.cacheW[id] = 1 * n
		f.total[id] = 110 * n
		f.cost[id] = 0.01 * float64(n)
	}

	// Outside the window: must never be counted.
	for i := 1; i <= 2; i++ {
		id := fmt.Sprintf("cost-old-%d", i)
		addRun(id, f.since.Add(-24*time.Duration(i)*time.Hour), 50, "wf-outside", "outside-p", "outside-m")
	}

	// Exactly on the since boundary: started_at >= since, so included.
	addRun("cost-since-boundary", f.since, 1000, "wf-boundary", "mistral", "mistral-large")

	// In-window runs, ids cost-run-01..cost-run-25 (id-descending pages).
	for i := 1; i <= 25; i++ {
		id := fmt.Sprintf("cost-run-%02d", i)
		wf := "wf-alpha"
		provider, model := "anthropic", "claude-3-5-sonnet"
		if i >= 14 {
			wf = "wf-beta"
			provider, model = "openai", "gpt-4o"
		}
		addRun(id, f.until.Add(-time.Duration(i)*time.Hour), i, wf, provider, model)
	}

	f.wantTotalRuns = len(f.runIDs) - 2
	f.wantTotalCost = 0
	for _, id := range f.runIDs[2:] {
		f.wantTotalCost += f.cost[id]
	}

	aggregate := func(key func(id string) string) map[string]costGroupExpect {
		groups := map[string]costGroupExpect{}
		for _, id := range f.runIDs[2:] {
			g := groups[key(id)]
			g.Runs++
			g.Tokens += int64(f.total[id])
			g.Cost += f.cost[id]
			groups[key(id)] = g
		}
		return groups
	}

	f.wantByWorkflow = aggregate(func(id string) string { return f.workflow[id] })
	f.wantByProvider = aggregate(func(id string) string { return f.provider[id] })
	f.wantByModel = aggregate(func(id string) string { return f.model[id] })

	return f
}

func (f *costFixture) seed(t *testing.T) store.TraceStore {
	t.Helper()

	traceStore, err := store.NewSQLiteStore(store.SQLiteOptions{Path: filepath.Join(t.TempDir(), "cost.db")})
	if err != nil {
		t.Fatalf("open test store: %v", err)
	}
	t.Cleanup(func() { _ = traceStore.Close() })

	ctx := context.Background()
	for _, id := range f.runIDs {
		run := &store.Run{
			ID:           id,
			WorkflowID:   "wf-" + f.workflow[id],
			WorkflowName: f.workflow[id],
			SourceKind:   "test",
			Status:       store.RunStatusCompleted,
			StartedAt:    f.started[id],
			TotalTokens: store.TokenSummary{
				InputTokens:      f.input[id],
				OutputTokens:     f.output[id],
				CacheReadTokens:  f.cacheR[id],
				CacheWriteTokens: f.cacheW[id],
				TotalTokens:      f.total[id],
			},
			EstimatedCost: store.CostEstimate{Currency: "USD", Total: f.cost[id]},
		}
		if err := traceStore.CreateRun(ctx, run); err != nil {
			t.Fatalf("create run %s: %v", id, err)
		}

		span := &store.Span{
			RunID:     id,
			TraceID:   "trace-" + id,
			Kind:      store.SpanKindLLM,
			Name:      "llm-call",
			Status:    store.SpanStatusOK,
			StartedAt: f.started[id],
			LLM: &store.LLMSpanData{
				Provider: f.provider[id],
				Model:    f.model[id],
				Tokens: store.TokenDetail{
					InputTokens:  f.input[id],
					OutputTokens: f.output[id],
					TotalTokens:  f.total[id],
					CostEstimate: f.cost[id],
				},
			},
		}
		if err := traceStore.CreateSpan(ctx, span); err != nil {
			t.Fatalf("create span for %s: %v", id, err)
		}
	}

	return traceStore
}

func newCostTestServer(t *testing.T, traceStore store.TraceStore) *httptest.Server {
	t.Helper()

	srv := NewServer(ServerConfig{Store: traceStore})
	ts := httptest.NewServer(srv.handler())
	t.Cleanup(ts.Close)
	return ts
}

func costGet(t *testing.T, ts *httptest.Server, path string) map[string]any {
	t.Helper()

	resp, err := http.Get(ts.URL + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s status = %d, want 200", path, resp.StatusCode)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response for %s: %v", path, err)
	}
	return body
}

func TestCostSummaryTraversesAllPages(t *testing.T) {
	previousPageSize := costRunsPageSize
	t.Cleanup(func() { costRunsPageSize = previousPageSize })
	costRunsPageSize = 10

	f := newCostFixture(time.Now())
	traceStore := f.seed(t)
	ts := newCostTestServer(t, traceStore)

	body := costGet(t, ts, fmt.Sprintf("/api/cost/summary?since=%s&until=%s",
		f.since.Format(time.RFC3339), f.until.Format(time.RFC3339)))

	if got := int(body["total_runs"].(float64)); got != f.wantTotalRuns {
		t.Errorf("total_runs = %d, want %d", got, f.wantTotalRuns)
	}

	var totalTokens int64
	var totalCost float64
	var inputTokens, outputTokens, cacheRead, cacheWrite int64
	for _, id := range f.runIDs[2:] {
		totalTokens += int64(f.total[id])
		totalCost += f.cost[id]
		inputTokens += int64(f.input[id])
		outputTokens += int64(f.output[id])
		cacheRead += int64(f.cacheR[id])
		cacheWrite += int64(f.cacheW[id])
	}

	if got := int64(body["total_tokens"].(float64)); got != totalTokens {
		t.Errorf("total_tokens = %d, want %d", got, totalTokens)
	}
	if got := int64(body["input_tokens"].(float64)); got != inputTokens {
		t.Errorf("input_tokens = %d, want %d", got, inputTokens)
	}
	if got := int64(body["output_tokens"].(float64)); got != outputTokens {
		t.Errorf("output_tokens = %d, want %d", got, outputTokens)
	}
	if got := int64(body["cache_read_tokens"].(float64)); got != cacheRead {
		t.Errorf("cache_read_tokens = %d, want %d", got, cacheRead)
	}
	if got := int64(body["cache_write_tokens"].(float64)); got != cacheWrite {
		t.Errorf("cache_write_tokens = %d, want %d", got, cacheWrite)
	}
	if got := body["total_cost"].(float64); !floatsEqual(got, totalCost) {
		t.Errorf("total_cost = %f, want %f", got, totalCost)
	}

	assertCostGroups(t, "by_workflow", body["by_workflow"], f.wantByWorkflow)
	assertCostGroups(t, "by_provider", body["by_provider"], f.wantByProvider)
	assertCostGroups(t, "by_model", body["by_model"], f.wantByModel)
}

// TestCostSummaryTimeWindowBoundaries verifies that runs outside the
// since/until window are excluded from every page of the traversal.
func TestCostSummaryTimeWindowBoundaries(t *testing.T) {
	previousPageSize := costRunsPageSize
	t.Cleanup(func() { costRunsPageSize = previousPageSize })
	costRunsPageSize = 10

	f := newCostFixture(time.Now())
	traceStore := f.seed(t)
	ts := newCostTestServer(t, traceStore)

	// Window that excludes every in-window run (the oldest seeded run,
	// cost-old-2, sits inside it, so exactly 1 run is expected).
	narrowSince := f.since.Add(-72 * time.Hour)
	narrowUntil := f.since.Add(-25 * time.Hour)
	body := costGet(t, ts, fmt.Sprintf("/api/cost/summary?since=%s&until=%s",
		narrowSince.Format(time.RFC3339), narrowUntil.Format(time.RFC3339)))

	if got := int(body["total_runs"].(float64)); got != 1 {
		t.Errorf("total_runs = %d, want 1 for window containing only cost-old-2", got)
	}
	assertCostGroups(t, "by_workflow", body["by_workflow"], map[string]costGroupExpect{
		"wf-outside": {Runs: 1, Tokens: 5500, Cost: 0.5},
	})

	// group_by filtering still applies across all pages.
	body = costGet(t, ts, fmt.Sprintf("/api/cost/summary?since=%s&until=%s&group_by=workflow",
		f.since.Format(time.RFC3339), f.until.Format(time.RFC3339)))
	if _, ok := body["by_provider"]; ok {
		t.Error("group_by=workflow should omit by_provider")
	}
	if _, ok := body["by_model"]; ok {
		t.Error("group_by=workflow should omit by_model")
	}
	if _, ok := body["by_workflow"]; !ok {
		t.Error("group_by=workflow should keep by_workflow")
	}
}

func TestCostTimeseriesTraversesAllPages(t *testing.T) {
	previousPageSize := costRunsPageSize
	t.Cleanup(func() { costRunsPageSize = previousPageSize })
	costRunsPageSize = 10

	f := newCostFixture(time.Now())
	traceStore := f.seed(t)
	ts := newCostTestServer(t, traceStore)

	bucket := 24 * time.Hour
	body := costGet(t, ts, fmt.Sprintf("/api/cost/timeseries?since=%s&until=%s&bucket=24h",
		f.since.Format(time.RFC3339), f.until.Format(time.RFC3339)))

	raw, err := json.Marshal(body["data"])
	if err != nil {
		t.Fatalf("marshal data: %v", err)
	}
	var buckets []struct {
		Timestamp time.Time `json:"timestamp"`
		Runs      int       `json:"runs"`
		Tokens    int64     `json:"tokens"`
		Cost      float64   `json:"cost"`
	}
	if err := json.Unmarshal(raw, &buckets); err != nil {
		t.Fatalf("unmarshal buckets: %v", err)
	}

	// Build the expected buckets the same way the handler does.
	expected := map[int64]struct {
		Runs   int
		Tokens int64
		Cost   float64
	}{}
	for _, id := range f.runIDs[2:] {
		key := f.started[id].Truncate(bucket).Unix()
		e := expected[key]
		e.Runs++
		e.Tokens += int64(f.total[id])
		e.Cost += f.cost[id]
		expected[key] = e
	}

	if len(buckets) != len(expected) {
		t.Errorf("len(buckets) = %d, want %d", len(buckets), len(expected))
	}

	got := map[int64]struct {
		Runs   int
		Tokens int64
		Cost   float64
	}{}
	for _, b := range buckets {
		got[b.Timestamp.Unix()] = struct {
			Runs   int
			Tokens int64
			Cost   float64
		}{b.Runs, b.Tokens, b.Cost}
	}

	for key, want := range expected {
		have, ok := got[key]
		if !ok {
			t.Errorf("missing bucket at %s", time.Unix(key, 0).UTC())
			continue
		}
		if have.Runs != want.Runs {
			t.Errorf("bucket %s runs = %d, want %d", time.Unix(key, 0).UTC(), have.Runs, want.Runs)
		}
		if have.Tokens != want.Tokens {
			t.Errorf("bucket %s tokens = %d, want %d", time.Unix(key, 0).UTC(), have.Tokens, want.Tokens)
		}
		if have.Cost != want.Cost && !floatsEqual(have.Cost, want.Cost) {
			t.Errorf("bucket %s cost = %f, want %f", time.Unix(key, 0).UTC(), have.Cost, want.Cost)
		}
	}
}

// floatsEqual compares float sums with a tolerance because the handler and
// the fixture may add the same values in a different order.
func floatsEqual(a, b float64) bool {
	return math.Abs(a-b) <= 1e-9
}

func assertCostGroups(t *testing.T, field string, raw any, want map[string]costGroupExpect) {
	t.Helper()

	rawJSON, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal %s: %v", field, err)
	}
	var groups []struct {
		Name        string  `json:"name"`
		RunCount    int     `json:"run_count"`
		TotalTokens int64   `json:"total_tokens"`
		TotalCost   float64 `json:"total_cost"`
	}
	if err := json.Unmarshal(rawJSON, &groups); err != nil {
		t.Fatalf("unmarshal %s: %v", field, err)
	}

	if len(groups) != len(want) {
		t.Errorf("%s has %d groups, want %d", field, len(groups), len(want))
	}
	for _, g := range groups {
		w, ok := want[g.Name]
		if !ok {
			t.Errorf("%s has unexpected group %q", field, g.Name)
			continue
		}
		if g.RunCount != w.Runs {
			t.Errorf("%s[%s].run_count = %d, want %d", field, g.Name, g.RunCount, w.Runs)
		}
		if g.TotalTokens != w.Tokens {
			t.Errorf("%s[%s].total_tokens = %d, want %d", field, g.Name, g.TotalTokens, w.Tokens)
		}
		if g.TotalCost != w.Cost && !floatsEqual(g.TotalCost, w.Cost) {
			t.Errorf("%s[%s].total_cost = %f, want %f", field, g.Name, g.TotalCost, w.Cost)
		}
	}
}
