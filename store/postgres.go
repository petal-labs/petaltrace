package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type PostgresOptions struct {
	DSN string
}

type PostgresStore struct {
	db *sql.DB
}

type postgresMigration struct {
	version    int
	name       string
	statements []string
}

var postgresMigrations = []postgresMigration{
	{
		version: 1,
		name:    "runs_and_spans",
		statements: []string{
			`CREATE TABLE runs (
				id TEXT PRIMARY KEY,
				workflow_id TEXT NOT NULL,
				workflow_name TEXT NOT NULL,
				workflow_version TEXT,
				source_kind TEXT NOT NULL,
				status TEXT NOT NULL,
				started_at TEXT NOT NULL,
				completed_at TEXT,
				duration_ms BIGINT,
				graph_snapshot TEXT,
				input_snapshot TEXT,
				config_snapshot TEXT,
				total_input_tokens BIGINT NOT NULL DEFAULT 0,
				total_output_tokens BIGINT NOT NULL DEFAULT 0,
				total_tokens BIGINT NOT NULL DEFAULT 0,
				cache_read_tokens BIGINT NOT NULL DEFAULT 0,
				cache_write_tokens BIGINT NOT NULL DEFAULT 0,
				estimated_cost_usd DOUBLE PRECISION NOT NULL DEFAULT 0,
				node_count BIGINT NOT NULL DEFAULT 0,
				error_count BIGINT NOT NULL DEFAULT 0,
				tags TEXT,
				trigger_source TEXT,
				parent_run_id TEXT,
				starred SMALLINT NOT NULL DEFAULT 0,
				created_at TEXT NOT NULL
			)`,
			`CREATE INDEX idx_runs_workflow ON runs(workflow_id, started_at DESC)`,
			`CREATE INDEX idx_runs_workflow_name ON runs(workflow_name, started_at DESC)`,
			`CREATE INDEX idx_runs_status ON runs(status, started_at DESC)`,
			`CREATE INDEX idx_runs_started ON runs(started_at DESC)`,
			`CREATE INDEX idx_runs_parent ON runs(parent_run_id) WHERE parent_run_id IS NOT NULL`,
			`CREATE INDEX idx_runs_starred ON runs(starred, started_at DESC) WHERE starred = 1`,
			`CREATE TABLE spans (
				id TEXT PRIMARY KEY,
				run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
				parent_id TEXT,
				trace_id TEXT NOT NULL,
				kind TEXT NOT NULL,
				name TEXT NOT NULL,
				status TEXT NOT NULL,
				started_at TEXT NOT NULL,
				completed_at TEXT,
				duration_ms BIGINT,
				node_data TEXT,
				llm_data TEXT,
				tool_data TEXT,
				edge_data TEXT,
				error_code TEXT,
				error_message TEXT,
				error_details TEXT,
				attributes TEXT,
				created_at TEXT NOT NULL
			)`,
			`CREATE INDEX idx_spans_run ON spans(run_id, started_at)`,
			`CREATE INDEX idx_spans_kind ON spans(run_id, kind)`,
			`CREATE INDEX idx_spans_parent ON spans(parent_id) WHERE parent_id IS NOT NULL`,
			`CREATE INDEX idx_spans_trace ON spans(trace_id)`,
		},
	},
	{
		version: 2,
		name:    "replay_records",
		statements: []string{
			`CREATE TABLE replays (
				id TEXT PRIMARY KEY,
				source_run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
				new_run_id TEXT,
				diff_id TEXT,
				mode TEXT NOT NULL,
				status TEXT NOT NULL,
				error_message TEXT,
				started_at TEXT NOT NULL,
				completed_at TEXT
			)`,
			`CREATE INDEX idx_replays_source ON replays(source_run_id, started_at DESC)`,
			`CREATE INDEX idx_replays_started ON replays(started_at DESC)`,
		},
	},
	{
		version: 3,
		name:    "search_diffs_pricing",
		statements: []string{
			// Span text index for full-text search, mirroring the SQLite
			// FTS5 spans_fts5 table. The tsvector is maintained on upsert
			// so the GIN index covers prompt and completion text.
			`CREATE TABLE span_text (
				span_id TEXT PRIMARY KEY REFERENCES spans(id) ON DELETE CASCADE,
				prompt_text TEXT,
				completion_text TEXT,
				search_vector tsvector NOT NULL
			)`,
			`CREATE INDEX idx_span_text_search ON span_text USING GIN (search_vector)`,
			// Cached diffs, mirroring the SQLite diffs table.
			`CREATE TABLE diffs (
				id TEXT PRIMARY KEY,
				base_run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
				compare_run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
				diff_data TEXT NOT NULL,
				created_at TEXT NOT NULL,
				UNIQUE (base_run_id, compare_run_id)
			)`,
			`CREATE INDEX idx_diffs_base ON diffs(base_run_id)`,
			`CREATE INDEX idx_diffs_compare ON diffs(compare_run_id)`,
			// Pricing table, mirroring the SQLite pricing table.
			`CREATE TABLE pricing (
				provider TEXT NOT NULL,
				model TEXT NOT NULL,
				input_per_1m DOUBLE PRECISION NOT NULL,
				output_per_1m DOUBLE PRECISION NOT NULL,
				cache_read_per_1m DOUBLE PRECISION,
				cache_write_per_1m DOUBLE PRECISION,
				effective_from TEXT NOT NULL,
				PRIMARY KEY (provider, model, effective_from)
			)`,
			`CREATE INDEX idx_pricing_lookup ON pricing(provider, model)`,
		},
	},
}

