package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/oklog/ulid/v2"
)

type SQLiteStore struct {
	db      *sql.DB
	path    string
	walMode bool
}

type SQLiteOptions struct {
	Path    string
	WALMode bool
}

func NewSQLiteStore(opts SQLiteOptions) (*SQLiteStore, error) {
	dir := filepath.Dir(opts.Path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("creating data directory: %w", err)
	}

	dsn := opts.Path + "?_foreign_keys=on"
	if opts.WALMode {
		dsn += "&_journal_mode=WAL"
	}

	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	db.SetMaxOpenConns(1)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("pinging database: %w", err)
	}

	if err := RunMigrations(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("running migrations: %w", err)
	}

	return &SQLiteStore{
		db:      db,
		path:    opts.Path,
		walMode: opts.WALMode,
	}, nil
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

func (s *SQLiteStore) DB() *sql.DB {
	return s.db
}

func NewID() string {
	return strings.ToLower(ulid.Make().String())
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func parseTime(s string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, s)
}

func parseTimePtr(s sql.NullString) *time.Time {
	if !s.Valid {
		return nil
	}
	t, err := parseTime(s.String)
	if err != nil {
		return nil
	}
	return &t
}

func nullString(s *string) sql.NullString {
	if s == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *s, Valid: true}
}

func nullStringPtr(ns sql.NullString) *string {
	if !ns.Valid {
		return nil
	}
	return &ns.String
}

func toJSON(v any) string {
	if v == nil {
		return ""
	}
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

func fromJSON[T any](s string) T {
	var v T
	if s != "" {
		json.Unmarshal([]byte(s), &v)
	}
	return v
}

func (s *SQLiteStore) GetStats(ctx context.Context) (*StoreStats, error) {
	stats := &StoreStats{}

	fi, err := os.Stat(s.path)
	if err == nil {
		stats.DatabaseSize = fi.Size()
	}

	row := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM runs")
	if err := row.Scan(&stats.RunCount); err != nil {
		return nil, fmt.Errorf("counting runs: %w", err)
	}

	row = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM spans")
	if err := row.Scan(&stats.SpanCount); err != nil {
		return nil, fmt.Errorf("counting spans: %w", err)
	}

	row = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM diffs")
	if err := row.Scan(&stats.DiffCount); err != nil {
		return nil, fmt.Errorf("counting diffs: %w", err)
	}

	var oldest, newest sql.NullString
	row = s.db.QueryRowContext(ctx, "SELECT MIN(started_at), MAX(started_at) FROM runs")
	if err := row.Scan(&oldest, &newest); err != nil {
		return nil, fmt.Errorf("getting run date range: %w", err)
	}
	if t := parseTimePtr(oldest); t != nil {
		stats.OldestRun = *t
	}
	if t := parseTimePtr(newest); t != nil {
		stats.NewestRun = *t
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT workflow_name, COUNT(*) as count
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
		var ws WorkflowStats
		if err := rows.Scan(&ws.WorkflowName, &ws.RunCount); err != nil {
			return nil, fmt.Errorf("scanning workflow stats: %w", err)
		}
		stats.TopWorkflows = append(stats.TopWorkflows, ws)
	}

	return stats, nil
}

func (s *SQLiteStore) GarbageCollect(ctx context.Context, retentionDays int, dryRun bool) (int, error) {
	cutoff := time.Now().AddDate(0, 0, -retentionDays).UTC()
	cutoffStr := formatTime(cutoff)

	var count int
	row := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM runs
		WHERE started_at < ? AND starred = 0
	`, cutoffStr)
	if err := row.Scan(&count); err != nil {
		return 0, fmt.Errorf("counting runs to delete: %w", err)
	}

	if dryRun {
		return count, nil
	}

	result, err := s.db.ExecContext(ctx, `
		DELETE FROM runs
		WHERE started_at < ? AND starred = 0
	`, cutoffStr)
	if err != nil {
		return 0, fmt.Errorf("deleting old runs: %w", err)
	}

	deleted, _ := result.RowsAffected()

	if _, err := s.db.ExecContext(ctx, "VACUUM"); err != nil {
		return int(deleted), fmt.Errorf("vacuuming database: %w", err)
	}

	return int(deleted), nil
}
