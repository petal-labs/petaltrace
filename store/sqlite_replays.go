package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

func (s *SQLiteStore) CreateReplay(ctx context.Context, replay *ReplayRecord) error {
	if replay.ID == "" {
		replay.ID = NewID()
	}
	if replay.StartedAt.IsZero() {
		replay.StartedAt = time.Now().UTC()
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO replays (
			id, source_run_id, new_run_id, diff_id, mode, status, error_message,
			started_at, completed_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		replay.ID,
		replay.SourceRunID,
		nullableText(replay.NewRunID),
		nullableText(replay.DiffID),
		replay.Mode,
		replay.Status,
		nullableText(replay.Error),
		formatTime(replay.StartedAt),
		formatTimePtr(replay.CompletedAt),
	)
	if err != nil {
		return fmt.Errorf("inserting replay: %w", err)
	}
	return nil
}

func (s *SQLiteStore) GetReplay(ctx context.Context, id string) (*ReplayRecord, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, source_run_id, new_run_id, diff_id, mode, status, error_message,
			started_at, completed_at
		FROM replays WHERE id = ?
	`, id)
	replay, err := scanReplay(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting replay: %w", err)
	}
	return replay, nil
}

func (s *SQLiteStore) UpdateReplay(ctx context.Context, replay *ReplayRecord) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE replays SET
			source_run_id = ?, new_run_id = ?, diff_id = ?, mode = ?, status = ?,
			error_message = ?, started_at = ?, completed_at = ?
		WHERE id = ?
	`,
		replay.SourceRunID,
		nullableText(replay.NewRunID),
		nullableText(replay.DiffID),
		replay.Mode,
		replay.Status,
		nullableText(replay.Error),
		formatTime(replay.StartedAt),
		formatTimePtr(replay.CompletedAt),
		replay.ID,
	)
	if err != nil {
		return fmt.Errorf("updating replay: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking updated replay: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("replay not found: %s", replay.ID)
	}
	return nil
}

func (s *SQLiteStore) ListReplays(ctx context.Context, sourceRunID string) ([]ReplayRecord, error) {
	query := `
		SELECT id, source_run_id, new_run_id, diff_id, mode, status, error_message,
			started_at, completed_at
		FROM replays
	`
	var rows *sql.Rows
	var err error
	if sourceRunID != "" {
		rows, err = s.db.QueryContext(ctx, query+` WHERE source_run_id = ? ORDER BY started_at DESC, id DESC`, sourceRunID)
	} else {
		rows, err = s.db.QueryContext(ctx, query+` ORDER BY started_at DESC, id DESC`)
	}
	if err != nil {
		return nil, fmt.Errorf("listing replays: %w", err)
	}
	defer rows.Close()

	var replays []ReplayRecord
	for rows.Next() {
		replay, err := scanReplayFromRows(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning replay: %w", err)
		}
		replays = append(replays, *replay)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading replays: %w", err)
	}
	return replays, nil
}

func scanReplay(row *sql.Row) (*ReplayRecord, error) {
	var replay ReplayRecord
	var newRunID, diffID, replayError sql.NullString
	var startedAt, completedAt sql.NullString
	if err := row.Scan(
		&replay.ID,
		&replay.SourceRunID,
		&newRunID,
		&diffID,
		&replay.Mode,
		&replay.Status,
		&replayError,
		&startedAt,
		&completedAt,
	); err != nil {
		return nil, err
	}
	replay.NewRunID = nullStringValue(newRunID)
	replay.DiffID = nullStringValue(diffID)
	replay.Error = nullStringValue(replayError)
	if startedAt.Valid {
		replay.StartedAt, _ = parseTime(startedAt.String)
	}
	replay.CompletedAt = parseTimePtr(completedAt)
	return &replay, nil
}

func scanReplayFromRows(rows *sql.Rows) (*ReplayRecord, error) {
	var replay ReplayRecord
	var newRunID, diffID, replayError sql.NullString
	var startedAt, completedAt sql.NullString
	if err := rows.Scan(
		&replay.ID,
		&replay.SourceRunID,
		&newRunID,
		&diffID,
		&replay.Mode,
		&replay.Status,
		&replayError,
		&startedAt,
		&completedAt,
	); err != nil {
		return nil, err
	}
	replay.NewRunID = nullStringValue(newRunID)
	replay.DiffID = nullStringValue(diffID)
	replay.Error = nullStringValue(replayError)
	if startedAt.Valid {
		replay.StartedAt, _ = parseTime(startedAt.String)
	}
	replay.CompletedAt = parseTimePtr(completedAt)
	return &replay, nil
}

func nullableText(value string) sql.NullString {
	return sql.NullString{String: value, Valid: value != ""}
}

func nullStringValue(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}
