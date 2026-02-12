# CLAUDE.md — GRID

## Project Overview

GRID (Grid Remote Infrastructure Director) is a distributed orchestration engine for executing tasks across fleets of nodes. A controller dispatches multi-step jobs to workers over NATS JetStream; workers execute backend actions and report results.

See `docs/DESIGN.md` for the full architecture and design rationale.

## Build & Run

```bash
# Build all binaries (grid, gridw, gridc) — requires Go 1.23+
task                  # or: task default

# Clean build artifacts
task clean

# Run with Docker Compose (basic: 1 controller, 2 workers)
docker compose up --build

# Run scenario tests (multi-group cluster with job validation)
task scenario -- scenarios/jobs/smoke-test.yaml
task scenario:up                    # start cluster, leave running
task scenario:down                  # tear down cluster
```

Binaries: `bin/grid` (controller), `bin/gridw` (worker), `bin/gridc` (CLI client).

The CLI client (`gridc`) is HTTP-only — it talks to the controller API, not NATS directly. Use `-a` or `GRID_API` env var to set the controller address.

## Project Structure

```
cmd/
  controller/main.go    Controller entry point (embedded NATS + scheduler + API)
  worker/main.go        Worker entry point (connects to controller, runs backends)
  client/main.go        CLI client (gridc)
internal/
  api/                  Echo HTTP API server (POST /job, GET /job/:id, etc.)
  client/               NATS client wrapper (streams, KV, publishing)
  common/               Signal handling utilities
  config/               YAML config loading and defaults
  controller/           Controller startup (streams, KV buckets, scheduler init)
  models/               All data types (Job, Task, Target, TaskResult, NodeInfo, etc.)
  registry/             Node registry and target resolution
  scheduler/            Orchestration engine (job execution, retries, cancellation)
  worker/               Worker agent (registration, heartbeat, task execution)
    backends/           Backend interface + implementations (apt, systemd, rke2, ping, test)
docs/
  DESIGN.md             Architecture, data model, NATS topology, implementation phases
scenarios/
  compose.yaml          Multi-group Docker Compose topology (controller + web + db workers)
  configs/              Per-role config files (controller.yaml, web.yaml, db.yaml)
  jobs/                 YAML job files for scenario testing
  run.sh                Scenario runner (builds gridc, starts cluster, runs job, tears down)
```

## Key Patterns

- **Backend interface**: `Run(ctx, action, params) (*Result, error)` + `Actions() []string`. Backends register via `init()` + `registerBackend()` in their source files.
- **NATS subjects**: `cmd.<scope>.<value>.<backend>.<action>` for commands, `result.<jobID>.<nodeID>` for results.
- **Streams**: `commands` (LimitsPolicy), `results` (LimitsPolicy), `requests` (WorkQueuePolicy).
- **KV buckets**: `jobs`, `nodes`, `cluster`.
- **Job execution**: Scheduler pulls from `requests` stream, resolves targets via registry, dispatches commands step-by-step, collects results via ephemeral consumers.
- **Config**: YAML-based (`github.com/goccy/go-yaml`). Config struct in `internal/config/config.go`. Worker groups set in `worker.groups`.

## Dependencies

- **CLI**: `github.com/alecthomas/kong`
- **HTTP**: `github.com/labstack/echo/v4`
- **Logging**: `github.com/charmbracelet/log`
- **YAML**: `github.com/goccy/go-yaml`
- **NATS**: `github.com/nats-io/nats-server/v2`, `github.com/nats-io/nats.go`
## Testing

```bash
task test                    # run all tests with race detector
go test ./... -v -race       # same, directly
go test ./internal/scheduler # single package
go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out
```

Tests use **embedded NATS** — no external dependencies. Each test spins up an in-process NATS server on a random port via `internal/testutil.NewTestEnv(t)`.

- **Unit tests**: models, config, backends (ping, test), client CRUD, registry target resolution
- **Scheduler tests**: enqueue, single/multi-step execution, fail-fast/continue strategies, retries, timeouts, cancellation — uses mock workers (goroutines consuming commands and publishing results)
- **API tests**: all HTTP endpoints via `httptest` and `API.ServeHTTP()`
- **Integration tests** (`internal/integration/`): full end-to-end with real workers (using test/ping backends), scheduler, and API all in-process
- **Test helper** (`internal/testutil/`): `TestEnv` (embedded NATS + infrastructure), `OnlineNode()`, `WaitFor()`, `RegisterNodes()`
- The `test` backend provides deterministic succeed/fail/sleep/flaky/output actions for testing
- The `ping` backend is pure Go (no shell) and safe for unit tests

## Implementation Status

- **Phase 1** (Core Redesign): Complete — orchestration engine, job model, worker refactor, backend interface, API, CLI.
- **Phase 2** (Production Orchestration): Complete — failure strategies (fail-fast/continue), per-job and per-task timeouts, retry with exponential backoff, job cancellation, result persistence with attempts tracking.
- **Phase 3** (Production Hardening): Not started — mTLS, auth, metrics, observability, graceful drain.

## Style & Conventions

- Go standard layout: `cmd/` for binaries, `internal/` for private packages
- Structured logging via `charmbracelet/log`
- NATS subject names use dots as delimiters
- Config field names use `snake_case` in YAML, Go struct tags for mapping
- Models use JSON and YAML struct tags for dual serialization
- `go.sum` is committed (Go best practice for reproducible builds)
- NATS is internal only (controller ↔ workers); clients use HTTP API
