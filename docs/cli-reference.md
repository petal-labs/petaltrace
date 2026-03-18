# CLI Reference

Complete reference for all PetalTrace CLI commands.

## Global Options

```bash
petaltrace [command] [flags]

Flags:
  --config string   Path to config file (default: petaltrace.yaml)
  -h, --help        Help for petaltrace
  -v, --version     Version for petaltrace
```

## serve

Start the PetalTrace daemon.

```bash
petaltrace serve [flags]
```

**Flags:**

| Flag | Description | Default |
|------|-------------|---------|
| `--config` | Path to configuration file | `petaltrace.yaml` |

**What it starts:**

- HTTP API server on port 8090
- OTLP/gRPC collector on port 4317
- OTLP/HTTP collector on port 4318
- SQLite store with automatic migrations

**Example:**

```bash
# Start with defaults
petaltrace serve

# Start with custom config
petaltrace serve --config /etc/petaltrace/petaltrace.yaml
```

**Output:**

```
time=2026-03-17T18:29:08.672-07:00 level=INFO msg="starting petaltrace" version=0.1.0-dev
time=2026-03-17T18:29:08.675-07:00 level=INFO msg="database initialized" path=/Users/user/.petaltrace/data.db
time=2026-03-17T18:29:08.676-07:00 level=INFO msg="starting API server" addr=0.0.0.0:8090
time=2026-03-17T18:29:08.676-07:00 level=INFO msg="petaltrace ready" api=0.0.0.0:8090 otlp_http=[::]:4318 otlp_grpc=[::]:4317
```

Shutdown gracefully with `Ctrl+C`.

---

## runs

Manage workflow runs.

### runs list

List recent runs with optional filtering.

```bash
petaltrace runs list [flags]
```

**Flags:**

| Flag | Description | Default |
|------|-------------|---------|
| `--workflow` | Filter by workflow name | |
| `--status` | Filter by status: `running`, `completed`, `failed` | |
| `--since` | Show runs since duration (e.g., `24h`, `7d`) | |
| `--limit` | Maximum number of runs | `50` |
| `--json` | Output as JSON | `false` |

**Examples:**

```bash
# List recent runs
petaltrace runs list

# List completed runs from the last 24 hours
petaltrace runs list --status completed --since 24h

# List runs for a specific workflow as JSON
petaltrace runs list --workflow email-processor --json

# Filter failed runs with limit
petaltrace runs list --status failed --limit 10
```

**Output:**

```
STATUS  WORKFLOW            RUN ID              DURATION  TOKENS  COST      STARTED
✓       email-processor     run-01JK3ABC...     1.2s      5000    $0.0150   2026-03-17 10:15:30
✗       research-pipeline   run-01JK3DEF...     3.4s      2100    $0.0089   2026-03-17 10:14:15
●       content-writer      run-01JK3GHI...     -         1200    $0.0042   2026-03-17 10:16:00

Showing 3 runs
```

Status icons: `✓` completed, `✗` failed, `●` running, `○` cancelled

### runs show

Show detailed information about a specific run.

```bash
petaltrace runs show <run-id> [flags]
```

**Flags:**

| Flag | Description | Default |
|------|-------------|---------|
| `--spans` | Include span tree | `false` |
| `--node` | Filter spans by node ID (requires `--spans`) | |
| `--json` | Output as JSON | `false` |

**Examples:**

```bash
# Show run details
petaltrace runs show run-01JK3ABC

# Show run with full span tree
petaltrace runs show run-01JK3ABC --spans

# Show spans for a specific node
petaltrace runs show run-01JK3ABC --spans --node researcher_agent

# Output as JSON
petaltrace runs show run-01JK3ABC --json
```

### runs delete

Delete a run and all its spans.

```bash
petaltrace runs delete <run-id> [flags]
```

**Flags:**

| Flag | Description | Default |
|------|-------------|---------|
| `-y`, `--yes` | Skip confirmation prompt | `false` |

**Example:**

```bash
# Delete with confirmation
petaltrace runs delete run-01JK3ABC

# Delete without confirmation
petaltrace runs delete run-01JK3ABC -y
```

---

## prompt

Display the full prompt and completion for an LLM node.

