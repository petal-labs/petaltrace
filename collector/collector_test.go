package collector

import (
	"bytes"
	"context"
	"net/http"
	"testing"
	"time"

	collectorpb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"

	"github.com/petal-labs/petaltrace/pricing"
)

func TestCollector_StartStop(t *testing.T) {
	s := setupTestStore(t)
	pt, _ := pricing.LoadBuiltin()

	collector, err := NewCollector(CollectorConfig{
		Store:        s,
		PricingTable: pt,
		HTTPAddr:     "127.0.0.1:0",
		GRPCAddr:     "127.0.0.1:0",
	})
	if err != nil {
		t.Fatalf("NewCollector error: %v", err)
	}

	ctx := context.Background()
	if err := collector.Start(ctx); err != nil {
		t.Fatalf("Start error: %v", err)
	}

	// Wait a bit for receivers to start
	time.Sleep(100 * time.Millisecond)

	// Check addresses are assigned
	httpAddr := collector.HTTPAddr()
	grpcAddr := collector.GRPCAddr()

	if httpAddr == "" || httpAddr == "127.0.0.1:0" {
		t.Error("HTTP address not assigned")
	}
	if grpcAddr == "" || grpcAddr == "127.0.0.1:0" {
		t.Error("gRPC address not assigned")
	}

	// Check health
	health := collector.Health()
	if !health.Healthy {
		t.Error("collector not healthy")
	}
	if health.Status != "ok" {
		t.Errorf("health status = %s, want ok", health.Status)
	}

	// Stop
	if err := collector.Stop(); err != nil {
		t.Fatalf("Stop error: %v", err)
	}

	// Check health after stop
	health = collector.Health()
	if health.Healthy {
		t.Error("collector should not be healthy after stop")
	}
}

func TestCollector_ReceiveSpansHTTP(t *testing.T) {
	s := setupTestStore(t)
	pt, _ := pricing.LoadBuiltin()

	collector, err := NewCollector(CollectorConfig{
		Store:        s,
		PricingTable: pt,
		HTTPAddr:     "127.0.0.1:0",
		GRPCAddr:     "127.0.0.1:0",
		BatchSize:    1, // Flush immediately
	})
	if err != nil {
		t.Fatalf("NewCollector error: %v", err)
	}

	ctx := context.Background()
	if err := collector.Start(ctx); err != nil {
		t.Fatalf("Start error: %v", err)
	}
	defer collector.Stop()

	time.Sleep(100 * time.Millisecond)

	// Send spans via HTTP
	traceID := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	spanID := []byte{1, 2, 3, 4, 5, 6, 7, 8}

	req := &collectorpb.ExportTraceServiceRequest{
		ResourceSpans: []*tracepb.ResourceSpans{
			{
				Resource: &resourcepb.Resource{
					Attributes: []*commonpb.KeyValue{
						{Key: "service.name", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "test-service"}}},
					},
				},
				ScopeSpans: []*tracepb.ScopeSpans{
					{
						Spans: []*tracepb.Span{
							{
								TraceId:           traceID,
								SpanId:            spanID,
								Name:              "test-operation",
								StartTimeUnixNano: uint64(time.Now().Add(-time.Second).UnixNano()),
								EndTimeUnixNano:   uint64(time.Now().UnixNano()),
								Attributes: []*commonpb.KeyValue{
									{Key: "petalflow.run.id", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "run-http-test"}}},
								},
							},
						},
					},
				},
			},
		},
	}

	body, _ := proto.Marshal(req)

	httpAddr := collector.HTTPAddr()
	httpReq, _ := http.NewRequest(http.MethodPost, "http://"+httpAddr+"/v1/traces", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/x-protobuf")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		t.Fatalf("HTTP request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("HTTP status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	// Wait for processing
	time.Sleep(100 * time.Millisecond)

	// Check stats
	stats := collector.Stats()
	if stats.SpansReceived < 1 {
		t.Errorf("SpansReceived = %d, want >= 1", stats.SpansReceived)
	}
	if stats.HTTP.Requests < 1 {
		t.Errorf("HTTP.Requests = %d, want >= 1", stats.HTTP.Requests)
	}

	// Verify run was created
	run, err := s.GetRun(ctx, "run-http-test")
	if err != nil {
		t.Fatalf("GetRun error: %v", err)
	}
	if run == nil {
		t.Fatal("run not created")
	}
}

func TestCollector_ReceiveSpansGRPC(t *testing.T) {
	s := setupTestStore(t)
	pt, _ := pricing.LoadBuiltin()

	collector, err := NewCollector(CollectorConfig{
		Store:        s,
		PricingTable: pt,
		HTTPAddr:     "127.0.0.1:0",
		GRPCAddr:     "127.0.0.1:0",
		BatchSize:    1,
	})
	if err != nil {
		t.Fatalf("NewCollector error: %v", err)
	}

	ctx := context.Background()
	if err := collector.Start(ctx); err != nil {
		t.Fatalf("Start error: %v", err)
	}
	defer collector.Stop()

	time.Sleep(100 * time.Millisecond)

	// Connect via gRPC
	grpcAddr := collector.GRPCAddr()
	conn, err := grpc.NewClient(grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("gRPC connect failed: %v", err)
	}
	defer conn.Close()

	client := collectorpb.NewTraceServiceClient(conn)

	traceID := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	spanID := []byte{1, 2, 3, 4, 5, 6, 7, 8}

	req := &collectorpb.ExportTraceServiceRequest{
		ResourceSpans: []*tracepb.ResourceSpans{
			{
				ScopeSpans: []*tracepb.ScopeSpans{
					{
						Spans: []*tracepb.Span{
							{
								TraceId:           traceID,
								SpanId:            spanID,
								Name:              "grpc-operation",
								StartTimeUnixNano: uint64(time.Now().Add(-time.Second).UnixNano()),
								EndTimeUnixNano:   uint64(time.Now().UnixNano()),
								Attributes: []*commonpb.KeyValue{
									{Key: "petalflow.run.id", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "run-grpc-test"}}},
								},
							},
						},
					},
				},
			},
		},
	}

	_, err = client.Export(ctx, req)
	if err != nil {
		t.Fatalf("gRPC Export failed: %v", err)
	}

	// Wait for processing
	time.Sleep(100 * time.Millisecond)

	// Check stats
	stats := collector.Stats()
	if stats.GRPC.Requests < 1 {
		t.Errorf("GRPC.Requests = %d, want >= 1", stats.GRPC.Requests)
	}

	// Verify run was created
	run, err := s.GetRun(ctx, "run-grpc-test")
	if err != nil {
		t.Fatalf("GetRun error: %v", err)
	}
	if run == nil {
		t.Fatal("run not created")
	}
}