const postgresRunColumns = `id, workflow_id, workflow_name, workflow_version, source_kind, status,
	started_at, completed_at, duration_ms,
	graph_snapshot, input_snapshot, config_snapshot,
	total_input_tokens, total_output_tokens, total_tokens,
	cache_read_tokens, cache_write_tokens, estimated_cost_usd,
	node_count, error_count, tags, trigger_source, parent_run_id, starred, created_at`

const postgresSpanColumns = `id, run_id, parent_id, trace_id, kind, name, status,
	started_at, completed_at, duration_ms,
	node_data, llm_data, tool_data, edge_data,
	error_code, error_message, error_details,
	attributes, created_at`

const postgresInsertRun = `INSERT INTO runs (
	id, workflow_id, workflow_name, workflow_version, source_kind, status,
	started_at, completed_at, duration_ms,
	graph_snapshot, input_snapshot, config_snapshot,
	total_input_tokens, total_output_tokens, total_tokens,
	cache_read_tokens, cache_write_tokens, estimated_cost_usd,
	node_count, error_count, tags, trigger_source, parent_run_id, starred, created_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13,
	$14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25
)`

const postgresInsertSpan = `INSERT INTO spans (
	id, run_id, parent_id, trace_id, kind, name, status,
	started_at, completed_at, duration_ms,
	node_data, llm_data, tool_data, edge_data,
	error_code, error_message, error_details,
	attributes, created_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
	$11, $12, $13, $14, $15, $16, $17, $18, $19
)`

func NewPostgresStore(ctx context.Context, opts PostgresOptions) (*PostgresStore, error) {
	if strings.TrimSpace(opts.DSN) == "" {
		return nil, fmt.Errorf("postgres DSN is required")
	}

	db, err := sql.Open("pgx", opts.DSN)
	if err != nil {
		return nil, fmt.Errorf("opening PostgreSQL database: %w", err)
	}
	db.SetMaxOpenConns(20)

	connectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := db.PingContext(connectCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("connecting to PostgreSQL: %w", err)
	}
	if err := runPostgresMigrations(connectCtx, db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("running PostgreSQL migrations: %w", err)
	}

	return &PostgresStore{db: db}, nil
}

func runPostgresMigrations(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`); err != nil {
		return fmt.Errorf("creating migrations table: %w", err)
	}

	var currentVersion int
	if err := db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&currentVersion); err != nil {
		return fmt.Errorf("getting current schema version: %w", err)
	}

	for _, migration := range postgresMigrations {
		if migration.version <= currentVersion {
			continue
		}

		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("beginning migration %d: %w", migration.version, err)
		}

		for _, statement := range migration.statements {
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("running migration %d (%s): %w", migration.version, migration.name, err)
			}
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version, name) VALUES ($1, $2)`, migration.version, migration.name); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("recording migration %d: %w", migration.version, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("committing migration %d: %w", migration.version, err)
		}
	}

	return nil
}

