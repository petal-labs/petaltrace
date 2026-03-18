package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Server.Port != 8090 {
		t.Errorf("Server.Port = %d, want 8090", cfg.Server.Port)
	}

	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("Server.Host = %s, want 0.0.0.0", cfg.Server.Host)
	}

	if cfg.Collector.OTLPGRPCPort != 4317 {
		t.Errorf("Collector.OTLPGRPCPort = %d, want 4317", cfg.Collector.OTLPGRPCPort)
	}

	if cfg.Collector.OTLPHTTPPort != 4318 {
		t.Errorf("Collector.OTLPHTTPPort = %d, want 4318", cfg.Collector.OTLPHTTPPort)
	}

	if !cfg.Collector.Enrichment.ComputeCost {
		t.Error("Collector.Enrichment.ComputeCost = false, want true")
	}

	if !cfg.Collector.Enrichment.ExtractText {
		t.Error("Collector.Enrichment.ExtractText = false, want true")
	}

	if cfg.Collector.Enrichment.MaxContentSize != 1048576 {
		t.Errorf("Collector.Enrichment.MaxContentSize = %d, want 1048576", cfg.Collector.Enrichment.MaxContentSize)
	}

	if cfg.Storage.Driver != "sqlite" {
		t.Errorf("Storage.Driver = %s, want sqlite", cfg.Storage.Driver)
	}

	if !cfg.Storage.SQLite.WALMode {
		t.Error("Storage.SQLite.WALMode = false, want true")
	}

	if cfg.Retention.Default.Duration != 30*24*time.Hour {
		t.Errorf("Retention.Default = %v, want 30d", cfg.Retention.Default.Duration)
	}

	if cfg.Retention.FailedRuns.Duration != 90*24*time.Hour {
		t.Errorf("Retention.FailedRuns = %v, want 90d", cfg.Retention.FailedRuns.Duration)
	}

	if !cfg.Pricing.Builtin {
		t.Error("Pricing.Builtin = false, want true")
	}

	if !cfg.MCP.Enabled {
		t.Error("MCP.Enabled = false, want true")
	}

	if cfg.MCP.Transport != "stdio" {
		t.Errorf("MCP.Transport = %s, want stdio", cfg.MCP.Transport)
	}

	if cfg.Auth.Enabled {
		t.Error("Auth.Enabled = true, want false")
	}
}

func TestLoadFromFile(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "petaltrace.yaml")

	cfgContent := `
server:
  port: 9090
  host: "127.0.0.1"
collector:
  otlp_grpc_port: 5317
  otlp_http_port: 5318
storage:
  driver: sqlite
  sqlite:
    path: "/tmp/test.db"
    wal_mode: false
retention:
  default: 7d
  failed_runs: 14d
mcp:
  enabled: false
  transport: http
`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0644); err != nil {
		t.Fatalf("writing config file: %v", err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Server.Port != 9090 {
		t.Errorf("Server.Port = %d, want 9090", cfg.Server.Port)
	}

	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("Server.Host = %s, want 127.0.0.1", cfg.Server.Host)
	}

	if cfg.Collector.OTLPGRPCPort != 5317 {
		t.Errorf("Collector.OTLPGRPCPort = %d, want 5317", cfg.Collector.OTLPGRPCPort)
	}

	if cfg.Storage.SQLite.Path != "/tmp/test.db" {
		t.Errorf("Storage.SQLite.Path = %s, want /tmp/test.db", cfg.Storage.SQLite.Path)
	}

	if cfg.Storage.SQLite.WALMode {
		t.Error("Storage.SQLite.WALMode = true, want false")
	}

	if cfg.Retention.Default.Duration != 7*24*time.Hour {
		t.Errorf("Retention.Default = %v, want 7d", cfg.Retention.Default.Duration)
	}

	if cfg.MCP.Enabled {
		t.Error("MCP.Enabled = true, want false")
	}

	if cfg.MCP.Transport != "http" {
		t.Errorf("MCP.Transport = %s, want http", cfg.MCP.Transport)
	}
}

