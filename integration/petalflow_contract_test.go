package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/petal-labs/petaltrace/collector"
	"github.com/petal-labs/petaltrace/replay"
	"github.com/petal-labs/petaltrace/store"
)

func TestPetalFlowV120RuntimeContract(t *testing.T) {
	binary := os.Getenv("PETALFLOW_TEST_BINARY")
	if binary == "" {
		t.Skip("set PETALFLOW_TEST_BINARY to the PetalFlow v1.2.0 candidate binary")
	}
	if version := os.Getenv("PETALFLOW_TEST_VERSION"); version != "v1.2.0-rc.0" {
		t.Fatalf("PETALFLOW_TEST_VERSION = %q, want v1.2.0-rc.0", version)
	}

	dir := t.TempDir()
	traceStore, err := store.NewSQLiteStore(store.SQLiteOptions{Path: filepath.Join(dir, "petaltrace.db")})
	if err != nil {
		t.Fatalf("create PetalTrace store: %v", err)
	}
	defer traceStore.Close()

	httpAddr := freeTCPAddress(t)
	grpcAddr := freeTCPAddress(t)
	traceCollector, err := collector.NewCollector(collector.CollectorConfig{
		Store:         traceStore,
		HTTPAddr:      httpAddr,
		GRPCAddr:      grpcAddr,
		BatchSize:     1,
		FlushInterval: 20 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("create PetalTrace collector: %v", err)
	}
	if err := traceCollector.Start(context.Background()); err != nil {
		t.Fatalf("start PetalTrace collector: %v", err)
	}
	collectorStopped := false
	defer func() {
		if !collectorStopped {
			_ = traceCollector.Stop()
		}
	}()

	petalFlowAddr := freeTCPAddress(t)
	petalFlowPort := petalFlowAddr[strings.LastIndex(petalFlowAddr, ":")+1:]
	configPath := filepath.Join(dir, "petalflow.yaml")
	config := fmt.Sprintf(`observability:
  petaltrace:
    enabled: true
    endpoint: http://%s
    capture_mode: full
    sample_rate: 1
`, httpAddr)
	if err := os.WriteFile(configPath, []byte(config), 0600); err != nil {
		t.Fatalf("write PetalFlow config: %v", err)
	}

	var processOutput bytes.Buffer
	cmd := exec.Command(binary,
		"serve",
		"--host", "127.0.0.1",
		"--port", petalFlowPort,
		"--sqlite-path", filepath.Join(dir, "petalflow.db"),
		"--config", configPath,
	)
	cmd.Env = append(petalFlowTestEnv(dir),
		"PETALTRACE_ENDPOINT=http://"+httpAddr,
		"PETALTRACE_CAPTURE_MODE=full",
	)
	cmd.Stdout = &processOutput
	cmd.Stderr = &processOutput
	if err := cmd.Start(); err != nil {
		t.Fatalf("start PetalFlow %s: %v", binary, err)
	}
	processDone := make(chan struct{})
	var processErr error
	go func() {
		processErr = cmd.Wait()
		close(processDone)
	}()
	processStopped := false
	defer func() {
		if processStopped {
			return
		}
		_ = cmd.Process.Kill()
		<-processDone
	}()

	baseURL := "http://" + petalFlowAddr
	client := &http.Client{Timeout: 2 * time.Second}
	waitForPetalFlow(t, client, baseURL+"/health", processDone, &processErr, &processOutput)

	liveGraph, err := json.Marshal(map[string]any{
		"id":             "petaltrace-contract-live",
		"version":        "1",
		"schema_version": "v1",
		"kind":           "graph",
		"nodes":          []any{map[string]any{"id": "start", "type": "noop"}},
		"edges":          []any{},
		"entry":          "start",
	})
	if err != nil {
		t.Fatalf("marshal live graph: %v", err)
	}
	liveInput := json.RawMessage(`{"question":"hello"}`)
	now := time.Now().UTC()
	liveSource := &store.Run{
		ID:             "source-live",
		WorkflowID:     "petaltrace-contract-live",
		WorkflowName:   "PetalTrace contract live",
		SourceKind:     "petalflow",
		Status:         store.RunStatusCompleted,
		StartedAt:      now,
		GraphSnapshot:  liveGraph,
		InputSnapshot:  liveInput,
		ConfigSnapshot: json.RawMessage(`{"environment":"contract-test"}`),
	}
	if err := traceStore.CreateRun(context.Background(), liveSource); err != nil {
		t.Fatalf("create live source run: %v", err)
	}

	hybridGraph, err := json.Marshal(map[string]any{
		"id":             "petaltrace-contract-hybrid",
		"schema_version": "v1",
		"kind":           "graph",
		"nodes": []any{map[string]any{
			"id":   "render",
			"type": "tool",
			"config": map[string]any{
				"tool_name":   "template_render.render",
				"static_args": map[string]any{"template": "real tool output"},
				"output_key":  "result",
			},
		}},
		"edges": []any{},
		"entry": "render",
	})
	if err != nil {
		t.Fatalf("marshal hybrid graph: %v", err)
	}
	hybridSource := &store.Run{
		ID:            "source-hybrid",
		WorkflowID:    "petaltrace-contract-hybrid",
		WorkflowName:  "PetalTrace contract hybrid",
		SourceKind:    "petalflow",
		Status:        store.RunStatusCompleted,
		StartedAt:     now,
		GraphSnapshot: hybridGraph,
		InputSnapshot: json.RawMessage(`{}`),
	}
	if err := traceStore.CreateRun(context.Background(), hybridSource); err != nil {
		t.Fatalf("create hybrid source run: %v", err)
	}
	toolCompletedAt := now.Add(time.Millisecond)
	if err := traceStore.CreateSpan(context.Background(), &store.Span{
		ID:          "source-hybrid-tool",
		RunID:       hybridSource.ID,
		TraceID:     "source-hybrid-trace",
		Kind:        store.SpanKindTool,
		Name:        "template_render.render",
		Status:      store.SpanStatusOK,
		StartedAt:   now,
		CompletedAt: &toolCompletedAt,
		Node: &store.NodeSpanData{
			NodeID:   "render",
			NodeType: "tool",
		},
		Tool: &store.ToolSpanData{
			ToolName: "template_render.render",
			Outputs:  json.RawMessage(`{"rendered":"captured tool output"}`),
		},
	}); err != nil {
		t.Fatalf("create hybrid source tool span: %v", err)
	}

	replayEngine := replay.NewEngine(replay.EngineConfig{
		Store:          traceStore,
		PetalFlowURL:   baseURL,
		PetalFlowToken: "integration-test-token",
	})
	liveResult, err := replayEngine.Execute(context.Background(), &replay.ReplayRequest{
		SourceRunID: "source-live",
		Mode:        replay.ReplayModeLive,
		Tags:        map[string]string{"replay_mode": "live"},
	})
	if err != nil {
		t.Fatalf("execute live replay: %v", err)
	}
	hybridResult, err := replayEngine.Execute(context.Background(), &replay.ReplayRequest{
		SourceRunID: "source-hybrid",
		Mode:        replay.ReplayModeHybrid,
		Tags:        map[string]string{"replay_mode": "hybrid"},
	})
	if err != nil {
		t.Fatalf("execute hybrid replay: %v", err)
	}

	stopPetalFlow(t, cmd, processDone, &processErr, &processStopped, &processOutput)
	if err := traceCollector.Stop(); err != nil {
		t.Fatalf("stop PetalTrace collector: %v", err)
	}
	collectorStopped = true

	if liveResult.Status != replay.ReplayStatusCompleted || liveResult.NewRunID == "" {
		t.Fatalf("live replay result = %#v, want completed replay with new run ID", liveResult)
	}
	if hybridResult.Status != replay.ReplayStatusCompleted || hybridResult.NewRunID == "" {
		t.Fatalf("hybrid replay result = %#v, want completed replay with new run ID", hybridResult)
	}

	liveRun, err := traceStore.GetRun(context.Background(), liveResult.NewRunID)
	if err != nil {
		t.Fatalf("load live run %q: %v", liveResult.NewRunID, err)
	}
	if liveRun == nil {
		t.Fatalf("live run %q was not ingested from OTLP; collector stats = %#v; PetalFlow output:\n%s",
			liveResult.NewRunID, traceCollector.Stats(), processOutput.String())
	}
	assertReplayRunMetadata(t, liveRun, "PetalTrace contract live", "live", "petaltrace-replay", "source-live")
	var graphDefinition map[string]any
	if err := json.Unmarshal(liveRun.GraphSnapshot, &graphDefinition); err != nil {
		t.Fatalf("decode emitted graph snapshot %q: %v", liveRun.GraphSnapshot, err)
	}
	if graphDefinition["id"] != "petaltrace-contract-live" {
		t.Fatalf("captured graph ID = %#v, want petaltrace-contract-live", graphDefinition["id"])
	}
	if liveRun.WorkflowVersion != "1" {
		t.Fatalf("captured workflow version = %q, want 1", liveRun.WorkflowVersion)
	}
	var inputs map[string]any
	if err := json.Unmarshal(liveRun.InputSnapshot, &inputs); err != nil {
		t.Fatalf("decode emitted input snapshot %q: %v", liveRun.InputSnapshot, err)
	}
	if inputs["question"] != "hello" {
		t.Fatalf("captured input snapshot = %#v, want question hello", inputs)
	}
	var configSnapshot map[string]any
	if err := json.Unmarshal(liveRun.ConfigSnapshot, &configSnapshot); err != nil {
		t.Fatalf("decode emitted config snapshot %q: %v", liveRun.ConfigSnapshot, err)
	}
	if configSnapshot["environment"] != "contract-test" {
		t.Fatalf("captured config snapshot = %#v, want contract-test", configSnapshot)
	}

	hybridRun, err := traceStore.GetRun(context.Background(), hybridResult.NewRunID)
	if err != nil {
		t.Fatalf("load hybrid run %q: %v", hybridResult.NewRunID, err)
	}
	if hybridRun == nil {
		t.Fatalf("hybrid run %q was not ingested from OTLP", hybridResult.NewRunID)
	}
	assertReplayRunMetadata(t, hybridRun, "PetalTrace contract hybrid", "hybrid", "petaltrace-hybrid-replay", "source-hybrid")

	spans, err := traceStore.GetSpanTree(context.Background(), hybridResult.NewRunID)
	if err != nil {
		t.Fatalf("load hybrid spans: %v", err)
	}
	foundMockedTool := false
	for _, span := range spans {
		if span.Tool == nil {
			continue
		}
		nodeID, _ := span.Attributes["petalflow.node_id"].(string)
		if span.Node != nil && span.Node.NodeID != "" {
			nodeID = span.Node.NodeID
		}
		if nodeID != "render" {
			continue
		}
		var output map[string]any
		if err := json.Unmarshal(span.Tool.Outputs, &output); err != nil {
			t.Fatalf("decode captured tool output %q: %v", span.Tool.Outputs, err)
		}
		if output["rendered"] != "captured tool output" {
			t.Fatalf("captured tool output = %#v, want mocked result", output)
		}
		foundMockedTool = true
	}
	if !foundMockedTool {
		t.Fatalf("no OTLP tool span for hybrid run %q", hybridResult.NewRunID)
	}
}

func freeTCPAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve a loopback port: %v", err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("release reserved port: %v", err)
	}
	return address
}

