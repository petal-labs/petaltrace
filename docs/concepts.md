# Concepts

This document explains the core concepts, architecture, and data model of PetalTrace.

## Overview

PetalTrace is an observability platform designed specifically for AI agent workflows. It captures and stores detailed execution data, enabling developers to:

- See what agents are thinking (full prompt/completion capture)
- Track token usage and costs across runs
- Replay workflows with different configurations
- Compare runs to identify what changed
- Enable agents to inspect their own execution history

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                            PetalFlow / Any OTel App                              │
│                                                                                  │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  ┌────────────────────┐   │
│  │  Execution   │  │  OTel Spans  │  │  Event       │  │  SSE Stream        │   │
│  │  Engine      │──│              │──│  System      │──│                    │   │
│  └──────────────┘  └──────┬───────┘  └──────────────┘  └────────────────────┘   │
│                           │                                                      │
└───────────────────────────┼──────────────────────────────────────────────────────┘
                            │  OTLP/gRPC or OTLP/HTTP
                            ▼
┌──────────────────────────────────────────────────────────────────────────────────┐
│                           PetalTrace                                             │
│                                                                                  │
│  ┌────────────────┐  ┌────────────────┐  ┌─────────────────┐  ┌──────────────┐  │
│  │  Collector     │  │  Trace Store   │  │  Replay Engine  │  │  Diff Engine │  │
│  │  (OTLP ingest) │  │  (SQLite)      │  │                 │  │              │  │
│  └───────┬────────┘  └───────┬────────┘  └────────┬────────┘  └──────┬───────┘  │
│          │                   │                    │                  │          │
│  ┌───────┴───────────────────┴────────────────────┴──────────────────┴───────┐  │
│  │                          HTTP API + SSE                                   │  │
│  └───────────────────────────┬───────────────────────────────────────────────┘  │
│                              │                                                   │
│  ┌───────────────────────────┴───────────────────────────────────────────────┐  │
│  │                          MCP Server                                       │  │
│  │  petaltrace.trace.* · petaltrace.prompt.* · petaltrace.diff.*            │  │
│  └───────────────────────────────────────────────────────────────────────────┘  │
│                                                                                  │
└──────────────────────────────────────────────────────────────────────────────────┘
```

### Components

| Component | Responsibility |
|-----------|----------------|
| **Collector** | Receives OTLP spans (gRPC + HTTP), classifies by kind, enriches with costs, writes to store |
| **Trace Store** | Persists runs, spans, LLM interactions, and metadata in SQLite |
| **Replay Engine** | Re-executes captured runs with live, mocked, or hybrid modes |
| **Diff Engine** | Compares two runs structurally, by content, and by cost |
| **HTTP API** | REST + SSE endpoints for querying traces and triggering operations |
| **MCP Server** | Exposes trace capabilities to agents via MCP protocol |
| **CLI** | Command-line interface for all operations |

## Data Model

### Runs

A **Run** represents a single execution of a workflow. It contains:

- Unique identifier (ULID format)
- Workflow name and version
- Status: `running`, `completed`, `failed`, or `cancelled`
- Timestamps for start and completion
- Snapshots of graph definition, inputs, and config
- Aggregated token counts and cost estimates
- User-defined tags

```go
type Run struct {
    ID              string
    WorkflowID      string
    WorkflowName    string
    Status          RunStatus
    StartedAt       time.Time
    CompletedAt     *time.Time
    GraphSnapshot   json.RawMessage
    InputSnapshot   json.RawMessage
    TotalTokens     TokenSummary
    EstimatedCost   CostEstimate
    Tags            map[string]string
    ParentRunID     *string  // For replay lineage
}
```

### Spans

A **Span** represents a single unit of work within a run. Spans form a tree (parent-child relationships) mirroring the OpenTelemetry span model.

```go
type Span struct {
    ID          string
    RunID       string
    ParentID    *string
    TraceID     string
    Kind        SpanKind
    Name        string
    Status      SpanStatus
    StartedAt   time.Time
    CompletedAt *time.Time
    DurationMs  int64
    Node        *NodeSpanData
    LLM         *LLMSpanData
    Tool        *ToolSpanData
    Edge        *EdgeSpanData
    Error       *SpanError
}
```

### Span Kinds

PetalTrace classifies spans into five kinds:

| Kind | Description | Captured Data |
|------|-------------|---------------|
| `node` | Graph node execution | Node ID, type, inputs, outputs, config, retry count |
| `llm` | LLM provider API call | Provider, model, full prompt, completion, tokens, latency, cache usage |
| `tool` | Tool invocation | Tool name, action, origin, inputs, outputs |
| `edge` | Data transfer between nodes | Source/target node and port, data size |
| `custom` | Any other span | Attributes from OTel |

### LLM Span Data

The most important span type for debugging. Captures the complete LLM interaction:

```go
type LLMSpanData struct {
    Provider        string           // "anthropic", "openai", "google"
    Model           string           // "claude-sonnet-4-20250514"
    SystemPrompt    string           // Full system prompt
    Messages        []LLMMessage     // Complete message history
    ToolDefinitions []ToolDefinition // Tools presented to the model
    Completion      LLMCompletion    // Full response
    Tokens          TokenDetail      // Input, output, cache tokens + cost
    TimeToFirstToken *int64          // Streaming TTFT
    TotalLatency    int64
    CacheRead       *int             // Prompt cache hits
    CacheCreation   *int             // Prompt cache writes
}
```

### Token and Cost Model

PetalTrace tracks token usage and computes costs automatically:

```go
type TokenSummary struct {
    InputTokens       int
    OutputTokens      int
    CacheReadTokens   int
    CacheWriteTokens  int
    TotalTokens       int
}

