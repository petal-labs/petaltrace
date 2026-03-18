package collector

import (
	"bytes"
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	collectorpb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
)

func TestOTLPHTTPReceiver_StartStop(t *testing.T) {
	receiver := NewOTLPHTTPReceiver(OTLPHTTPConfig{
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

	// Health check
	resp, err := http.Get("http://" + addr + "/health")
	if err != nil {
		t.Fatalf("health check failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("health check status = %d, want %d", resp.StatusCode, http.StatusOK)
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

func TestOTLPHTTPReceiver_ReceiveSpans(t *testing.T) {
	var mu sync.Mutex
	var receivedSpans []*IngestedSpan

	handler := SpanHandlerFunc(func(spans []*IngestedSpan) error {
		mu.Lock()
		receivedSpans = append(receivedSpans, spans...)
		mu.Unlock()
		return nil
	})

	receiver := NewOTLPHTTPReceiver(OTLPHTTPConfig{
		Addr:    "127.0.0.1:0",
		Handler: handler,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go receiver.Start(ctx)
	time.Sleep(100 * time.Millisecond)

	addr := receiver.Addr()

	// Create test trace request
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
								Name:              "test-span",
								StartTimeUnixNano: uint64(time.Now().Add(-time.Second).UnixNano()),
								EndTimeUnixNano:   uint64(time.Now().UnixNano()),
								Attributes: []*commonpb.KeyValue{
									{Key: "gen_ai.system", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "anthropic"}}},
									{Key: "gen_ai.request.model", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "claude-sonnet-4"}}},
									{Key: "gen_ai.usage.input_tokens", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: 1000}}},
								},
								Status: &tracepb.Status{
									Code:    tracepb.Status_STATUS_CODE_OK,
									Message: "success",
								},
							},
						},
					},
				},
			},
		},
	}

	body, err := proto.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal request: %v", err)
	}

	httpReq, err := http.NewRequest(http.MethodPost, "http://"+addr+"/v1/traces", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	httpReq.Header.Set("Content-Type", "application/x-protobuf")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(receivedSpans) != 1 {
		t.Fatalf("received %d spans, want 1", len(receivedSpans))
	}

	span := receivedSpans[0]
	if span.Name != "test-span" {
		t.Errorf("span.Name = %s, want test-span", span.Name)
	}

	if span.TraceID != "0102030405060708090a0b0c0d0e0f10" {
		t.Errorf("span.TraceID = %s, want 0102030405060708090a0b0c0d0e0f10", span.TraceID)
	}

	if span.SpanID != "0102030405060708" {
		t.Errorf("span.SpanID = %s, want 0102030405060708", span.SpanID)
	}

	if span.Attributes["gen_ai.system"] != "anthropic" {
		t.Errorf("gen_ai.system = %v, want anthropic", span.Attributes["gen_ai.system"])
	}

	if span.Attributes["gen_ai.request.model"] != "claude-sonnet-4" {
		t.Errorf("gen_ai.request.model = %v, want claude-sonnet-4", span.Attributes["gen_ai.request.model"])
	}

	if span.Attributes["gen_ai.usage.input_tokens"] != int64(1000) {
		t.Errorf("gen_ai.usage.input_tokens = %v, want 1000", span.Attributes["gen_ai.usage.input_tokens"])
	}

	if span.Resource["service.name"] != "test-service" {
		t.Errorf("service.name = %v, want test-service", span.Resource["service.name"])
	}

	if span.Status != SpanStatusOK {
		t.Errorf("span.Status = %v, want SpanStatusOK", span.Status)
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

func TestOTLPHTTPReceiver_InvalidContentType(t *testing.T) {
	receiver := NewOTLPHTTPReceiver(OTLPHTTPConfig{
		Addr: "127.0.0.1:0",
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go receiver.Start(ctx)
	time.Sleep(100 * time.Millisecond)

	addr := receiver.Addr()

	req, _ := http.NewRequest(http.MethodPost, "http://"+addr+"/v1/traces", bytes.NewReader([]byte("test")))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnsupportedMediaType {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusUnsupportedMediaType)
	}

	receiver.Stop()
}

func TestOTLPHTTPReceiver_InvalidProtobuf(t *testing.T) {
	receiver := NewOTLPHTTPReceiver(OTLPHTTPConfig{
		Addr: "127.0.0.1:0",
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go receiver.Start(ctx)
	time.Sleep(100 * time.Millisecond)

	addr := receiver.Addr()

	req, _ := http.NewRequest(http.MethodPost, "http://"+addr+"/v1/traces", bytes.NewReader([]byte("not-protobuf")))
	req.Header.Set("Content-Type", "application/x-protobuf")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}

	receiver.Stop()
}

func TestOTLPHTTPReceiver_BatchedSpans(t *testing.T) {
	var mu sync.Mutex
	var receivedSpans []*IngestedSpan

	handler := SpanHandlerFunc(func(spans []*IngestedSpan) error {
		mu.Lock()
		receivedSpans = append(receivedSpans, spans...)
		mu.Unlock()
		return nil
	})

	receiver := NewOTLPHTTPReceiver(OTLPHTTPConfig{
		Addr:    "127.0.0.1:0",
		Handler: handler,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go receiver.Start(ctx)
	time.Sleep(100 * time.Millisecond)

	addr := receiver.Addr()

	// Create batch with multiple spans
	traceID := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}

	req := &collectorpb.ExportTraceServiceRequest{
		ResourceSpans: []*tracepb.ResourceSpans{
			{
				ScopeSpans: []*tracepb.ScopeSpans{
					{
						Spans: []*tracepb.Span{
							{TraceId: traceID, SpanId: []byte{1, 2, 3, 4, 5, 6, 7, 1}, Name: "span-1"},
							{TraceId: traceID, SpanId: []byte{1, 2, 3, 4, 5, 6, 7, 2}, Name: "span-2"},
							{TraceId: traceID, SpanId: []byte{1, 2, 3, 4, 5, 6, 7, 3}, Name: "span-3"},
						},
					},
				},
			},
			{
				ScopeSpans: []*tracepb.ScopeSpans{
					{
						Spans: []*tracepb.Span{
							{TraceId: traceID, SpanId: []byte{1, 2, 3, 4, 5, 6, 7, 4}, Name: "span-4"},
							{TraceId: traceID, SpanId: []byte{1, 2, 3, 4, 5, 6, 7, 5}, Name: "span-5"},
						},
					},
				},
			},
		},
	}

	body, _ := proto.Marshal(req)

	httpReq, _ := http.NewRequest(http.MethodPost, "http://"+addr+"/v1/traces", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/x-protobuf")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(receivedSpans) != 5 {
		t.Fatalf("received %d spans, want 5", len(receivedSpans))
	}

	receiver.Stop()

	_, spans, _ := receiver.Stats()
	if spans != 5 {
		t.Errorf("stats spans = %d, want 5", spans)
	}
}

func TestConvertOTLPSpans_Attributes(t *testing.T) {
	traceID := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	spanID := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	parentID := []byte{2, 3, 4, 5, 6, 7, 8, 9}

	resourceSpans := []*tracepb.ResourceSpans{
		{
			Resource: &resourcepb.Resource{
				Attributes: []*commonpb.KeyValue{
					{Key: "service.name", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "test"}}},
					{Key: "service.version", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "1.0.0"}}},
				},
			},
			ScopeSpans: []*tracepb.ScopeSpans{
				{
					Spans: []*tracepb.Span{
						{
							TraceId:           traceID,
							SpanId:            spanID,
							ParentSpanId:      parentID,
							Name:              "test-span",
							StartTimeUnixNano: 1000000000,
							EndTimeUnixNano:   2000000000,
							Attributes: []*commonpb.KeyValue{
								{Key: "string_attr", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "hello"}}},
								{Key: "int_attr", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: 42}}},
								{Key: "double_attr", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_DoubleValue{DoubleValue: 3.14}}},
								{Key: "bool_attr", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_BoolValue{BoolValue: true}}},
								{Key: "array_attr", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_ArrayValue{
									ArrayValue: &commonpb.ArrayValue{
										Values: []*commonpb.AnyValue{
											{Value: &commonpb.AnyValue_StringValue{StringValue: "a"}},
											{Value: &commonpb.AnyValue_StringValue{StringValue: "b"}},
										},
									},
								}}},
							},
							Events: []*tracepb.Span_Event{
								{
									Name:         "event1",
									TimeUnixNano: 1500000000,
									Attributes: []*commonpb.KeyValue{
										{Key: "event_attr", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "event_value"}}},
									},
								},
							},
							Status: &tracepb.Status{
								Code:    tracepb.Status_STATUS_CODE_ERROR,
								Message: "something went wrong",
							},
						},
					},
				},
			},
		},
	}

	spans := ConvertOTLPSpans(resourceSpans)
	if len(spans) != 1 {
		t.Fatalf("got %d spans, want 1", len(spans))
	}

	span := spans[0]

	if span.TraceID != "0102030405060708090a0b0c0d0e0f10" {
		t.Errorf("TraceID = %s, want 0102030405060708090a0b0c0d0e0f10", span.TraceID)
	}

	if span.SpanID != "0102030405060708" {
		t.Errorf("SpanID = %s, want 0102030405060708", span.SpanID)
	}

	if span.ParentSpanID != "0203040506070809" {
		t.Errorf("ParentSpanID = %s, want 0203040506070809", span.ParentSpanID)
	}

	if span.Name != "test-span" {
		t.Errorf("Name = %s, want test-span", span.Name)
	}

	if span.StartTime.UnixNano() != 1000000000 {
		t.Errorf("StartTime = %v, want 1000000000ns", span.StartTime.UnixNano())
	}

	if span.EndTime.UnixNano() != 2000000000 {
		t.Errorf("EndTime = %v, want 2000000000ns", span.EndTime.UnixNano())
	}

	if span.Attributes["string_attr"] != "hello" {
		t.Errorf("string_attr = %v, want hello", span.Attributes["string_attr"])
	}

	if span.Attributes["int_attr"] != int64(42) {
		t.Errorf("int_attr = %v, want 42", span.Attributes["int_attr"])
	}

	if span.Attributes["double_attr"] != 3.14 {
		t.Errorf("double_attr = %v, want 3.14", span.Attributes["double_attr"])
	}

	if span.Attributes["bool_attr"] != true {
		t.Errorf("bool_attr = %v, want true", span.Attributes["bool_attr"])
	}

	arr, ok := span.Attributes["array_attr"].([]any)
	if !ok || len(arr) != 2 {
		t.Errorf("array_attr = %v, want [a, b]", span.Attributes["array_attr"])
	}

	if span.Resource["service.name"] != "test" {
		t.Errorf("service.name = %v, want test", span.Resource["service.name"])
	}

	if len(span.Events) != 1 {
		t.Fatalf("got %d events, want 1", len(span.Events))
	}

	if span.Events[0].Name != "event1" {
		t.Errorf("event.Name = %s, want event1", span.Events[0].Name)
	}

	if span.Events[0].Attributes["event_attr"] != "event_value" {
		t.Errorf("event.Attributes[event_attr] = %v, want event_value", span.Events[0].Attributes["event_attr"])
	}

	if span.Status != SpanStatusError {
		t.Errorf("Status = %v, want SpanStatusError", span.Status)
	}

	if span.StatusMsg != "something went wrong" {
		t.Errorf("StatusMsg = %s, want 'something went wrong'", span.StatusMsg)
	}
}
