# MCP Server

PetalTrace exposes an MCP (Model Context Protocol) server that enables AI agents to query trace data, inspect prompts, analyze costs, compare runs, and trigger replays. This enables self-reflective agent patterns where agents can diagnose their own execution history.

## Starting the MCP Server

```bash
petaltrace mcp
```

The MCP server uses stdio transport, reading JSON-RPC messages from stdin and writing responses to stdout. Logs are written to stderr.

## Claude Code Integration

Add PetalTrace to your Claude Code MCP configuration:

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

After configuration, Claude Code can use PetalTrace tools to inspect trace data.

## Available Tools

### petaltrace.trace.list

List recent runs with optional filtering.

**Input Schema:**

```json
{
  "workflow": "string (optional) - Filter by workflow name",
  "status": "string (optional) - running|completed|failed|cancelled",
  "since": "string (optional) - Duration like '24h' or '7d'",
  "limit": "integer (optional) - Max results, default 50",
  "cursor": "string (optional) - Pagination cursor"
}
```

**Example:**

```json
{
  "name": "petaltrace.trace.list",
  "arguments": {
    "status": "failed",
    "since": "24h",
    "limit": 10
  }
}
```

**Response:**

```json
{
  "runs": [
    {
      "id": "run-01JK3ABC",
      "workflow_name": "research-pipeline",
      "status": "failed",
      "duration_ms": 3400,
      "total_tokens": 2100,
      "estimated_cost": 0.0089,
      "started_at": "2026-03-17T10:14:15Z",
      "error_message": "Tool 'web_search' timed out"
    }
  ],
  "count": 1,
  "has_more": false
}
```

---

### petaltrace.trace.get

Get detailed information about a specific run.

**Input Schema:**

```json
{
  "run_id": "string (required) - Run identifier",
  "include_spans": "boolean (optional) - Include span tree, default true"
}
```

**Example:**

```json
{
  "name": "petaltrace.trace.get",
  "arguments": {
    "run_id": "run-01JK3ABC",
    "include_spans": true
  }
}
```

**Response:**

```json
{
  "run": {
    "id": "run-01JK3ABC",
    "workflow_name": "research-pipeline",
    "status": "failed",
    "started_at": "2026-03-17T10:14:15Z",
    "completed_at": "2026-03-17T10:14:18.4Z",
    "duration_ms": 3400,
    "total_tokens": {...},
    "estimated_cost": {...}
  },
  "spans": [
    {
      "id": "span-123",
      "kind": "node",
      "name": "researcher_agent",
      "status": "ok",
      "duration_ms": 2000
    },
    {
      "id": "span-456",
      "kind": "tool",
      "name": "web_search",
      "status": "error",
      "error_message": "Request timeout"
    }
  ]
}
```

---

### petaltrace.trace.search

Search runs by content in prompts and completions.

**Input Schema:**

```json
{
  "query": "string (required) - Search query",
  "workflow": "string (optional) - Filter by workflow",
  "limit": "integer (optional) - Max results, default 20"
}
```

**Example:**

```json
{
  "name": "petaltrace.trace.search",
  "arguments": {
    "query": "API authentication error",
    "limit": 5
  }
}
```

---

### petaltrace.prompt.get

Get the full prompt and completion for an LLM node.

**Input Schema:**

```json
{
  "run_id": "string (required) - Run identifier",
  "node_id": "string (required) - Node identifier",
  "include_completion": "boolean (optional) - Include response, default true"
}
```

**Example:**

```json
{
  "name": "petaltrace.prompt.get",
  "arguments": {
    "run_id": "run-01JK3ABC",
    "node_id": "researcher_agent"
  }
}
```

**Response:**

```json
{
  "span_id": "span-789",
  "node_id": "researcher_agent",
  "provider": "anthropic",
  "model": "claude-sonnet-4-20250514",
  "prompt": {
    "system_prompt": "You are a helpful research assistant.",
    "messages": [
      {"role": "user", "content": "Research the topic of AI observability"}
    ],
    "tool_definitions": [...]
  },
  "completion": {
    "text_content": "Based on my research...",
    "stop_reason": "end_turn"
  },
  "tokens": {
    "input_tokens": 500,
    "output_tokens": 1200,
    "cost_estimate": 0.0089
  },
  "timing": {
    "duration_ms": 1500,
    "time_to_first_token_ms": 150
  }
}
```

