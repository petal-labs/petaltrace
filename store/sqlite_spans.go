package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

func (s *SQLiteStore) CreateSpan(ctx context.Context, span *Span) error {
	if span.ID == "" {
		span.ID = NewID()
	}
	if span.CreatedAt.IsZero() {
		span.CreatedAt = time.Now().UTC()
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO spans (
			id, run_id, parent_id, trace_id, kind, name, status,
			started_at, completed_at, duration_ms,
			node_data, llm_data, tool_data, edge_data,
			error_code, error_message, error_details,
			attributes, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
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
	)

	if err != nil {
		return fmt.Errorf("inserting span: %w", err)
	}

	return nil
}

func (s *SQLiteStore) CreateSpanBatch(ctx context.Context, spans []*Span) error {
	if len(spans) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO spans (
			id, run_id, parent_id, trace_id, kind, name, status,
			started_at, completed_at, duration_ms,
			node_data, llm_data, tool_data, edge_data,
			error_code, error_message, error_details,
			attributes, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("preparing statement: %w", err)
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

		_, err := stmt.ExecContext(ctx,
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
		)
		if err != nil {
			return fmt.Errorf("inserting span %s: %w", span.ID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}

	return nil
}

func (s *SQLiteStore) GetSpan(ctx context.Context, id string) (*Span, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT
			id, run_id, parent_id, trace_id, kind, name, status,
			started_at, completed_at, duration_ms,
			node_data, llm_data, tool_data, edge_data,
			error_code, error_message, error_details,
			attributes, created_at
		FROM spans WHERE id = ?
	`, id)

	span, err := scanSpan(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting span: %w", err)
	}

	return span, nil
}

func (s *SQLiteStore) GetSpanTree(ctx context.Context, runID string) ([]*Span, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			id, run_id, parent_id, trace_id, kind, name, status,
			started_at, completed_at, duration_ms,
			node_data, llm_data, tool_data, edge_data,
			error_code, error_message, error_details,
			attributes, created_at
		FROM spans
		WHERE run_id = ?
		ORDER BY started_at ASC
	`, runID)
	if err != nil {
		return nil, fmt.Errorf("querying spans: %w", err)
	}
	defer rows.Close()

	return scanSpans(rows)
}

func (s *SQLiteStore) GetSpansByKind(ctx context.Context, runID string, kind SpanKind) ([]*Span, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			id, run_id, parent_id, trace_id, kind, name, status,
			started_at, completed_at, duration_ms,
			node_data, llm_data, tool_data, edge_data,
			error_code, error_message, error_details,
			attributes, created_at
		FROM spans
		WHERE run_id = ? AND kind = ?
		ORDER BY started_at ASC
	`, runID, string(kind))
	if err != nil {
		return nil, fmt.Errorf("querying spans by kind: %w", err)
	}
	defer rows.Close()

	return scanSpans(rows)
}

func (s *SQLiteStore) UpdateSpan(ctx context.Context, span *Span) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE spans SET
			parent_id = ?, trace_id = ?, kind = ?, name = ?, status = ?,
			started_at = ?, completed_at = ?, duration_ms = ?,
			node_data = ?, llm_data = ?, tool_data = ?, edge_data = ?,
			error_code = ?, error_message = ?, error_details = ?,
			attributes = ?
		WHERE id = ?
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

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("span not found: %s", span.ID)
	}

	return nil
}

func (s *SQLiteStore) IndexSpanText(ctx context.Context, spanID, promptText, completionText string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO spans_fts (span_id, prompt_text, completion_text)
		VALUES (?, ?, ?)
	`, spanID, promptText, completionText)

	if err != nil {
		return fmt.Errorf("indexing span text: %w", err)
	}

	return nil
}

func (s *SQLiteStore) SearchSpans(ctx context.Context, query string, limit int) ([]*Span, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT s.id, s.run_id, s.parent_id, s.trace_id, s.kind, s.name, s.status,
			s.started_at, s.completed_at, s.duration_ms,
			s.node_data, s.llm_data, s.tool_data, s.edge_data,
			s.error_code, s.error_message, s.error_details,
			s.attributes, s.created_at
		FROM spans s
		JOIN spans_fts fts ON s.id = fts.span_id
		WHERE spans_fts MATCH ?
		ORDER BY s.started_at DESC
		LIMIT ?
	`, query, limit)
	if err != nil {
		return nil, fmt.Errorf("searching spans: %w", err)
	}
	defer rows.Close()

	return scanSpans(rows)
}

func (s *SQLiteStore) CreateDiff(ctx context.Context, diff *RunDiff) error {
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
		VALUES (?, ?, ?, ?, ?)
	`, diff.ID, diff.BaseRunID, diff.CompareRunID, string(diffData), formatTime(diff.CreatedAt))

	if err != nil {
		return fmt.Errorf("inserting diff: %w", err)
	}

	return nil
}