func (s *PostgresStore) Close() error {
	return s.db.Close()
}

func (s *PostgresStore) CreateRun(ctx context.Context, run *Run) error {
	if run.ID == "" {
		run.ID = NewID()
	}
	if run.CreatedAt.IsZero() {
		run.CreatedAt = time.Now().UTC()
	}

	if _, err := s.db.ExecContext(ctx, postgresInsertRun, postgresRunValues(run)...); err != nil {
		return fmt.Errorf("inserting run: %w", err)
	}
	return nil
}

func (s *PostgresStore) GetRun(ctx context.Context, id string) (*Run, error) {
	run, err := scanRun(s.db.QueryRowContext(ctx, `SELECT `+postgresRunColumns+` FROM runs WHERE id = $1`, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting run: %w", err)
	}
	return run, nil
}

func (s *PostgresStore) UpdateRun(ctx context.Context, run *Run) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE runs SET
			workflow_id = $1, workflow_name = $2, workflow_version = $3,
			source_kind = $4, status = $5, started_at = $6, completed_at = $7, duration_ms = $8,
			graph_snapshot = $9, input_snapshot = $10, config_snapshot = $11,
			total_input_tokens = $12, total_output_tokens = $13, total_tokens = $14,
			cache_read_tokens = $15, cache_write_tokens = $16, estimated_cost_usd = $17,
			node_count = $18, error_count = $19, tags = $20, trigger_source = $21,
			parent_run_id = $22, starred = $23
		WHERE id = $24
	`,
		run.WorkflowID,
		run.WorkflowName,
		run.WorkflowVersion,
		run.SourceKind,
		string(run.Status),
		formatTime(run.StartedAt),
		formatTimePtr(run.CompletedAt),
		run.DurationMs,
		string(run.GraphSnapshot),
		string(run.InputSnapshot),
		string(run.ConfigSnapshot),
		run.TotalTokens.InputTokens,
		run.TotalTokens.OutputTokens,
		run.TotalTokens.TotalTokens,
		run.TotalTokens.CacheReadTokens,
		run.TotalTokens.CacheWriteTokens,
		run.EstimatedCost.Total,
		run.NodeCount,
		run.ErrorCount,
		toJSON(run.Tags),
		run.TriggerSource,
		nullString(run.ParentRunID),
		boolToInt(run.Starred),
		run.ID,
	)
	if err != nil {
		return fmt.Errorf("updating run: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking updated run: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("run not found: %s", run.ID)
	}
	return nil
}

func (s *PostgresStore) DeleteRun(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM runs WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("deleting run: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking deleted run: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("run not found: %s", id)
	}
	return nil
}

func (s *PostgresStore) ListRuns(ctx context.Context, opts ListRunsOptions) ([]Run, string, error) {
	var conditions []string
	var args []any
	addCondition := func(format string, value any) {
		args = append(args, value)
		conditions = append(conditions, fmt.Sprintf(format, len(args)))
	}

	if opts.WorkflowID != "" {
		addCondition("workflow_id = $%d", opts.WorkflowID)
	}
	if opts.WorkflowName != "" {
		addCondition("workflow_name ILIKE $%d", "%"+opts.WorkflowName+"%")
	}
	if opts.Status != "" {
		addCondition("status = $%d", string(opts.Status))
	}
	if opts.Since != nil {
		addCondition("started_at >= $%d", formatTime(*opts.Since))
	}
	if opts.Until != nil {
		addCondition("started_at <= $%d", formatTime(*opts.Until))
	}
	if opts.MinCost != nil {
		addCondition("estimated_cost_usd >= $%d", *opts.MinCost)
	}
	if opts.SourceKind != "" {
		addCondition("source_kind = $%d", opts.SourceKind)
	}
	if opts.TriggerSource != "" {
		addCondition("trigger_source = $%d", opts.TriggerSource)
	}
	if opts.Starred != nil {
		addCondition("starred = $%d", boolToInt(*opts.Starred))
	}
	if opts.Cursor != "" {
		addCondition("id < $%d", opts.Cursor)
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	limit := 50
	if opts.Limit > 0 && opts.Limit <= 1000 {
		limit = opts.Limit
	}
	args = append(args, limit+1)

	// #nosec G202 -- filter templates are fixed and ORDER BY columns are whitelisted; values are parameters.
	query := `
		SELECT ` + postgresRunColumns + `
		FROM runs
		` + whereClause + `
		` + buildRunsOrderByClause(opts.SortBy, opts.SortOrder) + fmt.Sprintf(`
		LIMIT $%d
	`, len(args))

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, "", fmt.Errorf("listing runs: %w", err)
	}
	defer rows.Close()

	var runs []Run
	for rows.Next() {
		run, err := scanRunFromRows(rows)
		if err != nil {
			return nil, "", fmt.Errorf("scanning run: %w", err)
		}
		runs = append(runs, *run)
	}
	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("reading runs: %w", err)
	}

	var nextCursor string
	if len(runs) > limit {
		nextCursor = runs[limit-1].ID
		runs = runs[:limit]
	}
	return runs, nextCursor, nil
}

func (s *PostgresStore) UpdateRunAggregates(ctx context.Context, runID string) error {
	run, err := s.GetRun(ctx, runID)
	if err != nil {
		return err
	}
	if run == nil {
		return fmt.Errorf("run not found: %s", runID)
	}

	spans, err := s.GetSpanTree(ctx, runID)
	if err != nil {
		return err
	}

	var inputTokens, outputTokens, totalTokens, nodeCount, errorCount int
	var cost float64
	for _, span := range spans {
		if span.Kind == SpanKindNode {
			nodeCount++
		}
		if span.Status == SpanStatusError {
			errorCount++
		}
		if span.Kind == SpanKindLLM && span.LLM != nil {
			inputTokens += span.LLM.Tokens.InputTokens
			outputTokens += span.LLM.Tokens.OutputTokens
			totalTokens += span.LLM.Tokens.TotalTokens
			cost += span.LLM.Tokens.CostEstimate
		}
	}
	run.TotalTokens.InputTokens = inputTokens
	run.TotalTokens.OutputTokens = outputTokens
	run.TotalTokens.TotalTokens = totalTokens
	run.EstimatedCost.Total = cost
	run.NodeCount = nodeCount
	run.ErrorCount = errorCount
	return s.UpdateRun(ctx, run)
}

func (s *PostgresStore) CreateSpan(ctx context.Context, span *Span) error {
	if span.ID == "" {
		span.ID = NewID()
	}
	if span.CreatedAt.IsZero() {
		span.CreatedAt = time.Now().UTC()
	}
	if _, err := s.db.ExecContext(ctx, postgresInsertSpan, postgresSpanValues(span)...); err != nil {
		return fmt.Errorf("inserting span: %w", err)
	}
	return nil
}

func (s *PostgresStore) CreateSpanBatch(ctx context.Context, spans []*Span) error {
	if len(spans) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning span batch transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, postgresInsertSpan)
	if err != nil {
		return fmt.Errorf("preparing span batch insert: %w", err)
	}
	defer stmt.Close()

	now := time.Now().UTC()
	for _, span := range spans {
		if span.ID == "" {
			span.ID = NewID()
		}
		if span.CreatedAt.IsZero() {
			span.CreatedAt = now
		}
		if _, err := stmt.ExecContext(ctx, postgresSpanValues(span)...); err != nil {
			return fmt.Errorf("inserting span %s: %w", span.ID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing span batch: %w", err)
	}
	return nil
}

func (s *PostgresStore) GetSpan(ctx context.Context, id string) (*Span, error) {
	span, err := scanSpan(s.db.QueryRowContext(ctx, `SELECT `+postgresSpanColumns+` FROM spans WHERE id = $1`, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting span: %w", err)
	}
	return span, nil
}

func (s *PostgresStore) GetSpanTree(ctx context.Context, runID string) ([]*Span, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+postgresSpanColumns+` FROM spans WHERE run_id = $1 ORDER BY started_at ASC`, runID)
	if err != nil {
		return nil, fmt.Errorf("querying spans: %w", err)
	}
	defer rows.Close()
	spans, err := scanSpans(rows)
	if err != nil {
		return nil, fmt.Errorf("scanning spans: %w", err)
	}
	return spans, nil
}

func (s *PostgresStore) GetSpansByKind(ctx context.Context, runID string, kind SpanKind) ([]*Span, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+postgresSpanColumns+` FROM spans WHERE run_id = $1 AND kind = $2 ORDER BY started_at ASC`, runID, string(kind))
	if err != nil {
		return nil, fmt.Errorf("querying spans by kind: %w", err)
	}
	defer rows.Close()
	spans, err := scanSpans(rows)
	if err != nil {
		return nil, fmt.Errorf("scanning spans by kind: %w", err)
	}
	return spans, nil
}

func (s *PostgresStore) UpdateSpan(ctx context.Context, span *Span) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE spans SET
			parent_id = $1, trace_id = $2, kind = $3, name = $4, status = $5,
			started_at = $6, completed_at = $7, duration_ms = $8,
			node_data = $9, llm_data = $10, tool_data = $11, edge_data = $12,
			error_code = $13, error_message = $14, error_details = $15,
			attributes = $16
		WHERE id = $17
	`,
		nullString(span.ParentID),
		span.TraceID,
		string(span.Kind),
		span.Name,
		string(span.Status),
		formatTime(span.StartedAt),
		formatTimePtr(span.CompletedAt),
		span.DurationMs,
		toJSONNullable(span.Node),
		toJSONNullable(span.LLM),
		toJSONNullable(span.Tool),
		toJSONNullable(span.Edge),
		errorCode(span.Error),
		errorMessage(span.Error),
		errorDetails(span.Error),
		toJSONNullable(span.Attributes),
		span.ID,
	)
	if err != nil {
		return fmt.Errorf("updating span: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking updated span: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("span not found: %s", span.ID)
	}
	return nil
}

