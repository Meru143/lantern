# Contributing

Contributions should keep the project aligned with the PRD: one binary, standard OTLP ports, SQLite storage, and a local-first debugging flow. Small, reviewable changes are easier to land than large speculative refactors.

## Development environment

Install Go 1.24.x, Docker, GNU Make, and `golangci-lint`. Clone the repository, then download dependencies and run the local checks:

```bash
go mod download
make test
make test-integration
make lint
make build
```

The integration suite runs against the local runtime and expects Docker to be available on the host. `make release-dry` exercises the same GoReleaser snapshot flow used by the tagged release workflow.

## Sample services

`docker-compose.dev.yml` starts three services: a long-lived dev container, a headless `lantern` instance, and a small instrumented Python workload that emits spans to the local receiver. Start it with:

```bash
docker compose -f docker-compose.dev.yml up --build
```

Open [http://localhost:4000](http://localhost:4000) after the stack is up. The sample service continuously emits checkout-style spans so you can exercise the traces, services, and queries views without wiring another application first.

## Configuration and secrets

Copy `.env.example` to `.env` when you want to override local ports or the database path. Never commit `.env`, snapshot bundles with sensitive payloads, or local SQLite files.

## Code changes

Keep exported names and command signatures in sync with the PRD. Add or update tests alongside behavior changes, then rerun the targeted check before opening a pull request. If you touch the web UI or README screenshots, regenerate the assets with:

```powershell
pwsh -File docs/scripts/run_capture_assets.ps1
```

## Pull requests

Use the pull request template. Include the user-visible change, the verification you ran, and any follow-up work that should stay out of the current patch.