```bash
petaltrace prompt <run-id> <node-id> [flags]
```

**Flags:**

| Flag | Description | Default |
|------|-------------|---------|
| `-c`, `--completion` | Include the completion/response | `false` |
| `-f`, `--format` | Output format: `text`, `json`, `curl`, `sdk` | `text` |

**Examples:**

```bash
# Show prompt only
petaltrace prompt run-01JK3ABC researcher_agent

# Show prompt and completion
petaltrace prompt run-01JK3ABC researcher_agent --completion

# Generate cURL command
petaltrace prompt run-01JK3ABC researcher_agent --format curl

# Generate Python SDK code
petaltrace prompt run-01JK3ABC researcher_agent --format sdk
```

**Output formats:**

- `text`: Human-readable prompt with message formatting
- `json`: Structured JSON with all prompt/completion data
- `curl`: Copy-paste ready cURL command for the API call
- `sdk`: Python SDK code (Anthropic or OpenAI based on provider)

---

## cost

Cost analysis commands.

### cost summary

Aggregate cost metrics across runs.

```bash
petaltrace cost summary [flags]
```

**Flags:**

| Flag | Description | Default |
|------|-------------|---------|
| `--since` | Time window | `7d` |
| `--group-by` | Group by: `workflow`, `provider`, `model` | |
| `--json` | Output as JSON | `false` |

**Examples:**

```bash
# Summary for last 7 days
petaltrace cost summary

# Summary for last 30 days grouped by provider
petaltrace cost summary --since 30d --group-by provider

# Summary grouped by model as JSON
petaltrace cost summary --group-by model --json
```

**Output:**

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

### cost run

Per-run cost breakdown.

```bash
petaltrace cost run <run-id> [flags]
```

**Flags:**

| Flag | Description | Default |
|------|-------------|---------|
| `--by-node` | Show breakdown by node | `false` |
| `--json` | Output as JSON | `false` |

**Examples:**

```bash
# Run cost summary
petaltrace cost run run-01JK3ABC

# Run cost with node breakdown
petaltrace cost run run-01JK3ABC --by-node
```

---

## diff

Compare two runs.

```bash
petaltrace diff <base-run-id> <compare-run-id> [flags]
```

**Flags:**

| Flag | Description | Default |
|------|-------------|---------|
| `--include-content` | Include full text diffs | `false` |
| `--include-inputs` | Include input/output data diffs | `false` |
| `--format` | Output format: `table`, `json`, `summary` | `table` |
| `-o`, `--output` | Write output to file | |
| `--no-cache` | Don't use or store cached diff | `false` |

**Examples:**

```bash
# Compare two runs (summary)
petaltrace diff run-01JK3ABC run-01JK3XYZ --format summary

# Compare with full text diffs
petaltrace diff run-01JK3ABC run-01JK3XYZ --include-content

# Export diff as JSON
petaltrace diff run-01JK3ABC run-01JK3XYZ --format json -o diff.json
```

**Output (summary format):**

```
=== Diff Summary ===

Base Run:    run-01JK3ABC
Compare Run: run-01JK3XYZ

Status Match:     Yes
Path Divergence:  No
Duration Delta:   +1250ms
Token Delta:      +342
Cost Delta:       +$0.002150
Nodes Changed:    3

=== Cost Breakdown ===

Base Cost:    $0.015230
Compare Cost: $0.017380
Delta:        +$0.002150

By Model:
  claude-sonnet-4-20250514: +$0.002150
```

---

## replay

Replay a captured run.

```bash
petaltrace replay <run-id> [flags]
```

**Flags:**

| Flag | Description | Default |
|------|-------------|---------|
| `--mode` | Replay mode: `live`, `mocked`, `hybrid` | `live` |
| `--model` | Override LLM model | |
| `--provider` | Override LLM provider | |
| `--temperature` | Override sampling temperature | |
| `--max-tokens` | Override max tokens | |
| `--diff` | Auto-diff after completion | `false` |
| `--sync` | Wait for replay to complete | `true` |
| `--tag` | Add tags (format: `key=value`, repeatable) | |
| `--json` | Output as JSON | `false` |
| `--petalflow-url` | PetalFlow daemon URL | `http://localhost:8080` |

