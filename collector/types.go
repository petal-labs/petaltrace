package collector

import (
	"time"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

// IngestedSpan represents a span extracted from OTLP format with parsed attributes.
type IngestedSpan struct {
	TraceID      string
	SpanID       string
	ParentSpanID string
	Name         string
	Kind         SpanKindHint
	StartTime    time.Time
	EndTime      time.Time
	Status       SpanStatusHint
	StatusMsg    string
	Attributes   map[string]any
	Resource     map[string]any
	Events       []SpanEvent
}

// SpanKindHint indicates the type of span based on OTel conventions.
type SpanKindHint int

const (
	SpanKindUnknown SpanKindHint = iota
	SpanKindLLM
	SpanKindNode
	SpanKindTool
	SpanKindEdge
	SpanKindCustom
)

// SpanStatusHint indicates the status of the span.
type SpanStatusHint int

const (
	SpanStatusUnset SpanStatusHint = iota
	SpanStatusOK
	SpanStatusError
)

// SpanEvent represents an event within a span.
type SpanEvent struct {
	Name       string
	Timestamp  time.Time
	Attributes map[string]any
}

// SpanHandler processes ingested spans.
type SpanHandler interface {
	HandleSpans(spans []*IngestedSpan) error
}

// SpanHandlerFunc is a function adapter for SpanHandler.
type SpanHandlerFunc func(spans []*IngestedSpan) error

func (f SpanHandlerFunc) HandleSpans(spans []*IngestedSpan) error {
	return f(spans)
}

// ConvertOTLPSpans converts OTLP trace data to IngestedSpans.
func ConvertOTLPSpans(resourceSpans []*tracepb.ResourceSpans) []*IngestedSpan {
	var result []*IngestedSpan

	for _, rs := range resourceSpans {
		resourceAttrs := extractResourceAttributes(rs.Resource)

		for _, ss := range rs.ScopeSpans {
			for _, span := range ss.Spans {
				ingested := convertSpan(span, resourceAttrs)
				result = append(result, ingested)
			}
		}
	}

	return result
}

func convertSpan(span *tracepb.Span, resourceAttrs map[string]any) *IngestedSpan {
	ingested := &IngestedSpan{
		TraceID:      traceIDToString(span.TraceId),
		SpanID:       spanIDToString(span.SpanId),
		ParentSpanID: spanIDToString(span.ParentSpanId),
		Name:         span.Name,
		StartTime:    time.Unix(0, int64(span.StartTimeUnixNano)),
		EndTime:      time.Unix(0, int64(span.EndTimeUnixNano)),
		Attributes:   extractAttributes(span.Attributes),
		Resource:     resourceAttrs,
		Events:       extractEvents(span.Events),
	}

	ingested.Status, ingested.StatusMsg = convertStatus(span.Status)

	return ingested
}

func extractResourceAttributes(resource *resourcepb.Resource) map[string]any {
	if resource == nil {
		return make(map[string]any)
	}
	return extractAttributes(resource.Attributes)
}

func extractAttributes(attrs []*commonpb.KeyValue) map[string]any {
	result := make(map[string]any)
	for _, kv := range attrs {
		result[kv.Key] = extractValue(kv.Value)
	}
	return result
}

func extractValue(v *commonpb.AnyValue) any {
	if v == nil {
		return nil
	}

	switch val := v.Value.(type) {
	case *commonpb.AnyValue_StringValue:
		return val.StringValue
	case *commonpb.AnyValue_IntValue:
		return val.IntValue
	case *commonpb.AnyValue_DoubleValue:
		return val.DoubleValue
	case *commonpb.AnyValue_BoolValue:
		return val.BoolValue
	case *commonpb.AnyValue_BytesValue:
		return val.BytesValue
	case *commonpb.AnyValue_ArrayValue:
		if val.ArrayValue == nil {
			return nil
		}
		arr := make([]any, len(val.ArrayValue.Values))
		for i, elem := range val.ArrayValue.Values {
			arr[i] = extractValue(elem)
		}
		return arr
	case *commonpb.AnyValue_KvlistValue:
		if val.KvlistValue == nil {
			return nil
		}
		return extractAttributes(val.KvlistValue.Values)
	default:
		return nil
	}
}

func extractEvents(events []*tracepb.Span_Event) []SpanEvent {
	result := make([]SpanEvent, 0, len(events))
	for _, e := range events {
		result = append(result, SpanEvent{
			Name:       e.Name,
			Timestamp:  time.Unix(0, int64(e.TimeUnixNano)),
			Attributes: extractAttributes(e.Attributes),
		})
	}
	return result
}

func convertStatus(status *tracepb.Status) (SpanStatusHint, string) {
	if status == nil {
		return SpanStatusUnset, ""
	}

	switch status.Code {
	case tracepb.Status_STATUS_CODE_OK:
		return SpanStatusOK, status.Message
	case tracepb.Status_STATUS_CODE_ERROR:
		return SpanStatusError, status.Message
	default:
		return SpanStatusUnset, status.Message
	}
}

func traceIDToString(id []byte) string {
	if len(id) != 16 {
		return ""
	}
	return bytesToHex(id)
}

func spanIDToString(id []byte) string {
	if len(id) != 8 {
		return ""
	}
	return bytesToHex(id)
}

func bytesToHex(b []byte) string {
	const hexChars = "0123456789abcdef"
	result := make([]byte, len(b)*2)
	for i, v := range b {
		result[i*2] = hexChars[v>>4]
		result[i*2+1] = hexChars[v&0x0f]
	}
	return string(result)
}
