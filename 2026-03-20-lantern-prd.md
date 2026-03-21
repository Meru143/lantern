# 2026-03-20-lantern-prd.md
# lantern — Local Observability Platform

---

## Section 1 — Project Overview

**Name:** lantern
**Type:** CLI Tool + Embedded Web Server
**Language/Runtime:** Go 1.24
**License:** MIT

`lantern` is a single binary that gives local developer environments the observability they deserve. It embeds an OpenTelemetry Collector-compatible receiver (OTLP gRPC on `:4317`, OTLP HTTP on `:4318`), stores all incoming traces, logs, and spans in a local SQLite database, and serves a real-time web UI at `localhost:4000` that shows distributed traces, unified log streams, service dependency maps, DB query analysis, and error grouping. Zero cloud, zero config, zero separate processes — one binary replaces the Jaeger + Grafana Loki + OTEL Collector stack for local development.

---

## Section 2 — Problem Statement

- Running 3+ microservices locally gives developers 3 terminal tabs of unstructured logs with no cross-service correlation.
- Setting up Jaeger + Loki + Grafana + OpenTelemetry Collector locally takes hours and requires Docker Compose with 5+ services consuming 2GB+ RAM.
- Distributed trace correlation (matching a log line to the trace that caused it) requires Grafana + Tempo + Loki + correlations config — completely unreasonable for local dev.
- Slow SQL queries, N+1 patterns, and missing indexes are invisible until production; no local tool surfaces DB query timing in the context of the request that triggered them.
- Error deduplication doesn't exist locally — the same exception appearing 50 times across 3 services shows up as 150 unrelated log lines.
- Sharing a bug with a teammate requires copy-pasting log lines and trace IDs with no context — no portable format exists for "the last 5 seconds of my local traces."

---

## Section 3 — Solution

1. Embed an OTLP receiver (both gRPC and HTTP) using `go.opentelemetry.io/collector/receiver/otlpreceiver` so any OTEL-instrumented service can point its exporter at `localhost:4317` with zero config.
2. Persist all incoming `pdata.Traces`, `pdata.Logs`, and `pdata.Metrics` in SQLite using `modernc.org/sqlite` (CGO-free) with normalized tables for fast querying.
3. Serve a real-time web UI at `:4000` using embedded `embed.FS` static assets + a JSON/WebSocket API built with `github.com/go-chi/chi`.
4. Build a TUI (Bubbletea) for terminal-native users with the same data, navigable without a browser.
5. Implement error grouping by normalizing stack traces using fingerprinting (strip line numbers, group by exception type + normalized message).
6. Implement `lantern snapshot` to export all data for the last N minutes as a `.lantern` bundle (SQLite + metadata JSON) for sharing.

---

## Section 4 — Target Users

**Primary:** Backend developers running 2+ microservices locally who are tired of grepping logs across multiple terminal windows to understand a failing request.

**Secondary:** Full-stack developers who want to see DB queries, HTTP calls, and frontend requests correlated in a single trace timeline without setting up a cloud observability stack.

**Tertiary:** Developer experience engineers who want to recommend a single tool new team members can install on day one and immediately get distributed tracing on their local machine.

---

## Section 5 — Tech Stack

| Component | Library | Version | Purpose |
|---|---|---|---|
| Language | Go | 1.24 | Single compiled binary |
| OTLP Receiver | go.opentelemetry.io/collector/receiver/otlpreceiver | 0.144.x | Accept traces/logs/metrics on 4317+4318 |
| pdata types | go.opentelemetry.io/collector/pdata | 1.20.x | Parse/store OTLP signal types |
| SQLite (CGO-free) | modernc.org/sqlite | 1.34.x | Local storage, no CGO dependency |
| SQL query builder | github.com/Masterminds/squirrel | 1.5.x | Type-safe SQL building |
| HTTP router | github.com/go-chi/chi/v5 | 5.2.x | Web UI API + WebSocket |
| WebSocket | github.com/gorilla/websocket | 1.5.x | Real-time push to web UI |
| TUI framework | github.com/charmbracelet/bubbletea | 1.3.x | Terminal UI |
| TUI styling | github.com/charmbracelet/lipgloss | 1.1.x | Terminal colors/layout |
| TUI components | github.com/charmbracelet/bubbles | 0.20.x | Viewport, table, spinner |
| CLI framework | github.com/spf13/cobra | 1.9.x | Subcommands |
| Config | github.com/spf13/viper | 1.20.x | ~/.lantern/config.toml |
| Logging | go.uber.org/zap | 1.27.x | Structured internal logging |
| Testing | github.com/stretchr/testify | 1.10.x | Assertions |
| Integration tests | github.com/testcontainers/testcontainers-go | 0.35.x | Real OTLP service tests |
| Release | goreleaser | 2.x | Cross-platform binaries |