**Replay Modes:**

| Mode | LLM Calls | Tool Calls | Use Case |
|------|-----------|------------|----------|
| `live` | Real | Real | Re-execute with different config |
| `mocked` | Captured | Captured | Deterministic testing |
| `hybrid` | Real | Captured | Test prompt changes |

**Examples:**

```bash
# Live replay with different model
petaltrace replay run-01JK3ABC --mode live --model claude-3-opus-20240229 --diff

# Deterministic mocked replay
petaltrace replay run-01JK3ABC --mode mocked

# Hybrid replay with temperature override
petaltrace replay run-01JK3ABC --mode hybrid --temperature 0.2 --diff

# Add tags to replay run
petaltrace replay run-01JK3ABC --tag experiment=v2 --tag team=platform
```

### replay status

Check status of a replay operation.

```bash
petaltrace replay status <replay-id>
```

### replay diff

Compute diff for a completed replay.

```bash
petaltrace replay diff <replay-id>
```

---

## export

Export a run to a JSON file.

```bash
petaltrace export <run-id> [output-file] [flags]
```

**Flags:**

| Flag | Description | Default |
|------|-------------|---------|
| `--include-search-text` | Include extracted FTS text | `false` |

**Examples:**

```bash
# Export to file
petaltrace export run-01JK3ABC my-run.json

# Export with search text
petaltrace export run-01JK3ABC my-run.json --include-search-text

# Export to stdout (omit filename)
petaltrace export run-01JK3ABC
```

---

## import

Import a run from a JSON file.

```bash
petaltrace import <file> [flags]
```

**Flags:**

| Flag | Description | Default |
|------|-------------|---------|
| `--new-id` | Generate new run and span IDs | `false` |

**Examples:**

```bash
# Import preserving IDs
petaltrace import my-run.json

# Import with new IDs (avoids conflicts)
petaltrace import my-run.json --new-id
```

---

## gc

Run garbage collection to delete old runs.

```bash
petaltrace gc [flags]
```

**Flags:**

| Flag | Description | Default |
|------|-------------|---------|
| `--retain` | Override retention period | Config value |
| `--dry-run` | Preview deletions without executing | `false` |
| `--include-starred` | Also delete starred runs | `false` |
| `--verbose` | Show each run being deleted | `false` |

**Examples:**

```bash
# Preview what would be deleted
petaltrace gc --dry-run

# Run GC with custom retention
petaltrace gc --retain 14d

# Run GC including starred runs (verbose)
petaltrace gc --include-starred --verbose
```

**Output:**

```
Garbage collection complete
  Deleted: 42 runs
  Freed: 128 MB
  Remaining: 158 runs
```

---

## stats

Display storage and system statistics.

```bash
petaltrace stats [flags]
```

**Flags:**

| Flag | Description | Default |
|------|-------------|---------|
| `--json` | Output as JSON | `false` |

**Example:**

```bash
petaltrace stats
```

**Output:**

```
PetalTrace Statistics
────────────────────────────────────────
Database:
  Path:         /Users/user/.petaltrace/data.db
  Size:         256 MB
  Total Runs:   200
  Total Spans:  4,521

Data Range:
  Oldest Run:   2026-03-01 08:15:30
  Newest Run:   2026-03-17 18:29:08

Top Workflows (by run count):
  email-processor      85
  research-pipeline    62
  content-writer       53

Top Workflows (by cost):
  research-pipeline    $45.23
  email-processor      $28.90
  content-writer       $18.45
```

---

## mcp

Run the MCP server for agent integration.

```bash
petaltrace mcp [flags]
```

**Flags:**

| Flag | Description | Default |
|------|-------------|---------|
| `--config` | Path to configuration file | `petaltrace.yaml` |

The MCP server uses stdio transport (stdin/stdout) for the MCP protocol. Logs are written to stderr.

**Example:**

```bash
# Run MCP server (typically invoked by MCP client)
petaltrace mcp
```

**Claude Code Configuration:**

```json
{
  "mcpServers": {
    "petaltrace": {
      "command": "petaltrace",
      "args": ["mcp"],
      "env": {}
    }
  }
}
```

See [MCP Server](./mcp-server.md) for available tools.