---

### petaltrace.cost.summary

Get aggregate cost metrics.

**Input Schema:**

```json
{
  "since": "string (optional) - Time window, default '7d'",
  "workflow": "string (optional) - Filter by workflow",
  "group_by": "string (optional) - workflow|provider|model"
}
```

**Example:**

```json
{
  "name": "petaltrace.cost.summary",
  "arguments": {
    "since": "30d",
    "group_by": "workflow"
  }
}
```

**Response:**

```json
{
  "since": "2026-02-17T00:00:00Z",
  "until": "2026-03-17T23:59:59Z",
  "total_runs": 450,
  "total_tokens": 5234567,
  "total_cost": 52.34,
  "by_workflow": {
    "research-pipeline": {"runs": 200, "cost": 25.00},
    "email-processor": {"runs": 250, "cost": 27.34}
  }
}
```

---

### petaltrace.cost.run

Get per-run cost breakdown.

**Input Schema:**

```json
{
  "run_id": "string (required) - Run identifier"
}
```

**Example:**

```json
{
  "name": "petaltrace.cost.run",
  "arguments": {
    "run_id": "run-01JK3ABC"
  }
}
```

---

### petaltrace.diff.compare

Compare two runs.

**Input Schema:**

```json
{
  "base_run_id": "string (required) - Base run for comparison",
  "compare_run_id": "string (required) - Run to compare against base",
  "include_content": "boolean (optional) - Include text diffs",
  "include_similarity": "boolean (optional) - Include similarity scores"
}
```

**Example:**

```json
{
  "name": "petaltrace.diff.compare",
  "arguments": {
    "base_run_id": "run-01JK3ABC",
    "compare_run_id": "run-01JK3XYZ",
    "include_content": true
  }
}
```

**Response:**

```json
{
  "summary": {
    "status_match": true,
    "path_divergence": false,
    "duration_delta_ms": 500,
    "token_delta": 150,
    "cost_delta": 0.0012,
    "node_diff_count": 2
  },
  "node_diffs": [
    {
      "node_id": "researcher_agent",
      "status": "content_diff",
      "output_diff": {
        "similarity": 0.85
      },
      "token_diff": {
        "base_total": 1700,
        "compare_total": 1850
      }
    }
  ],
  "cost_diff": {
    "base_cost": 0.0089,
    "compare_cost": 0.0101,
    "delta": 0.0012
  }
}
```

---

### petaltrace.run.replay

Trigger a replay of a prior run.

**Input Schema:**

```json
{
  "run_id": "string (required) - Run to replay",
  "mode": "string (optional) - live|mocked|hybrid, default 'live'",
  "model": "string (optional) - Override model",
  "temperature": "number (optional) - Override temperature",
  "auto_diff": "boolean (optional) - Compute diff after completion",
  "tags": "object (optional) - Tags for the new run"
}
```

**Example:**

```json
{
  "name": "petaltrace.run.replay",
  "arguments": {
    "run_id": "run-01JK3ABC",
    "mode": "live",
    "model": "claude-3-opus-20240229",
    "auto_diff": true,
    "tags": {"experiment": "opus-test"}
  }
}
```

**Response:**

```json
{
  "replay_id": "replay-456",
  "source_run_id": "run-01JK3ABC",
  "new_run_id": "run-01JK3NEW",
  "diff_id": "diff-789",
  "mode": "live",
  "status": "completed"
}
```

## Self-Reflective Agent Patterns

### Failure Analysis

An agent can analyze its own failed runs:

```
User: Analyze why my research workflow has been failing recently.

Agent: I'll use PetalTrace to investigate recent failures.

[Calls petaltrace.trace.list with status="failed", workflow="research-pipeline", since="24h"]

I found 3 failed runs in the last 24 hours. Let me examine the most recent one.

[Calls petaltrace.trace.get with run_id="run-01JK3ABC", include_spans=true]

The failure occurred in the 'web_search' tool with a timeout error. Let me check the prompt to understand what was being searched.

[Calls petaltrace.prompt.get with run_id="run-01JK3ABC", node_id="researcher_agent"]

I see the issue. The search query was too broad: "comprehensive analysis of all machine learning frameworks from 2020-2026". This likely caused the web search to timeout.

Recommendation: Modify the prompt to generate more focused, specific search queries.
```

### Cost Optimization

An agent can identify expensive workflows:

```
User: Help me reduce my AI costs.

Agent: I'll analyze your recent usage patterns.

[Calls petaltrace.cost.summary with since="30d", group_by="workflow"]

Your top cost driver is 'content-writer' at $45.23/month. Let me examine a typical run.

[Calls petaltrace.cost.run with run_id="run-expensive-123"]

The 'draft_generation' node uses 80% of tokens. It's using claude-3-opus. Let me compare costs if we used a different model.

[Calls petaltrace.run.replay with run_id="run-expensive-123", mode="live", model="claude-sonnet-4-20250514", auto_diff=true]

The replay completed with similar quality but 40% lower cost. I recommend switching the draft_generation node to Sonnet.
```

### Regression Testing

An agent can compare behavior across versions:

```
User: I updated my research prompt. Can you verify it still works?

Agent: I'll compare the new behavior against a previous successful run.

[Calls petaltrace.trace.list with workflow="research-pipeline", status="completed", limit=1]

Found a successful run from before your changes. Let me trigger a new run and compare.

[After new run completes, calls petaltrace.diff.compare with base_run_id="run-before", compare_run_id="run-after", include_content=true]

The diff shows:
- Token usage increased by 15% (more detailed responses)
- Output similarity is 0.78 (some structural changes)
- Cost increased by $0.002 per run

The changes look intentional - the new prompt produces more detailed research. No regressions detected.
```

## PetalFlow Workflow Example

Using PetalTrace tools in a PetalFlow workflow:

```yaml
# diagnostic-workflow.yaml
agents:
  diagnostic_agent:
    role: "Workflow Diagnostician"
    goal: "Analyze execution failures and recommend improvements"
    tools:
      - petaltrace.trace.list
      - petaltrace.trace.get
      - petaltrace.prompt.get
      - petaltrace.diff.compare
      - petaltrace.cost.summary

tasks:
  diagnose_failures:
    description: |
      Review the last 10 failed runs across all workflows.
      For each failure:
      1. Identify the failing span
      2. Examine the prompt that led to the failure
      3. Categorize the failure type (timeout, rate limit, prompt issue, etc.)

      Produce a report with:
      - Failure categories and counts
      - Most problematic workflows
      - Specific recommendations for each category
    agent: diagnostic_agent
    expected_output: "Structured failure analysis report with recommendations"

  cost_analysis:
    description: |
      Analyze cost trends over the past 30 days.
      Identify:
      - Top 3 most expensive workflows
      - Any cost anomalies (runs >2x average)
      - Opportunities for model downgrades

      For any workflow spending >$10/day, examine specific runs
      to understand the cost drivers.
    agent: diagnostic_agent
    expected_output: "Cost analysis report with optimization recommendations"
```

## Protocol Details

PetalTrace MCP server implements JSON-RPC 2.0 over stdio.

### Request Format

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/call",
  "params": {
    "name": "petaltrace.trace.list",
    "arguments": {
      "status": "failed",
      "limit": 10
    }
  }
}
```

### Response Format

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "content": [
      {
        "type": "text",
        "text": "{\"runs\": [...], \"count\": 5}"
      }
    ]
  }
}
```

### Supported Methods

| Method | Description |
|--------|-------------|
| `initialize` | Server capability negotiation |
| `tools/list` | List available tools |
| `tools/call` | Invoke a tool |

### Protocol Version

PetalTrace MCP server implements protocol version `2024-11-05`.
