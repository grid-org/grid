# TODO

## Phase 2 Complete

- [x] Configurable failure strategies (fail-fast, continue)
- [x] Per-job and per-task timeouts
- [x] Retry with exponential backoff (`max_retries`)
- [x] Job cancellation (`POST /job/:id/cancel`)
- [x] Result persistence with attempts tracking
- [x] Conditional task execution (`always`, `on_success`, `on_failure`)
- [x] Job queuing and admission control (`max_concurrent`, `max_pending`, HTTP 429)

## Up Next: HA Foundation

KV safety and worker reliability — prerequisites for multi-controller support.

- [ ] KV CAS for job updates (replace blind `Put()` with revision-tracked `Update()`)
- [ ] `MaxAckPending: 1` on worker consumers (per-worker FIFO ordering guarantee)
- [ ] Configurable `InactiveThreshold` on worker consumers (currently hardcoded 10m)

## Multi-Controller HA

Requires HA Foundation. Enables running multiple controllers for failover and load balancing.

- [ ] Controller registration + heartbeat (`controllers` KV bucket)
- [ ] Job ownership (`Owner` field on Job, CAS claiming on pickup)
- [ ] Stale job detection (startup scan of `running` jobs, check owner liveness)
- [ ] Job resumption (pick up from last completed step using persisted results + stream replay)

## Hierarchical Execution Model

New job spec format that uses YAML nesting to express execution semantics.

- [x] `Phase` type: recursive tree (leaf = single Task, branch = `tasks` list)
- [x] Top-level list items are barrier-synchronized phases (all nodes sync between them)
- [x] Nested `tasks` blocks are per-node pipelines (no inter-node sync between sub-steps)
- [ ] `sync` scope on barriers: `all` (default), `group`, `none` (deferred — can be added to Phase later)
- [x] Scheduler becomes recursive tree walker instead of flat loop
- [x] Conditions (`on_failure`, etc.) work at any level of the tree

## Phase 3: Production Hardening

Independent of HA and execution model work.

- [ ] mTLS between all components
- [ ] Token auth with NATS accounts + NKeys
- [ ] Backend action allowlisting in controller config
- [ ] Prometheus metrics, structured logging, OpenTelemetry traces
- [ ] Health check endpoints (`/healthz`, `/readyz`)
- [ ] Graceful drain (worker finishes current task before shutdown)
- [ ] Backend idempotency metadata (safe-to-retry declaration)
