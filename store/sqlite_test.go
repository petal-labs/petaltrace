package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewSQLiteStore(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewSQLiteStore(SQLiteOptions{
		Path:    dbPath,
		WALMode: true,
	})
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("database file was not created")
	}

	version, err := GetSchemaVersion(store.DB())
	if err != nil {
		t.Fatalf("GetSchemaVersion() error = %v", err)
	}
	if version != 1 {
		t.Errorf("schema version = %d, want 1", version)
	}
}

func TestMigrationsIdempotent(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store1, err := NewSQLiteStore(SQLiteOptions{Path: dbPath})
	if err != nil {
		t.Fatalf("first NewSQLiteStore() error = %v", err)
	}
	store1.Close()

	store2, err := NewSQLiteStore(SQLiteOptions{Path: dbPath})
	if err != nil {
		t.Fatalf("second NewSQLiteStore() error = %v", err)
	}
	defer store2.Close()

	version, err := GetSchemaVersion(store2.DB())
	if err != nil {
		t.Fatalf("GetSchemaVersion() error = %v", err)
	}
	if version != 1 {
		t.Errorf("schema version = %d, want 1", version)
	}
}

func TestGetStats(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewSQLiteStore(SQLiteOptions{Path: dbPath})
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	stats, err := store.GetStats(ctx)
	if err != nil {
		t.Fatalf("GetStats() error = %v", err)
	}

	if stats.RunCount != 0 {
		t.Errorf("RunCount = %d, want 0", stats.RunCount)
	}

	if stats.SpanCount != 0 {
		t.Errorf("SpanCount = %d, want 0", stats.SpanCount)
	}

	if stats.DatabaseSize <= 0 {
		t.Errorf("DatabaseSize = %d, want > 0", stats.DatabaseSize)
	}
}

func TestGarbageCollectDryRun(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewSQLiteStore(SQLiteOptions{Path: dbPath})
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	count, err := store.GarbageCollect(ctx, 30, true)
	if err != nil {
		t.Fatalf("GarbageCollect() error = %v", err)
	}

	if count != 0 {
		t.Errorf("count = %d, want 0", count)
	}
}

func TestNewID(t *testing.T) {
	id1 := NewID()
	id2 := NewID()

	if id1 == "" {
		t.Error("NewID() returned empty string")
	}

	if id1 == id2 {
		t.Error("NewID() returned duplicate IDs")
	}

	if len(id1) != 26 {
		t.Errorf("NewID() length = %d, want 26", len(id1))
	}
}

func TestTimeHelpers(t *testing.T) {
	now := testTime()

	formatted := formatTime(now)
	parsed, err := parseTime(formatted)
	if err != nil {
		t.Fatalf("parseTime() error = %v", err)
	}

	if !parsed.Equal(now) {
		t.Errorf("parsed time = %v, want %v", parsed, now)
	}
}

func testTime() time.Time {
	return time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
}

func testStore(t *testing.T) *SQLiteStore {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewSQLiteStore(SQLiteOptions{
		Path:    dbPath,
		WALMode: false,
	})
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	return store
}
