# Configuration

PetalTrace is configured via a YAML file and environment variables.

## Configuration File

By default, PetalTrace looks for `petaltrace.yaml` in the current directory. Override with `--config`:

```bash
petaltrace serve --config /path/to/petaltrace.yaml
```

### Complete Configuration Reference

```yaml
# Server configuration
server:
  # HTTP API server address
  address: ":8090"      # default: ":8090"
  host: "0.0.0.0"       # default: "0.0.0.0"
  port: 8090            # default: 8090

# OTLP collector configuration
collector:
  # OTLP/HTTP receiver port
  otlp_http_port: 4318  # default: 4318

  # OTLP/gRPC receiver port
  otlp_grpc_port: 4317  # default: 4317

  # Batch processing settings
  batch_size: 100       # default: 100
  flush_interval: "1s"  # default: "1s"

  # Enrichment settings
  enrichment:
    compute_cost: true        # Compute cost estimates for LLM spans
    extract_text: true        # Index text for FTS search
    max_content_size: 1048576 # Max bytes per prompt/completion (1MB)

# Storage configuration
storage:
  # Database file path
  path: "~/.petaltrace/data.db"  # default: "~/.petaltrace/data.db"

  # Enable WAL mode for better concurrent performance
  wal_mode: true                  # default: true

# Data retention configuration
retention:
  # Default retention period for runs
  default: "30d"         # default: "30d"

  # Extended retention for failed runs
  failed_runs: "90d"     # default: "90d"

  # Maximum retention period
  max: "365d"            # default: "365d"

  # Starred runs exempt from GC
  starred_runs: "never"  # default: "never"

# Pricing configuration
pricing:
  # Use built-in pricing table
  builtin: true          # default: true

  # Path to pricing overrides file
  overrides_path: ""     # default: "" (no overrides)

# MCP server configuration
mcp:
  # Enable MCP server
  enabled: true          # default: true

  # Transport mode (only stdio supported)
  transport: "stdio"     # default: "stdio"

# Authentication (future)
auth:
  enabled: false         # default: false
```

## Environment Variables

All configuration options can be set via environment variables using the `PETALTRACE_` prefix:

| Variable | Description | Default |
|----------|-------------|---------|
| `PETALTRACE_SERVER_ADDRESS` | HTTP API address | `:8090` |
| `PETALTRACE_SERVER_HOST` | HTTP API host | `0.0.0.0` |
| `PETALTRACE_SERVER_PORT` | HTTP API port | `8090` |
| `PETALTRACE_COLLECTOR_OTLP_HTTP_PORT` | OTLP/HTTP port | `4318` |
| `PETALTRACE_COLLECTOR_OTLP_GRPC_PORT` | OTLP/gRPC port | `4317` |
| `PETALTRACE_COLLECTOR_BATCH_SIZE` | Batch size | `100` |
| `PETALTRACE_COLLECTOR_FLUSH_INTERVAL` | Flush interval | `1s` |
| `PETALTRACE_STORAGE_PATH` | Database path | `~/.petaltrace/data.db` |
| `PETALTRACE_STORAGE_WAL_MODE` | Enable WAL mode | `true` |
| `PETALTRACE_RETENTION_DEFAULT` | Default retention | `30d` |
| `PETALTRACE_RETENTION_FAILED_RUNS` | Failed run retention | `90d` |
| `PETALTRACE_PRICING_OVERRIDES_PATH` | Pricing overrides path | |

Environment variables take precedence over the configuration file.

## Duration Format

Duration values support human-readable formats:

- `30s` — 30 seconds
- `5m` — 5 minutes
- `1h` — 1 hour
- `24h` — 24 hours
- `7d` — 7 days
- `30d` — 30 days
- `1h30m` — 1 hour 30 minutes
- `never` — No expiration (for retention settings)

## Storage Configuration

### Database Location

The default database location is `~/.petaltrace/data.db`. The directory is created automatically if it doesn't exist.

For production deployments, consider a dedicated data directory:

```yaml
storage:
  path: "/var/lib/petaltrace/data.db"
```

### WAL Mode

WAL (Write-Ahead Logging) mode is enabled by default and provides:

- Better concurrent read performance
- Faster writes
- Crash recovery

