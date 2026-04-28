# Technical Plan: FunnelBarn — Analytics & Funnel Tracking

---

## Architecture Overview

```
                    ┌─────────────────────────────────────────────┐
                    │              FunnelBarn Process              │
                    │                                              │
  Browser/SDK ──────┼──► POST /api/v1/events                      │
                    │         │ auth + validate                    │
                    │         ▼                                    │
                    │    In-Memory Queue                           │
                    │    (chan spool.Record, 32k cap)               │
                    │         │ 5ms batch flush                    │
                    │         ▼                                    │
                    │    Spool File (ingest.ndjson)                │
                    │    append-only NDJSON on disk                │
                    │         │ Background worker (1s tick)        │
                    │         ▼                                    │
                    │    ProcessRecord                             │
                    │    (decode + enrich: UA, UTM, referrer,      │
                    │     session fingerprint, user ID hash)       │
                    │         │                                    │
                    │         ├──► InsertEvent (SQLite)            │
                    │         └──► UpsertSession (SQLite)          │
                    │                                              │
  Dashboard UI ─────┼──► GET /api/v1/projects/{id}/dashboard      │
  (React SPA)       │    GET /api/v1/projects/{id}/funnels/...    │
                    │    POST /api/v1/login                        │
                    │                                              │
                    │    Litestream ──► S3-compatible object store │
                    └─────────────────────────────────────────────┘
```

---

## Key Architecture Decisions

### Decision 1: SQLite + WAL Mode

**Decision**: Use SQLite as the sole persistent store, with WAL (Write-Ahead Logging) mode enabled.

**Rationale**:
- SQLite handles tens of millions of rows comfortably on a single SSD. Most self-hosted analytics workloads for indie products and small teams never approach SQLite's limits.
- WAL mode allows concurrent reads during writes — the background worker can write events while the dashboard API serves queries without contention.
- Single file backup: Litestream continuously replicates the WAL to S3-compatible storage, providing near-zero data loss and point-in-time recovery.
- Zero administrative overhead: no PostgreSQL connection pool tuning, no migration scripts requiring a running database server, no connection management.

**Tradeoff**: No horizontal write scaling. A single SQLite writer serializes all database mutations. This is acceptable for single-tenant self-hosted deployments and is unlikely to be a bottleneck below ~50M events/month.

### Decision 2: Durable Spool Pattern for Ingest Decoupling

**Decision**: The HTTP ingest handler never writes to SQLite. Events are appended to a durable NDJSON file (the spool) and processed by a background worker.

**Rationale**:
- The ingest endpoint must be fast and available even under database pressure. Synchronous SQLite writes in the request path would create backpressure that hurts ingest latency.
- The spool file is append-only: even if the process crashes mid-write, NDJSON ensures only complete records (newline-terminated JSON objects) are processed.
- A cursor file (`cursor.json`) records the byte offset of the last successfully processed record. On restart, the worker resumes exactly where it left off — no events are lost or double-processed.
- Dead-lettered records (failed after N retries) are appended to `dead-letter.ndjson` for manual inspection. They never block the worker.

**Spool file structure**:
- `{spooldir}/ingest.ndjson` — active spool (append-only)
- `{spooldir}/cursor.json` — `{"offset": N}` updated after each successful record
- `{spooldir}/dead-letter.ndjson` — records that failed after `workerMaxRetries` attempts

### Decision 3: Cookie-Free Session Fingerprinting

**Decision**: Derive session IDs server-side from `SHA256(anonymized_ip + "|" + user_agent)[:32 hex chars]`.

