# Technical Plan: Analytics & Funnel Tracking Foundation

## Architecture Decisions

### SQLite + Spool Pattern

**Decision**: Use SQLite for persistent storage with a durable file spool for ingest decoupling.

**Rationale**:
- SQLite handles millions of rows comfortably on a single disk. Most self-hosted analytics workloads fit well within its capacity.
- The spool pattern (append-only NDJSON file) ensures ingest latency stays below 1ms regardless of database pressure. The HTTP handler writes to the spool queue and returns immediately; a background worker does the expensive work asynchronously.
- Single file backup via Litestream to S3-compatible storage is operationally simple.

**Tradeoff**: No horizontal write scaling. Acceptable for single-tenant self-hosted use.

### Anonymous Session Fingerprinting

**Decision**: Derive session IDs from SHA256(IP-prefix + User-Agent), anonymized to /24 (IPv4) or /48 (IPv6).

**Rationale**:
- Avoids cookies entirely. No consent banner needed for session-level analytics.
- IP prefix anonymization prevents linking sessions to specific users while maintaining reasonable session stability.
- 30-minute idle timeout implemented client-side (localStorage) for better accuracy in JavaScript SDK.

### Background Worker with Cursor

**Decision**: Single-threaded background worker reads spool from a persisted byte offset (cursor.json).

**Rationale**:
- Cursor file persists across restarts. On restart, the worker resumes from where it left off — no events are lost or double-processed.
- Dead-letter file captures records that fail after N retries without blocking the worker.
- Rotation strategy prevents unbounded spool growth.

### Go + Standard Library HTTP

**Decision**: Use `net/http` with Go 1.22 path parameters. No external HTTP frameworks.

**Rationale**:
- Go 1.22 added `r.PathValue("id")` and method-aware routing, eliminating the need for gorilla/mux or chi.
- Minimal binary size and zero dependency surface area.

### BugBarn Self-Reporting

Trailpost reports its own panics and errors to BugBarn when `TRAILPOST_SELF_ENDPOINT` and `TRAILPOST_SELF_API_KEY` are configured. This creates a useful feedback loop when Trailpost is used alongside BugBarn in the same infrastructure.

## Implementation Phases

### Phase 1 — MVP: Events + Sessions + Basic Dashboard (current)

- [x] Go module, config, spool, auth packages
- [x] SQLite schema (events, sessions, projects, api_keys, users)
- [x] Ingest handler with async spool queue
- [x] Background worker: decode → enrich → persist
- [x] User-agent parsing, UTM extraction, referrer domain extraction
- [x] Session fingerprinting
- [x] Dashboard API: event count, sessions, top pages, referrers, time series
- [x] Admin auth (session cookie + CSRF)
- [x] CLI subcommands: user create, project create, apikey create
- [x] Docker image + Litestream
- [x] K8s manifests (testing/staging/production)
- [x] CI/CD workflows

### Phase 2 — Funnels

- [x] Funnel + funnel_steps schema
- [x] Funnel CRUD API
- [x] Funnel analysis: per-step conversion rates and drop-off
- [x] Funnels page in web UI
- [ ] Funnel property filters (spec exists, implementation pending)
- [ ] Session-path funnel analysis (ordered step completion tracking)

### Phase 3 — Alerts & Digests

- [ ] Alert rules: spike detection, threshold alerts
- [ ] Weekly digest email (SMTP)
- [ ] Webhook notifications

### Phase 4 — SDKs & Distribution

- [x] JavaScript SDK (browser + Node)
- [x] Go SDK
- [x] Python SDK stub
- [ ] Python SDK full implementation with Django/Flask middleware
- [ ] APT package (nfpm)
- [ ] Homebrew formula
- [ ] Documentation site (Astro)

## Technical Risks

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| SQLite write contention under high ingest | Medium | Medium | Spool pattern decouples writes; WAL mode enabled |
| Session fingerprint collisions (NAT users sharing IP) | Low | Low | Acceptable for analytics use case; not security-critical |
| Litestream replication lag | Low | Low | WAL mode + 1s sync interval; data loss window < 1s |
| Go sqlite3 CGO dependency on ARM | Low | Medium | Use prebuilt CGO binaries in Dockerfile |

## Data Flow

```
Browser/SDK
    │
    │  POST /api/v1/events
    ▼
Ingest Handler
    │  auth + validate + base64 body
    │
    ▼
In-Memory Queue (chan spool.Record, 32k capacity)
    │
    │  5ms batch flush
    ▼
Spool File (ingest.ndjson, append-only)
    │
    │  Background Worker (1s tick)
    ▼
ProcessRecord (decode + enrich)
    │
    ├── InsertEvent (SQLite)
    └── UpsertSession (SQLite)
```

## Key Invariants

1. The HTTP ingest handler MUST NOT write to SQLite.
2. The spool cursor is the source of truth for worker progress. It is written after each successful record, not in batch.
3. Dead-lettered records are never retried automatically. Manual replay via `trailpost worker-once`.
4. API keys are stored as SHA256 hashes only. Plaintext is shown once at creation and never stored.
5. User IDs are hashed before storage. The hash is one-way; there is no way to recover the original user ID.
