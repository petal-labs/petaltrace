package collector

import (
	"context"
	"sync"
	"testing"
	"time"

	collectorpb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

func TestOTLPGRPCReceiver_StartStop(t *testing.T) {
	receiver := NewOTLPGRPCReceiver(OTLPGRPCConfig{
		Addr: "127.0.0.1:0",
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- receiver.Start(ctx)
	}()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	addr := receiver.Addr()
	if addr == "" {
		t.Fatal("expected non-empty address")
	}

	// Connect and check health
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer conn.Close()

	healthClient := healthpb.NewHealthClient(conn)
	resp, err := healthClient.Check(context.Background(), &healthpb.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("health check failed: %v", err)
	}

	if resp.Status != healthpb.HealthCheckResponse_SERVING {
		t.Errorf("health status = %v, want SERVING", resp.Status)
	}

	// Stop
	if err := receiver.Stop(); err != nil {
		t.Errorf("Stop() error = %v", err)
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Start() returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for server to stop")
	}
}

func TestOTLPGRPCReceiver_ReceiveSpans(t *testing.T) {
	var mu sync.Mutex
	var receivedSpans []*IngestedSpan

	handler := SpanHandlerFunc(func(spans []*IngestedSpan) error {
		mu.Lock()
		receivedSpans = append(receivedSpans, spans...)
		mu.Unlock()
		return nil
	})

	receiver := NewOTLPGRPCReceiver(OTLPGRPCConfig{
		Addr:    "127.0.0.1:0",
		Handler: handler,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go receiver.Start(ctx)
	time.Sleep(100 * time.Millisecond)

	addr := receiver.Addr()

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer conn.Close()

	client := collectorpb.NewTraceServiceClient(conn)

	traceID := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	spanID := []byte{1, 2, 3, 4, 5, 6, 7, 8}

	req := &collectorpb.ExportTraceServiceRequest{
		ResourceSpans: []*tracepb.ResourceSpans{
			{
				Resource: &resourcepb.Resource{
					Attributes: []*commonpb.KeyValue{
						{Key: "service.name", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "grpc-test-service"}}},
					},
				},
				ScopeSpans: []*tracepb.ScopeSpans{
					{
						Spans: []*tracepb.Span{
							{
								TraceId:           traceID,
								SpanId:            spanID,
								Name:              "grpc-test-span",
								StartTimeUnixNano: uint64(time.Now().Add(-time.Second).UnixNano()),
								EndTimeUnixNano:   uint64(time.Now().UnixNano()),
								Attributes: []*commonpb.KeyValue{
									{Key: "gen_ai.system", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "openai"}}},
									{Key: "gen_ai.request.model", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "gpt-4o"}}},
								},
								Status: &tracepb.Status{
									Code: tracepb.Status_STATUS_CODE_OK,
								},
							},
						},
					},
				},
			},
		},
	}

	_, err = client.Export(context.Background(), req)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(receivedSpans) != 1 {
		t.Fatalf("received %d spans, want 1", len(receivedSpans))
	}

	span := receivedSpans[0]
	if span.Name != "grpc-test-span" {
		t.Errorf("span.Name = %s, want grpc-test-span", span.Name)
	}

	if span.Attributes["gen_ai.system"] != "openai" {
		t.Errorf("gen_ai.system = %v, want openai", span.Attributes["gen_ai.system"])
	}

	if span.Resource["service.name"] != "grpc-test-service" {
		t.Errorf("service.name = %v, want grpc-test-service", span.Resource["service.name"])
	}

	receiver.Stop()

	requests, spans, errors := receiver.Stats()
	if requests != 1 {
		t.Errorf("requests = %d, want 1", requests)
	}
	if spans != 1 {
		t.Errorf("spans = %d, want 1", spans)
	}
	if errors != 0 {
		t.Errorf("errors = %d, want 0", errors)
	}
}

func TestOTLPGRPCReceiver_BatchedSpans(t *testing.T) {
	var mu sync.Mutex
	var receivedSpans []*IngestedSpan

	handler := SpanHandlerFunc(func(spans []*IngestedSpan) error {
		mu.Lock()
		receivedSpans = append(receivedSpans, spans...)
		mu.Unlock()
		return nil
	})

	receiver := NewOTLPGRPCReceiver(OTLPGRPCConfig{
		Addr:    "127.0.0.1:0",
		Handler: handler,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go receiver.Start(ctx)
	time.Sleep(100 * time.Millisecond)

	conn, err := grpc.NewClient(receiver.Addr(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer conn.Close()

	client := collectorpb.NewTraceServiceClient(conn)

	traceID := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}

	req := &collectorpb.ExportTraceServiceRequest{
		ResourceSpans: []*tracepb.ResourceSpans{
			{
				ScopeSpans: []*tracepb.ScopeSpans{
					{
						Spans: []*tracepb.Span{
							{TraceId: traceID, SpanId: []byte{1, 2, 3, 4, 5, 6, 7, 1}, Name: "grpc-span-1"},
							{TraceId: traceID, SpanId: []byte{1, 2, 3, 4, 5, 6, 7, 2}, Name: "grpc-span-2"},
							{TraceId: traceID, SpanId: []byte{1, 2, 3, 4, 5, 6, 7, 3}, Name: "grpc-span-3"},
						},
					},
				},
			},
		},
	}

	_, err = client.Export(context.Background(), req)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(receivedSpans) != 3 {
		t.Fatalf("received %d spans, want 3", len(receivedSpans))
	}

	receiver.Stop()

	_, spans, _ := receiver.Stats()
	if spans != 3 {
		t.Errorf("stats spans = %d, want 3", spans)
	}
}

func TestOTLPGRPCReceiver_EmptyRequest(t *testing.T) {
	receiver := NewOTLPGRPCReceiver(OTLPGRPCConfig{
		Addr: "127.0.0.1:0",
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go receiver.Start(ctx)
	time.Sleep(100 * time.Millisecond)

	conn, err := grpc.NewClient(receiver.Addr(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer conn.Close()

	client := collectorpb.NewTraceServiceClient(conn)

	// Empty request
	req := &collectorpb.ExportTraceServiceRequest{}

	resp, err := client.Export(context.Background(), req)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	if resp == nil {
		t.Error("expected non-nil response")
	}

	receiver.Stop()
}
