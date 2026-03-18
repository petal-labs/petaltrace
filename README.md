# PetalTrace

Agent observability platform for AI workflows. Capture the full execution lifecycle — from compiled graph topology through runtime node execution, LLM provider interactions, tool calls, token consumption, and data flow.

See what your agents are thinking.

## Features

- **OTLP Collector** — Receive traces via OTLP/HTTP (port 4318) and OTLP/gRPC (port 4317)
- **Span Classification** — Automatic detection of LLM calls, tool invocations, node executions, and graph edges
- **Cost Tracking** — Token consumption and cost calculation with configurable pricing
- **Run Comparison** — Diff two workflow runs to identify structural, content, and cost differences
- **Replay** — Re-execute workflows in live, mocked, or hybrid modes
- **MCP Server** — Expose trace data to AI agents via Model Context Protocol
- **Web UI** — Visual exploration of traces, costs, and workflow graphs

## Installation

### From Source

```bash
git clone https://github.com/petal-labs/petaltrace.git
cd petaltrace
make build
```

### From Release

Download the latest binary from [Releases](https://github.com/petal-labs/petaltrace/releases).

## Quick Start

Start the daemon:

```bash
./bin/petaltrace serve
```

This starts:
- OTLP/HTTP collector on `:4318`
- OTLP/gRPC collector on `:4317`
- HTTP API on `:8090`

Configure your OpenTelemetry SDK to send traces to `http://localhost:4318`.

## CLI Commands

```bash
petaltrace serve          # Start the daemon (collector + API + UI)
petaltrace mcp            # Start MCP server for AI agent integration
petaltrace runs           # List and manage workflow runs
petaltrace diff           # Compare two runs
petaltrace replay         # Replay a workflow run
petaltrace cost           # View cost summaries and breakdowns
petaltrace prompt         # View LLM prompts and completions
petaltrace version        # Print version information
```

## MCP Integration

PetalTrace exposes an MCP server for AI agent integration:

```bash
petaltrace mcp
```

Available tools:

| Tool | Description |
|------|-------------|
| `petaltrace.trace.list` | List recent runs with filters |
| `petaltrace.trace.get` | Get run detail with span tree |
| `petaltrace.trace.search` | Search runs by prompt/completion content |
| `petaltrace.prompt.get` | Get full LLM prompt and completion |
| `petaltrace.cost.summary` | Aggregate cost metrics |
| `petaltrace.cost.run` | Per-run cost breakdown |
| `petaltrace.diff.compare` | Compare two runs |
| `petaltrace.run.replay` | Trigger workflow replay |

### Claude Code Configuration

Add to your MCP settings:

```json
{
  "mcpServers": {
    "petaltrace": {
      "command": "petaltrace",
      "args": ["mcp"]
    }
  }
}
```

## Configuration

Create `petaltrace.yaml`:

```yaml
server:
  host: "0.0.0.0"
  port: 8090

collector:
  otlp_http_port: 4318
  otlp_grpc_port: 4317

storage:
  sqlite:
    path: "./data/petaltrace.db"
    wal_mode: true

pricing:
  overrides: ""  # Path to custom pricing YAML
```

Environment variables with `PETALTRACE_` prefix override config file values.

## Development

### Prerequisites

- Go 1.24+
- Node.js 20+ (for UI development)

### Build

```bash
make build          # Build CLI binary
make test           # Run tests
make lint           # Run linter
make fmt            # Format code
make install-hooks  # Install git pre-commit hook
```

### Release

```bash
make release-build  # Build binaries for all platforms
```

## License

See [LICENSE](LICENSE) for details.
