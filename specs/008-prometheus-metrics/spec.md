# Spec 008: Prometheus Metrics

**Spec**: `specs/008-prometheus-metrics/`
**Status**: Partial
**Last verified against the tree**: 2026-10-17 at `ea1b0e6`
**Issues**: none found
**Principles touched**: V

## Purpose

`GET /metrics` serves Prometheus metrics so the operator sees request rates, error rates, ingest queue depth and worker throughput. No test in the tree reads a metric value or calls the endpoint; every requirement below is marked `[inferred]`.

## Functional Requirements

- **FR-001**: The system MUST serve `GET /metrics` in the Prometheus exposition format from `promhttp.Handler()`, outside `requireSession`. Issues: none. [inferred]
- **FR-002**: The system MUST require `Authorization: Bearer <token>` on `/metrics` when `FUNNELBARN_METRICS_TOKEN` is set, compare it in constant time and answer 401 with `WWW-Authenticate` otherwise. Issues: none. [inferred]
- **FR-003**: The system MUST count every request in `funnelbarn_http_requests_total` (method, path, status) and observe its duration in `funnelbarn_http_request_duration_seconds` (`requestLogger` in `internal/api/middleware.go`). Issues: none. [inferred]
- **FR-004**: The ingest handler MUST increment `funnelbarn_events_ingested_total` for each accepted event and set `funnelbarn_ingest_queue_depth` after each drain (`internal/ingest/handler.go`). Issues: none. [inferred]
- **FR-005**: The worker MUST increment `funnelbarn_events_processed_total` on success and `funnelbarn_event_errors_total{reason}` on a failure (`cmd/funnelbarn/ingest_apply.go`, `cmd/funnelbarn/worker_spool.go`). Issues: none. [inferred]
- **FR-006**: The maintenance pass MUST add the deleted row count to `funnelbarn_events_purged_total` (`cmd/funnelbarn/maintenance.go`). Issues: none. [inferred]
- **FR-007**: The system MUST expose metrics for rejected events, fingerprinted sessions, geo lookups, dead letter bytes, spool depth, ingest stalls, recording chunk drops, misrouted requests, R2, IAMBarn and OIDC calls, flag evaluations and funnel analysis duration (`internal/metrics/metrics.go`). Issues: none. [inferred]

## Drift

| Claim | Source | Tree |
|---|---|---|
| Six metrics are defined | Original task | `internal/metrics/metrics.go` defines more than 25 (see FR-007) |
| `EventErrors` is a plain counter | Original task | It carries a `reason` label and the callers use `WithLabelValues` |
| `/metrics` is open to unauthenticated scrapes | Acceptance criteria | Open when `FUNNELBARN_METRICS_TOKEN` is empty; bearer-protected otherwise (`internal/api/metrics_handler.go`) |
| The path label is the route | Original task | `requestLogger` labels with the raw `r.URL.Path`, so a path with an ID in it produces a new series per ID; not established whether production hits this |
| `client_golang` v1.19.1 | Original task | `go.mod` pins v1.23.2 |
| `EventsIngested.Inc()` in `ServeHTTP` after the enqueue | Original task | Incremented in two places in `internal/ingest/handler.go` (the single and the batch path) |
| `/metrics` content type check | Acceptance criteria | `internal/api/golden_cases_test.go` lists `GET /metrics` as an excluded case because its values vary; no test asserts the content type |

## Proof contract

- [ ] `curl` against a running server returns 200 on `/metrics` with `funnelbarn_http_requests_total`, `funnelbarn_events_ingested_total` and `funnelbarn_events_processed_total` in the body (no automated test exists).
- [ ] With `FUNNELBARN_METRICS_TOKEN` set, a request without the bearer token answers 401 (no automated test exists).

## Sources

- `internal/metrics/metrics.go`, `internal/api/metrics_handler.go`, `internal/api/middleware.go`, `internal/ingest/handler.go`, `cmd/funnelbarn/`
- `internal/api/golden_cases_test.go`