func (s *PostgresStore) IndexSpanText(ctx context.Context, spanID, promptText, completionText string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO span_text (span_id, prompt_text, completion_text, search_vector)
		VALUES ($1, $2, $3, to_tsvector('english', coalesce($2, '') || ' ' || coalesce($3, '')))
		ON CONFLICT (span_id) DO UPDATE SET
			prompt_text = EXCLUDED.prompt_text,
			completion_text = EXCLUDED.completion_text,
			search_vector = EXCLUDED.search_vector
	`, spanID, promptText, completionText)

	if err != nil {
		return fmt.Errorf("indexing span text: %w", err)
	}

	return nil
}

func (s *PostgresStore) SearchSpans(ctx context.Context, query string, limit int) ([]*Span, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT
			s.id, s.run_id, s.parent_id, s.trace_id, s.kind, s.name, s.status,
			s.started_at, s.completed_at, s.duration_ms,
			s.node_data, s.llm_data, s.tool_data, s.edge_data,
			s.error_code, s.error_message, s.error_details,
			s.attributes, s.created_at
		FROM spans s
		JOIN span_text st ON s.id = st.span_id
		WHERE st.search_vector @@ websearch_to_tsquery('english', $1)
		ORDER BY s.started_at DESC
		LIMIT $2
	`, query, limit)
	if err != nil {
		return nil, fmt.Errorf("searching spans: %w", err)
	}
	defer rows.Close()

	return scanSpans(rows)
}

