package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func (s *SQLiteStore) CreateRun(ctx context.Context, run *Run) error {
	if run.ID == "" {
		run.ID = NewID()
	}
	if run.CreatedAt.IsZero() {
		run.CreatedAt = time.Now().UTC()
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO runs (
			id, workflow_id, workflow_name, workflow_version, source_kind, status,
			started_at, completed_at, duration_ms,
			graph_snapshot, input_snapshot, config_snapshot,
			total_input_tokens, total_output_tokens, total_tokens,
			cache_read_tokens, cache_write_tokens, estimated_cost_usd,
			node_count, error_count, tags, trigger_source, parent_run_id, starred, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
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
	)

	if err != nil {
		return fmt.Errorf("inserting run: %w", err)
	}

	return nil
}

func (s *SQLiteStore) GetRun(ctx context.Context, id string) (*Run, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT
			id, workflow_id, workflow_name, workflow_version, source_kind, status,
			started_at, completed_at, duration_ms,
			graph_snapshot, input_snapshot, config_snapshot,
			total_input_tokens, total_output_tokens, total_tokens,
			cache_read_tokens, cache_write_tokens, estimated_cost_usd,
			node_count, error_count, tags, trigger_source, parent_run_id, starred, created_at
		FROM runs WHERE id = ?
	`, id)

	run, err := scanRun(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting run: %w", err)
	}

	return run, nil
}

func (s *SQLiteStore) UpdateRun(ctx context.Context, run *Run) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE runs SET
			workflow_id = ?, workflow_name = ?, workflow_version = ?,
			source_kind = ?, status = ?, started_at = ?, completed_at = ?, duration_ms = ?,
			graph_snapshot = ?, input_snapshot = ?, config_snapshot = ?,
			total_input_tokens = ?, total_output_tokens = ?, total_tokens = ?,
			cache_read_tokens = ?, cache_write_tokens = ?, estimated_cost_usd = ?,
			node_count = ?, error_count = ?, tags = ?, trigger_source = ?,
			parent_run_id = ?, starred = ?
		WHERE id = ?
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

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("run not found: %s", run.ID)
	}

	return nil
}

func (s *SQLiteStore) DeleteRun(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, "DELETE FROM runs WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("deleting run: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("run not found: %s", id)
	}

	return nil
}

func (s *SQLiteStore) ListRuns(ctx context.Context, opts ListRunsOptions) ([]Run, string, error) {
	var conditions []string
	var args []any

	if opts.WorkflowID != "" {
		conditions = append(conditions, "workflow_id = ?")
		args = append(args, opts.WorkflowID)
	}

	if opts.WorkflowName != "" {
		conditions = append(conditions, "workflow_name LIKE ?")
		args = append(args, "%"+opts.WorkflowName+"%")
	}

	if opts.Status != "" {
		conditions = append(conditions, "status = ?")
		args = append(args, string(opts.Status))
	}

	if opts.Since != nil {
		conditions = append(conditions, "started_at >= ?")
		args = append(args, formatTime(*opts.Since))
	}

	if opts.Until != nil {
		conditions = append(conditions, "started_at <= ?")
		args = append(args, formatTime(*opts.Until))
	}

	if opts.MinCost != nil {
		conditions = append(conditions, "estimated_cost_usd >= ?")
		args = append(args, *opts.MinCost)
	}

	if opts.SourceKind != "" {
		conditions = append(conditions, "source_kind = ?")
		args = append(args, opts.SourceKind)
	}

	if opts.TriggerSource != "" {
		conditions = append(conditions, "trigger_source = ?")
		args = append(args, opts.TriggerSource)
	}

	if opts.Starred != nil {
		conditions = append(conditions, "starred = ?")
		args = append(args, boolToInt(*opts.Starred))
	}

	if opts.Cursor != "" {
		conditions = append(conditions, "id < ?")
		args = append(args, opts.Cursor)
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	sortBy := "started_at"
	if opts.SortBy != "" {
		allowedSorts := map[string]bool{
			"started_at": true, "duration_ms": true, "total_tokens": true,
			"estimated_cost_usd": true, "workflow_name": true,
		}
		if allowedSorts[opts.SortBy] {
			sortBy = opts.SortBy
		}
	}

	sortOrder := "DESC"
	if opts.SortOrder == "asc" || opts.SortOrder == "ASC" {
		sortOrder = "ASC"
	}

	limit := 50
	if opts.Limit > 0 && opts.Limit <= 1000 {
		limit = opts.Limit
	}

	query := fmt.Sprintf(`
		SELECT
			id, workflow_id, workflow_name, workflow_version, source_kind, status,
			started_at, completed_at, duration_ms,
			graph_snapshot, input_snapshot, config_snapshot,
			total_input_tokens, total_output_tokens, total_tokens,
			cache_read_tokens, cache_write_tokens, estimated_cost_usd,
			node_count, error_count, tags, trigger_source, parent_run_id, starred, created_at
		FROM runs
		%s
		ORDER BY %s %s
		LIMIT ?
	`, whereClause, sortBy, sortOrder)

	args = append(args, limit+1)

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

	var nextCursor string
	if len(runs) > limit {
		nextCursor = runs[limit-1].ID
		runs = runs[:limit]
	}

	return runs, nextCursor, nil
}

func (s *SQLiteStore) UpdateRunAggregates(ctx context.Context, runID string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE runs SET
			total_input_tokens = COALESCE((
				SELECT SUM(json_extract(llm_data, '$.tokens.input_tokens'))
				FROM spans WHERE run_id = ? AND kind = 'llm' AND llm_data IS NOT NULL
			), 0),
			total_output_tokens = COALESCE((
				SELECT SUM(json_extract(llm_data, '$.tokens.output_tokens'))
				FROM spans WHERE run_id = ? AND kind = 'llm' AND llm_data IS NOT NULL
			), 0),
			total_tokens = COALESCE((
				SELECT SUM(json_extract(llm_data, '$.tokens.total_tokens'))
				FROM spans WHERE run_id = ? AND kind = 'llm' AND llm_data IS NOT NULL
			), 0),
			estimated_cost_usd = COALESCE((
				SELECT SUM(json_extract(llm_data, '$.tokens.cost_estimate'))
				FROM spans WHERE run_id = ? AND kind = 'llm' AND llm_data IS NOT NULL
			), 0),
			node_count = (
				SELECT COUNT(*) FROM spans WHERE run_id = ? AND kind = 'node'
			),
			error_count = (
				SELECT COUNT(*) FROM spans WHERE run_id = ? AND status = 'error'
			)
		WHERE id = ?
	`, runID, runID, runID, runID, runID, runID, runID)

	if err != nil {
		return fmt.Errorf("updating run aggregates: %w", err)
	}

	return nil
}

