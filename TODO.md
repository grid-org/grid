# TODO — Current Branch: phase3/docker-compose-and-build

## Done

- [x] Remove go.sum from .gitignore (commit checksum file per Go best practice)
- [x] `go mod tidy` — clean up unused deps
- [x] Update compose.yaml — add worker.groups, depends_on, 2 replicas, remove stale comments
- [x] Verify full cluster: build, start, submit job, collect results, tear down

## In Progress

- [ ] Scenarios directory (`scenarios/`) for compose-based cluster testing
  - [ ] Multi-group topology (web + db workers with separate configs)
  - [ ] Controller healthcheck in compose
  - [ ] Job files exercising different backends and strategies
  - [ ] Taskfile tasks for running scenarios (one-shot + interactive)

## Up Next

- [ ] Revisit deferred Phase 2: conditional task execution (only-if, on-failure)
- [ ] Revisit deferred Phase 2: job queuing and admission control

## Phase 3: Production Hardening (later)

- [ ] mTLS between all components
- [ ] Token auth with NATS accounts + NKeys
- [ ] Backend action allowlisting in controller config
- [ ] Prometheus metrics, structured logging, OpenTelemetry traces
- [ ] Health check endpoints (`/healthz`, `/readyz`)
- [ ] Graceful drain (worker finishes current task before shutdown)
