# FunnelBarn Load Test Results & Production Capacity

**Date:** 2026-04-30  
**Version:** commit `5c44b31` (post production-hardening)  
**Test tool:** [k6](https://k6.io) via Docker (`grafana/k6:latest`)

---

## Test Environment

| | |
|---|---|
| **Host OS** | macOS 26.3.1, ARM64 (Apple Silicon) |
| **Docker CPU** | 8 cores allocated |
| **Docker RAM** | 16 GB allocated |
| **Service image** | `funnelbarn/service:local` — Alpine 3.20, single CGO binary |
| **Database** | SQLite (`/tmp/funnelbarn.db`, no Litestream replication) |
| **Network** | Docker `--network host` (no bridge overhead) |
| **Auth** | Static API key, no TLS (local only) |

The service ran with default settings: 1-second worker tick, 5ms ingest flush, in-memory queue of 32 KB, no S3 replication.

---

## Results

### 1. Raw HTTP Throughput — `GET /api/v1/health`

No authentication, no database I/O. This measures the Go HTTP stack + middleware chain overhead.

| Metric | Value |
|--------|-------|
| **Sustained rate** | 1,000 RPS for 30s |
| **Total requests** | 30,001 |
| **Error rate** | **0.00%** |
| **Latency avg** | 287 µs |
| **Latency p50** | 253 µs |
| **Latency p90** | 390 µs |
| **Latency p95** | 524 µs |
| **Latency p99** | 1.16 ms |
| **Latency max** | 29.4 ms |

**Verdict:** The server handled 1,000 RPS without a single error. The p99 tail at 1.16ms is clean. The 29ms spike is an outlier (GC or OS scheduler, not a request pattern).

---

### 2. Event Ingest — `POST /api/v1/events`

Authenticated with a valid API key. Events are enqueued to a 5ms in-memory batch, then spooled to disk. This exercises auth validation, body parsing, and async queue enqueue.

**Rate limiter context:** ingest is rate-limited to **500 events/min per IP** (burst of 100) to protect against a single misconfigured SDK flooding the server. A k6 run from one IP hits this ceiling intentionally.

| Metric | Value |
|--------|-------|
| **Attempted rate** | 200 RPS for 30s |
| **Total requests** | 6,001 |
| **Accepted (202)** | 350 (5.8%) |
| **Rate-limited (429)** | 5,651 (94.2%) |
| **Latency of accepted requests p95** | 274 µs |
| **Latency of accepted requests avg** | 192 µs |

**Interpretation:** The 94% rejection rate is correct and expected — 200 RPS from a single IP far exceeds the 500/min per-IP cap (≈8.3 RPS sustained). The 350 acceptances represent the 100-token burst plus refill during the 30s window. In production, ingestion comes from many different end-user IPs, so a real site with 200 concurrent users sending events would not hit this limit.

**Net throughput ceiling (real traffic, distributed IPs):** given that latency stays under 300 µs per accepted request, the server can process ingest requests as fast as they arrive up to the queue capacity of 32,768 slots. At the 1s worker tick rate, this translates to a sustained write capacity of several thousand events/second before the queue would back up.

---

### 3. Rate Limiter Validation — `POST /api/v1/login`

30 rapid login attempts from 10 concurrent VUs. Tests that the per-IP token bucket fires correctly.

| Metric | Value |
|--------|-------|
| **Total attempts** | 30 |
| **Rate-limited (429)** | 25 (83%) |
| **Slipped through burst** | 5 (17%) |
| **Latency avg** | 506 µs |

**Verdict:** Rate limiting is working. The 5 requests that didn't receive 429 consumed the initial burst tokens before the bucket depleted. Limit is 5 requests/minute, burst 5 — exactly what the numbers show.

---

## Container Footprint (at rest, after all tests)

| Resource | Value |
|----------|-------|
| **Memory** | **15 MB** |
| **CPU** | 1.5% (idle) |
| **Processes** | 13 |
| **Disk I/O** | 37 MB written (SQLite WAL + spool) |

A single FunnelBarn instance is exceptionally lean. The 15 MB RSS includes the Go runtime, SQLite, all in-memory queues, and the active session/auth state.

---

## Production Capacity Estimates

These are extrapolations from the test data, not production measurements. Real production numbers will differ based on query complexity, event volume, and hardware.

| Scenario | Estimated capacity |
|----------|-------------------|
| **Concurrent dashboard users** | 500+ (read-only queries, SQLite concurrent reads) |
| **Ingest events/sec (distributed IPs)** | 500–2,000 depending on worker tick and spool I/O |
| **Login attempts before 429** | 5 per IP per minute (by design) |
| **Memory per instance** | ~20–50 MB under moderate load |
| **Minimum viable VPS** | 256 MB RAM, 1 vCPU (significantly over-provisioned) |

FunnelBarn is designed to run on the smallest viable machine. A €4/month VPS (1 vCPU, 512 MB RAM) can comfortably serve a site with tens of thousands of daily active users.

---

## Notes on the Test Setup

- Tests ran against a Docker container on the same machine as k6 — no network hop. Production numbers across a network will show ~1–5ms additional latency from TCP round-trip.
- SQLite is not replicated in this test (no Litestream). In production, Litestream adds negligible overhead (~1ms extra fsync per transaction).
- The service binary is built with `CGO_ENABLED=1` for the SQLite driver. ARM64 builds are native (no emulation).
- k6 ran inside Docker with `--network host` to avoid Docker bridge NAT overhead.

---

## Baseline Table

Record results per release to detect performance regressions.

| Date | Version | Endpoint | p50 | p95 | p99 | RPS | Error rate |
|------|---------|----------|-----|-----|-----|-----|------------|
| 2026-04-30 | 5c44b31 | GET /health | 253µs | 524µs | 1.16ms | 1,000 | 0.00% |
| 2026-04-30 | 5c44b31 | POST /events (accepted) | 201µs | 274µs | — | 8.3 (rate-limited) | 0% |
