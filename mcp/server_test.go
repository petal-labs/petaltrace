package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/petal-labs/petaltrace/store"
)

// mockStore implements store.TraceStore for testing
type mockStore struct {
	runs    map[string]*store.Run
	spans   map[string][]*store.Span
	diffs   map[string]*store.RunDiff
	pricing map[string]*store.PricingEntry
	stats   *store.StoreStats
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
func (m *mockStore) DeleteRun(ctx context.Context, id string) error      { return nil }
func (m *mockStore) ListRuns(ctx context.Context, opts store.ListRunsOptions) ([]store.Run, string, error) {
	runs := make([]store.Run, 0)
	for _, r := range m.runs {
		runs = append(runs, *r)
	}
	return runs, "", nil
}
func (m *mockStore) UpdateRunAggregates(ctx context.Context, runID string) error { return nil }

func (m *mockStore) CreateSpan(ctx context.Context, span *store.Span) error         { return nil }
func (m *mockStore) CreateSpanBatch(ctx context.Context, spans []*store.Span) error { return nil }
func (m *mockStore) GetSpan(ctx context.Context, id string) (*store.Span, error)    { return nil, nil }
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

func (m *mockStore) CreateDiff(ctx context.Context, diff *store.RunDiff) error { return nil }
func (m *mockStore) GetDiff(ctx context.Context, id string) (*store.RunDiff, error) {
	return m.diffs[id], nil
}
func (m *mockStore) GetDiffByRuns(ctx context.Context, baseRunID, compareRunID string) (*store.RunDiff, error) {
	return nil, nil
}

func (m *mockStore) GetPricing(ctx context.Context, provider, model string) (*store.PricingEntry, error) {
	return nil, nil
}
func (m *mockStore) UpsertPricing(ctx context.Context, entry *store.PricingEntry) error { return nil }
func (m *mockStore) ListPricing(ctx context.Context) ([]store.PricingEntry, error)      { return nil, nil }

func (m *mockStore) GetStats(ctx context.Context) (*store.StoreStats, error) { return m.stats, nil }
func (m *mockStore) GarbageCollect(ctx context.Context, retentionDays int, dryRun bool) (int, error) {
	return 0, nil
}

func TestNewServer(t *testing.T) {
	ms := newMockStore()
	var stdin bytes.Buffer
	var stdout bytes.Buffer

	s := NewServer(ServerConfig{
		Store:  ms,
		Stdin:  &stdin,
		Stdout: &stdout,
	})

	if s == nil {
		t.Fatal("NewServer() returned nil")
	}

	if s.store == nil {
		t.Error("store is nil")
	}

	if s.tools == nil {
		t.Error("tools is nil")
	}

	// Check that tools are registered
	if len(s.tools) == 0 {
		t.Error("no tools registered")
	}
}

func TestServer_Close(t *testing.T) {
	ms := newMockStore()
	s := NewServer(ServerConfig{Store: ms})

	// First close should succeed
	err := s.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// Second close should also succeed (no-op)
	err = s.Close()
	if err != nil {
		t.Errorf("second Close() error = %v", err)
	}
}

func TestServer_handleInitialize(t *testing.T) {
	ms := newMockStore()
	var stdout bytes.Buffer

	s := NewServer(ServerConfig{
		Store:  ms,
		Stdout: &stdout,
	})

	id := json.RawMessage(`1`)
	req := &Request{
		JSONRPC: "2.0",
		ID:      &id,
		Method:  "initialize",
	}

	s.handleInitialize(req)

	// Parse response
	var resp Response
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.Error != nil {
		t.Errorf("unexpected error: %v", resp.Error)
	}

	var result InitializeResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	if result.ServerInfo.Name != "petaltrace" {
		t.Errorf("ServerInfo.Name = %q, want %q", result.ServerInfo.Name, "petaltrace")
	}

	if result.Capabilities.Tools == nil {
		t.Error("Tools capability should be set")
	}
}

func TestServer_handleToolsList(t *testing.T) {
	ms := newMockStore()
	var stdout bytes.Buffer

	s := NewServer(ServerConfig{
		Store:  ms,
		Stdout: &stdout,
	})

	id := json.RawMessage(`2`)
	req := &Request{
		JSONRPC: "2.0",
		ID:      &id,
		Method:  "tools/list",
	}

	s.handleToolsList(req)

	// Parse response
	var resp Response
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.Error != nil {
		t.Errorf("unexpected error: %v", resp.Error)
	}

	var result ToolsListResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	if len(result.Tools) == 0 {
		t.Error("expected at least one tool")
	}

	// Check that each tool has required fields
	for _, tool := range result.Tools {
		if tool.Name == "" {
			t.Error("tool name is empty")
		}
		if tool.Description == "" {
			t.Errorf("tool %q has empty description", tool.Name)
		}
	}
}

func TestServer_handlePing(t *testing.T) {
	ms := newMockStore()
	var stdout bytes.Buffer

	s := NewServer(ServerConfig{
		Store:  ms,
		Stdout: &stdout,
	})

	id := json.RawMessage(`3`)
	req := &Request{
		JSONRPC: "2.0",
		ID:      &id,
		Method:  "ping",
	}

	s.handlePing(req)

	// Parse response
	var resp Response
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.Error != nil {
		t.Errorf("unexpected error: %v", resp.Error)
	}
}

func TestServer_handleRequest_UnknownMethod(t *testing.T) {
	ms := newMockStore()
	var stdout bytes.Buffer

	s := NewServer(ServerConfig{
		Store:  ms,
		Stdout: &stdout,
	})

	id := json.RawMessage(`4`)
	req := &Request{
		JSONRPC: "2.0",
		ID:      &id,
		Method:  "unknown/method",
	}

	s.handleRequest(context.Background(), req)

	// Parse response
	var resp Response
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.Error == nil {
		t.Error("expected error for unknown method")
	}

	if resp.Error.Code != -32601 {
		t.Errorf("error code = %d, want %d", resp.Error.Code, -32601)
	}
}

func TestServer_handleToolsCall_UnknownTool(t *testing.T) {
	ms := newMockStore()
	var stdout bytes.Buffer

	s := NewServer(ServerConfig{
		Store:  ms,
		Stdout: &stdout,
	})

	id := json.RawMessage(`5`)
	params, _ := json.Marshal(ToolsCallParams{
		Name: "unknown_tool",
	})
	req := &Request{
		JSONRPC: "2.0",
		ID:      &id,
		Method:  "tools/call",
		Params:  params,
	}

	s.handleToolsCall(context.Background(), req)

	// Parse response
	var resp Response
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.Error == nil {
		t.Error("expected error for unknown tool")
	}
}

func TestServer_sendResult(t *testing.T) {
	ms := newMockStore()
	var stdout bytes.Buffer

	s := NewServer(ServerConfig{
		Store:  ms,
		Stdout: &stdout,
	})

	id := json.RawMessage(`6`)
	s.sendResult(&id, map[string]string{"key": "value"})

	var resp Response
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.JSONRPC != "2.0" {
		t.Errorf("JSONRPC = %q, want %q", resp.JSONRPC, "2.0")
	}

	if resp.Error != nil {
		t.Error("unexpected error in response")
	}
}

func TestServer_sendError(t *testing.T) {
	ms := newMockStore()
	var stdout bytes.Buffer

	s := NewServer(ServerConfig{
		Store:  ms,
		Stdout: &stdout,
	})

	id := json.RawMessage(`7`)
	s.sendError(&id, -32600, "Invalid Request", "test error")

	var resp Response
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.Error == nil {
		t.Fatal("expected error in response")
	}

	if resp.Error.Code != -32600 {
		t.Errorf("error code = %d, want %d", resp.Error.Code, -32600)
	}

	if resp.Error.Message != "Invalid Request" {
		t.Errorf("error message = %q, want %q", resp.Error.Message, "Invalid Request")
	}
}

func TestServer_Run_ParseError(t *testing.T) {
	ms := newMockStore()
	stdin := strings.NewReader("invalid json\n")
	var stdout bytes.Buffer

	s := NewServer(ServerConfig{
		Store:  ms,
		Stdin:  stdin,
		Stdout: &stdout,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_ = s.Run(ctx)

	// Check for parse error response
	var resp Response
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.Error == nil {
		t.Error("expected parse error")
	}

	if resp.Error.Code != -32700 {
		t.Errorf("error code = %d, want %d (parse error)", resp.Error.Code, -32700)
	}
}

func TestServer_Run_ValidRequest(t *testing.T) {
	ms := newMockStore()

	// Create a valid JSON-RPC request
	id := json.RawMessage(`1`)
	req := Request{
		JSONRPC: "2.0",
		ID:      &id,
		Method:  "ping",
	}
	reqBytes, _ := json.Marshal(req)
	stdin := strings.NewReader(string(reqBytes) + "\n")
	var stdout bytes.Buffer

	s := NewServer(ServerConfig{
		Store:  ms,
		Stdin:  stdin,
		Stdout: &stdout,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_ = s.Run(ctx)

	// Parse response
	var resp Response
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.Error != nil {
		t.Errorf("unexpected error: %v", resp.Error)
	}
}

func TestToolInfo(t *testing.T) {
	info := ToolInfo{
		Name:        "test_tool",
		Description: "A test tool",
		InputSchema: json.RawMessage(`{"type": "object"}`),
	}

	if info.Name != "test_tool" {
		t.Errorf("Name = %q, want %q", info.Name, "test_tool")
	}
}

func TestContentBlock(t *testing.T) {
	block := ContentBlock{
		Type: "text",
		Text: "Hello world",
	}

	if block.Type != "text" {
		t.Errorf("Type = %q, want %q", block.Type, "text")
	}

	if block.Text != "Hello world" {
		t.Errorf("Text = %q, want %q", block.Text, "Hello world")
	}
}

func TestToolsCallResult(t *testing.T) {
	result := ToolsCallResult{
		Content: []ContentBlock{
			{Type: "text", Text: "result"},
		},
		IsError: false,
	}

	if len(result.Content) != 1 {
		t.Errorf("len(Content) = %d, want 1", len(result.Content))
	}

	if result.IsError {
		t.Error("IsError should be false")
	}

	// Error result
	errResult := ToolsCallResult{
		Content: []ContentBlock{
			{Type: "text", Text: "error message"},
		},
		IsError: true,
	}

	if !errResult.IsError {
		t.Error("IsError should be true")
	}
}

func TestRequest(t *testing.T) {
	id := json.RawMessage(`123`)
	req := Request{
		JSONRPC: "2.0",
		ID:      &id,
		Method:  "test/method",
		Params:  json.RawMessage(`{"key": "value"}`),
	}

	if req.JSONRPC != "2.0" {
		t.Errorf("JSONRPC = %q, want %q", req.JSONRPC, "2.0")
	}

	if req.Method != "test/method" {
		t.Errorf("Method = %q, want %q", req.Method, "test/method")
	}
}

func TestResponse(t *testing.T) {
	id := json.RawMessage(`456`)
	resp := Response{
		JSONRPC: "2.0",
		ID:      &id,
		Result:  json.RawMessage(`{"status": "ok"}`),
	}

	if resp.JSONRPC != "2.0" {
		t.Errorf("JSONRPC = %q, want %q", resp.JSONRPC, "2.0")
	}

	if resp.Error != nil {
		t.Error("Error should be nil for success response")
	}
}

func TestErrorObject(t *testing.T) {
	err := ErrorObject{
		Code:    -32600,
		Message: "Invalid Request",
		Data:    "additional info",
	}

	if err.Code != -32600 {
		t.Errorf("Code = %d, want %d", err.Code, -32600)
	}

	if err.Message != "Invalid Request" {
		t.Errorf("Message = %q, want %q", err.Message, "Invalid Request")
	}
}
