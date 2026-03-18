# API Reference

Complete reference for the PetalTrace HTTP API.

**Base URL:** `http://localhost:8090/api`

## Response Format

All endpoints return JSON. Successful responses have a `2xx` status code. Errors return:

```json
{
  "error": "Error message",
  "code": "ERROR_CODE",
  "details": {}
}
```

## Runs

### List Runs

```
GET /api/runs
```

List runs with optional filtering and pagination.

**Query Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `workflow` | string | Filter by workflow name |
| `workflow_id` | string | Filter by workflow ID |
| `status` | string | Filter by status: `running`, `completed`, `failed`, `cancelled` |
| `since` | string | Start time (RFC3339 or duration like `24h`, `7d`) |
| `until` | string | End time (RFC3339 or duration) |
| `min_cost` | float | Minimum estimated cost in USD |
| `starred` | bool | Filter starred runs only |
| `cursor` | string | Pagination cursor |
| `limit` | int | Maximum results (default: 50) |
| `sort_by` | string | Field to sort by |
| `sort_order` | string | `asc` or `desc` |

**Response:**

```json
{
  "data": [
    {
      "id": "run-01JK3ABC",
      "workflow_id": "wf-123",
      "workflow_name": "email-processor",
      "status": "completed",
      "started_at": "2026-03-17T10:15:30Z",
      "completed_at": "2026-03-17T10:15:31.2Z",
      "duration_ms": 1200,
      "total_tokens": {
        "input_tokens": 3000,
        "output_tokens": 2000,
        "total_tokens": 5000
      },
      "estimated_cost": {
        "total": 0.015,
        "currency": "USD"
      },
      "tags": {"environment": "production"}
    }
  ],
  "cursor": "next-page-cursor",
  "has_more": true
}
```

**Example:**

```bash
curl "http://localhost:8090/api/runs?status=completed&since=24h&limit=10"
```

### Get Run

```
GET /api/runs/{id}
```

Get a single run by ID.

**Query Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `include_spans` | bool | Include full span tree |

**Response:**

```json
{
  "id": "run-01JK3ABC",
  "workflow_name": "email-processor",
  "status": "completed",
  "started_at": "2026-03-17T10:15:30Z",
  "completed_at": "2026-03-17T10:15:31.2Z",
  "duration_ms": 1200,
  "graph_snapshot": {...},
  "input_snapshot": {...},
  "total_tokens": {...},
  "estimated_cost": {...},
  "tags": {}
}
```

With `include_spans=true`:

```json
{
  "run": {...},
  "spans": [...]
}
```

### Delete Run

```
DELETE /api/runs/{id}
```

Delete a run and all associated spans.

**Response:**

```json
{
  "status": "deleted",
  "run_id": "run-01JK3ABC"
}
```

### Get Span Tree

```
GET /api/runs/{id}/spans
```

Get all spans for a run.

**Query Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `kind` | string | Filter by kind: `llm`, `node`, `tool`, `edge`, `custom` |
| `node` | string | Filter by node ID |

**Response:**

```json
[
  {
    "id": "span-123",
    "run_id": "run-01JK3ABC",
    "parent_id": null,
    "trace_id": "trace-xyz",
    "kind": "node",
    "name": "researcher_agent",
    "status": "ok",
    "started_at": "2026-03-17T10:15:30Z",
    "completed_at": "2026-03-17T10:15:31Z",
    "duration_ms": 1000,
    "node": {
      "node_id": "researcher_agent",
      "node_type": "llm_prompt"
    }
  }
]
```

### Get Single Span

```
GET /api/runs/{id}/spans/{spanId}
```

Get a specific span with full payload.

### Get Graph

```
GET /api/runs/{id}/graph
```

Get the graph snapshot with node execution status overlay.

**Response:**

```json
{
  "run_id": "run-01JK3ABC",
  "graph_snapshot": {
    "nodes": [...],
    "edges": [...]
  },
  "node_statuses": {
    "researcher_agent": {
      "status": "ok",
      "duration_ms": 1000,
      "tokens": 5000,
      "cost": 0.015
    }
  }
}
```

---

## Prompts

### Get Prompt

```
GET /api/runs/{id}/prompts/{nodeId}
```

Get the full LLM prompt and completion for a node.

**Response:**

