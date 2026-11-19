package replay

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/petal-labs/petaltrace/store"
)

func TestHybridReplayerSendsPetalFlowBearerToken(t *testing.T) {
	tokenReceived := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenReceived = r.Header.Get("Authorization") == "Bearer petalflow-test-token"
		if r.URL.Path != "/api/execute/hybrid" {
			t.Errorf("request path = %q, want /api/execute/hybrid", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(ExecuteResponse{RunID: "new-run", Status: "completed"})
	}))
	defer server.Close()

	replayer := NewHybridReplayer(HybridReplayerConfig{
		PetalFlowURL:   server.URL,
		PetalFlowToken: "petalflow-test-token",
	})
	response, err := replayer.executeHybrid(context.Background(), HybridExecuteRequest{
		ExecuteRequest: ExecuteRequest{WorkflowID: "workflow-1"},
	})
	if err != nil {
		t.Fatalf("executeHybrid(): %v", err)
	}
	if !tokenReceived {
		t.Error("PetalFlow hybrid request did not include its bearer token")
	}
	if response.RunID != "new-run" || response.Status != "completed" {
		t.Fatalf("executeHybrid() response = %#v, want completed new-run", response)
	}
}

func TestExtractToolMocksUsesPetalFlowNodeID(t *testing.T) {
	toolUseID := "llm-tool-use-id"
	spans := []*store.Span{
		{
			ID:         "tool-span-id",
			Attributes: map[string]any{"petalflow.node_id": "render"},
			Tool: &store.ToolSpanData{
				ToolUseID: &toolUseID,
				Outputs:   json.RawMessage(`{"rendered":"captured"}`),
			},
		},
	}

	mocks := ExtractToolMocks(spans)
	if len(mocks) != 1 {
		t.Fatalf("ExtractToolMocks() returned %d mocks, want 1", len(mocks))
	}
	if got := string(mocks["render"]); got != `{"rendered":"captured"}` {
		t.Fatalf("mock for node render = %s, want captured output", got)
	}
	if _, ok := mocks[toolUseID]; ok {
		t.Fatalf("ExtractToolMocks() keyed by tool-use ID %q, want node ID", toolUseID)
	}
}
