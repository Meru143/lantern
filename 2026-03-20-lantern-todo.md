<!-- Active skills: golang-pro, architecture-patterns, error-handling-patterns,
     distributed-tracing, supabase-postgres-best-practices,
     secure-code-guardian, web-design-guidelines, frontend-design,
     ui-ux-pro-max, accessibility-a11y, browser-use, webapp-testing,
     audit-website, devops-engineer, ci-cd-best-practices,
     systematic-debugging, test-driven-development, git-commit, gh-cli,
     requesting-code-review, receiving-code-review,
     technical-writing, verification-before-completion -->

# 2026-03-20-lantern-todo.md
# lantern — Detailed TODO List

---

## Phase 1: Project Setup

### 1.1 Repository Initialization
- [x] Run `go mod init github.com/<owner>/lantern`
- [x] Create `cmd/lantern/main.go` with `func main() { cli.Execute() }`
- [x] Create `README.md`, `LICENSE` (MIT), `.gitignore`, `CHANGELOG.md`
- [x] Create all directories: `internal/cli`, `internal/receiver`, `internal/store`, `internal/api`, `internal/tui`, `internal/snapshot`, `internal/bus`, `internal/config`, `web/`, `test/integration/`, `test/fixtures/`
- [x] Run `git init && git commit -m "chore: initial scaffold"`

### 1.2 Install Dependencies
- [x] `go get github.com/spf13/cobra@v1.9.0`
- [ ] `go get github.com/spf13/viper@v1.20.0`
- [ ] `go get go.opentelemetry.io/collector/receiver/otlpreceiver@v0.144.0`
- [ ] `go get go.opentelemetry.io/collector/pdata@v1.20.0`
- [ ] `go get modernc.org/sqlite@v1.34.0`
- [ ] `go get github.com/Masterminds/squirrel@v1.5.4`
- [ ] `go get github.com/go-chi/chi/v5@v5.2.0`
- [ ] `go get github.com/gorilla/websocket@v1.5.3`
- [ ] `go get github.com/charmbracelet/bubbletea@v1.3.0`
- [ ] `go get github.com/charmbracelet/lipgloss@v1.1.0`
- [ ] `go get github.com/charmbracelet/bubbles@v0.20.0`
- [ ] `go get go.uber.org/zap@v1.27.0`
- [ ] `go get github.com/stretchr/testify@v1.10.0`
- [ ] `go get github.com/testcontainers/testcontainers-go@v0.35.0`
- [ ] Run `go mod tidy`

### 1.3 Root CLI
- [ ] Create `internal/cli/root.go` with `var rootCmd = &cobra.Command{Use: "lantern"}`
- [ ] Add `Execute()` function calling `rootCmd.Execute()` with `os.Exit(1)` on error
- [ ] Add persistent flags: `--config`, `--verbose`
- [ ] Add `PersistentPreRun` loading config via `config.Load()`

### 1.4 Config Loader
- [ ] Create `internal/config/loader.go` with `Config` struct
- [ ] Fields: `OTLPGRPCPort int`, `OTLPHTTPPort int`, `UIPort int`, `Retention string`, `DBPath string`
- [ ] `viper.SetDefault` for all fields: 4317, 4318, 4000, "24h", "~/.lantern/data.db"
- [ ] Bind env vars: `LANTERN_OTLP_GRPC_PORT`, `LANTERN_OTLP_HTTP_PORT`, etc.
- [ ] Expand `~` in DBPath using `os.UserHomeDir()`
- [ ] Export `Load(cfgFile string) (*Config, error)`

