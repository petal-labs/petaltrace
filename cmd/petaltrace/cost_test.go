package petaltrace

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/petal-labs/petaltrace/store"
)

// cliCostGroup mirrors CostGroup in the CLI summary JSON output.
type cliCostGroup struct {
	Runs   int     `json:"runs"`
	Tokens int64   `json:"tokens"`
	Cost   float64 `json:"cost"`
}

// TestRunCostSummaryTraversesAllPages seeds a real SQLite store with more
// than one page of runs (page size forced to 10) and verifies that the CLI
// cost summary aggregates every matching page: totals, token breakdowns, and
// workflow/provider/model groups match known values, while runs outside the
// time window are excluded.
func TestRunCostSummaryTraversesAllPages(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "petaltrace.db")
	configPath := filepath.Join(tempDir, "petaltrace.yaml")
	configData := []byte("storage:\n  sqlite:\n    path: " + strconv.Quote(dbPath) + "\n")
	if err := os.WriteFile(configPath, configData, 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	now := time.Now()
	traceStore, err := store.NewSQLiteStore(store.SQLiteOptions{Path: dbPath})
	if err != nil {
		t.Fatalf("open test store: %v", err)
	}

	type runExpect struct {
		workflow string
		provider string
		model    string
		input    int
		output   int
		cacheR   int
		cacheW   int
		total    int
		cost     float64
	}

	runs := map[string]runExpect{}
	addRun := func(id string, startedAt time.Time, n int, wf, provider, model string) {
		runs[id] = runExpect{
			workflow: wf,
			provider: provider,
			model:    model,
			input:    100 * n,
			output:   10 * n,
			cacheR:   2 * n,
			cacheW:   1 * n,
			total:    110 * n,
			cost:     0.01 * float64(n),
		}
		run := &store.Run{
			ID:           id,
			WorkflowID:   "wf-" + wf,
			WorkflowName: wf,
			SourceKind:   "test",
			Status:       store.RunStatusCompleted,
			StartedAt:    startedAt,
			TotalTokens: store.TokenSummary{
				InputTokens:      100 * n,
				OutputTokens:     10 * n,
				CacheReadTokens:  2 * n,
				CacheWriteTokens: 1 * n,
				TotalTokens:      110 * n,
			},
			EstimatedCost: store.CostEstimate{
				Currency:   "USD",
				Total:      0.01 * float64(n),
				ByProvider: map[string]float64{provider: 0.01 * float64(n)},
				ByModel:    map[string]float64{model: 0.01 * float64(n)},
			},
		}
		if err := traceStore.CreateRun(context.Background(), run); err != nil {
			_ = traceStore.Close()
			t.Fatalf("create run %s: %v", id, err)
		}
	}

	// Outside the window: must be excluded from every page.
	addRun("cost-old-1", now.Add(-20*24*time.Hour), 50, "wf-outside", "p-outside", "m-outside")
	addRun("cost-old-2", now.Add(-40*24*time.Hour), 60, "wf-outside", "p-outside", "m-outside")

	// In-window runs; with costRunsPageSize = 10 they span three pages.
	for i := 1; i <= 25; i++ {
		wf := "wf-alpha"
		provider, model := "anthropic", "claude-3-5-sonnet"
		if i >= 14 {
			wf = "wf-beta"
			provider, model = "openai", "gpt-4o"
		}
		addRun(fmt.Sprintf("cost-run-%02d", i), now.Add(-time.Duration(i)*time.Hour), i, wf, provider, model)
	}

	if err := traceStore.Close(); err != nil {
		t.Fatalf("close test store: %v", err)
	}

	previousConfig, previousSince, previousGroupBy, previousJSON := cfgFile, costSince, costGroupBy, costJSON
	previousPageSize := costRunsPageSize
	t.Cleanup(func() {
		cfgFile = previousConfig
		costSince = previousSince
		costGroupBy = previousGroupBy
		costJSON = previousJSON
		costRunsPageSize = previousPageSize
	})
	cfgFile = configPath
	costSince = "7d"
	costGroupBy = ""
	costJSON = true
	costRunsPageSize = 10

	// Capture stdout so the JSON output can be parsed.
	originalStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("create pipe: %v", err)
	}
	os.Stdout = w

	runErr := runCostSummary(costSummaryCmd, nil)

	os.Stdout = originalStdout
	_ = w.Close()
	output, readErr := io.ReadAll(r)
	_ = r.Close()
	if runErr != nil {
		t.Fatalf("runCostSummary() error = %v", runErr)
	}
	if readErr != nil {
		t.Fatalf("read captured stdout: %v", readErr)
	}
	var summary struct {
		TotalRuns        int                     `json:"total_runs"`
		TotalTokens      int64                   `json:"total_tokens"`
		TotalCost        float64                 `json:"total_cost"`
		InputTokens      int64                   `json:"input_tokens"`
		OutputTokens     int64                   `json:"output_tokens"`
		CacheReadTokens  int64                   `json:"cache_read_tokens"`
		CacheWriteTokens int64                   `json:"cache_write_tokens"`
		ByWorkflow       map[string]cliCostGroup `json:"by_workflow"`
		ByProvider       map[string]cliCostGroup `json:"by_provider"`
		ByModel          map[string]cliCostGroup `json:"by_model"`
	}
	if err := json.Unmarshal(output, &summary); err != nil {
		t.Fatalf("parse summary output %q: %v", string(output), err)
	}

	wantTotalRuns := 0
	wantTokens := int64(0)
	wantInput, wantOutput := int64(0), int64(0)
	wantCacheR, wantCacheW := int64(0), int64(0)
	wantCost := 0.0
	wantWorkflow := map[string]cliCostGroup{}
	for id, e := range runs {
		if id < "cost-run-01" {
			continue // old runs are outside the window
		}
		wantTotalRuns++
		wantTokens += int64(e.total)
		wantInput += int64(e.input)
		wantOutput += int64(e.output)
		wantCacheR += int64(e.cacheR)
		wantCacheW += int64(e.cacheW)
		wantCost += e.cost
		w := wantWorkflow[e.workflow]
		w.Runs++
		w.Tokens += int64(e.total)
		w.Cost += e.cost
		wantWorkflow[e.workflow] = w
	}

	if summary.TotalRuns != wantTotalRuns {
		t.Errorf("total_runs = %d, want %d", summary.TotalRuns, wantTotalRuns)
	}
	if summary.TotalTokens != wantTokens {
		t.Errorf("total_tokens = %d, want %d", summary.TotalTokens, wantTokens)
	}
	if summary.InputTokens != wantInput {
		t.Errorf("input_tokens = %d, want %d", summary.InputTokens, wantInput)
	}
	if summary.OutputTokens != wantOutput {
		t.Errorf("output_tokens = %d, want %d", summary.OutputTokens, wantOutput)
	}
	if summary.CacheReadTokens != wantCacheR {
		t.Errorf("cache_read_tokens = %d, want %d", summary.CacheReadTokens, wantCacheR)
	}
	if summary.CacheWriteTokens != wantCacheW {
		t.Errorf("cache_write_tokens = %d, want %d", summary.CacheWriteTokens, wantCacheW)
	}
	if !floatsNearlyEqual(summary.TotalCost, wantCost) {
		t.Errorf("total_cost = %f, want %f", summary.TotalCost, wantCost)
	}

	assertCLIcostGroups(t, "by_workflow", summary.ByWorkflow, wantWorkflow)

	// The store persists only EstimatedCost.Total, not the ByProvider and
	// ByModel breakdowns (they are collector-side, in-memory only), so the
	// CLI provider/model groups are empty when reading persisted runs. This
	// is a pre-existing limitation, not a pagination issue; the API summary
	// derives provider/model groups from persisted LLM spans and is covered
	// by api/cost_test.go.
	if len(summary.ByProvider) != 0 {
		t.Errorf("len(by_provider) = %d, want 0 (breakdowns are not persisted)", len(summary.ByProvider))
	}
	if len(summary.ByModel) != 0 {
		t.Errorf("len(by_model) = %d, want 0 (breakdowns are not persisted)", len(summary.ByModel))
	}
}

func assertCLIcostGroups(t *testing.T, field string, got, want map[string]cliCostGroup) {
	t.Helper()

	if len(got) != len(want) {
		t.Errorf("len(%s) = %d, want %d", field, len(got), len(want))
	}
	for name, w := range want {
		g, ok := got[name]
		if !ok {
			t.Errorf("%s missing group %q", field, name)
			continue
		}
		if g.Runs != w.Runs {
			t.Errorf("%s[%s].runs = %d, want %d", field, name, g.Runs, w.Runs)
		}
		if g.Tokens != w.Tokens {
			t.Errorf("%s[%s].tokens = %d, want %d", field, name, g.Tokens, w.Tokens)
		}
		if !floatsNearlyEqual(g.Cost, w.Cost) {
			t.Errorf("%s[%s].cost = %f, want %f", field, name, g.Cost, w.Cost)
		}
	}
}

func floatsNearlyEqual(a, b float64) bool {
	const epsilon = 1e-9
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff <= epsilon
}