func (s *SQLiteStore) GetDiff(ctx context.Context, id string) (*RunDiff, error) {
	var diffData string
	row := s.db.QueryRowContext(ctx, "SELECT diff_data FROM diffs WHERE id = ?", id)
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

func (s *SQLiteStore) GetDiffByRuns(ctx context.Context, baseRunID, compareRunID string) (*RunDiff, error) {
	var diffData string
	row := s.db.QueryRowContext(ctx, `
		SELECT diff_data FROM diffs
		WHERE base_run_id = ? AND compare_run_id = ?
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

func (s *SQLiteStore) GetPricing(ctx context.Context, provider, model string) (*PricingEntry, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT provider, model, input_per_1m, output_per_1m,
			cache_read_per_1m, cache_write_per_1m, effective_from
		FROM pricing
		WHERE provider = ? AND model = ?
		ORDER BY effective_from DESC
		LIMIT 1
	`, provider, model)

	var entry PricingEntry
	var effectiveFrom string
	var cacheRead, cacheWrite sql.NullFloat64

	err := row.Scan(
		&entry.Provider,
		&entry.Model,
		&entry.InputPer1M,
		&entry.OutputPer1M,
		&cacheRead,
		&cacheWrite,
		&effectiveFrom,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting pricing: %w", err)
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

func (s *SQLiteStore) UpsertPricing(ctx context.Context, entry *PricingEntry) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT OR REPLACE INTO pricing (
			provider, model, input_per_1m, output_per_1m,
			cache_read_per_1m, cache_write_per_1m, effective_from
		) VALUES (?, ?, ?, ?, ?, ?, ?)
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

func (s *SQLiteStore) ListPricing(ctx context.Context) ([]PricingEntry, error) {
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
		var entry PricingEntry
		var effectiveFrom string
		var cacheRead, cacheWrite sql.NullFloat64

		if err := rows.Scan(
			&entry.Provider,
			&entry.Model,
			&entry.InputPer1M,
			&entry.OutputPer1M,
			&cacheRead,
			&cacheWrite,
			&effectiveFrom,
		); err != nil {
			return nil, fmt.Errorf("scanning pricing: %w", err)
		}

		if cacheRead.Valid {
			entry.CacheReadPer1M = cacheRead.Float64
		}
		if cacheWrite.Valid {
			entry.CacheWritePer1M = cacheWrite.Float64
		}
		entry.EffectiveFrom, _ = parseTime(effectiveFrom)
		entries = append(entries, entry)
	}

	return entries, nil
}

func scanSpan(row *sql.Row) (*Span, error) {
	var span Span
	var parentID, nodeData, llmData, toolData, edgeData sql.NullString
	var startedAt, completedAt, createdAt sql.NullString
	var errorCode, errorMessage, errorDetails sql.NullString
	var attributes sql.NullString

	err := row.Scan(
		&span.ID,
		&span.RunID,
		&parentID,
		&span.TraceID,
		&span.Kind,
		&span.Name,
		&span.Status,
		&startedAt,
		&completedAt,
		&span.DurationMs,
		&nodeData,
		&llmData,
		&toolData,
		&edgeData,
		&errorCode,
		&errorMessage,
		&errorDetails,
		&attributes,
		&createdAt,
	)
	if err != nil {
		return nil, err
	}

	span.ParentID = nullStringPtr(parentID)

	if startedAt.Valid {
		t, _ := parseTime(startedAt.String)
		span.StartedAt = t
	}
	span.CompletedAt = parseTimePtr(completedAt)
	if createdAt.Valid {
		t, _ := parseTime(createdAt.String)
		span.CreatedAt = t
	}

	if nodeData.Valid && nodeData.String != "" {
		var data NodeSpanData
		if err := json.Unmarshal([]byte(nodeData.String), &data); err == nil {
			span.Node = &data
		}
	}
	if llmData.Valid && llmData.String != "" {
		var data LLMSpanData
		if err := json.Unmarshal([]byte(llmData.String), &data); err == nil {
			span.LLM = &data
		}
	}
	if toolData.Valid && toolData.String != "" {
		var data ToolSpanData
		if err := json.Unmarshal([]byte(toolData.String), &data); err == nil {
			span.Tool = &data
		}
	}
	if edgeData.Valid && edgeData.String != "" {
		var data EdgeSpanData
		if err := json.Unmarshal([]byte(edgeData.String), &data); err == nil {
			span.Edge = &data
		}
	}

	if errorMessage.Valid {
		span.Error = &SpanError{
			Code:    errorCode.String,
			Message: errorMessage.String,
			Details: errorDetails.String,
		}
	}

	if attributes.Valid && attributes.String != "" {
		var attrs map[string]any
		if err := json.Unmarshal([]byte(attributes.String), &attrs); err == nil {
			span.Attributes = attrs
		}
	}

	return &span, nil
}

func scanSpans(rows *sql.Rows) ([]*Span, error) {
	var spans []*Span
	for rows.Next() {
		var span Span
		var parentID, nodeData, llmData, toolData, edgeData sql.NullString
		var startedAt, completedAt, createdAt sql.NullString
		var errorCode, errorMessage, errorDetails sql.NullString
		var attributes sql.NullString

		err := rows.Scan(
			&span.ID,
			&span.RunID,
			&parentID,
			&span.TraceID,
			&span.Kind,
			&span.Name,
			&span.Status,
			&startedAt,
			&completedAt,
			&span.DurationMs,
			&nodeData,
			&llmData,
			&toolData,
			&edgeData,
			&errorCode,
			&errorMessage,
			&errorDetails,
			&attributes,
			&createdAt,
		)
		if err != nil {
			return nil, err
		}

		span.ParentID = nullStringPtr(parentID)

		if startedAt.Valid {
			t, _ := parseTime(startedAt.String)
			span.StartedAt = t
		}
		span.CompletedAt = parseTimePtr(completedAt)
		if createdAt.Valid {
			t, _ := parseTime(createdAt.String)
			span.CreatedAt = t
		}

		if nodeData.Valid && nodeData.String != "" {
			var data NodeSpanData
			if err := json.Unmarshal([]byte(nodeData.String), &data); err == nil {
				span.Node = &data
			}
		}
		if llmData.Valid && llmData.String != "" {
			var data LLMSpanData
			if err := json.Unmarshal([]byte(llmData.String), &data); err == nil {
				span.LLM = &data
			}
		}
		if toolData.Valid && toolData.String != "" {
			var data ToolSpanData
			if err := json.Unmarshal([]byte(toolData.String), &data); err == nil {
				span.Tool = &data
			}
		}
		if edgeData.Valid && edgeData.String != "" {
			var data EdgeSpanData
			if err := json.Unmarshal([]byte(edgeData.String), &data); err == nil {
				span.Edge = &data
			}
		}

		if errorMessage.Valid {
			span.Error = &SpanError{
				Code:    errorCode.String,
				Message: errorMessage.String,
				Details: errorDetails.String,
			}
		}

		if attributes.Valid && attributes.String != "" {
			var attrs map[string]any
			if err := json.Unmarshal([]byte(attributes.String), &attrs); err == nil {
				span.Attributes = attrs
			}
		}

		spans = append(spans, &span)
	}

	return spans, nil
}

func toJSONNullable(v any) sql.NullString {
	if v == nil {
		return sql.NullString{}
	}
	b, err := json.Marshal(v)
	if err != nil || string(b) == "null" {
		return sql.NullString{}
	}
	return sql.NullString{String: string(b), Valid: true}
}

func errorCode(e *SpanError) sql.NullString {
	if e == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: e.Code, Valid: e.Code != ""}
}

func errorMessage(e *SpanError) sql.NullString {
	if e == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: e.Message, Valid: e.Message != ""}
}

func errorDetails(e *SpanError) sql.NullString {
	if e == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: e.Details, Valid: e.Details != ""}
}