### 1.5 SQLite Database Setup
- [ ] Create `internal/store/db.go`
- [ ] Open connection: `sql.Open("sqlite", dbPath)` using `modernc.org/sqlite` driver
- [ ] Set `PRAGMA journal_mode=WAL` and `PRAGMA foreign_keys=ON`
- [ ] Create `internal/store/migrations.go` with `RunMigrations(db *sql.DB) error`
- [ ] Migration 1: `CREATE TABLE IF NOT EXISTS spans (trace_id TEXT, span_id TEXT PRIMARY KEY, parent_span_id TEXT, service_name TEXT, name TEXT, kind INTEGER, start_time INTEGER, end_time INTEGER, status_code INTEGER, status_message TEXT, attributes TEXT, events TEXT)`
- [ ] Migration 2: `CREATE TABLE IF NOT EXISTS log_records (id TEXT PRIMARY KEY, trace_id TEXT, span_id TEXT, service_name TEXT, timestamp INTEGER, severity_num INTEGER, severity_text TEXT, body TEXT, attributes TEXT)`
- [ ] Migration 3: `CREATE TABLE IF NOT EXISTS error_groups (fingerprint TEXT PRIMARY KEY, exc_type TEXT, message_norm TEXT, count INTEGER, first_seen INTEGER, last_seen INTEGER, services TEXT, sample_span_id TEXT)`
- [ ] Migration 4: `CREATE INDEX IF NOT EXISTS idx_spans_trace_id ON spans(trace_id)`
- [ ] Migration 5: `CREATE INDEX IF NOT EXISTS idx_spans_service ON spans(service_name, start_time)`
- [ ] Migration 6: `CREATE INDEX IF NOT EXISTS idx_logs_trace_id ON log_records(trace_id)`
- [ ] Migration 7: `CREATE INDEX IF NOT EXISTS idx_logs_service ON log_records(service_name, timestamp)`

### 1.6 Event Bus
- [ ] Create `internal/bus/eventbus.go`
- [ ] Define `EventType` const: `EventTypeSpan`, `EventTypeLog`, `EventTypeError`
- [ ] Define `Event` struct: `{ Type EventType; Payload interface{} }`
- [ ] Implement `EventBus` with `mu sync.RWMutex`, `subs map[string]chan Event`
- [ ] Implement `Subscribe(id string, bufSize int) chan Event`
- [ ] Implement `Unsubscribe(id string)`
- [ ] Implement `Publish(e Event)` — iterate subs, non-blocking send with `select { case ch <- e: default: }` to avoid blocking publisher

### 1.7 Build & Release
- [ ] Create `.goreleaser.yml` targeting linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64
- [ ] Create `Makefile` with: `build`, `test`, `test-integration`, `lint`, `docker`, `release-dry`
- [ ] Create `.github/workflows/ci.yml` with lint + test + build jobs
- [ ] Create `.github/workflows/release.yml` with goreleaser on tag push

---

## Phase 2: OTLP Receiver

### 2.1 Consumer Implementation
- [ ] Create `internal/receiver/consumer.go`
- [ ] Define `SignalConsumer` struct implementing `consumer.Traces` and `consumer.Logs`
- [ ] Implement `ConsumeTraces(ctx context.Context, td pdata.Traces) error`
- [ ] In `ConsumeTraces`: iterate `td.ResourceSpans()`, extract service name from `resource.Attributes().Get("service.name")`
- [ ] For each span: build `SpanRecord`, call `store.InsertSpan()`
- [ ] Detect DB spans: check `span.Kind() == ptrace.SpanKindClient` and `span.Attributes().Get("db.system")`
- [ ] Implement `ConsumeCapabilities() consumer.Capabilities { return consumer.Capabilities{MutatesData: false} }`
- [ ] Implement `ConsumeLogs(ctx context.Context, ld pdata.Logs) error`
- [ ] Iterate `ld.ResourceLogs()`, extract service name, build `LogRecord`, call `store.InsertLog()`
- [ ] After each successful insert, publish event to `bus.EventBus`

### 2.2 OTLP Receiver Startup
- [ ] Create `internal/receiver/otlp.go`
- [ ] Implement `Start(ctx context.Context, cfg *config.Config, consumer *SignalConsumer) error`
- [ ] Build `otlpreceiver.Config` with grpc endpoint `0.0.0.0:<port>` and http endpoint `0.0.0.0:<port>`
- [ ] Use `otlpreceiver.NewFactory()` to create factory
- [ ] Call `factory.CreateTraces(ctx, receiverSettings, cfg, consumer)` — store returned receiver
- [ ] Call `factory.CreateLogs(ctx, receiverSettings, cfg, consumer)` — store returned receiver
- [ ] Call `receiver.Start(ctx, componenttest.NewNopHost())` for each
- [ ] Return first `net.Listen` error as `LT001`/`LT002` appropriately

---

## Phase 3: Data Stores