**Why `modernc.org/sqlite` over `mattn/go-sqlite3`?** Pure Go — no CGO means `goreleaser` can cross-compile without a C toolchain for all platforms. Same performance for our workload.

**Why embedded OTLP receiver over running otelcol separately?** One binary, zero config. The whole point of lantern is eliminating setup friction. Requiring users to also run otelcol defeats the purpose.

**Why chi over gin/echo?** chi is stdlib-compatible (`net/http`), has no magic, and is the most actively maintained minimal router. gin and echo add global state complexity.

---

## Section 6 — Core Features (v1)

**1. OTLP Ingestion**
- gRPC receiver on `:4317` (configurable)
- HTTP receiver on `:4318` (configurable)
- Accept traces, logs, and metrics in all OTLP formats
- Drop metrics silently (v1 — traces and logs only stored)
- Configurable retention: keep last N hours of data (default 24h)
- Auto-cleanup of data older than retention window

**2. Trace Viewer**
- List all traces sorted by start time descending
- Waterfall view showing span hierarchy and durations
- Span details panel: attributes, events, status, resource
- Cross-service trace stitching (spans from different services in same trace)
- Filter by service name, status code (OK/ERROR), min duration
- Full-text search across span names and attributes

**3. Unified Log Stream**
- All logs from all services in chronological order
- Filter by service, severity (TRACE/DEBUG/INFO/WARN/ERROR/FATAL)
- Trace ID correlation: click a log line to jump to its trace
- Full-text search across log bodies and attributes
- Timestamp, service badge, severity badge per row

**4. Service Dependency Map**
- Auto-build a graph of service→service calls from trace data
- Show call counts and error rates per edge
- Rendered as SVG in web UI, ASCII in TUI
- Refreshes live as new traces arrive

**5. DB Query Analyzer**
- Detect DB spans (span kind == `CLIENT`, `db.system` attribute set)
- Surface slowest queries, grouped by normalized SQL
- Show P50/P95/P99 latency per query fingerprint
- Detect N+1: same normalized query appearing >10× in one trace
- Show which trace triggered each slow query

**6. Error Grouping**
- Fingerprint errors: hash of (exception.type + normalized exception.message)
- Group identical errors across spans/services/time
- Show occurrence count, first seen, last seen, affected services
- "Error detail" view: one representative full stack trace per group

**7. Snapshot & Share**
- `lantern snapshot --last 5m` exports a `.lantern` bundle (ZIP of SQLite + metadata)
- `lantern replay <file.lantern>` loads a bundle and serves it locally
- Bundles are self-contained: no external dependencies to replay

**8. TUI Mode**
- `lantern tui` launches a fullscreen Bubbletea terminal UI
- Tab navigation: Traces | Logs | Services | Errors
- Same data as web UI, keyboard-navigable

---

## Section 7 — Interface Spec

### CLI Commands

```bash
# Start lantern (serves web UI + OTLP receiver)
lantern serve

# Start with custom ports
lantern serve --otlp-grpc 4317 --otlp-http 4318 --ui-port 4000

# Launch TUI
lantern tui

# Export last 5 minutes as a bundle
lantern snapshot --last 5m --out ./debug-session.lantern

# Load and replay a bundle
lantern replay ./debug-session.lantern

# Wipe all stored data
lantern clear

# Show current config
lantern config show

# Initialize config file
lantern init
```

### Flags Table

| Flag | Type | Default | Description |
|---|---|---|---|
| `--otlp-grpc` | `int` | `4317` | OTLP gRPC port |
| `--otlp-http` | `int` | `4318` | OTLP HTTP port |
| `--ui-port` | `int` | `4000` | Web UI HTTP port |
| `--retention` | `string` | `24h` | Data retention window |
| `--db` | `string` | `~/.lantern/data.db` | SQLite database path |
| `--no-tui` | `bool` | `false` | Disable TUI, run headless |
| `--verbose` | `bool` | `false` | Verbose internal logging |
| `--config` | `string` | `~/.lantern/config.toml` | Config file path |

### Config File (`~/.lantern/config.toml`)

```toml
otlp_grpc_port = 4317
otlp_http_port = 4318
ui_port        = 4000
retention      = "24h"
db_path        = "~/.lantern/data.db"

[ui]
theme = "dark"   # "dark" | "light"
```

