# Getting Started with PetalTrace

PetalTrace is an agent observability platform that provides deep visibility into AI agent workflows. It captures the full execution lifecycle — LLM prompts and completions, tool calls, token usage, costs, and execution timelines — and exposes them through a CLI, HTTP API, and MCP server.

## Installation

### From Source

```bash
git clone https://github.com/petal-labs/petaltrace.git
cd petaltrace
go build -o petaltrace .
```

Move the binary to your PATH:

```bash
mv petaltrace /usr/local/bin/
```

### Verify Installation

```bash
petaltrace --version
```

## Quick Start

### 1. Start the PetalTrace Daemon

```bash
petaltrace serve
```

This starts:
- **HTTP API** on port `8090`
- **OTLP/gRPC collector** on port `4317`
- **OTLP/HTTP collector** on port `4318`

### 2. Send Traces

PetalTrace accepts traces via the standard OpenTelemetry protocol (OTLP). Configure your application to send traces to:

- **gRPC**: `localhost:4317`
- **HTTP**: `localhost:4318/v1/traces`

For PetalFlow workflows, enable the traceflow adapter:

```yaml
# petalflow.yaml
observability:
  petaltrace:
    enabled: true
    endpoint: "http://localhost:4318"
    capture_mode: standard
```

### 3. View Traces

List recent runs:

```bash
petaltrace runs list
```

Output:

```
STATUS  WORKFLOW            RUN ID              DURATION  TOKENS  COST      STARTED
✓       email-processor     run-01JK3ABC...     1.2s      5000    $0.0150   2026-03-17 10:15:30
✓       research-pipeline   run-01JK3DEF...     8.4s      12340   $0.0890   2026-03-17 10:14:15
```

### 4. Inspect a Run

View run details with span tree:

```bash
petaltrace runs show run-01JK3ABC --spans
```

### 5. View Full Prompts

Inspect the complete prompt and completion for an LLM node:

```bash
petaltrace prompt run-01JK3ABC researcher_agent --completion
```

### 6. Analyze Costs

Get a cost summary for the last 7 days:

```bash
petaltrace cost summary
```

Output:

```
Cost Summary (last 7 days)
────────────────────────────────────────
Total Runs:      142
Total Tokens:    1,234,567
Total Cost:      $12.34

By Provider:
  anthropic      $8.90  (72%)
  openai         $3.44  (28%)

By Workflow:
  research-pipeline    $6.50
  email-processor      $3.20
  content-writer       $2.64
```

## What's Next

- [Concepts](./concepts.md) — Understand the data model and architecture
- [CLI Reference](./cli-reference.md) — Complete command documentation
- [API Reference](./api-reference.md) — HTTP API endpoints
- [Configuration](./configuration.md) — Configure PetalTrace for your environment
- [PetalFlow Integration](./integrations/petalflow.md) — Deep integration with PetalFlow
- [OpenTelemetry Integration](./integrations/opentelemetry.md) — Use with any OTel-instrumented app
- [MCP Server](./mcp-server.md) — Enable agent self-inspection

## Example Workflows

### Compare Two Runs

```bash
petaltrace diff run-01JK3ABC run-01JK3XYZ --include-content
```

### Replay a Run with Different Model

```bash
petaltrace replay run-01JK3ABC --mode live --model claude-3-opus-20240229 --diff
```

### Export a Run for Sharing

```bash
petaltrace export run-01JK3ABC my-run.json
```

## Default Ports

| Service | Port | Protocol |
|---------|------|----------|
| HTTP API | 8090 | REST |
| OTLP gRPC | 4317 | gRPC |
| OTLP HTTP | 4318 | HTTP |

## Data Storage

By default, PetalTrace stores data in `~/.petaltrace/data.db` (SQLite). Configure the storage path in `petaltrace.yaml`:

```yaml
storage:
  path: "/path/to/data.db"
  wal_mode: true
```