```json
{
  "span_id": "span-123",
  "run_id": "run-01JK3ABC",
  "node_id": "researcher_agent",
  "node_type": "llm_prompt",
  "name": "researcher_agent",
  "provider": "anthropic",
  "model": "claude-sonnet-4-20250514",
  "prompt": {
    "system_prompt": "You are a helpful research assistant.",
    "messages": [
      {
        "role": "user",
        "content": "Research the topic of AI observability"
      }
    ],
    "tool_definitions": [
      {
        "name": "web_search",
        "description": "Search the web",
        "input_schema": {...}
      }
    ],
    "temperature": 0.7,
    "max_tokens": 4096
  },
  "completion": {
    "content": [...],
    "text_content": "Based on my research...",
    "stop_reason": "end_turn"
  },
  "tokens": {
    "input_tokens": 500,
    "output_tokens": 1200,
    "total_tokens": 1700,
    "cost_estimate": 0.0089,
    "cache_read_tokens": 100,
    "cache_write_tokens": 0
  },
  "timing": {
    "started_at": "2026-03-17T10:15:30Z",
    "completed_at": "2026-03-17T10:15:31Z",
    "duration_ms": 1000,
    "time_to_first_token_ms": 150,
    "total_latency_ms": 1000
  }
}
```

---

## Cost

### Cost Summary

```
GET /api/cost/summary
```

Aggregate cost metrics across runs.

**Query Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `since` | string | Start time (default: `7d`) |
| `until` | string | End time (default: now) |
| `group_by` | string | Group by: `workflow`, `provider`, `model` |

**Response:**

```json
{
  "since": "2026-03-10T00:00:00Z",
  "until": "2026-03-17T23:59:59Z",
  "total_runs": 142,
  "total_tokens": 1234567,
  "total_cost": 12.34,
  "input_tokens": 800000,
  "output_tokens": 434567,
  "cache_read_tokens": 50000,
  "cache_write_tokens": 10000,
  "by_workflow": {
    "research-pipeline": {"runs": 62, "tokens": 600000, "cost": 6.50},
    "email-processor": {"runs": 80, "tokens": 634567, "cost": 5.84}
  },
  "by_provider": {
    "anthropic": {"runs": 100, "tokens": 900000, "cost": 8.90},
    "openai": {"runs": 42, "tokens": 334567, "cost": 3.44}
  },
  "by_model": {
    "claude-sonnet-4-20250514": {"runs": 80, "tokens": 700000, "cost": 7.00},
    "gpt-4o": {"runs": 42, "tokens": 334567, "cost": 3.44}
  }
}
```

### Cost by Run

```
GET /api/cost/runs/{id}
```

Per-run cost breakdown.

**Response:**

```json
{
  "run_id": "run-01JK3ABC",
  "workflow_name": "email-processor",
  "total_tokens": {
    "input_tokens": 3000,
    "output_tokens": 2000,
    "total_tokens": 5000
  },
  "estimated_cost": {
    "total": 0.015,
    "currency": "USD",
    "by_provider": {"anthropic": 0.015},
    "by_model": {"claude-sonnet-4-20250514": 0.015},
    "by_node": {"researcher_agent": 0.010, "writer_agent": 0.005}
  },
  "llm_calls": 2,
  "by_node": [
    {
      "node_id": "researcher_agent",
      "input_tokens": 2000,
      "output_tokens": 1500,
      "cost": 0.010
    },
    {
      "node_id": "writer_agent",
      "input_tokens": 1000,
      "output_tokens": 500,
      "cost": 0.005
    }
  ]
}
```

### Cost Timeseries

```
GET /api/cost/timeseries
```

Time-bucketed cost data for charts.

**Query Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `since` | string | Start time (default: `7d`) |
| `until` | string | End time (default: now) |
| `bucket` | string | Bucket size: `1h`, `1d`, etc. (default: `1h`) |

**Response:**

```json
{
  "since": "2026-03-10T00:00:00Z",
  "until": "2026-03-17T23:59:59Z",
  "bucket_size": "1h",
  "data_points": [
    {"timestamp": "2026-03-17T10:00:00Z", "runs": 5, "tokens": 25000, "cost": 0.25},
    {"timestamp": "2026-03-17T11:00:00Z", "runs": 8, "tokens": 40000, "cost": 0.40}
  ]
}
```

---

## Diff

### Compute Diff

```
POST /api/diff
```

Compare two runs.

**Request Body:**

```json
{
  "base_run_id": "run-01JK3ABC",
  "compare_run_id": "run-01JK3XYZ",
  "include_content": true,
  "include_inputs": false,
  "no_cache": false
}
```

**Response:**

```json
{
  "id": "diff-123",
  "base_run_id": "run-01JK3ABC",
  "compare_run_id": "run-01JK3XYZ",
  "summary": {
    "status_match": true,
    "path_divergence": false,
    "duration_delta_ms": 1250,
    "token_delta": 342,
    "cost_delta": 0.00215,
    "node_diff_count": 3
  },
  "node_diffs": [
    {
      "node_id": "researcher_agent",
      "node_type": "llm_prompt",
      "status": "content_diff",
      "prompt_diff": {
        "similarity": 0.95
      },
      "output_diff": {
        "similarity": 0.72,
        "hunks": [...]
      },
      "token_diff": {
        "base_input": 2000,
        "compare_input": 2100,
        "base_output": 1500,
        "compare_output": 1600
      },
      "duration_base_ms": 1000,
      "duration_compare_ms": 1200
    }
  ],
  "cost_diff": {
    "base_cost": 0.01523,
    "compare_cost": 0.01738,
    "delta": 0.00215,
    "by_model": {
      "claude-sonnet-4-20250514": 0.00215
    }
  }
}
```

