# Changelog

All notable changes to PetalTrace are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.0] - 2026-11-19

Initial release of PetalTrace — a self-reflection and observability tool for
AI agent workflows. PetalTrace receives OpenTelemetry traces (OTLP over HTTP
and gRPC) from PetalFlow and any OpenTelemetry-instrumented application,
classifies and correlates spans into workflow runs, enriches LLM calls with
cost estimates, and makes the results queryable through a CLI, HTTP API,
React UI, and an MCP server for agent self-inspection.

### Added

#### Tracing & Collector
- OTLP trace ingestion over HTTP (port 4318) and gRPC (port 4317), with
  span classification by kind (llm, node, tool, edge), correlation into
  workflow runs, batched persistence, LLM span enrichment with cost
  estimates, and prompt/completion text extraction for full-text search.

#### Storage
- SQLite persistence layer with runs, spans, full-text search (FTS5),
  diffs, pricing, replays, and aggregate updates; schema migrations with
  legacy FTS4 index backfill.
- PostgreSQL backend selectable via `storage.driver: postgres`, with full
  `TraceStore` parity: runs/spans CRUD, span text indexing/search
  (tsvector + GIN), diffs, pricing, stats, retention garbage collection,
  and aggregate updates. No operation silently falls back to SQLite.

#### CLI (`petaltrace`)
- `serve` — starts the OTLP collector and HTTP API daemon.
- `runs`, `run` — list and inspect workflow runs.
- `prompts` — view prompts and completions captured from LLM calls.
- `cost summary` / `cost run` — token and cost analytics with workflow,
  provider, and model groupings and JSON output.
- `diff` — compare two runs (structural path, content similarity, tokens,
  and costs).
- `replay` — re-execute a run in `live`, `mocked`, or `hybrid` mode with
  optional auto-diff; replay records persist across daemon restarts.
- `export` / `import` — move run data between stores.
- `gc` — retention-based garbage collection with `--retain`, `--dry-run`,
  `--include-starred`, and starred-run preservation; traverses every run
  page so all eligible runs are evaluated.
- `stats`, `version` — storage statistics and version information.
- `mcp` — run the MCP server over stdio for integration with MCP clients.

#### HTTP API (`petaltrace serve`, default port 8090, loopback only)
- Run management endpoints, prompt inspection, span queries, cost
  summary/timeseries analytics, diff endpoints, replay endpoints, health,
  stats, and pricing endpoints, plus SSE streaming for live monitoring.
- CORS, logging, and panic-recovery middleware; listener bind failures
  propagate at startup instead of failing silently.

#### Web UI
- React UI for run browsing, live monitoring, cost dashboard with charts,
  diff view with side-by-side comparison, and execution graph topology
  with React Flow.

#### Replay Engine
- Live, mocked, and hybrid replay modes with request validation and
  optional auto-diff, exposed through the CLI, HTTP API, and MCP.
- Replay records persisted through `TraceStore` (SQLite and PostgreSQL)
  so status and identifiers survive daemon restarts.

#### Diff Engine
- Structural comparison of execution paths, content diffing with
  similarity scores, token/cost comparison, and cached diff retrieval.

#### MCP Server (`petaltrace mcp`)
- Tools under the `petaltrace.*` namespace: trace query, prompt
  inspection, cost analysis, diff comparison, and replay — enabling
  agents to inspect their own execution history.

#### PetalFlow Runtime Contract
- Declares PetalFlow v1.2.0 as the minimum supported runtime version and
  verifies both replay endpoints plus trace ingestion in CI.

### Fixed
- Garbage collection traverses every `ListRuns` page, so eligible runs on
  later pages are deleted (or reported in dry-run) instead of only the
  newest page being evaluated.
- Cost summaries and timeseries aggregate all matching run pages instead
  of silently dropping every page beyond the first.
- Retention `never` preserves all runs; a zero retention period no longer
  falls back to 30 days.
- API and collector listeners bind only to loopback addresses and report
  bind failures at startup.
- CGO-free SQLite builds so release binaries (linux, darwin, windows,
  amd64/arm64) build without a C toolchain.

### Platform
- Release pipeline publishes binaries for linux/amd64, linux/arm64,
  darwin/amd64, darwin/arm64, and windows/amd64 with SHA-256 checksums on
  every `v*.*.*` tag.

[Unreleased]: https://github.com/petal-labs/petaltrace/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/petal-labs/petaltrace/releases/tag/v0.1.0