func (s *PostgresStore) CreateDiff(ctx context.Context, diff *RunDiff) error {
	if diff.ID == "" {
		diff.ID = NewID()
	}
	if diff.CreatedAt.IsZero() {
		diff.CreatedAt = time.Now().UTC()
	}

	diffData, err := json.Marshal(diff)
	if err != nil {
		return fmt.Errorf("marshaling diff: %w", err)
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO diffs (id, base_run_id, compare_run_id, diff_data, created_at)
		VALUES ($1, $2, $3, $4, $5)
	`, diff.ID, diff.BaseRunID, diff.CompareRunID, string(diffData), formatTime(diff.CreatedAt))

	if err != nil {
		return fmt.Errorf("inserting diff: %w", err)
	}

	return nil
}

func (s *PostgresStore) GetDiff(ctx context.Context, id string) (*RunDiff, error) {
	var diffData string
	row := s.db.QueryRowContext(ctx, `SELECT diff_data FROM diffs WHERE id = $1`, id)
	if err := row.Scan(&diffData); err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("getting diff: %w", err)
	}

	var diff RunDiff
	if err := json.Unmarshal([]byte(diffData), &diff); err != nil {
		return nil, fmt.Errorf("unmarshaling diff: %w", err)
	}

	return &diff, nil
}

func (s *PostgresStore) GetDiffByRuns(ctx context.Context, baseRunID, compareRunID string) (*RunDiff, error) {
	var diffData string
	row := s.db.QueryRowContext(ctx, `
		SELECT diff_data FROM diffs
		WHERE base_run_id = $1 AND compare_run_id = $2
	`, baseRunID, compareRunID)

	if err := row.Scan(&diffData); err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("getting diff by runs: %w", err)
	}

	var diff RunDiff
	if err := json.Unmarshal([]byte(diffData), &diff); err != nil {
		return nil, fmt.Errorf("unmarshaling diff: %w", err)
	}

	return &diff, nil
}

func (s *PostgresStore) GetPricing(ctx context.Context, provider, model string) (*PricingEntry, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT provider, model, input_per_1m, output_per_1m,
			cache_read_per_1m, cache_write_per_1m, effective_from
		FROM pricing
		WHERE provider = $1 AND model = $2
		ORDER BY effective_from DESC
		LIMIT 1
	`, provider, model)

	return scanPricingRow(row)
}

