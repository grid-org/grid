# TODO

## Phase 2 Complete

- [x] Conditional task execution (always, on_success, on_failure)
- [x] Job queuing and admission control (max_concurrent, max_pending, HTTP 429)

## Phase 3: Production Hardening

- [ ] mTLS between all components
- [ ] Token auth with NATS accounts + NKeys
- [ ] Backend action allowlisting in controller config
- [ ] Prometheus metrics, structured logging, OpenTelemetry traces
- [ ] Health check endpoints (`/healthz`, `/readyz`)
- [ ] Graceful drain (worker finishes current task before shutdown)