func petalFlowTestEnv(home string) []string {
	var env []string
	for _, pair := range os.Environ() {
		key, _, _ := strings.Cut(pair, "=")
		if strings.HasPrefix(key, "OTEL_") || strings.HasPrefix(key, "PETALTRACE_") {
			continue
		}
		env = append(env, pair)
	}
	return append(env,
		"HOME="+home,
		"PETALFLOW_API_TOKEN=integration-test-token",
		"PETALFLOW_TENANT_ID=petaltrace-contract-test",
	)
}

func waitForPetalFlow(t *testing.T, client *http.Client, healthURL string, processDone <-chan struct{}, processErr *error, output *bytes.Buffer) {
	t.Helper()
	deadline := time.NewTimer(20 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		request, err := http.NewRequest(http.MethodGet, healthURL, nil)
		if err == nil {
			response, err := client.Do(request)
			if err == nil {
				response.Body.Close()
				if response.StatusCode == http.StatusOK {
					return
				}
			}
		}

		select {
		case <-processDone:
			t.Fatalf("PetalFlow exited before becoming healthy: %v\n%s", *processErr, output.String())
		case <-deadline.C:
			t.Fatalf("PetalFlow did not become healthy at %s\n%s", healthURL, output.String())
		case <-ticker.C:
		}
	}
}

func stopPetalFlow(t *testing.T, cmd *exec.Cmd, processDone <-chan struct{}, processErr *error, stopped *bool, output *bytes.Buffer) {
	t.Helper()
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatalf("signal PetalFlow shutdown: %v\n%s", err, output.String())
	}
	select {
	case <-processDone:
		if *processErr != nil {
			t.Fatalf("PetalFlow shutdown failed: %v\n%s", *processErr, output.String())
		}
		*stopped = true
	case <-time.After(15 * time.Second):
		_ = cmd.Process.Kill()
		*stopped = true
		<-processDone
		t.Fatalf("PetalFlow did not shut down cleanly\n%s", output.String())
	}
}

func assertReplayRunMetadata(t *testing.T, run *store.Run, workflowName, mode, triggerSource, parentRunID string) {
	t.Helper()
	if run.WorkflowName != workflowName || run.WorkflowID == "" || run.TriggerSource != triggerSource ||
		run.ParentRunID == nil || *run.ParentRunID != parentRunID ||
		run.Tags["replay_mode"] != mode {
		t.Fatalf("run metadata = %#v, want workflow %q and replay mode %q", run, workflowName, mode)
	}
}