### Get Diff by ID

```
GET /api/diff/{id}
```

Retrieve a cached diff by ID.

### Get Diff by Runs

```
GET /api/diff/runs?base_run_id=X&compare_run_id=Y
```

Retrieve a diff by run IDs.

---

## Replay

### Trigger Replay

```
POST /api/replay
```

Start a replay operation.

**Request Body:**

```json
{
  "source_run_id": "run-01JK3ABC",
  "mode": "live",
  "model": "claude-3-opus-20240229",
  "temperature": 0.5,
  "tags": {"experiment": "v2"},
  "auto_diff": true,
  "sync": true
}
```

**Replay Modes:**

| Mode | Description |
|------|-------------|
| `live` | Re-execute against real LLM providers |
| `mocked` | Use captured responses (deterministic) |
| `hybrid` | Mock tools, make live LLM calls |

**Response:**

```json
{
  "replay_id": "replay-456",
  "source_run_id": "run-01JK3ABC",
  "new_run_id": "run-01JK3NEW",
  "diff_id": "diff-789",
  "mode": "live",
  "status": "completed",
  "started_at": "2026-03-17T10:20:00Z",
  "completed_at": "2026-03-17T10:20:05Z"
}
```

### Get Replay Status

```
GET /api/replay/{id}
```

Get the status of a replay operation.

### List Replays

```
GET /api/replays?source_run_id=X
```

List replay operations, optionally filtered by source run.

---

## Live Streaming (SSE)

### Stream Run Updates

```
GET /api/runs/{id}/stream
```

Server-Sent Events stream for a single run.

**Events:**

| Event | Data |
|-------|------|
| `run` | Run object (initial + updates) |
| `spans` | Initial span tree |
| `span` | New span added |
| `done` | `{status: "completed"}` |
| `error` | `{error: "message"}` |

**Example:**

```bash
curl -N "http://localhost:8090/api/runs/run-01JK3ABC/stream"
```

### Stream All Active Runs

```
GET /api/live
```

SSE stream for all currently running workflows.

**Events:**

| Event | Data |
|-------|------|
| `init` | `{active_runs: [...]}` |
| `run_started` | Run object |
| `run_updated` | Run object |
| `run_completed` | `{run_id: "..."}` |
| `heartbeat` | `{timestamp: ..., active_count: ...}` |

---

## System

### Health Check

```
GET /api/health
```

**Response:**

```json
{
  "status": "healthy",
  "timestamp": "2026-03-17T18:29:11Z",
  "version": "0.1.0-dev",
  "details": {
    "store": "connected"
  }
}
```

### Statistics

```
GET /api/stats
```

**Response:**

```json
{
  "database": {
    "size_bytes": 268435456,
    "run_count": 200,
    "span_count": 4521,
    "diff_count": 15,
    "oldest_run": "2026-03-01T08:15:30Z",
    "newest_run": "2026-03-17T18:29:08Z"
  },
  "top_workflows": [
    {"workflow_name": "email-processor", "run_count": 85},
    {"workflow_name": "research-pipeline", "run_count": 62}
  ],
  "runtime": {
    "go_version": "go1.25.0",
    "num_goroutine": 12,
    "num_cpu": 8,
    "alloc_bytes": 10485760,
    "total_alloc_bytes": 104857600,
    "sys_bytes": 52428800
  }
}
```

### Get Pricing

```
GET /api/pricing
```

Get all pricing entries.

**Query Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `provider` | string | Filter by provider |
| `model` | string | Get specific model (requires provider) |

**Response:**

```json
{
  "entries": [
    {
      "provider": "anthropic",
      "model": "claude-sonnet-4-20250514",
      "input_per_1m": 3.00,
      "output_per_1m": 15.00,
      "cache_read_per_1m": 0.30,
      "cache_write_per_1m": 3.75,
      "effective_from": "2026-01-01T00:00:00Z"
    }
  ],
  "by_provider": {
    "anthropic": [...],
    "openai": [...]
  },
  "updated_at": "2026-03-17T00:00:00Z"
}
```

### Update Pricing

```
PUT /api/pricing
```

Add or update a pricing entry.

**Request Body:**

```json
{
  "provider": "anthropic",
  "model": "claude-3-opus-20240229",
  "input_per_1m": 15.0,
  "output_per_1m": 75.0,
  "cache_read_per_1m": 1.5,
  "cache_write_per_1m": 18.75
}
```

**Response:**

```json
{
  "status": "updated",
  "entry": {...}
}
```