func (s *PostgresStore) UpsertPricing(ctx context.Context, entry *PricingEntry) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO pricing (
			provider, model, input_per_1m, output_per_1m,
			cache_read_per_1m, cache_write_per_1m, effective_from
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (provider, model, effective_from) DO UPDATE SET
			input_per_1m = EXCLUDED.input_per_1m,
			output_per_1m = EXCLUDED.output_per_1m,
			cache_read_per_1m = EXCLUDED.cache_read_per_1m,
			cache_write_per_1m = EXCLUDED.cache_write_per_1m
	`,
		entry.Provider,
		entry.Model,
		entry.InputPer1M,
		entry.OutputPer1M,
		entry.CacheReadPer1M,
		entry.CacheWritePer1M,
		formatTime(entry.EffectiveFrom),
	)

	if err != nil {
		return fmt.Errorf("upserting pricing: %w", err)
	}

	return nil
}

func (s *PostgresStore) ListPricing(ctx context.Context) ([]PricingEntry, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT provider, model, input_per_1m, output_per_1m,
			cache_read_per_1m, cache_write_per_1m, effective_from
		FROM pricing
		ORDER BY provider, model, effective_from DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("listing pricing: %w", err)
	}
	defer rows.Close()

	var entries []PricingEntry
	for rows.Next() {
		entry, err := scanPricingRow(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, *entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading pricing: %w", err)
	}

	return entries, nil
}

func scanPricingRow(row scanner) (*PricingEntry, error) {
	var entry PricingEntry
	var effectiveFrom string
	var cacheRead, cacheWrite sql.NullFloat64

	if err := row.Scan(
		&entry.Provider,
		&entry.Model,
		&entry.InputPer1M,
		&entry.OutputPer1M,
		&cacheRead,
		&cacheWrite,
		&effectiveFrom,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("scanning pricing: %w", err)
	}

	if cacheRead.Valid {
		entry.CacheReadPer1M = cacheRead.Float64
	}
	if cacheWrite.Valid {
		entry.CacheWritePer1M = cacheWrite.Float64
	}
	entry.EffectiveFrom, _ = parseTime(effectiveFrom)

	return &entry, nil
}

func (s *PostgresStore) GetStats(ctx context.Context) (*StoreStats, error) {
	stats := &StoreStats{}
	if err := s.db.QueryRowContext(ctx, `SELECT pg_database_size(current_database())`).Scan(&stats.DatabaseSize); err != nil {
		return nil, fmt.Errorf("getting database size: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM runs`).Scan(&stats.RunCount); err != nil {
		return nil, fmt.Errorf("counting runs: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM spans`).Scan(&stats.SpanCount); err != nil {
		return nil, fmt.Errorf("counting spans: %w", err)
	}
	var oldest, newest sql.NullString
	if err := s.db.QueryRowContext(ctx, `SELECT MIN(started_at), MAX(started_at) FROM runs`).Scan(&oldest, &newest); err != nil {
		return nil, fmt.Errorf("getting run date range: %w", err)
	}
	if t := parseTimePtr(oldest); t != nil {
		stats.OldestRun = *t
	}
	if t := parseTimePtr(newest); t != nil {
		stats.NewestRun = *t
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT workflow_name, COUNT(*) AS count
		FROM runs
		GROUP BY workflow_name
		ORDER BY count DESC
		LIMIT 10
	`)
	if err != nil {
		return nil, fmt.Errorf("getting top workflows: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var workflow WorkflowStats
		if err := rows.Scan(&workflow.WorkflowName, &workflow.RunCount); err != nil {
			return nil, fmt.Errorf("scanning workflow stats: %w", err)
		}
		stats.TopWorkflows = append(stats.TopWorkflows, workflow)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading workflow stats: %w", err)
	}
	return stats, nil
}

func (s *PostgresStore) GarbageCollect(ctx context.Context, retentionDays int, dryRun bool) (int, error) {
	cutoff := time.Now().AddDate(0, 0, -retentionDays).UTC()
	cutoffStr := formatTime(cutoff)

	var count int
	row := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM runs
		WHERE started_at < $1 AND starred = 0
	`, cutoffStr)
	if err := row.Scan(&count); err != nil {
		return 0, fmt.Errorf("counting runs to delete: %w", err)
	}

	if dryRun {
		return count, nil
	}

	result, err := s.db.ExecContext(ctx, `
		DELETE FROM runs
		WHERE started_at < $1 AND starred = 0
	`, cutoffStr)
	if err != nil {
		return 0, fmt.Errorf("deleting old runs: %w", err)
	}

	deleted, err := result.RowsAffected()
	if err != nil {
		return count, fmt.Errorf("checking deleted runs: %w", err)
	}

	return int(deleted), nil
}