### Web API Endpoints

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/traces` | List traces (paginated, filterable) |
| `GET` | `/api/traces/:traceId` | Get full trace with all spans |
| `GET` | `/api/logs` | List logs (paginated, filterable) |
| `GET` | `/api/services` | List services and dependency graph |
| `GET` | `/api/errors` | List error groups |
| `GET` | `/api/errors/:fingerprint` | Get error group details |
| `GET` | `/api/queries` | DB query stats |
| `WS` | `/api/stream` | WebSocket stream for live updates |

---

## Section 8 — Data Flow Diagram

```
  ┌─────────────────────────────────────────────────────┐
  │          Your Microservices (OTEL instrumented)     │
  └──────────┬──────────────────────┬───────────────────┘
             │ gRPC :4317           │ HTTP :4318
  ┌──────────▼──────────────────────▼───────────────────┐
  │              OTLP Receiver (embedded)               │
  │    otlpreceiver.NewFactory() + ConsumerFunc         │
  └──────────────────────┬──────────────────────────────┘
                         │ pdata.Traces / pdata.Logs
  ┌──────────────────────▼──────────────────────────────┐
  │              Signal Router                          │
  │   traces → TraceStore | logs → LogStore             │
  └──────┬───────────────────────────┬──────────────────┘
         │                           │
  ┌──────▼──────────┐    ┌───────────▼─────────────┐
  │   TraceStore    │    │      LogStore            │
  │ SQLite: spans,  │    │ SQLite: log_records,     │
  │ traces, services│    │ service_logs             │
  └──────┬──────────┘    └───────────┬─────────────┘
         │                           │
  ┌──────▼───────────────────────────▼──────────────────┐
  │           Event Bus (Go channels)                   │
  │   Broadcasts new data to WebSocket clients          │
  └──────────────────────┬──────────────────────────────┘
                         │
               ┌─────────▼────────────────────────┐
               │        chi HTTP Server            │
               │  REST API + WebSocket + Static    │
               │  assets (embedded via embed.FS)   │
               └──────────┬───────────────────────┘
                          │
              ┌───────────▼────────────────┐
              │    Web UI (localhost:4000)  │
              │  Vanilla JS + CSS served   │
              │  from embedded assets      │
              └────────────────────────────┘
```

---

## Section 9 — Architecture / Package Structure

```
lantern/
├── cmd/
│   └── lantern/
│       └── main.go              # Entry point
├── internal/
│   ├── cli/
│   │   ├── root.go              # Root cobra command
│   │   ├── serve.go             # `lantern serve` command
│   │   ├── tui.go               # `lantern tui` command
│   │   ├── snapshot.go          # `lantern snapshot` command
│   │   ├── replay.go            # `lantern replay` command
│   │   └── init.go              # `lantern init` command
│   ├── receiver/
│   │   ├── otlp.go              # Start OTLP gRPC+HTTP receiver
│   │   └── consumer.go          # pdata.Consumer implementation
│   ├── store/
│   │   ├── db.go                # SQLite connection + migrations
│   │   ├── traces.go            # Insert/query trace/span records
│   │   ├── logs.go              # Insert/query log records
│   │   ├── errors.go            # Error fingerprinting + grouping
│   │   └── queries.go           # DB span analysis
│   ├── api/
│   │   ├── server.go            # chi router + embedded static assets
│   │   ├── traces.go            # /api/traces handlers
│   │   ├── logs.go              # /api/logs handlers
│   │   ├── services.go          # /api/services handlers
│   │   ├── errors.go            # /api/errors handlers
│   │   ├── queries.go           # /api/queries handlers
│   │   └── stream.go            # WebSocket live stream handler
│   ├── tui/
│   │   ├── model.go             # Root Bubbletea model
│   │   ├── traces.go            # Traces tab model
│   │   ├── logs.go              # Logs tab model
│   │   ├── services.go          # Services tab model
│   │   └── errors.go            # Errors tab model
│   ├── snapshot/
│   │   ├── export.go            # Export .lantern bundle
│   │   └── import.go            # Import/replay .lantern bundle
│   ├── bus/
│   │   └── eventbus.go          # In-process pub/sub for live updates
│   └── config/
│       └── loader.go            # viper config loading
├── web/                         # Embedded web assets
│   ├── index.html
│   ├── app.js
│   └── style.css
├── test/
│   ├── integration/
│   └── fixtures/
├── .goreleaser.yml
└── go.mod
```

**Key Go Types:**

```go
// internal/store/traces.go
type SpanRecord struct {
    TraceID       string
    SpanID        string
    ParentSpanID  string
    ServiceName   string
    Name          string
    Kind          int32
    StartTimeUnix int64
    EndTimeUnix   int64
    StatusCode    int32
    StatusMessage string
    Attributes    string // JSON
    Events        string // JSON
}

