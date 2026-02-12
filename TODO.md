# TODO

## Up Next

- [ ] Conditional task execution (only-if, on-failure)
- [ ] Job queuing and admission control

## Phase 3: Production Hardening

- [ ] mTLS between all components
- [ ] Token auth with NATS accounts + NKeys
- [ ] Backend action allowlisting in controller config
- [ ] Prometheus metrics, structured logging, OpenTelemetry traces
- [ ] Health check endpoints (`/healthz`, `/readyz`)
- [ ] Graceful drain (worker finishes current task before shutdown)
