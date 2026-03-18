package store

import (
	"database/sql"
	"fmt"
)

type Migration struct {
	Version int
	Name    string
	Up      string
}

var migrations = []Migration{
	{
		Version: 1,
		Name:    "initial_schema",
		Up: `
-- Schema version tracking
CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    applied_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

-- Runs
CREATE TABLE runs (
    id              TEXT PRIMARY KEY,
    workflow_id     TEXT NOT NULL,
    workflow_name   TEXT NOT NULL,
    workflow_version TEXT,
    source_kind     TEXT NOT NULL,
    status          TEXT NOT NULL,
    started_at      TEXT NOT NULL,
    completed_at    TEXT,
    duration_ms     INTEGER,

    graph_snapshot  TEXT,
    input_snapshot  TEXT,
    config_snapshot TEXT,

    total_input_tokens  INTEGER DEFAULT 0,
    total_output_tokens INTEGER DEFAULT 0,
    total_tokens        INTEGER DEFAULT 0,
    cache_read_tokens   INTEGER DEFAULT 0,
    cache_write_tokens  INTEGER DEFAULT 0,
    estimated_cost_usd  REAL DEFAULT 0.0,
    node_count          INTEGER DEFAULT 0,
    error_count         INTEGER DEFAULT 0,

    tags            TEXT,
    trigger_source  TEXT,
    parent_run_id   TEXT,
    starred         INTEGER DEFAULT 0,

    created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX idx_runs_workflow ON runs(workflow_id, started_at DESC);
CREATE INDEX idx_runs_workflow_name ON runs(workflow_name, started_at DESC);
CREATE INDEX idx_runs_status ON runs(status, started_at DESC);
CREATE INDEX idx_runs_started ON runs(started_at DESC);
CREATE INDEX idx_runs_parent ON runs(parent_run_id) WHERE parent_run_id IS NOT NULL;
CREATE INDEX idx_runs_starred ON runs(starred, started_at DESC) WHERE starred = 1;

-- Spans
CREATE TABLE spans (
    id          TEXT PRIMARY KEY,
    run_id      TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    parent_id   TEXT,
    trace_id    TEXT NOT NULL,
    kind        TEXT NOT NULL,
    name        TEXT NOT NULL,
    status      TEXT NOT NULL,
    started_at  TEXT NOT NULL,
    completed_at TEXT,
    duration_ms INTEGER,

    node_data   TEXT,
    llm_data    TEXT,
    tool_data   TEXT,
    edge_data   TEXT,

    error_code    TEXT,
    error_message TEXT,
    error_details TEXT,

    attributes  TEXT,

    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX idx_spans_run ON spans(run_id, started_at);
CREATE INDEX idx_spans_kind ON spans(run_id, kind);
CREATE INDEX idx_spans_parent ON spans(parent_id) WHERE parent_id IS NOT NULL;
CREATE INDEX idx_spans_trace ON spans(trace_id);

-- Full-text search on LLM prompts and completions
CREATE VIRTUAL TABLE spans_fts USING fts4(
    span_id,
    prompt_text,
    completion_text,
    tokenize=porter
);

-- Cached diffs
CREATE TABLE diffs (
    id              TEXT PRIMARY KEY,
    base_run_id     TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    compare_run_id  TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    diff_data       TEXT NOT NULL,
    created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),

    UNIQUE(base_run_id, compare_run_id)
);

CREATE INDEX idx_diffs_base ON diffs(base_run_id);
CREATE INDEX idx_diffs_compare ON diffs(compare_run_id);

-- Pricing table
CREATE TABLE pricing (
    provider        TEXT NOT NULL,
    model           TEXT NOT NULL,
    input_per_1m    REAL NOT NULL,
    output_per_1m   REAL NOT NULL,
    cache_read_per_1m  REAL,
    cache_write_per_1m REAL,
    effective_from  TEXT NOT NULL,

    PRIMARY KEY (provider, model, effective_from)
);

CREATE INDEX idx_pricing_lookup ON pricing(provider, model);
`,
	},
}

func RunMigrations(db *sql.DB) error {
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		)
	`); err != nil {
		return fmt.Errorf("creating migrations table: %w", err)
	}

	var currentVersion int
	row := db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_migrations")
	if err := row.Scan(&currentVersion); err != nil {
		return fmt.Errorf("getting current version: %w", err)
	}

	for _, m := range migrations {
		if m.Version <= currentVersion {
			continue
		}

		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("beginning transaction for migration %d: %w", m.Version, err)
		}

		if _, err := tx.Exec(m.Up); err != nil {
			tx.Rollback()
			return fmt.Errorf("running migration %d (%s): %w", m.Version, m.Name, err)
		}

		if _, err := tx.Exec(
			"INSERT INTO schema_migrations (version, name) VALUES (?, ?)",
			m.Version, m.Name,
		); err != nil {
			tx.Rollback()
			return fmt.Errorf("recording migration %d: %w", m.Version, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("committing migration %d: %w", m.Version, err)
		}
	}

	return nil
}

func GetSchemaVersion(db *sql.DB) (int, error) {
	var version int
	row := db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_migrations")
	if err := row.Scan(&version); err != nil {
		return 0, err
	}
	return version, nil
}
