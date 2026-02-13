# TODO

## Completed

- [x] Phase 1: Core Redesign (orchestration engine, job model, worker refactor, backend interface, API, CLI)
- [x] Phase 2: Production Orchestration (failure strategies, timeouts, retries, cancellation, result persistence)
- [x] Hierarchical Execution Model (Phase type, barrier + pipeline execution, recursive tree walker)
- [x] HA Foundation (KV CAS, worker FIFO, configurable intervals)
- [x] Multi-Controller HA (job ownership, recovery, cross-controller cancellation)
- [x] Web Dashboard (Vue 3 SPA, NATS monitoring, cluster views)
- [x] Backend Expansion: 12 new backends, ~70 new actions (sysinfo, file, net, proc, journald, health, cert, dns, user, firewall, config, package)

## Up Next: Backend Library Integration

Adopt well-tested Go libraries to replace shell-outs and hand-rolled parsing in new backends.

- [ ] `miekg/dns` — Rewrite dns backend with proper record-type queries + TTLs
- [ ] `prometheus-community/pro-bing` — Rewrite net ping action with structured ICMP stats (RTT min/avg/max/stddev, packet loss %)
- [ ] `pelletier/go-toml/v2` — Add TOML format support to config backend
- [ ] `go-ini/ini` — Replace hand-rolled INI parser in config backend

## Deferred

- [ ] `sync` scope on barriers: `all` (default), `group`, `none`
- [ ] Container backend (separate effort)

## Phase 3: Production Hardening

- [ ] mTLS between all components
- [ ] Token auth with NATS accounts + NKeys
- [ ] Backend action allowlisting in controller config
- [ ] Prometheus metrics, structured logging, OpenTelemetry traces
- [ ] Health check endpoints (`/healthz`, `/readyz`)
- [ ] Graceful drain (worker finishes current task before shutdown)
- [ ] Backend idempotency metadata (safe-to-retry declaration)