Only disable WAL mode if you have specific requirements:

```yaml
storage:
  wal_mode: false
```

## Pricing Configuration

### Built-in Pricing

PetalTrace includes built-in pricing for major providers:

- **Anthropic**: Claude 3, 3.5, 4 (Haiku, Sonnet, Opus)
- **OpenAI**: GPT-4, GPT-4o, o1, o3-mini
- **Google**: Gemini 1.5, 2.0
- **DeepSeek**: deepseek-chat, deepseek-reasoner
- **Mistral**: mistral-large, mistral-small, codestral

### Custom Pricing Overrides

Create a pricing overrides file for custom rates:

```yaml
# pricing-overrides.yaml
providers:
  anthropic:
    models:
      # Override existing model pricing
      claude-sonnet-4-20250514:
        input_per_1m: 2.50      # Custom negotiated rate
        output_per_1m: 12.00
        cache_read_per_1m: 0.25
        cache_write_per_1m: 3.00

      # Add custom/private model
      claude-internal-v1:
        input_per_1m: 1.00
        output_per_1m: 5.00

  # Add new provider
  custom-provider:
    models:
      custom-model-v1:
        input_per_1m: 0.50
        output_per_1m: 2.00
```

Reference in your configuration:

```yaml
pricing:
  builtin: true
  overrides_path: "/path/to/pricing-overrides.yaml"
```

Override prices take precedence over built-in prices.

### Update Pricing via API

Pricing can also be updated at runtime via the HTTP API:

```bash
curl -X PUT "http://localhost:8090/api/pricing" \
  -H "Content-Type: application/json" \
  -d '{
    "provider": "anthropic",
    "model": "claude-3-opus-20240229",
    "input_per_1m": 15.0,
    "output_per_1m": 75.0
  }'
```

## Collector Configuration

### Batch Processing

The collector batches spans before writing to the database:

```yaml
collector:
  batch_size: 100         # Write after 100 spans
  flush_interval: "1s"    # Or after 1 second, whichever comes first
```

For high-volume deployments, increase batch size:

```yaml
collector:
  batch_size: 500
  flush_interval: "5s"
```

### Content Limits

Limit the size of stored prompts and completions:

```yaml
collector:
  enrichment:
    max_content_size: 1048576  # 1 MB
```

Content exceeding this limit is truncated.

## Retention Configuration

### Automatic Garbage Collection

PetalTrace automatically removes old runs based on retention settings:

```yaml
retention:
  default: "30d"       # Normal runs: 30 days
  failed_runs: "90d"   # Failed runs: 90 days (longer for debugging)
  starred_runs: "never" # Starred runs: never deleted
```

### Manual Garbage Collection

Run garbage collection manually:

```bash
# Preview what will be deleted
petaltrace gc --dry-run

# Run GC with custom retention
petaltrace gc --retain 14d

# Include starred runs
petaltrace gc --include-starred
```

## Example Configurations

### Development

```yaml
server:
  address: ":8090"

storage:
  path: "./data/petaltrace.db"

retention:
  default: "7d"

collector:
  batch_size: 10
  flush_interval: "100ms"
```

### Production

```yaml
server:
  address: ":8090"
  host: "0.0.0.0"

storage:
  path: "/var/lib/petaltrace/data.db"
  wal_mode: true

collector:
  batch_size: 500
  flush_interval: "5s"
  enrichment:
    compute_cost: true
    extract_text: true
    max_content_size: 2097152  # 2 MB

retention:
  default: "90d"
  failed_runs: "180d"
  max: "365d"

pricing:
  builtin: true
  overrides_path: "/etc/petaltrace/pricing.yaml"
```

### Docker

```yaml
server:
  address: ":8090"
  host: "0.0.0.0"

storage:
  path: "/data/petaltrace.db"

collector:
  otlp_http_port: 4318
  otlp_grpc_port: 4317
```

With environment variable overrides:

```bash
docker run -d \
  -e PETALTRACE_STORAGE_PATH=/data/petaltrace.db \
  -e PETALTRACE_RETENTION_DEFAULT=60d \
  -v petaltrace-data:/data \
  -p 8090:8090 \
  -p 4317:4317 \
  -p 4318:4318 \
  petaltrace serve
```
