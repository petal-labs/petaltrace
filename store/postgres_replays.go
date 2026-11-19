package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

func (s *PostgresStore) CreateReplay(ctx context.Context, replay *ReplayRecord) error {
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
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
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

func (s *PostgresStore) GetReplay(ctx context.Context, id string) (*ReplayRecord, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, source_run_id, new_run_id, diff_id, mode, status, error_message,
			started_at, completed_at
		FROM replays WHERE id = $1
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

func (s *PostgresStore) UpdateReplay(ctx context.Context, replay *ReplayRecord) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE replays SET
			source_run_id = $1, new_run_id = $2, diff_id = $3, mode = $4, status = $5,
			error_message = $6, started_at = $7, completed_at = $8
		WHERE id = $9
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

func (s *PostgresStore) ListReplays(ctx context.Context, sourceRunID string) ([]ReplayRecord, error) {
	query := `
		SELECT id, source_run_id, new_run_id, diff_id, mode, status, error_message,
			started_at, completed_at
		FROM replays
	`
	var rows *sql.Rows
	var err error
	if sourceRunID != "" {
		rows, err = s.db.QueryContext(ctx, query+` WHERE source_run_id = $1 ORDER BY started_at DESC, id DESC`, sourceRunID)
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