func scanRun(row *sql.Row) (*Run, error) {
	var run Run
	var startedAt, completedAt, createdAt sql.NullString
	var graphSnapshot, inputSnapshot, configSnapshot sql.NullString
	var tags sql.NullString
	var parentRunID sql.NullString
	var starred int

	err := row.Scan(
		&run.ID,
		&run.WorkflowID,
		&run.WorkflowName,
		&run.WorkflowVersion,
		&run.SourceKind,
		&run.Status,
		&startedAt,
		&completedAt,
		&run.DurationMs,
		&graphSnapshot,
		&inputSnapshot,
		&configSnapshot,
		&run.TotalTokens.InputTokens,
		&run.TotalTokens.OutputTokens,
		&run.TotalTokens.TotalTokens,
		&run.TotalTokens.CacheReadTokens,
		&run.TotalTokens.CacheWriteTokens,
		&run.EstimatedCost.Total,
		&run.NodeCount,
		&run.ErrorCount,
		&tags,
		&run.TriggerSource,
		&parentRunID,
		&starred,
		&createdAt,
	)
	if err != nil {
		return nil, err
	}

	if startedAt.Valid {
		t, _ := parseTime(startedAt.String)
		run.StartedAt = t
	}
	run.CompletedAt = parseTimePtr(completedAt)
	if createdAt.Valid {
		t, _ := parseTime(createdAt.String)
		run.CreatedAt = t
	}

	if graphSnapshot.Valid {
		run.GraphSnapshot = json.RawMessage(graphSnapshot.String)
	}
	if inputSnapshot.Valid {
		run.InputSnapshot = json.RawMessage(inputSnapshot.String)
	}
	if configSnapshot.Valid {
		run.ConfigSnapshot = json.RawMessage(configSnapshot.String)
	}

	run.Tags = fromJSON[map[string]string](tags.String)
	run.ParentRunID = nullStringPtr(parentRunID)
	run.Starred = starred == 1
	run.EstimatedCost.Currency = "USD"

	return &run, nil
}

func scanRunFromRows(rows *sql.Rows) (*Run, error) {
	var run Run
	var startedAt, completedAt, createdAt sql.NullString
	var graphSnapshot, inputSnapshot, configSnapshot sql.NullString
	var tags sql.NullString
	var parentRunID sql.NullString
	var starred int

	err := rows.Scan(
		&run.ID,
		&run.WorkflowID,
		&run.WorkflowName,
		&run.WorkflowVersion,
		&run.SourceKind,
		&run.Status,
		&startedAt,
		&completedAt,
		&run.DurationMs,
		&graphSnapshot,
		&inputSnapshot,
		&configSnapshot,
		&run.TotalTokens.InputTokens,
		&run.TotalTokens.OutputTokens,
		&run.TotalTokens.TotalTokens,
		&run.TotalTokens.CacheReadTokens,
		&run.TotalTokens.CacheWriteTokens,
		&run.EstimatedCost.Total,
		&run.NodeCount,
		&run.ErrorCount,
		&tags,
		&run.TriggerSource,
		&parentRunID,
		&starred,
		&createdAt,
	)
	if err != nil {
		return nil, err
	}

	if startedAt.Valid {
		t, _ := parseTime(startedAt.String)
		run.StartedAt = t
	}
	run.CompletedAt = parseTimePtr(completedAt)
	if createdAt.Valid {
		t, _ := parseTime(createdAt.String)
		run.CreatedAt = t
	}

	if graphSnapshot.Valid {
		run.GraphSnapshot = json.RawMessage(graphSnapshot.String)
	}
	if inputSnapshot.Valid {
		run.InputSnapshot = json.RawMessage(inputSnapshot.String)
	}
	if configSnapshot.Valid {
		run.ConfigSnapshot = json.RawMessage(configSnapshot.String)
	}

	run.Tags = fromJSON[map[string]string](tags.String)
	run.ParentRunID = nullStringPtr(parentRunID)
	run.Starred = starred == 1
	run.EstimatedCost.Currency = "USD"

	return &run, nil
}

func formatTimePtr(t *time.Time) sql.NullString {
	if t == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: formatTime(*t), Valid: true}
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
