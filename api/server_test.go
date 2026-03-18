package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/petal-labs/petaltrace/store"
)

// mockStore implements store.TraceStore for testing
type mockStore struct {
	runs      map[string]*store.Run
	spans     map[string][]*store.Span
	diffs     map[string]*store.RunDiff
	pricing   map[string]*store.PricingEntry
	stats     *store.StoreStats
	statsErr  error
}

func newMockStore() *mockStore {
	return &mockStore{
		runs:    make(map[string]*store.Run),
		spans:   make(map[string][]*store.Span),
		diffs:   make(map[string]*store.RunDiff),
		pricing: make(map[string]*store.PricingEntry),
		stats: &store.StoreStats{
			DatabaseSize: 1024,
			RunCount:     10,
			SpanCount:    100,
		},
	}
}

// Implement TraceStore interface
func (m *mockStore) Close() error { return nil }

func (m *mockStore) CreateRun(ctx context.Context, run *store.Run) error { return nil }
func (m *mockStore) GetRun(ctx context.Context, id string) (*store.Run, error) {
	return m.runs[id], nil
}
func (m *mockStore) UpdateRun(ctx context.Context, run *store.Run) error { return nil }
func (m *mockStore) DeleteRun(ctx context.Context, id string) error {
	delete(m.runs, id)
	return nil
}
func (m *mockStore) ListRuns(ctx context.Context, opts store.ListRunsOptions) ([]store.Run, string, error) {
	runs := make([]store.Run, 0)
	for _, r := range m.runs {
		runs = append(runs, *r)
	}
	return runs, "", nil
}
func (m *mockStore) UpdateRunAggregates(ctx context.Context, runID string) error { return nil }

func (m *mockStore) CreateSpan(ctx context.Context, span *store.Span) error      { return nil }
func (m *mockStore) CreateSpanBatch(ctx context.Context, spans []*store.Span) error { return nil }
func (m *mockStore) GetSpan(ctx context.Context, id string) (*store.Span, error) { return nil, nil }
func (m *mockStore) GetSpanTree(ctx context.Context, runID string) ([]*store.Span, error) {
	return m.spans[runID], nil
}
func (m *mockStore) GetSpansByKind(ctx context.Context, runID string, kind store.SpanKind) ([]*store.Span, error) {
	var result []*store.Span
	for _, span := range m.spans[runID] {
		if span.Kind == kind {
			result = append(result, span)
		}
	}
	return result, nil
}
func (m *mockStore) UpdateSpan(ctx context.Context, span *store.Span) error { return nil }

func (m *mockStore) IndexSpanText(ctx context.Context, spanID, promptText, completionText string) error {
	return nil
}
func (m *mockStore) SearchSpans(ctx context.Context, query string, limit int) ([]*store.Span, error) {
	return nil, nil
}

func (m *mockStore) CreateDiff(ctx context.Context, diff *store.RunDiff) error {
	m.diffs[diff.ID] = diff
	return nil
}
func (m *mockStore) GetDiff(ctx context.Context, id string) (*store.RunDiff, error) {
	return m.diffs[id], nil
}
func (m *mockStore) GetDiffByRuns(ctx context.Context, baseRunID, compareRunID string) (*store.RunDiff, error) {
	for _, d := range m.diffs {
		if d.BaseRunID == baseRunID && d.CompareRunID == compareRunID {
			return d, nil
		}
	}
	return nil, nil
}

func (m *mockStore) GetPricing(ctx context.Context, provider, model string) (*store.PricingEntry, error) {
	key := provider + ":" + model
	return m.pricing[key], nil
}
func (m *mockStore) UpsertPricing(ctx context.Context, entry *store.PricingEntry) error {
	key := entry.Provider + ":" + entry.Model
	m.pricing[key] = entry
	return nil
}
func (m *mockStore) ListPricing(ctx context.Context) ([]store.PricingEntry, error) {
	var entries []store.PricingEntry
	for _, e := range m.pricing {
		entries = append(entries, *e)
	}
	return entries, nil
}

func (m *mockStore) GetStats(ctx context.Context) (*store.StoreStats, error) {
	return m.stats, m.statsErr
}
func (m *mockStore) GarbageCollect(ctx context.Context, retentionDays int, dryRun bool) (int, error) {
	return 0, nil
}

func TestNewServer(t *testing.T) {
	ms := newMockStore()

	// Test with defaults
	s := NewServer(ServerConfig{
		Store: ms,
	})

	if s == nil {
		t.Fatal("NewServer() returned nil")
	}

	if s.store == nil {
		t.Error("store is nil")
	}

	if s.addr != ":8090" {
		t.Errorf("default addr = %q, want %q", s.addr, ":8090")
	}

	if s.liveSubscribers == nil {
		t.Error("liveSubscribers is nil")
	}

	// Test with custom addr
	s2 := NewServer(ServerConfig{
		Store: ms,
		Addr:  ":9000",
	})

	if s2.addr != ":9000" {
		t.Errorf("custom addr = %q, want %q", s2.addr, ":9000")
	}
}

func TestServer_Addr(t *testing.T) {
	ms := newMockStore()
	s := NewServer(ServerConfig{
		Store: ms,
		Addr:  ":8888",
	})

	if s.Addr() != ":8888" {
		t.Errorf("Addr() = %q, want %q", s.Addr(), ":8888")
	}
}

func TestServer_writeJSON(t *testing.T) {
	ms := newMockStore()
	s := NewServer(ServerConfig{Store: ms})

	w := httptest.NewRecorder()
	data := map[string]string{"message": "hello"}

	s.writeJSON(w, http.StatusOK, data)

	if w.Code != http.StatusOK {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusOK)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Content-Type = %q, want %q", contentType, "application/json")
	}

	var result map[string]string
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if result["message"] != "hello" {
		t.Errorf("message = %q, want %q", result["message"], "hello")
	}
}

