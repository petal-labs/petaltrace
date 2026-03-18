package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"
)

type Config struct {
	Server    ServerConfig    `mapstructure:"server"`
	Collector CollectorConfig `mapstructure:"collector"`
	Storage   StorageConfig   `mapstructure:"storage"`
	Retention RetentionConfig `mapstructure:"retention"`
	Pricing   PricingConfig   `mapstructure:"pricing"`
	MCP       MCPConfig       `mapstructure:"mcp"`
	Auth      AuthConfig      `mapstructure:"auth"`
}

type ServerConfig struct {
	Port int    `mapstructure:"port"`
	Host string `mapstructure:"host"`
}

type CollectorConfig struct {
	OTLPGRPCPort int              `mapstructure:"otlp_grpc_port"`
	OTLPHTTPPort int              `mapstructure:"otlp_http_port"`
	Enrichment   EnrichmentConfig `mapstructure:"enrichment"`
}

type EnrichmentConfig struct {
	ComputeCost    bool `mapstructure:"compute_cost"`
	ExtractText    bool `mapstructure:"extract_text"`
	MaxContentSize int  `mapstructure:"max_content_size"`
}

type StorageConfig struct {
	Driver   string         `mapstructure:"driver"`
	SQLite   SQLiteConfig   `mapstructure:"sqlite"`
	Postgres PostgresConfig `mapstructure:"postgres"`
}

type SQLiteConfig struct {
	Path    string `mapstructure:"path"`
	WALMode bool   `mapstructure:"wal_mode"`
}

type PostgresConfig struct {
	DSN string `mapstructure:"dsn"`
}

type RetentionConfig struct {
	Default     Duration `mapstructure:"default"`
	FailedRuns  Duration `mapstructure:"failed_runs"`
	StarredRuns string   `mapstructure:"starred_runs"`
}

type PricingConfig struct {
	Builtin   bool   `mapstructure:"builtin"`
	Overrides string `mapstructure:"overrides"`
}

type MCPConfig struct {
	Enabled   bool   `mapstructure:"enabled"`
	Transport string `mapstructure:"transport"`
}

type AuthConfig struct {
	Enabled bool `mapstructure:"enabled"`
}

type Duration struct {
	time.Duration
}

func (d *Duration) UnmarshalText(text []byte) error {
	s := string(text)
	if s == "never" {
		d.Duration = 0
		return nil
	}

	if strings.HasSuffix(s, "d") {
		days := strings.TrimSuffix(s, "d")
		var n int
		if _, err := fmt.Sscanf(days, "%d", &n); err != nil {
			return fmt.Errorf("invalid duration: %s", s)
		}
		d.Duration = time.Duration(n) * 24 * time.Hour
		return nil
	}

	dur, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	d.Duration = dur
	return nil
}

func (d Duration) MarshalText() ([]byte, error) {
	if d.Duration == 0 {
		return []byte("never"), nil
	}
	days := int(d.Duration.Hours() / 24)
	if days > 0 && d.Duration == time.Duration(days)*24*time.Hour {
		return []byte(fmt.Sprintf("%dd", days)), nil
	}
	return []byte(d.Duration.String()), nil
}

func Load(cfgFile string) (*Config, error) {
	v := viper.New()

	setDefaults(v)

	v.SetEnvPrefix("PETALTRACE")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if cfgFile != "" {
		v.SetConfigFile(cfgFile)
	} else {
		v.SetConfigName("petaltrace")
		v.SetConfigType("yaml")
		v.AddConfigPath(".")
		v.AddConfigPath("$HOME/.petaltrace")
		v.AddConfigPath("/etc/petaltrace")
	}

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("reading config file: %w", err)
		}
	}

	var cfg Config
	decodeHook := mapstructure.ComposeDecodeHookFunc(
		mapstructure.StringToTimeDurationHookFunc(),
		stringToDurationHookFunc(),
	)
	if err := v.Unmarshal(&cfg, viper.DecodeHook(decodeHook)); err != nil {
		return nil, fmt.Errorf("unmarshaling config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	cfg.expandPaths()

	return &cfg, nil
}

func stringToDurationHookFunc() mapstructure.DecodeHookFunc {
	return func(f reflect.Type, t reflect.Type, data interface{}) (interface{}, error) {
		if f.Kind() != reflect.String {
			return data, nil
		}

		if t != reflect.TypeOf(Duration{}) {
			return data, nil
		}

		var d Duration
		if err := d.UnmarshalText([]byte(data.(string))); err != nil {
			return nil, err
		}
		return d, nil
	}
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("server.port", 8090)
	v.SetDefault("server.host", "0.0.0.0")

	v.SetDefault("collector.otlp_grpc_port", 4317)
	v.SetDefault("collector.otlp_http_port", 4318)
	v.SetDefault("collector.enrichment.compute_cost", true)
	v.SetDefault("collector.enrichment.extract_text", true)
	v.SetDefault("collector.enrichment.max_content_size", 1048576)

	v.SetDefault("storage.driver", "sqlite")
	v.SetDefault("storage.sqlite.path", "~/.petaltrace/data.db")
	v.SetDefault("storage.sqlite.wal_mode", true)

	v.SetDefault("retention.default", "30d")
	v.SetDefault("retention.failed_runs", "90d")
	v.SetDefault("retention.starred_runs", "never")

	v.SetDefault("pricing.builtin", true)
	v.SetDefault("pricing.overrides", "")

	v.SetDefault("mcp.enabled", true)
	v.SetDefault("mcp.transport", "stdio")

	v.SetDefault("auth.enabled", false)
}

func (c *Config) Validate() error {
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("server.port must be between 1 and 65535")
	}

	if c.Collector.OTLPGRPCPort < 1 || c.Collector.OTLPGRPCPort > 65535 {
		return fmt.Errorf("collector.otlp_grpc_port must be between 1 and 65535")
	}

	if c.Collector.OTLPHTTPPort < 1 || c.Collector.OTLPHTTPPort > 65535 {
		return fmt.Errorf("collector.otlp_http_port must be between 1 and 65535")
	}

	if c.Storage.Driver != "sqlite" && c.Storage.Driver != "postgres" {
		return fmt.Errorf("storage.driver must be 'sqlite' or 'postgres'")
	}

	if c.Storage.Driver == "sqlite" && c.Storage.SQLite.Path == "" {
		return fmt.Errorf("storage.sqlite.path is required when driver is sqlite")
	}

	if c.Storage.Driver == "postgres" && c.Storage.Postgres.DSN == "" {
		return fmt.Errorf("storage.postgres.dsn is required when driver is postgres")
	}

	if c.MCP.Transport != "stdio" && c.MCP.Transport != "http" {
		return fmt.Errorf("mcp.transport must be 'stdio' or 'http'")
	}

	return nil
}

func (c *Config) expandPaths() {
	c.Storage.SQLite.Path = expandPath(c.Storage.SQLite.Path)
	if c.Pricing.Overrides != "" {
		c.Pricing.Overrides = expandPath(c.Pricing.Overrides)
	}
}

func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}

func (c *Config) DataDir() string {
	return filepath.Dir(c.Storage.SQLite.Path)
}