**Rationale**:
- Avoids all cookie consent requirements (ePrivacy Directive). No consent banner needed.
- IP prefix anonymization prevents linking sessions to specific individuals while maintaining session stability for typical households (IPv4 /24) and offices/ISPs (IPv6 /48).
- Client-side session IDs (from the JS SDK's localStorage) take precedence when provided, giving better accuracy for single-page applications where the full URL changes without a new HTTP request.

**Privacy properties**:
- Raw IP addresses never reach SQLite
- Session hash is one-way; no reversal is possible
- Two users behind the same NAT with the same browser will collide — this is acceptable for analytics (not authentication)

### Decision 4: HMAC-Signed Session Tokens (Not JWT)

**Decision**: Dashboard sessions use custom HMAC-SHA256 signed tokens, not JWTs.

**Rationale**:
- JWTs have algorithm confusion attacks (`alg: none`, RSA→HMAC). A custom signed format with a fixed HMAC-SHA256 scheme eliminates this class of vulnerability entirely.
- Token structure: `base64url(payload) + "." + base64url(HMAC-SHA256(secret, payload))`
- The payload contains `{u: username, e: expiry_unix, n: random_nonce}`
- The nonce ensures tokens cannot be replayed even if two tokens have the same username and expiry

### Decision 5: Go Standard Library HTTP (No Framework)

**Decision**: Use `net/http` with Go 1.22 path parameters. No external HTTP framework.

**Rationale**:
- Go 1.22 added `r.PathValue("id")` and method-aware routing patterns (`"GET /api/v1/projects/{id}/dashboard"`), eliminating the primary reasons for gorilla/mux or chi.
- Minimal binary size (the binary is ~25MB with SQLite, compared to >100MB for frameworks with bundled runtimes).
- Zero dependency surface area for security vulnerabilities in HTTP routing.

### Decision 6: BugBarn Self-Reporting

**Decision**: FunnelBarn optionally wraps its HTTP handler with BugBarn's panic recovery middleware, reporting its own errors to a BugBarn instance.

**Rationale**:
- FunnelBarn and BugBarn are designed to be deployed together. This creates a useful operational feedback loop: FunnelBarn sends analytics, BugBarn catches FunnelBarn's own errors.
- Requires `FUNNELBARN_SELF_ENDPOINT` and `FUNNELBARN_SELF_API_KEY` to be configured. When absent, the behavior is identical to without BugBarn.

---

## Phase 1 — Core Ingest + Dashboard (MVP) ✓ DONE

All work in this phase is complete unless marked TODO.

### 1.1 Go Module and Configuration

- [x] `go.mod` with module `github.com/wiebe-xyz/funnelbarn`
- [x] `internal/config/config.go`: `Config` struct loaded from `FUNNELBARN_*` env vars and `/etc/funnelbarn/funnelbarn.conf`
- [x] Config file loading: system-wide `/etc/funnelbarn/funnelbarn.conf`, then `~/.config/funnelbarn/funnelbarn.conf`

### 1.2 SQLite Storage Layer

- [x] `internal/storage/schema.go`: Full DDL (projects, api_keys, users, sessions_http, events, sessions, funnels, funnel_steps)
- [x] `internal/storage/db.go`: `Store.Open()` with WAL mode, foreign keys, connection pooling
- [x] `internal/storage/events.go`: `InsertEvent`, `GetEventByIngestID`, `CountEvents`, `TopPages`, `TopReferrers`, `DailyEventCounts`, `DailyUniqueSessions`, `TopBrowsers`, `TopDeviceTypes`, `TopEventNames`, `TopUTMSources`, `BounceRate`, `AvgEventsPerSession`, `UniqueSessionCount`
- [x] `internal/storage/sessions.go`: `UpsertSession`, `ListSessions`
- [x] `internal/storage/funnels.go`: `CreateFunnel`, `FunnelByID`, `ListFunnels`, `AnalyzeFunnel`
- [x] `internal/storage/helpers.go`: UUID v4 generation
- [x] `internal/storage/db.go`: `ValidAPIKeySHA256`, `TouchAPIKey`, `CreateProject`, `ProjectByID`, `ProjectBySlug`, `EnsureProject`, `ListProjects`, `UpsertUser`, `UserByUsername`, `CreateAPIKey`, `ListAPIKeys`
- [ ] `internal/storage/db.go`: `DeleteSessionEvents(ctx, projectID, sessionHash)` for right-to-erasure

### 1.3 Spool

- [x] `internal/spool/spool.go`: Spool struct, `NewWithLimit`, `AppendBatch`, `ReadRecords`, `ReadRecordsFrom`, `ReadCursor`, `WriteCursor`, `Path`, `RotateIfExceedsPath`, `AppendDeadLetter`
- [x] `ErrFull` sentinel for backpressure propagation
- [ ] `internal/spool/spool_test.go`: unit tests for append, cursor, rotation, dead-letter

### 1.4 Authentication

- [x] `internal/auth/auth.go`: `Authorizer` (static SHA256 key + DB key lookup), `UserAuthenticator` (bcrypt), `SessionManager` (HMAC-signed tokens)
- [x] Cookie names: `funnelbarn_session` (HttpOnly) + `funnelbarn_csrf` (readable by JS)
- [x] API key header: `x-funnelbarn-api-key`
- [x] Project header: `x-funnelbarn-project`
- [ ] `internal/auth/auth_test.go`: unit tests for authorizer, session manager, CSRF

### 1.5 Ingest Handler

- [x] `internal/ingest/handler.go`: `Handler` with in-memory queue (32k chan), `Start()` goroutine, `ServeHTTP()`, `APIKeyProjectScope()`
- [x] 5ms batch flush ticker
- [ ] `internal/ingest/handler_test.go`: unit tests for queue full → 429, auth rejection

### 1.6 Event Enrichment

- [x] `internal/enrich/enrich.go`: `ParseUA()`, `ExtractUTM()`, `ExtractReferrerDomain()`, `HashUserID()`
- [x] `internal/session/fingerprint.go`: `Fingerprint()`, `IsValidSessionID()`
- [ ] `internal/enrich/enrich_test.go`: unit tests for UA parsing edge cases
- [ ] `internal/session/fingerprint_test.go`: unit tests for IPv4/IPv6 normalization

### 1.7 Background Worker

- [x] `internal/worker/worker.go`: `ProcessRecord()`, `PersistEvent()`
- [x] `internal/worker/uuid.go`: local UUID v4 generation (avoids import cycle with storage)
- [x] `cmd/funnelbarn/main.go`: `runBackgroundWorker()` with 1s ticker, retry counting, dead-letter on max retries, spool rotation
- [ ] `internal/worker/worker_test.go`: unit tests for record decoding, enrichment pipeline

### 1.8 API Server

- [x] `internal/api/server.go`: `Server` with CORS, route registration
- [x] `internal/api/health.go`: `GET /api/v1/health`
- [x] `internal/api/auth.go`: `POST /api/v1/login`, `POST /api/v1/logout`, `GET /api/v1/me`, `GET /api/v1/projects`, `POST /api/v1/projects`, `GET /api/v1/apikeys`, `POST /api/v1/apikeys`
- [x] `internal/api/events.go`: `GET /api/v1/projects/{id}/events` (paginated)
- [x] `internal/api/sessions.go`: `GET /api/v1/projects/{id}/sessions` (paginated)
- [x] `internal/api/dashboard.go`: `GET /api/v1/projects/{id}/dashboard`
- [x] `internal/api/funnels.go`: `GET/POST /api/v1/projects/{id}/funnels`, `GET /api/v1/projects/{id}/funnels/{fid}/analysis`
- [ ] `internal/api/export.go`: `GET /api/v1/projects/{id}/export` (CSV streaming)
- [ ] `internal/api/erasure.go`: `DELETE /api/v1/projects/{id}/sessions/{hash}` (right-to-erasure)

### 1.9 CLI

- [x] `cmd/funnelbarn/main.go`: `funnelbarn user create`, `funnelbarn project create`, `funnelbarn apikey create`, `funnelbarn worker-once`, `funnelbarn version`
- [ ] `cmd/funnelbarn/main.go`: `funnelbarn alerts list/create/delete`

### 1.10 Web Dashboard

- [x] `web/package.json`, `web/vite.config.ts`, `web/index.html`
- [x] `web/src/main.tsx`, `web/src/App.tsx` with React Router
- [x] `web/src/pages/Dashboard.tsx`: stats cards, top pages, top referrers, top events
- [x] `web/src/pages/Funnels.tsx`: funnel list + conversion bar chart
- [ ] Login page with session cookie auth (currently bypassed)
- [ ] Project selector / project list page
- [ ] Time range picker (last 7d / 30d / 90d / custom)
- [ ] Real time-series chart (SVG sparkline)
- [ ] Event list page with pagination
- [ ] Session list page
- [ ] API key management page
- [ ] Alert rules management page

---

## Phase 2 — Funnels (Enhancements)

- [ ] Funnel step property filter evaluation (e.g., `{"property":"plan","value":"pro"}`)
- [ ] Session-ordered funnel analysis: a session must complete steps 1→2→3 in order, not just be present in all three event sets
- [ ] Funnel time-to-convert metric: median and p95 time between step 1 and step N
- [ ] Funnel comparison: compare the same funnel across two date ranges
- [ ] Web UI: funnel builder with drag-and-drop step ordering
- [ ] Web UI: funnel analysis visualization (conversion bars, drop-off %, time-to-convert histogram)
- [ ] Web UI: create/edit funnel modal

---

## Phase 3 — Attribution + UTM

- [ ] UTM attribution report API endpoint: `GET /api/v1/projects/{id}/attribution`
- [ ] Top UTM sources/mediums/campaigns tables in dashboard
- [ ] Campaign comparison view: select two campaigns, compare session and conversion counts
- [ ] First-touch vs last-touch attribution model toggle
- [ ] UTM parameter storage in sessions table (already done in session upsert, needs dashboard UI)

---

## Phase 4 — Alerts + Digests

- [ ] Alert rules data model: `alert_rules` table (project_id, metric, condition, threshold, window_minutes, delivery_type, delivery_config)
- [ ] Alert evaluation background job: runs every minute, evaluates all active rules
- [ ] Alert state machine: pending → active → resolved → pending; no re-fire while active
- [ ] Alert delivery: SMTP email (using Go's `net/smtp` standard library)
- [ ] Alert delivery: webhook (POST JSON body)
- [ ] Weekly digest: generate summary stats (top 5 pages, top 3 referrers, event count, session count)
- [ ] Weekly digest: HTML email template (inline CSS for email client compatibility)
- [ ] Weekly digest: webhook delivery
- [ ] Web UI: alert rules CRUD page

**SMTP configuration env vars**:
- `FUNNELBARN_SMTP_HOST`, `FUNNELBARN_SMTP_PORT`, `FUNNELBARN_SMTP_USERNAME`, `FUNNELBARN_SMTP_PASSWORD`
- `FUNNELBARN_SMTP_FROM`, `FUNNELBARN_DIGEST_TO` (comma-separated recipients)

---

## Phase 5 — SDKs + Distribution

### JavaScript SDK (`@funnelbarn/js`)

- [x] `FunnelBarnClient` class: `page()`, `track()`, `identify()`, `flush()`
- [x] Auto-detect URL/referrer, extract UTMs from URL
- [x] localStorage session ID with 30-minute idle timeout
- [x] Background flush timer (5s) + `beforeunload` flush
- [x] Node < 18 HTTP fallback (no `fetch`)
- [ ] Build pipeline (ESM + CJS outputs via `tsc`)
- [ ] Unit tests: session ID generation, UTM extraction, event batching, flush on unload
- [ ] Publish to npm as `@funnelbarn/js`
- [ ] CDN IIFE bundle for script-tag usage (`<script src="https://cdn.jsdelivr.net/npm/@funnelbarn/js">`)

### Go SDK (`github.com/wiebe-xyz/funnelbarn-go`)

- [x] `Init()`, `Track()`, `Page()`, `Flush()`, `Shutdown()`
- [x] Background queue with configurable size and flush goroutine
- [x] Headers: `x-funnelbarn-api-key`, `x-funnelbarn-project`
- [ ] Unit tests for transport: enqueue, flush timeout, shutdown drain
- [ ] Publish to GitHub as `github.com/wiebe-xyz/funnelbarn-go`

### Python SDK (`funnelbarn`)

- [x] `FunnelBarnClient`: `page()`, `track()`, `identify()`, `flush()`, `shutdown()`
- [x] Thread-safe background worker with configurable queue size
- [x] Headers: `x-funnelbarn-api-key`, `x-funnelbarn-project`
- [ ] Unit tests: thread safety, flush, shutdown drain
- [ ] Django middleware helper: `FunnelBarnMiddleware` that auto-tracks `page_view` on each request
- [ ] FastAPI dependency example
- [ ] Publish to PyPI as `funnelbarn`

### APT Package

- [x] `nfpm-funnelbarn.yaml`: package metadata, binary, systemd service, config example, directories
- [x] `deploy/deb/postinstall.sh`: create `funnelbarn` system user, directories, sample config, enable service
- [x] `deploy/deb/preremove.sh`: stop and disable service
- [x] `deploy/systemd/funnelbarn.service`: `EnvironmentFile=/etc/funnelbarn/funnelbarn.conf`
- [x] `deploy/etc/funnelbarn.conf.example`: all `FUNNELBARN_*` env vars documented
- [ ] Verify `apt install funnelbarn` end-to-end on a clean Debian container

### Homebrew Formula

- [x] Homebrew formula generated in `binary-release.yml` CI workflow
- [x] Formula pushed to `webwiebe/homebrew-funnelbarn` GitHub tap
- [ ] Create `webwiebe/homebrew-funnelbarn` GitHub repository
- [ ] Verify `brew tap webwiebe/funnelbarn && brew install funnelbarn` end-to-end

### Docker

- [x] `deploy/docker/service.Dockerfile`: multi-stage build, `funnelbarn` system user, Litestream sidecar
- [x] `deploy/docker/web.Dockerfile`: React SPA built with Vite, served by nginx
- [x] `deploy/docker/entrypoint.sh`: conditional Litestream supervision
- [x] `deploy/docker/litestream.yml`: S3-compatible replica config
- [x] `deploy/docker/nginx.conf`: SPA routing (`try_files $uri /index.html`)
- [x] `docker-compose.yml`: local dev stack with service + web
- [ ] Publish multi-arch images to GitHub Container Registry (`ghcr.io/webwiebe/funnelbarn`)
- [ ] Publish to Docker Hub (`docker.io/funnelbarn/service`)

---

## Phase 6 — CI/CD + Deployment

- [x] `.github/workflows/build-and-test.yml`: build images → run Go tests → deploy to testing k8s
- [x] `.github/workflows/binary-release.yml`: build .deb packages + macOS tarballs → GitHub Release → APT via rapid-root → Homebrew tap
- [x] `.github/workflows/deploy-production.yml`: manual gate with version + confirmation → deploy to production k8s
- [x] K8s testing: namespace, pvc, deployment, web-deployment, service, web-service, ingress, kustomization
- [x] K8s staging: same structure
- [x] K8s production: same + Litestream env vars for S3 replication
- [x] `.sops.yaml`: age key configuration for testing/staging/production secrets
- [ ] Create SOPS secret templates for each environment (`secret.yaml.example`)
- [ ] Set up GitHub repository `wiebe-xyz/funnelbarn` with required secrets
- [ ] First binary release tag
- [ ] First deployment to testing environment

---

## Phase 7 — Advanced Analytics

- [ ] Cohort analysis: group sessions by week, track retention over 8 weeks
- [ ] Cohort retention matrix: heatmap of `week N retention %` for each cohort
- [ ] A/B test tracking: `variant` property on events, conversion rate comparison per variant
- [ ] Retention curves: day 1/7/30 retention calculation for returning session IDs
- [ ] Custom dashboards: user-defined metric cards saved to database
- [ ] Geo-IP: MaxMind GeoLite2 free database, opt-in via `FUNNELBARN_GEOIP_DB` path
- [ ] Real-time event stream: `GET /api/v1/projects/{id}/stream` (SSE) for live dashboard
- [ ] Data export: CSV streaming (`GET /api/v1/projects/{id}/export`)
- [ ] API rate limiting: per-key configurable limits with Redis-free token bucket in SQLite
- [ ] Pagination cursors: keyset pagination on event list (replace offset with `before_id`)
- [ ] Privacy: right-to-erasure (`DELETE /api/v1/projects/{id}/sessions/{hash}`)
- [ ] Privacy: configurable data retention (`FUNNELBARN_RETENTION_DAYS`) with nightly cleanup job

---

## Phase 8 — Documentation Site

- [ ] Astro docs site (`site/`): getting started guide
- [ ] Self-hosting guide: binary, Docker Compose, k8s with Litestream
- [ ] Configuration reference: all `FUNNELBARN_*` env vars with defaults and descriptions
- [ ] API reference: full OpenAPI spec embedded as Swagger UI
- [ ] JS SDK documentation with code examples
- [ ] Go SDK documentation with code examples
- [ ] Python SDK documentation with code examples
- [ ] Funnel guide: step-by-step example (signup funnel)
- [ ] Privacy guide: what data is collected, what is hashed, what is never stored, GDPR compliance statement
- [ ] Upgrade guide: breaking changes between major versions
- [ ] Deploy docs site to `funnelbarn.com`

---

## Technical Risks and Mitigations

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| SQLite write contention under high-volume ingest | Medium | Medium | Spool pattern decouples writes; WAL mode allows concurrent reads; background worker is single-threaded (no lock contention) |
| Session fingerprint collisions (large NAT, shared corporate proxy) | Low | Low | Acceptable for analytics use case; not security-critical; client-side session IDs from JS SDK improve accuracy |
| Spool file grows unbounded on worker failure | Low | Medium | `FUNNELBARN_MAX_SPOOL_BYTES` cap; spool rotation at 64MB threshold; worker rotation check on each tick |
| Go sqlite3 CGO compilation on ARM64 CI | Low | Medium | Use `golang:1.22-alpine` with `gcc` and `musl-dev` in Dockerfile; test `CGO_ENABLED=1 GOARCH=arm64 go build` in CI |
| Litestream replication lag | Low | Low | WAL mode + 1s sync interval; data loss window < 1s under normal operations |
| Dead-letter accumulation without monitoring | Medium | Low | Log dead-letter writes at ERROR level; size of dead-letter file logged at startup; future: alert rule on dead-letter size |
| Browser session ID collisions in localStorage | Very Low | Low | 16 random bytes = 2^128 collision space; negligible |
| HMAC secret rotation breaks active sessions | Low | Low | Document that `FUNNELBARN_SESSION_SECRET` rotation logs out all users; provide guidance in upgrade docs |

---

## Privacy Model: What Gets Stored, What Gets Hashed, What Never Gets Stored

### Stored in plaintext
- Event name (e.g., `page_view`, `signup`)
- URL (including query parameters — operators should strip sensitive query params before sending)
- Referrer URL
- UTM parameters
- Custom event properties (operator-controlled — no PII should be placed here)
- Browser name, OS name, device type (derived from User-Agent)
- Timestamps

### Stored as one-way hash
- Session ID: `SHA256(normalized_ip_prefix + "|" + user_agent)[:32 hex chars]` — computed server-side; raw IP never touches the DB
- User ID: `SHA256(user_id)` — the plaintext user_id is shown once at creation and never persisted

### Never stored
- Raw IP address
- Full User-Agent string in events table (stored for enrichment only, not retained beyond the spool processing window; *note: currently stored in events table — see FR-033 to add retention policy*)
- Cookie values
- Browser fingerprinting attributes (canvas, fonts, etc.)
- Device identifiers (IDFA, GAID)

### Operator responsibility
- The operator controls what goes into the `properties` JSON blob on events. Operators should not send PII (email addresses, names, phone numbers) in event properties.
- The `user_id` field should be an opaque internal identifier (e.g., a UUID or sequential integer), not an email address.
- URL query parameters may contain sensitive data (e.g., `?email=user@example.com`). Operators should strip such parameters before sending events.
