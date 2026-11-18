package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestNewStoreSelectsSQLite(t *testing.T) {
	traceStore, err := NewStore(context.Background(), StoreOptions{
		Driver: "sqlite",
		SQLite: SQLiteOptions{Path: filepath.Join(t.TempDir(), "petaltrace.db")},
	})
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	defer traceStore.Close()

	if _, ok := traceStore.(*SQLiteStore); !ok {
		t.Fatalf("NewStore() type = %T, want *SQLiteStore", traceStore)
	}
}

func TestNewStoreRejectsUnsupportedDriver(t *testing.T) {
	if _, err := NewStore(context.Background(), StoreOptions{Driver: "mysql"}); err == nil {
		t.Fatal("NewStore() error = nil, want unsupported driver error")
	}
}