// internal/store/logs.go
type LogRecord struct {
    TraceID       string
    SpanID        string
    ServiceName   string
    Timestamp     int64
    SeverityNum   int32
    SeverityText  string
    Body          string
    Attributes    string // JSON
}

// internal/store/errors.go
type ErrorGroup struct {
    Fingerprint  string
    ExcType      string
    MessageNorm  string
    Count        int
    FirstSeen    int64
    LastSeen     int64
    Services     []string
    SampleSpanID string
}
```

---

## Section 10 — Error Handling

- **OTLP port already in use:** Check `net.Listen` error for `EADDRINUSE`. Exit 1 with: `"Port 4317 is already in use. Use --otlp-grpc to specify a different port."`
- **SQLite write failure:** Log error with zap, drop the record, do not crash. Emit a warning to TUI.
- **Web UI port already in use:** Exit 1 with: `"Port 4000 is already in use. Use --ui-port to specify a different port."`
- **OTLP payload decode error:** Log warning with service name and drop span batch. Continue.
- **SQLite DB locked (concurrent write):** Retry with 10ms backoff up to 3 times. Log warning if still locked.
- **Snapshot export I/O error:** Print error, exit 1 with full path that failed.

| Code | Meaning | Action |
|---|---|---|
| `LT001` | OTLP port conflict | Exit 1 with port suggestion |
| `LT002` | UI port conflict | Exit 1 with port suggestion |
| `LT003` | DB write failure | Log warn, drop record, continue |
| `LT004` | DB locked | Retry 3×, then log error |
| `LT005` | Invalid OTLP payload | Log warn, drop batch, continue |
| `LT006` | Snapshot I/O error | Exit 1 with path |

---

## Section 11 — Edge Cases

1. **Large traces (>1000 spans):** Waterfall rendering must paginate or virtualise the span list — do not render all spans to DOM at once.
2. **Services reusing the same TraceID:** Poorly configured instrumentation can reuse trace IDs. Lantern must handle this gracefully, grouping by TraceID + service boundary.
3. **Out-of-order spans:** Spans from async processes may arrive after child spans. The store must assemble the trace tree by parentSpanID after all spans for a trace window arrive (use a 5s assembly window).
4. **UTF-8 span attribute values:** JSON marshaling of attribute maps must handle binary values — base64-encode non-UTF8 byte sequences.
5. **Rapid burst ingestion (>10k spans/sec):** Implement a bounded channel buffer; drop oldest if buffer fills. Emit a warning metric.
6. **SQLite retention cleanup:** Running `DELETE FROM spans WHERE end_time < cutoff` on a large table blocks writes. Use `LIMIT 500` batched deletes.
7. **WebSocket client disconnect:** Goroutine managing the websocket must use `select` with a done channel to avoid goroutine leaks.
8. **Multiple `lantern serve` instances:** Second instance should detect port conflict and exit cleanly, not panic.

---

## Section 12 — Testing Strategy

**Unit Tests:**
- `store/traces.go` — insert span batch, query by traceID, assert correct reconstruction
- `store/errors.go` — fingerprint two identical exceptions, assert same fingerprint
- `store/errors.go` — fingerprint two different exceptions, assert different fingerprints
- `receiver/consumer.go` — feed synthetic `pdata.Traces`, assert records in store

**Integration Tests:**
- Spin up lantern with testcontainers-go helper
- Send OTLP traces via `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc`
- Assert `/api/traces` returns the sent trace
- Assert `/api/services` reflects the sending service
- Assert WebSocket receives event after ingestion

**Mock Strategy:**
- No external services to mock — all local
- Use `testing.T` temp directories for SQLite DB files

---

## Section 13 — Distribution

```bash
# Install via go install
go install github.com/<owner>/lantern/cmd/lantern@latest

# Install via Homebrew (post v1.0)
brew install lantern