### 3.1 Trace Store
- [ ] Create `internal/store/traces.go`
- [ ] Implement `InsertSpan(ctx context.Context, s SpanRecord) error` using squirrel `INSERT OR REPLACE`
- [ ] Implement `GetTrace(ctx context.Context, traceID string) ([]SpanRecord, error)` querying by trace_id, ordered by start_time ASC
- [ ] Implement `ListTraces(ctx context.Context, filter TraceFilter) ([]TraceSummary, error)` — GROUP BY trace_id, get min(start_time), max(end_time), count(*), service_names, has_error
- [ ] `TraceFilter` struct: `ServiceName string`, `HasError bool`, `MinDuration time.Duration`, `Limit int`, `Offset int`
- [ ] Implement `DeleteTracesOlderThan(ctx context.Context, cutoff time.Time) error` with batched DELETE LIMIT 500

### 3.2 Log Store
- [ ] Create `internal/store/logs.go`
- [ ] Implement `InsertLog(ctx context.Context, l LogRecord) error`
- [ ] Implement `ListLogs(ctx context.Context, filter LogFilter) ([]LogRecord, error)`
- [ ] `LogFilter` struct: `ServiceName string`, `MinSeverity int32`, `TraceID string`, `Search string`, `Limit int`, `Offset int`
- [ ] For `Search`, use `WHERE body LIKE '%' || ? || '%'`

### 3.3 Error Store
- [ ] Create `internal/store/errors.go`
- [ ] Implement `FingerprintError(excType, excMessage string) string` — normalize message (strip numbers, UUIDs, hashes), SHA256 hash of (excType + normalized)
- [ ] Implement `UpsertErrorGroup(ctx context.Context, spanID, excType, excMessage, service string) error`
- [ ] On insert conflict (same fingerprint): increment count, update last_seen, append service to services JSON array
- [ ] Implement `ListErrorGroups(ctx context.Context) ([]ErrorGroup, error)` ordered by count DESC

### 3.4 DB Query Store
- [ ] Create `internal/store/queries.go`
- [ ] Implement `NormalizeSQL(query string) string` — replace numeric literals with `?`, string literals with `'?'`, UUIDs with `<uuid>`
- [ ] Implement `RecordDBSpan(ctx context.Context, s SpanRecord) error` — extract `db.statement` attribute, normalize, upsert into `db_queries` table
- [ ] Migration: `CREATE TABLE IF NOT EXISTS db_queries (normalized TEXT PRIMARY KEY, raw_sample TEXT, count INTEGER, total_duration INTEGER, max_duration INTEGER, trace_ids TEXT)`
- [ ] Implement `ListSlowQueries(ctx context.Context, limit int) ([]DBQueryStat, error)` ordered by max_duration DESC

### 3.5 Retention Cleanup
- [ ] Create `internal/store/cleanup.go`
- [ ] Implement `RunCleanup(ctx context.Context, db *sql.DB, retention time.Duration) error`
- [ ] Parse retention string to `time.Duration`
- [ ] Call `DeleteTracesOlderThan`, `DeleteLogsOlderThan`, `DeleteOldDBSpans`
- [ ] Schedule via `time.NewTicker(1 * time.Hour)` in `serve` command

---

## Phase 4: HTTP API Server

### 4.1 Server Setup
- [ ] Create `internal/api/server.go`
- [ ] Embed `web/` directory: `//go:embed web/* var webAssets embed.FS`
- [ ] Initialize `chi.NewRouter()` with `middleware.Logger`, `middleware.Recoverer`, `middleware.CORS`
- [ ] Mount static assets: `r.Handle("/*", http.FileServer(http.FS(webAssets)))`
- [ ] Mount API routes under `/api/`
- [ ] Start with `http.ListenAndServe(fmt.Sprintf(":%d", port), r)`

### 4.2 Traces Handlers
- [ ] Create `internal/api/traces.go`
- [ ] `GET /api/traces` — parse query params: `service`, `hasError`, `minDuration`, `limit`, `offset`; call `store.ListTraces()`; return JSON array
- [ ] `GET /api/traces/{traceId}` — call `store.GetTrace()`; return full trace with spans as tree

### 4.3 Logs Handler
- [ ] `GET /api/logs` — parse query params: `service`, `severity`, `traceId`, `search`, `limit`, `offset`; call `store.ListLogs()`

### 4.4 Services Handler
- [ ] `GET /api/services` — query `SELECT DISTINCT service_name FROM spans`; build dependency graph by querying cross-service span relationships; return `{ services: [], edges: [{from, to, count, errorRate}] }`

### 4.5 Errors Handler
- [ ] `GET /api/errors` — call `store.ListErrorGroups()`
- [ ] `GET /api/errors/{fingerprint}` — get group + sample span

