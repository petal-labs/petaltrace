package store

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestRunMigrationsCopiesLegacyFTS4SearchIndex(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "legacy.db"))
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(migrations[0].Up); err != nil {
		t.Fatalf("apply initial schema: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO schema_migrations (version, name) VALUES (1, 'initial_schema')`); err != nil {
		t.Fatalf("record initial schema: %v", err)
	}
	if _, err := db.Exec(`
		CREATE TABLE spans_fts_content (
			docid INTEGER PRIMARY KEY,
			c0span_id,
			c1prompt_text,
			c2completion_text
		)
	`); err != nil {
		t.Fatalf("create legacy FTS4 content table: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO spans_fts_content (c0span_id, c1prompt_text, c2completion_text)
		VALUES ('span-1', 'quantum prompt', 'machine learning completion')
	`); err != nil {
		t.Fatalf("insert legacy FTS4 content: %v", err)
	}

	if err := RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations() error = %v", err)
	}

	var spanID string
	if err := db.QueryRow(`
		SELECT span_id FROM spans_fts5
		WHERE spans_fts5 MATCH ?
	`, "quantum").Scan(&spanID); err != nil {
		t.Fatalf("query migrated FTS5 index: %v", err)
	}
	if spanID != "span-1" {
		t.Errorf("migrated span ID = %q, want %q", spanID, "span-1")
	}
}