# Binary download (goreleaser)
curl -sSL https://github.com/<owner>/lantern/releases/latest/download/lantern_linux_amd64.tar.gz | tar xz
```

**Platforms:** Linux amd64/arm64, macOS amd64/arm64 (Universal), Windows amd64

**Docker:** `docker run -p 4317:4317 -p 4318:4318 -p 4000:4000 ghcr.io/<owner>/lantern`

---

## Section 14 — Differentiators

1. **vs Jaeger:** Jaeger only does traces — no logs, no DB query analysis, no error grouping. Requires separate deployment.
2. **vs Grafana + Loki + Tempo:** 3 separate services, 3 config files, Prometheus, Grafana dashboards setup. Takes hours. lantern is one binary and one command.
3. **vs OpenTelemetry Collector:** otelcol is a router, not a UI. It has no visualization.
4. **vs Datadog/Honeycomb local agent:** Both send data to the cloud. lantern is 100% local.

---

## Section 15 — Future Scope (v2+)

- [ ] Metrics support (histogram, gauge, counter panels)
- [ ] Flame graph view for profiling spans
- [ ] HTTP request/response body capture (via OTLP semantic conventions)
- [ ] Slack/webhook notifications for error rate spikes
- [ ] `lantern share` — encrypted bundle sharing via a relay URL
- [ ] VS Code extension showing trace data inline
- [ ] GitHub Actions integration for CI trace analysis

---

## Section 16 — Success Metrics

- [ ] Handles 10,000 spans/second without dropping on a 4-core laptop
- [ ] Cold start time under 200ms
- [ ] Web UI loads in under 500ms for a database with 100,000 spans
- [ ] Single binary under 30MB
- [ ] Works on Linux, macOS, Windows with zero OS-level dependencies
- [ ] 80%+ unit test coverage

---

## Section 17 — Additional Deliverables

**Documentation:**
- [ ] README.md with GIF demo, quick start (5 lines), and OTEL SDK configuration examples for Go/Node/Python
- [ ] CONTRIBUTING.md
- [ ] SECURITY.md
- [ ] CODE_OF_CONDUCT.md
- [ ] .github/ISSUE_TEMPLATE/bug_report.md

**Dev Environment:**
- [ ] `docker-compose.dev.yml` with sample instrumented services to test against
- [ ] `.devcontainer/devcontainer.json`
- [ ] `.env.example`

**Environment Variables:**
- [ ] `LANTERN_DB_PATH` — override DB file location
- [ ] `LANTERN_OTLP_GRPC_PORT` — override gRPC port
- [ ] `LANTERN_OTLP_HTTP_PORT` — override HTTP port
- [ ] `LANTERN_UI_PORT` — override web UI port
- [ ] `LANTERN_RETENTION` — override retention window
- [ ] `LOG_LEVEL` — internal log level (default: `warn`)

---

## Section 18 — Expanded Testing

**Unit Tests (target: 80%+):**
- [ ] `store/traces.go` — InsertSpanBatch with 5 spans, GetTrace returns all 5
- [ ] `store/traces.go` — GetTrace returns nil for unknown traceID
- [ ] `store/traces.go` — ListTraces pagination: page 2 returns correct offset
- [ ] `store/logs.go` — InsertLogBatch, GetLogs filter by service
- [ ] `store/logs.go` — GetLogs filter by severity
- [ ] `store/errors.go` — FingerprintError: same type+message → same hash
- [ ] `store/errors.go` — FingerprintError: different type → different hash
- [ ] `store/errors.go` — IncrementErrorGroup updates count and last_seen
- [ ] `store/queries.go` — NormalizeSQL strips literal values from WHERE clause
- [ ] `bus/eventbus.go` — Subscribe receives published events
- [ ] `bus/eventbus.go` — Unsubscribe stops receiving events
- [ ] `config/loader.go` — loads TOML, env var overrides config

**Integration Tests:**
- [ ] Start receiver, send 10 spans via otlptracegrpc exporter, assert in DB
- [ ] Send spans from 2 services with same traceID, assert stitched trace
- [ ] Send log record with traceID, assert log linked to trace
- [ ] Hit `/api/traces` REST endpoint, assert JSON response shape
- [ ] Connect WebSocket to `/api/stream`, send span, assert event received

**E2E Tests:**
- [ ] `lantern serve` starts, OTEL app sends spans, web UI shows trace
- [ ] `lantern snapshot --last 1m` produces valid ZIP file
- [ ] `lantern replay snapshot.lantern` restores and serves data

---

## Section 19 — CI/CD Pipeline

- [ ] `.github/workflows/ci.yml` — push and PR triggers
- [ ] Job: `golangci-lint run ./...`
- [ ] Job: `go test ./internal/...`
- [ ] Job: `go test ./test/integration/... -tags integration`
- [ ] Job: `go build ./cmd/lantern/`
- [ ] Job: Codecov upload
- [ ] `.github/workflows/release.yml` — tag `v*` trigger
- [ ] `goreleaser release --clean` with Linux/macOS/Windows cross-compilation
- [ ] Push Docker image to `ghcr.io`
- [ ] `make lint` | `make test` | `make test-integration` | `make build` | `make docker`