func TestCollector_LLMSpanWithCost(t *testing.T) {
	s := setupTestStore(t)
	pt, _ := pricing.LoadBuiltin()

	collector, err := NewCollector(CollectorConfig{
		Store:        s,
		PricingTable: pt,
		HTTPAddr:     "127.0.0.1:0",
		GRPCAddr:     "127.0.0.1:0",
		BatchSize:    1,
	})
	if err != nil {
		t.Fatalf("NewCollector error: %v", err)
	}

	ctx := context.Background()
	if err := collector.Start(ctx); err != nil {
		t.Fatalf("Start error: %v", err)
	}
	defer collector.Stop()

	time.Sleep(100 * time.Millisecond)

	// Send LLM span via HTTP
	traceID := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	spanID := []byte{1, 2, 3, 4, 5, 6, 7, 8}

	req := &collectorpb.ExportTraceServiceRequest{
		ResourceSpans: []*tracepb.ResourceSpans{
			{
				ScopeSpans: []*tracepb.ScopeSpans{
					{
						Spans: []*tracepb.Span{
							{
								TraceId:           traceID,
								SpanId:            spanID,
								Name:              "llm-completion",
								StartTimeUnixNano: uint64(time.Now().Add(-time.Second).UnixNano()),
								EndTimeUnixNano:   uint64(time.Now().UnixNano()),
								Attributes: []*commonpb.KeyValue{
									{Key: "petalflow.run.id", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "run-llm-test"}}},
									{Key: "gen_ai.system", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "anthropic"}}},
									{Key: "gen_ai.request.model", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "claude-sonnet-4-20250514"}}},
									{Key: "gen_ai.usage.input_tokens", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: 1000}}},
									{Key: "gen_ai.usage.output_tokens", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: 500}}},
									{Key: "gen_ai.system_prompt", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "You are helpful."}}},
									{Key: "gen_ai.completion", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "Here is my response."}}},
								},
							},
						},
					},
				},
			},
		},
	}

	body, _ := proto.Marshal(req)

	httpAddr := collector.HTTPAddr()
	httpReq, _ := http.NewRequest(http.MethodPost, "http://"+httpAddr+"/v1/traces", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/x-protobuf")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		t.Fatalf("HTTP request failed: %v", err)
	}
	defer resp.Body.Close()

	// Wait for processing
	time.Sleep(100 * time.Millisecond)

	// Verify run has cost
	run, err := s.GetRun(ctx, "run-llm-test")
	if err != nil {
		t.Fatalf("GetRun error: %v", err)
	}

	if run.TotalTokens.InputTokens != 1000 {
		t.Errorf("TotalTokens.InputTokens = %d, want 1000", run.TotalTokens.InputTokens)
	}
	if run.TotalTokens.OutputTokens != 500 {
		t.Errorf("TotalTokens.OutputTokens = %d, want 500", run.TotalTokens.OutputTokens)
	}
	if run.EstimatedCost.Total == 0 {
		t.Error("EstimatedCost.Total should be > 0")
	}

	// Verify span has LLM data
	spans, err := s.GetSpanTree(ctx, "run-llm-test")
	if err != nil {
		t.Fatalf("GetSpanTree error: %v", err)
	}

	if len(spans) != 1 {
		t.Fatalf("got %d spans, want 1", len(spans))
	}

	if spans[0].LLM == nil {
		t.Fatal("LLM data is nil")
	}
	if spans[0].LLM.Provider != "anthropic" {
		t.Errorf("LLM.Provider = %s, want anthropic", spans[0].LLM.Provider)
	}

	// Verify text was indexed
	results, err := s.SearchSpans(ctx, "helpful", 10)
	if err != nil {
		t.Fatalf("SearchSpans error: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("got %d search results, want 1", len(results))
	}
}

func TestCollector_Stats(t *testing.T) {
	s := setupTestStore(t)

	collector, err := NewCollector(CollectorConfig{
		Store:    s,
		HTTPAddr: "127.0.0.1:0",
		GRPCAddr: "127.0.0.1:0",
	})
	if err != nil {
		t.Fatalf("NewCollector error: %v", err)
	}

	ctx := context.Background()
	if err := collector.Start(ctx); err != nil {
		t.Fatalf("Start error: %v", err)
	}
	defer collector.Stop()

	stats := collector.Stats()

	// Initial stats should be zero
	if stats.SpansReceived != 0 {
		t.Errorf("initial SpansReceived = %d, want 0", stats.SpansReceived)
	}
	if stats.SpansProcessed != 0 {
		t.Errorf("initial SpansProcessed = %d, want 0", stats.SpansProcessed)
	}
	if stats.ActiveRuns != 0 {
		t.Errorf("initial ActiveRuns = %d, want 0", stats.ActiveRuns)
	}
}