func TestLoadFromEnv(t *testing.T) {
	os.Setenv("PETALTRACE_SERVER_PORT", "7070")
	os.Setenv("PETALTRACE_COLLECTOR_OTLP_GRPC_PORT", "6317")
	defer func() {
		os.Unsetenv("PETALTRACE_SERVER_PORT")
		os.Unsetenv("PETALTRACE_COLLECTOR_OTLP_GRPC_PORT")
	}()

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Server.Port != 7070 {
		t.Errorf("Server.Port = %d, want 7070", cfg.Server.Port)
	}

	if cfg.Collector.OTLPGRPCPort != 6317 {
		t.Errorf("Collector.OTLPGRPCPort = %d, want 6317", cfg.Collector.OTLPGRPCPort)
	}
}

func TestValidation(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*Config)
		wantErr bool
	}{
		{
			name:    "valid defaults",
			modify:  func(c *Config) {},
			wantErr: false,
		},
		{
			name: "invalid server port - too low",
			modify: func(c *Config) {
				c.Server.Port = 0
			},
			wantErr: true,
		},
		{
			name: "invalid server port - too high",
			modify: func(c *Config) {
				c.Server.Port = 70000
			},
			wantErr: true,
		},
		{
			name: "invalid storage driver",
			modify: func(c *Config) {
				c.Storage.Driver = "mysql"
			},
			wantErr: true,
		},
		{
			name: "missing sqlite path",
			modify: func(c *Config) {
				c.Storage.Driver = "sqlite"
				c.Storage.SQLite.Path = ""
			},
			wantErr: true,
		},
		{
			name: "missing postgres dsn",
			modify: func(c *Config) {
				c.Storage.Driver = "postgres"
				c.Storage.Postgres.DSN = ""
			},
			wantErr: true,
		},
		{
			name: "invalid mcp transport",
			modify: func(c *Config) {
				c.MCP.Transport = "websocket"
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := defaultConfig()
			tt.modify(cfg)
			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDurationUnmarshal(t *testing.T) {
	tests := []struct {
		input   string
		want    time.Duration
		wantErr bool
	}{
		{"1d", 24 * time.Hour, false},
		{"7d", 7 * 24 * time.Hour, false},
		{"30d", 30 * 24 * time.Hour, false},
		{"90d", 90 * 24 * time.Hour, false},
		{"1h", time.Hour, false},
		{"30m", 30 * time.Minute, false},
		{"never", 0, false},
		{"invalid", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			var d Duration
			err := d.UnmarshalText([]byte(tt.input))
			if (err != nil) != tt.wantErr {
				t.Errorf("UnmarshalText() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && d.Duration != tt.want {
				t.Errorf("Duration = %v, want %v", d.Duration, tt.want)
			}
		})
	}
}

func TestExpandPath(t *testing.T) {
	home, _ := os.UserHomeDir()

	tests := []struct {
		input string
		want  string
	}{
		{"~/foo/bar", filepath.Join(home, "foo/bar")},
		{"/absolute/path", "/absolute/path"},
		{"relative/path", "relative/path"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := expandPath(tt.input)
			if got != tt.want {
				t.Errorf("expandPath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func defaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Port: 8090,
			Host: "0.0.0.0",
		},
		Collector: CollectorConfig{
			OTLPGRPCPort: 4317,
			OTLPHTTPPort: 4318,
			Enrichment: EnrichmentConfig{
				ComputeCost:    true,
				ExtractText:    true,
				MaxContentSize: 1048576,
			},
		},
		Storage: StorageConfig{
			Driver: "sqlite",
			SQLite: SQLiteConfig{
				Path:    "/tmp/test.db",
				WALMode: true,
			},
		},
		Retention: RetentionConfig{
			Default:    Duration{30 * 24 * time.Hour},
			FailedRuns: Duration{90 * 24 * time.Hour},
		},
		Pricing: PricingConfig{
			Builtin: true,
		},
		MCP: MCPConfig{
			Enabled:   true,
			Transport: "stdio",
		},
		Auth: AuthConfig{
			Enabled: false,
		},
	}
}