// scanner abstracts over *sql.Row and *sql.Rows for shared row scanning.
type scanner interface {
	Scan(dest ...any) error
}

func postgresRunValues(run *Run) []any {
	return []any{
		run.ID,
		run.WorkflowID,
		run.WorkflowName,
		run.WorkflowVersion,
		run.SourceKind,
		string(run.Status),
		formatTime(run.StartedAt),
		formatTimePtr(run.CompletedAt),
		run.DurationMs,
		string(run.GraphSnapshot),
		string(run.InputSnapshot),
		string(run.ConfigSnapshot),
		run.TotalTokens.InputTokens,
		run.TotalTokens.OutputTokens,
		run.TotalTokens.TotalTokens,
		run.TotalTokens.CacheReadTokens,
		run.TotalTokens.CacheWriteTokens,
		run.EstimatedCost.Total,
		run.NodeCount,
		run.ErrorCount,
		toJSON(run.Tags),
		run.TriggerSource,
		nullString(run.ParentRunID),
		boolToInt(run.Starred),
		formatTime(run.CreatedAt),
	}
}

func postgresSpanValues(span *Span) []any {
	return []any{
		span.ID,
		span.RunID,
		nullString(span.ParentID),
		span.TraceID,
		string(span.Kind),
		span.Name,
		string(span.Status),
		formatTime(span.StartedAt),
		formatTimePtr(span.CompletedAt),
		span.DurationMs,
		toJSONNullable(span.Node),
		toJSONNullable(span.LLM),
		toJSONNullable(span.Tool),
		toJSONNullable(span.Edge),
		errorCode(span.Error),
		errorMessage(span.Error),
		errorDetails(span.Error),
		toJSONNullable(span.Attributes),
		formatTime(span.CreatedAt),
	}
}