### 4.6 Queries Handler
- [ ] `GET /api/queries` — call `store.ListSlowQueries(limit=20)`

### 4.7 WebSocket Stream
- [ ] Create `internal/api/stream.go`
- [ ] `GET /api/stream` — upgrade to WebSocket using `gorilla/websocket`
- [ ] Subscribe to `bus.EventBus` with a unique subscriber ID (UUID)
- [ ] For each event: marshal to JSON, write to WebSocket connection
- [ ] On disconnect: call `bus.Unsubscribe(id)`, close channel

---

## Phase 5: Web UI

### 5.1 Base HTML
- [ ] Create `web/index.html` with tab navigation: Traces | Logs | Services | Errors | Queries
- [ ] Add WebSocket connection script that reconnects on disconnect
- [ ] Add live badge counter that increments on WebSocket events

### 5.2 Traces Tab
- [ ] Fetch `GET /api/traces` on load, render as sortable table: Service, Trace Name, Duration, Spans, Time
- [ ] Click row → fetch `GET /api/traces/:id` → render waterfall view (SVG timeline)
- [ ] Waterfall: each span as a horizontal bar, position = (startTime - traceStart) / traceDuration, width = spanDuration / traceDuration
- [ ] Color spans by service (consistent color per service name)
- [ ] Show span detail panel on span click (attributes, events, status)

### 5.3 Logs Tab
- [ ] Fetch and render log records with service badge, severity badge, timestamp, body
- [ ] Severity color coding: ERROR=red, WARN=yellow, INFO=blue, DEBUG=gray
- [ ] Filter controls: service dropdown, severity dropdown, search input
- [ ] Click trace ID badge → navigate to trace waterfall

### 5.4 Services Tab
- [ ] Render service dependency graph as SVG with circles (services) and arrows (calls)
- [ ] Show call count on each arrow
- [ ] Error rate coloring: green < 1%, yellow 1-5%, red >5%

### 5.5 Errors Tab
- [ ] Render error groups: exception type, normalized message, count, first/last seen, services
- [ ] Click row → show sample stack trace

### 5.6 Queries Tab
- [ ] Render DB query stats table: normalized SQL, count, P95 latency, max latency

---

## Phase 6: TUI

### 6.1 Root TUI Model
- [ ] Create `internal/tui/model.go`
- [ ] Define `model` struct: `activeTab int`, `tabs []tabModel`, `width int`, `height int`
- [ ] Implement `tea.Model` interface: `Init()`, `Update(tea.Msg)`, `View() string`
- [ ] Handle `tea.WindowSizeMsg` to resize child models
- [ ] Tab bar rendered with lipgloss: active tab highlighted

### 6.2 Traces Tab TUI
- [ ] Create `internal/tui/traces.go` implementing tab interface
- [ ] Use `bubbles/table` to render trace list
- [ ] Columns: Service, Name, Duration, Spans, Error
- [ ] On Enter: fetch full trace, render waterfall as ASCII bars using lipgloss

### 6.3 Logs Tab TUI
- [ ] Create `internal/tui/logs.go`
- [ ] Use `bubbles/viewport` for scrollable log stream
- [ ] Prepend new log lines as WebSocket events arrive
- [ ] Color severity levels with lipgloss styles

### 6.4 Services + Errors TUI
- [ ] Create `internal/tui/services.go` — ASCII adjacency list of services and call counts
- [ ] Create `internal/tui/errors.go` — table of error groups, expandable to show sample trace

---

## Phase 7: Snapshot

### 7.1 Export
- [ ] Create `internal/snapshot/export.go`
- [ ] Implement `Export(ctx context.Context, dbPath string, duration time.Duration, outPath string) error`
- [ ] Open source SQLite, run `VACUUM INTO tempPath` to copy
- [ ] Copy only records within `duration` window: create filtered copy DB, insert filtered records
- [ ] Create ZIP file at `outPath` containing: `data.db`, `meta.json`
- [ ] `meta.json`: `{ version: "1", exportedAt: ISO8601, durationSeconds: N, spansCount: N, logsCount: N }`

### 7.2 Import/Replay
- [ ] Create `internal/snapshot/import.go`
- [ ] Implement `Import(bundlePath string, targetDBPath string) error`
- [ ] Unzip bundle, validate `meta.json` version
- [ ] Copy `data.db` from bundle to `targetDBPath`

