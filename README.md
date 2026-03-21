# lantern

`lantern` is a local observability platform for developer environments. It runs as a single Go binary, accepts OTLP traces and logs on the standard local ports, stores them in SQLite, and serves both a web UI and a terminal UI for inspection.

The project is being implemented from the attached product requirements document. The planned v1 scope includes OTLP ingestion, a trace viewer, a unified log stream, a service dependency map, DB query analysis, error grouping, and snapshot export and replay.

## Planned Commands

The CLI surface defined in the PRD includes `lantern serve`, `lantern tui`, `lantern snapshot`, `lantern replay`, `lantern clear`, `lantern config show`, and `lantern init`.

## Development Status

The repository is currently in the initial scaffold phase. Expect frequent commits while the core packages, API server, UI assets, TUI, and release tooling are added.
