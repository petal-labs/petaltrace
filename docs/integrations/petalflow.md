# PetalFlow Integration

PetalTrace provides deep integration with PetalFlow for rich observability of AI agent workflows. While PetalTrace works with any OpenTelemetry-instrumented application, PetalFlow integration captures additional context including graph topology, node-level inputs/outputs, and replay-capable snapshots.

## Quick Setup

### 1. Start PetalTrace

```bash
petaltrace serve
```

### 2. Configure PetalFlow

Add the PetalTrace configuration to your `petalflow.yaml`:

```yaml
observability:
  petaltrace:
    enabled: true
    endpoint: "http://localhost:4318"
    capture_mode: standard
    tags:
      environment: development
      team: platform
```

### 3. Run Your Workflow

PetalFlow automatically sends traces to PetalTrace when the configuration is enabled.

## Configuration Options

### Full Configuration Reference

```yaml
observability:
  petaltrace:
    # Enable/disable PetalTrace integration
    enabled: true

    # PetalTrace collector endpoint (OTLP/HTTP)
    endpoint: "http://localhost:4318"

    # Capture mode determines what data is captured
    # - minimal: Latency, status, token counts, errors (~1 KB/span)
    # - standard: + Full prompts, completions, tool I/O (~10-100 KB/span)
    # - full: + All edge data, graph/config snapshots (~100 KB-1 MB/span)
    capture_mode: standard

    # Custom tags added to all runs
    tags:
      environment: production
      team: platform
      version: "1.0.0"

    # Sampling rate (1.0 = capture everything)
    sample_rate: 1.0

    # Always capture failed runs regardless of sample rate
    always_capture_errors: true

    # Sample rate overrides by tag
    sample_overrides:
      environment:staging: 1.0    # Always capture staging
      environment:production: 0.1 # Sample 10% of production
```

### Environment Variables

```bash
PETALTRACE_ENDPOINT=http://localhost:4318
PETALTRACE_CAPTURE_MODE=standard
PETALTRACE_SAMPLE_RATE=1.0
```

## Capture Modes

### Minimal Mode

Best for production monitoring where storage is a concern.

**Captures:**
- Run and span metadata (IDs, timestamps, status)
- Token counts and cost estimates
- Error messages and stack traces
- Latency metrics

**Does NOT capture:**
- Full prompt text
- LLM completions
- Tool inputs/outputs
- Edge data payloads

```yaml
observability:
  petaltrace:
    capture_mode: minimal
```

### Standard Mode

Recommended for development and debugging.

**Captures everything in minimal, plus:**
- Full system prompts
- Complete message history
- LLM completions
- Tool definitions
- Tool inputs and outputs
- Cache usage metrics

```yaml
observability:
  petaltrace:
    capture_mode: standard
```

### Full Mode

Required for replay functionality.

**Captures everything in standard, plus:**
- Graph definition snapshot
- Workflow inputs snapshot
- Configuration snapshot (secrets masked)
- All edge data payloads

```yaml
observability:
  petaltrace:
    capture_mode: full
```

## Replay Requirements

To replay a PetalFlow run, it must be captured at the appropriate level:

| Replay Mode | Required Capture Level |
|-------------|----------------------|
| `live` | `standard` or higher |
| `mocked` | `full` |
| `hybrid` | `full` |

Runs captured at `minimal` cannot be replayed.

## Semantic Attributes

PetalFlow enriches OpenTelemetry spans with these attributes:

### Run-Level Attributes (Root Span)

| Attribute | Description |
|-----------|-------------|
| `petalflow.run.id` | Unique run identifier |
| `petalflow.workflow.id` | Workflow identifier |
| `petalflow.workflow.name` | Human-readable workflow name |
| `petalflow.workflow.version` | Workflow version |
| `petalflow.source_kind` | `agent_workflow`, `graph`, or `sdk` |
| `petalflow.trigger_source` | `cli`, `api`, `ui`, or `schedule` |
| `petalflow.run.root` | `true` for root span |
| `petalflow.graph` | Graph definition JSON (full mode) |
| `petalflow.input` | Workflow inputs JSON (full mode) |
| `petalflow.config` | Configuration JSON (full mode) |

### Node-Level Attributes

| Attribute | Description |
|-----------|-------------|
| `petalflow.node.id` | Graph node identifier |
| `petalflow.node.type` | Node type (e.g., `llm_prompt`) |
| `petalflow.node.retry_count` | Number of retries |

### LLM Attributes

PetalFlow uses OTel GenAI semantic conventions plus extensions:

| Attribute | Description |
|-----------|-------------|
| `gen_ai.system` | Provider name |
| `gen_ai.request.model` | Model identifier |
| `gen_ai.request.temperature` | Sampling temperature |
| `gen_ai.request.max_tokens` | Max tokens |
| `gen_ai.usage.input_tokens` | Input token count |
| `gen_ai.usage.output_tokens` | Output token count |
| `gen_ai.response.finish_reason` | Stop reason |
| `petalflow.llm.system_prompt` | Full system prompt |
| `petalflow.llm.messages` | Message array JSON |
| `petalflow.llm.completion` | Completion JSON |
| `petalflow.llm.tool_definitions` | Tool definitions |
| `petalflow.llm.ttft_ms` | Time to first token |
| `petalflow.llm.cache_read_tokens` | Prompt cache reads |
| `petalflow.llm.cache_creation_tokens` | Prompt cache writes |

### Tool Attributes

| Attribute | Description |
|-----------|-------------|
| `petalflow.tool.name` | Tool registry name |
| `petalflow.tool.action` | Action invoked |
| `petalflow.tool.origin` | `native`, `mcp`, `http`, `stdio` |
| `petalflow.tool.invoked_by` | LLM span ID (for function calling) |
| `tool.use.id` | Provider's tool_use block ID |

### Edge Attributes

| Attribute | Description |
|-----------|-------------|
| `petalflow.edge.source_node` | Source node ID |
| `petalflow.edge.source_port` | Source port name |
| `petalflow.edge.target_node` | Target node ID |
| `petalflow.edge.target_port` | Target port name |
| `petalflow.edge.data_size_bytes` | Payload size |

## MCP Overlay

PetalTrace provides an MCP overlay for PetalFlow integration, allowing workflows to query their own execution history.

### Installing the Overlay

Copy the overlay to your PetalFlow installation:

```bash
cp petaltrace/mcp/overlay.yaml $PETALFLOW_HOME/tools/overlays/petaltrace.overlay.yaml
```

### Using PetalTrace in Workflows

```yaml
# workflow.yaml
agents:
  diagnostic_agent:
    role: "Workflow Diagnostician"
    goal: "Analyze execution failures and recommend improvements"
    tools:
      - petaltrace.trace.list
      - petaltrace.trace.get
      - petaltrace.prompt.get
      - petaltrace.diff.compare

tasks:
  diagnose:
    description: |
      Review the last 5 failed runs of the 'research_pipeline' workflow.
      Identify common failure patterns and suggest prompt or configuration changes.
    agent: diagnostic_agent
    expected_output: "A structured report with failure patterns and recommendations."
```

### Self-Reflective Agent Pattern

Agents can inspect their own prior executions:

```yaml
agents:
  research_agent:
    role: "Research Assistant"
    goal: "Research topics and learn from past performance"
    tools:
      - web_search
      - petaltrace.trace.list
      - petaltrace.prompt.get

tasks:
  research_with_learning:
    description: |
      Research the topic: {topic}

      Before researching, check your recent runs for similar topics
      using petaltrace.trace.list. If you find relevant prior research,
      use petaltrace.prompt.get to review what worked well.
    agent: research_agent
```

## Troubleshooting

### Traces Not Appearing

1. **Verify PetalTrace is running:**
   ```bash
   curl http://localhost:8090/api/health
   ```

2. **Check the endpoint configuration:**
   ```yaml
   observability:
     petaltrace:
       endpoint: "http://localhost:4318"  # OTLP/HTTP port
   ```

3. **Verify PetalFlow logging:**
   Look for trace export logs in PetalFlow output.

### Missing Prompts/Completions

Ensure capture mode is at least `standard`:

```yaml
observability:
  petaltrace:
    capture_mode: standard
```

### Replay Fails

Ensure the run was captured at `full` mode for mocked/hybrid replay:

```yaml
observability:
  petaltrace:
    capture_mode: full
```

### High Storage Usage

For production, consider:

1. **Reduce capture mode:**
   ```yaml
   capture_mode: minimal
   ```

2. **Enable sampling:**
   ```yaml
   sample_rate: 0.1  # 10% sampling
   always_capture_errors: true
   ```

3. **Reduce retention:**
   ```yaml
   # petaltrace.yaml
   retention:
     default: "7d"
   ```

## Example: Full Production Setup

### PetalFlow Configuration

```yaml
# petalflow.yaml
observability:
  petaltrace:
    enabled: true
    endpoint: "http://petaltrace.internal:4318"
    capture_mode: standard
    sample_rate: 0.1
    always_capture_errors: true
    sample_overrides:
      environment:staging: 1.0
    tags:
      environment: production
      service: research-api
      version: "2.1.0"
```

### PetalTrace Configuration

```yaml
# petaltrace.yaml
server:
  address: ":8090"

storage:
  path: "/var/lib/petaltrace/data.db"

collector:
  batch_size: 500
  flush_interval: "5s"

retention:
  default: "30d"
  failed_runs: "90d"
```

### Monitoring Dashboard

Query recent failures:

```bash
petaltrace runs list --status failed --since 24h --limit 20
```

Analyze costs by workflow:

```bash
petaltrace cost summary --since 7d --group-by workflow
```

Compare a failed run to a successful one:

```bash
petaltrace diff run-failed-123 run-success-456 --include-content
```
