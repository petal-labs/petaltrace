package petaltrace

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestOpenStoreDoesNotFallBackToSQLiteForPostgresConfig(t *testing.T) {
	tempDir := t.TempDir()
	sqlitePath := filepath.Join(tempDir, "unexpected.db")
	configPath := filepath.Join(tempDir, "petaltrace.yaml")
	config := []byte("storage:\n  driver: postgres\n  postgres:\n    dsn: not-a-postgres-dsn\n  sqlite:\n    path: " + strconv.Quote(sqlitePath) + "\n")
	if err := os.WriteFile(configPath, config, 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	previousConfig := cfgFile
	t.Cleanup(func() { cfgFile = previousConfig })
	cfgFile = configPath

	traceStore, err := openStore()
	if traceStore != nil {
		_ = traceStore.Close()
		t.Fatal("openStore() returned a store for an invalid PostgreSQL DSN")
	}
	if err == nil {
		t.Fatal("openStore() error = nil, want PostgreSQL connection error")
	}

	if _, err := os.Stat(sqlitePath); !os.IsNotExist(err) {
		t.Fatalf("SQLite fallback database exists, stat error = %v", err)
	}
}
