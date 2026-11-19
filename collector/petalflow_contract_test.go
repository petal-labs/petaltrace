package collector

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/petal-labs/petaltrace/store"
)

func TestCorrelator_PetalFlowV1RuntimeAttributes(t *testing.T) {
	traceStore := setupTestStore(t)
	correlator := NewCorrelator(CorrelatorConfig{Store: traceStore})
	now := time.Now().UTC()
	graph := map[string]any{
		"id":    "workflow-1",
		"nodes": []any{map[string]any{"id": "answer", "type": "llm_prompt"}},
	}
	graphJSON, err := json.Marshal(graph)
	if err != nil {
		t.Fatalf("marshal graph fixture: %v", err)
	}
	input := map[string]any{"question": "hello"}
	inputJSON, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal input fixture: %v", err)
	}

	spans, err := correlator.ProcessSpans(context.Background(), []*IngestedSpan{
		{
			TraceID:   "trace-1",
			SpanID:    "run-span",
			Name:      "run:workflow-1",
			StartTime: now,
			EndTime:   now.Add(time.Second),
			Status:    SpanStatusOK,
			Attributes: map[string]any{
				"petalflow.run_id":           "run-v1",
				"petalflow.workflow_id":      "workflow-1",
				"petalflow.graph":            "workflow-1",
				"petalflow.graph_definition": string(graphJSON),
				"petalflow.input_snapshot":   string(inputJSON),
				"petalflow.trigger":          "petaltrace-replay",
				"petalflow.parent_run_id":    "source-run",
				"petalflow.source_kind":      "petalflow-replay",
				"petalflow.tag.replay_mode":  "hybrid",
				"petalflow.status":           "completed",
			},
			Resource: map[string]any{"service.name": "petalflow"},
		},
		{
			TraceID:      "trace-1",
			SpanID:       "node-span",
			ParentSpanID: "run-span",
			Name:         "node:answer",
			StartTime:    now,
			EndTime:      now.Add(250 * time.Millisecond),
			Status:       SpanStatusOK,
			Attributes: map[string]any{
				"petalflow.run_id":    "run-v1",
				"petalflow.node_id":   "answer",
				"petalflow.node_kind": "llm",
			},
			Resource: map[string]any{"service.name": "petalflow"},
		},
		{
			TraceID:      "trace-1",
			SpanID:       "llm-span",
			ParentSpanID: "node-span",
			Name:         "chat test-model",
			StartTime:    now,
			EndTime:      now.Add(200 * time.Millisecond),
			Status:       SpanStatusOK,
			Attributes: map[string]any{
				"petalflow.run_id":           "run-v1",
				"petalflow.node_id":          "answer",
				"gen_ai.operation.name":      "chat",
				"gen_ai.request.model":       "test-model",
				"gen_ai.provider.name":       "test-provider",
				"gen_ai.system":              "test-provider",
				"gen_ai.usage.input_tokens":  int64(7),
				"gen_ai.usage.output_tokens": int64(3),
				"petalflow.llm.completion":   "hello",
			},
			Resource: map[string]any{"service.name": "petalflow"},
		},
	})
	if err != nil {
		t.Fatalf("ProcessSpans(): %v", err)
	}
	if len(spans) != 3 {
		t.Fatalf("ProcessSpans() returned %d spans, want 3", len(spans))
	}
	if spans[0].Run.ID != "run-v1" {
		t.Fatalf("run ID = %q, want run-v1", spans[0].Run.ID)
	}
	if spans[0].Run.WorkflowID != "workflow-1" ||
		spans[0].Run.WorkflowName != "workflow-1" ||
		spans[0].Run.SourceKind != "petalflow-replay" ||
		spans[0].Run.ParentRunID == nil ||
		*spans[0].Run.ParentRunID != "source-run" ||
		spans[0].Run.Tags["replay_mode"] != "hybrid" {
		t.Fatalf("correlated run = %#v, want PetalFlow v1 runtime metadata", spans[0].Run)
	}
	if string(spans[0].Run.GraphSnapshot) != string(graphJSON) {
		t.Errorf("GraphSnapshot = %s, want %s", spans[0].Run.GraphSnapshot, graphJSON)
	}
	if string(spans[0].Run.InputSnapshot) != string(inputJSON) {
		t.Errorf("InputSnapshot = %s, want %s", spans[0].Run.InputSnapshot, inputJSON)
	}

	node := spans[1].Span
	if node.Kind != store.SpanKindNode || node.Node == nil || node.Node.NodeID != "answer" {
		t.Fatalf("node span = %#v, want node ID answer", node)
	}
	llm := spans[2].Span
	if llm.Kind != store.SpanKindLLM || llm.LLM == nil || llm.LLM.Provider != "test-provider" ||
		llm.LLM.Model != "test-model" || llm.LLM.Tokens.InputTokens != 7 ||
		llm.LLM.Tokens.OutputTokens != 3 || llm.LLM.Completion.TextContent != "hello" {
		t.Fatalf("LLM span = %#v, want PetalFlow v1 GenAI attributes", llm)
	}
}