type CostEstimate struct {
    Currency   string              // "USD"
    Total      float64
    ByProvider map[string]float64  // Provider → cost
    ByModel    map[string]float64  // Model → cost
    ByNode     map[string]float64  // Node ID → cost
}
```

Costs are computed using a built-in pricing table that includes current rates for:
- Anthropic (Claude 3, 3.5, 4, Haiku, Sonnet, Opus)
- OpenAI (GPT-4o, o1, o3-mini)
- Google (Gemini 1.5, 2.0)
- DeepSeek
- Mistral

Custom pricing can be configured via overrides.

## Trace Flow

### Ingestion Pipeline

```
OTel Span → Receiver → Classifier → Correlator → Enricher → Writer → Store
```

1. **Receiver**: Accepts OTLP spans via gRPC (4317) or HTTP (4318)
2. **Classifier**: Determines span kind from attributes (`gen_ai.*`, `petalflow.*`, etc.)
3. **Correlator**: Groups spans into runs, identifies root spans, extracts snapshots
4. **Enricher**: Computes costs, normalizes provider names, extracts text for search
5. **Writer**: Batches writes to SQLite with FTS indexing

### Classification Rules

PetalTrace classifies spans using these attribute checks:

1. `gen_ai.system` or `gen_ai.request.model` → **LLM span**
2. `petalflow.node.id` or `petalflow.node.type` → **Node span**
3. `tool.name` or `petalflow.tool.name` → **Tool span**
4. `petalflow.edge.source` or `petalflow.edge.target` → **Edge span**
5. Span name contains "llm", "chat", "completion" → **LLM span**
6. Default → **Custom span**

### Run Correlation

Spans are grouped into runs using:

1. `petalflow.run.id` span attribute (preferred)
2. `petalflow.run.id` resource attribute
3. Fallback: `trace-{traceID}`

## Capture Modes

When using PetalFlow, you can configure how much data is captured:

| Mode | What's Captured | Storage Impact | Use Case |
|------|-----------------|----------------|----------|
| `minimal` | Latency, status, token counts, errors | ~1 KB/span | Production monitoring |
| `standard` | + Full prompts, completions, tool I/O | ~10-100 KB/span | Development, debugging |
| `full` | + All edge data, graph/config snapshots | ~100 KB-1 MB/span | Replay-capable runs |

Replay requires `standard` (live mode) or `full` (mocked/hybrid mode) capture.

## Replay Modes

The replay engine supports three modes:

| Mode | LLM Calls | Tool Calls | Use Case |
|------|-----------|------------|----------|
| `live` | Real providers | Real tools | Re-execute with different config |
| `mocked` | Captured responses | Captured results | Deterministic testing, cost-free |
| `hybrid` | Real providers | Captured results | Test prompt changes |

Replayed runs maintain lineage via `parent_run_id`.

## Diff Strategies

The diff engine compares runs across multiple dimensions:

- **Structural diff**: Are the same nodes executed in the same order?
- **Content diff**: Compare LLM outputs with unified diff and similarity scores
- **Cost diff**: Token and cost comparison per node, per provider, aggregate

## MCP Integration

PetalTrace exposes an MCP server that agents can use for self-inspection:

```
petaltrace.trace.list      List recent runs
petaltrace.trace.get       Get run detail with spans
petaltrace.trace.search    Search by content
petaltrace.prompt.get      Get full prompt/completion
petaltrace.cost.summary    Aggregate cost metrics
petaltrace.cost.run        Per-run cost breakdown
petaltrace.diff.compare    Compare two runs
petaltrace.run.replay      Trigger replay
```

This enables self-reflective agent patterns where an agent can diagnose its own failures by inspecting prior executions.

## Related Documentation

- [CLI Reference](./cli-reference.md)
- [API Reference](./api-reference.md)
- [Configuration](./configuration.md)