---

## Phase 8: `lantern serve` Command

### 8.1 Serve Orchestration
- [ ] Create `internal/cli/serve.go`
- [ ] Parse flags: `--otlp-grpc`, `--otlp-http`, `--ui-port`, `--retention`, `--db`, `--no-tui`
- [ ] Call `store.Open(dbPath)` and `store.RunMigrations()`
- [ ] Start `bus.NewEventBus()`
- [ ] Start OTLP receiver via `receiver.Start()`
- [ ] Start retention cleanup ticker
- [ ] Start HTTP API server via `api.Start()`
- [ ] If not `--no-tui`: launch `tea.NewProgram(tui.NewModel(...))` in goroutine
- [ ] Block on `signal.NotifyContext` for SIGINT/SIGTERM
- [ ] On shutdown: stop receiver, close DB, exit 0

---

## Phase 9: Unit Tests

### 9.1 Store Tests
- [ ] Create `internal/store/traces_test.go`
- [ ] Test InsertSpan + GetTrace round-trip (5 spans same trace)
- [ ] Test ListTraces filter by service name
- [ ] Test ListTraces filter by hasError=true
- [ ] Test DeleteTracesOlderThan removes old records
- [ ] Create `internal/store/errors_test.go`
- [ ] Test FingerprintError: same inputs → same fingerprint
- [ ] Test FingerprintError: different exception type → different fingerprint
- [ ] Test FingerprintError: message with UUID → normalized matches message without UUID
- [ ] Create `internal/store/queries_test.go`
- [ ] Test NormalizeSQL: `SELECT * FROM users WHERE id = 123` → `SELECT * FROM users WHERE id = ?`
- [ ] Test NormalizeSQL: string literals replaced

### 9.2 Bus Tests
- [ ] Create `internal/bus/eventbus_test.go`
- [ ] Test Subscribe + Publish: subscriber receives event
- [ ] Test Unsubscribe: subscriber stops receiving after unsubscribe
- [ ] Test non-blocking publish: slow subscriber doesn't block publisher

### 9.3 Config Tests
- [ ] Test LoadConfig returns correct defaults
- [ ] Test env var `LANTERN_UI_PORT=9000` overrides default
- [ ] Test `~` expansion in DBPath

---

## Phase 10: Integration Tests

### 10.1 OTLP Integration
- [ ] Create `test/integration/otlp_test.go`
- [ ] Start lantern with temp DB and random ports
- [ ] Send 5 spans using `otlptracegrpc.New()` exporter pointed at test GRPC port
- [ ] Assert `store.GetTrace()` returns 5 spans
- [ ] Assert `/api/traces` REST response contains trace ID

### 10.2 WebSocket Integration
- [ ] Send span to OTLP receiver
- [ ] Connect WebSocket client to `/api/stream`
- [ ] Assert event received within 2 seconds

### 10.3 Retention Test
- [ ] Insert 10 spans with timestamps 25 hours ago
- [ ] Run cleanup with 24h retention
- [ ] Assert spans deleted

---

## Phase 11: CI/CD

### 11.1 GitHub Actions CI
- [ ] Create `.github/workflows/ci.yml`
- [ ] Job: `golangci-lint run ./...`
- [ ] Job: `go test ./internal/...`
- [ ] Job: `go test ./test/integration/... -tags integration` (Docker required)
- [ ] Job: `go build ./cmd/lantern/`
- [ ] Upload Codecov

### 11.2 Release
- [ ] `.github/workflows/release.yml` on tag `v*`
- [ ] `goreleaser release --clean`
- [ ] Push Docker image to ghcr.io

### 11.3 Makefile
- [ ] `make build`, `make test`, `make test-integration`, `make lint`, `make docker`, `make release-dry`

---

## Phase 12: Documentation

### 12.1 README.md
- [ ] Demo GIF showing traces appearing in web UI
- [ ] "Point your OTEL exporter at localhost:4317 and run lantern serve"
- [ ] SDK configuration examples: Go, Node.js, Python (OTEL env vars)
- [ ] Installation section: go install, Homebrew, Docker
- [ ] Screenshots of trace waterfall, log stream, error groups

### 12.2 Community Files
- [ ] CONTRIBUTING.md with dev setup (docker-compose.dev.yml for test services)
- [ ] SECURITY.md, CODE_OF_CONDUCT.md
- [ ] Issue templates, PR template, .editorconfig