func TestServer_writeError(t *testing.T) {
	ms := newMockStore()
	s := NewServer(ServerConfig{Store: ms})

	w := httptest.NewRecorder()
	s.writeError(w, http.StatusBadRequest, "bad request")

	if w.Code != http.StatusBadRequest {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusBadRequest)
	}

	var result map[string]string
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if result["error"] != "bad request" {
		t.Errorf("error = %q, want %q", result["error"], "bad request")
	}
}

func TestServer_corsMiddleware(t *testing.T) {
	ms := newMockStore()
	s := NewServer(ServerConfig{Store: ms})

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := s.corsMiddleware(inner)

	// Test OPTIONS request
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodOptions, "/", nil)
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusNoContent {
		t.Errorf("OPTIONS status code = %d, want %d", w.Code, http.StatusNoContent)
	}

	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("missing CORS allow origin header")
	}

	// Test normal request
	w2 := httptest.NewRecorder()
	r2 := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(w2, r2)

	if w2.Code != http.StatusOK {
		t.Errorf("GET status code = %d, want %d", w2.Code, http.StatusOK)
	}
}

func TestServer_recoveryMiddleware(t *testing.T) {
	ms := newMockStore()
	s := NewServer(ServerConfig{Store: ms})

	// Handler that panics
	panicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	})

	handler := s.recoveryMiddleware(panicHandler)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)

	// Should not panic
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestResponseWriter(t *testing.T) {
	w := httptest.NewRecorder()
	rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

	// Test WriteHeader
	rw.WriteHeader(http.StatusCreated)

	if rw.statusCode != http.StatusCreated {
		t.Errorf("statusCode = %d, want %d", rw.statusCode, http.StatusCreated)
	}
}

func TestAPIError(t *testing.T) {
	err := APIError{
		Error:   "something went wrong",
		Code:    "ERR_TEST",
		Details: map[string]string{"field": "value"},
	}

	if err.Error != "something went wrong" {
		t.Errorf("Error = %q, want %q", err.Error, "something went wrong")
	}

	if err.Code != "ERR_TEST" {
		t.Errorf("Code = %q, want %q", err.Code, "ERR_TEST")
	}
}

func TestPaginatedResponse(t *testing.T) {
	resp := PaginatedResponse{
		Data:       []int{1, 2, 3},
		Cursor:     "next_page",
		HasMore:    true,
		TotalCount: 100,
	}

	if resp.Cursor != "next_page" {
		t.Errorf("Cursor = %q, want %q", resp.Cursor, "next_page")
	}

	if !resp.HasMore {
		t.Error("HasMore should be true")
	}
}

func TestSSEEvent(t *testing.T) {
	event := SSEEvent{
		Event: "update",
		Data:  map[string]string{"status": "running"},
	}

	if event.Event != "update" {
		t.Errorf("Event = %q, want %q", event.Event, "update")
	}
}

func TestServer_writeSSE(t *testing.T) {
	ms := newMockStore()
	s := NewServer(ServerConfig{Store: ms})

	w := httptest.NewRecorder()
	event := &SSEEvent{
		Event: "test",
		Data:  map[string]string{"key": "value"},
	}

	err := s.writeSSE(w, event)
	if err != nil {
		t.Fatalf("writeSSE failed: %v", err)
	}

	body := w.Body.String()
	if !bytes.Contains([]byte(body), []byte("event: test")) {
		t.Error("SSE output should contain event name")
	}
	if !bytes.Contains([]byte(body), []byte("data:")) {
		t.Error("SSE output should contain data")
	}
}

func TestServer_handleHealth(t *testing.T) {
	ms := newMockStore()
	s := NewServer(ServerConfig{Store: ms})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/health", nil)

	s.handleHealth(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusOK)
	}

	var resp HealthResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != "healthy" {
		t.Errorf("Status = %q, want %q", resp.Status, "healthy")
	}

	if resp.Version != Version {
		t.Errorf("Version = %q, want %q", resp.Version, Version)
	}

	if resp.Details["store"] != "connected" {
		t.Errorf("store detail = %q, want %q", resp.Details["store"], "connected")
	}
}

func TestServer_handleHealth_Degraded(t *testing.T) {
	ms := newMockStore()
	ms.statsErr = http.ErrServerClosed // Use any error
	s := NewServer(ServerConfig{Store: ms})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/health", nil)

	s.handleHealth(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}

	var resp HealthResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != "degraded" {
		t.Errorf("Status = %q, want %q", resp.Status, "degraded")
	}
}

func TestServer_handleStats(t *testing.T) {
	ms := newMockStore()
	now := time.Now()
	ms.stats = &store.StoreStats{
		DatabaseSize: 2048,
		RunCount:     50,
		SpanCount:    500,
		DiffCount:    10,
		OldestRun:    now.Add(-24 * time.Hour),
		NewestRun:    now,
		TopWorkflows: []store.WorkflowStats{
			{WorkflowName: "workflow1", RunCount: 25},
		},
	}

	s := NewServer(ServerConfig{Store: ms})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/stats", nil)

	s.handleStats(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusOK)
	}

	var resp StatsResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Database.RunCount != 50 {
		t.Errorf("RunCount = %d, want %d", resp.Database.RunCount, 50)
	}

	if resp.Database.SpanCount != 500 {
		t.Errorf("SpanCount = %d, want %d", resp.Database.SpanCount, 500)
	}

	if len(resp.TopWorkflows) != 1 {
		t.Errorf("len(TopWorkflows) = %d, want 1", len(resp.TopWorkflows))
	}

	if resp.Runtime.GoVersion == "" {
		t.Error("GoVersion should not be empty")
	}
}
