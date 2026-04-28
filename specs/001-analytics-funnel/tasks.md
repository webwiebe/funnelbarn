# Task Backlog: FunnelBarn — Analytics & Funnel Tracking

---

## UI Overhaul — See Spec 002

> **The web dashboard tasks in this file (T-130 through T-147) are superseded by the UI overhaul spec.**
>
> A full UI redesign with a marketing site, flashy dashboard, live stats, funnel visualization, and A/B tests has been specced in:
>
> - **Spec**: [`specs/002-ui-overhaul/spec.md`](../002-ui-overhaul/spec.md)
> - **Tasks**: [`specs/002-ui-overhaul/tasks.md`](../002-ui-overhaul/tasks.md) (tasks UI-001 through UI-208)
> - **Design System**: [`specs/002-ui-overhaul/design-system.md`](../002-ui-overhaul/design-system.md)
>
> **Superseded tasks** (replaced by spec 002 equivalents):
> - T-130 through T-136: replaced by UI-001 through UI-050 (app shell, dashboard)
> - T-137 (Login.tsx): replaced by UI-002
> - T-138 (Projects.tsx): replaced by UI-006, UI-008
> - T-139 (TimeRangePicker): replaced by UI-042
> - T-140 (TimeSeriesChart): replaced by UI-043
> - T-141 (Events.tsx): still needed, not superseded
> - T-142 (Sessions.tsx): still needed, not superseded
> - T-143 (APIKeys.tsx): replaced by UI-101, UI-102
> - T-144 (Attribution.tsx): still needed, addressed in spec 002 Overview page
> - T-145 (Alerts.tsx): still needed, not superseded
> - T-146 (401 auth redirect): replaced by UI-003
> - T-147 (project context): replaced by UI-005
>
> All new web UI work should be tracked in `002-ui-overhaul/tasks.md`. The Go backend tasks below remain unchanged.

---

## Phase 1 — Core Go Binary (MVP)

### Config Package

- [x] T-001: Create `go.mod` with module `github.com/wiebe-xyz/funnelbarn` and dependencies (sqlite3, bugbarn-go, bcrypt)
- [x] T-002: Implement `internal/config/config.go` — `Config` struct with all `FUNNELBARN_*` env var fields
- [x] T-003: Implement `loadConfigFiles()` — parse `/etc/funnelbarn/funnelbarn.conf` and `~/.config/funnelbarn/funnelbarn.conf`
- [x] T-004: Implement `applyConfigFile()` — KEY=VALUE parser with quote stripping and comment support
- [ ] T-005: Write unit tests for `config.Load()` — env var override, config file parsing, defaults
- [ ] T-006: Write unit tests for `applyConfigFile()` — blank lines, comments, quoted values, existing env var wins

### SQLite Schema and Migrations

- [x] T-010: Write complete SQLite DDL in `internal/storage/schema.go` (projects, api_keys, users, sessions_http, events, sessions, funnels, funnel_steps)
- [x] T-011: Create all indexes (idx_events_project_occurred, idx_events_session, idx_events_name, idx_sessions_project, idx_funnel_steps_funnel)
- [x] T-012: Implement `storage.Open()` — WAL mode, foreign keys, `SetMaxOpenConns(1)`, run schema migration
- [ ] T-013: Add `APIKeyScopeFull` and `APIKeyScopeIngest` constants to storage package
- [ ] T-014: Write integration test for schema migration (idempotency — applying schema twice succeeds)
- [ ] T-015: Add `expires_at` column to `api_keys` table for future key expiration support
- [ ] T-016: Add index `idx_api_keys_project (project_id)` for project-scoped key listing

### Spool Implementation

- [x] T-020: Implement `internal/spool/spool.go` — `Spool` struct, `NewWithLimit()`, `AppendBatch()`, `Close()`
- [x] T-021: Implement `ReadRecords()` and `ReadRecordsFrom()` — NDJSON parsing with byte offset tracking
- [x] T-022: Implement `ReadCursor()` and `WriteCursor()` — `cursor.json` persistence
- [x] T-023: Implement `RotateIfExceedsPath()` — rename active spool to timestamped archive when size exceeds threshold
- [x] T-024: Implement `AppendDeadLetter()` — dead-letter.ndjson for failed records
- [x] T-025: Define `ErrFull` sentinel error for backpressure signaling
- [ ] T-026: Write unit tests for `AppendBatch()` — verify NDJSON format, newline termination
- [ ] T-027: Write unit tests for `ReadRecordsFrom()` — cursor correctness, partial reads, empty file
- [ ] T-028: Write unit tests for spool rotation — file rename, new file creation, cursor reset
- [ ] T-029: Write unit tests for dead-letter append — verify records are written even on repeated failure

### Ingest HTTP Handler

- [x] T-030: Implement `internal/ingest/handler.go` — `Handler` struct, `NewHandler()`, `ServeHTTP()`
- [x] T-031: Implement in-memory queue (`chan spool.Record`, 32k capacity) — enqueue on POST, return 429 on full
- [x] T-032: Implement `Start()` goroutine — 5ms batch flush ticker, graceful drain on ctx cancellation
- [x] T-033: Implement `APIKeyProjectScope()` — delegate to `auth.Authorizer.ValidWithProject()`
- [x] T-034: Return 429 + `Retry-After: 1` header when queue is full
- [x] T-035: Return 413 when request body exceeds `maxBodyBytes`
- [ ] T-036: Write unit tests for `ServeHTTP()` — 401 on bad key, 429 on full queue, 202 on success
- [ ] T-037: Write unit tests for `Start()` — verify batch flushed to spool, graceful drain

### Background Worker

- [x] T-040: Implement `internal/worker/worker.go` — `ProcessRecord()`, `PersistEvent()`
- [x] T-041: Implement event payload decoding (base64 decode → JSON unmarshal)
- [x] T-042: Implement event enrichment pipeline: UA parse, UTM extract, referrer domain, session fingerprint, user ID hash
- [x] T-043: Implement `PersistEvent()` — idempotency check via IngestID, `InsertEvent`, `UpsertSession`
- [x] T-044: Implement `runBackgroundWorker()` in main — 1s tick, retry counting, dead-letter on `workerMaxRetries=3`, spool rotation
- [ ] T-045: Write unit tests for `ProcessRecord()` — valid payload, missing name error, timestamp fallback
- [ ] T-046: Write unit tests for enrichment — UTM extraction from URL, referrer domain extraction, user ID hashing
- [ ] T-047: Write integration test for full ingest → spool → worker → SQLite pipeline

### Event Enrichment

- [x] T-050: Implement `internal/enrich/enrich.go` — `ParseUA()`, `ExtractUTM()`, `ExtractReferrerDomain()`, `HashUserID()`
- [x] T-051: `ParseUA()` — detect browser (Chrome, Firefox, Safari, Edge, IE, curl, Go-http-client), OS (macOS, Windows, Linux, iOS, Android), device type (desktop, mobile, tablet, bot)
- [x] T-052: `ExtractUTM()` — parse `utm_source`, `utm_medium`, `utm_campaign`, `utm_term`, `utm_content` from URL query string
- [x] T-053: `ExtractReferrerDomain()` — extract hostname from full referrer URL, return empty string on parse failure
- [x] T-054: `HashUserID()` — `SHA256(user_id)` hex string, return empty string on empty input
- [ ] T-055: Write unit tests for `ParseUA()` — Chrome/Firefox/Safari/Edge/curl/bot UA strings
- [ ] T-056: Write unit tests for `ExtractUTM()` — full UTM, partial UTM, no UTM, malformed URL
- [ ] T-057: Write unit tests for `ExtractReferrerDomain()` — valid referrer, empty referrer, malformed URL

### Session Fingerprinting

- [x] T-060: Implement `internal/session/fingerprint.go` — `Fingerprint()`, `IsValidSessionID()`
- [x] T-061: `Fingerprint()` — anonymize IPv4 to /24 (zero last octet), IPv6 to /48 (zero bytes 7-16), compute `SHA256(normalized_ip + "|" + ua)[:32]`
- [x] T-062: `IsValidSessionID()` — validate 32-character lowercase hex string
- [ ] T-063: Write unit tests for `Fingerprint()` — IPv4 normalization, IPv6 normalization, empty IP fallback, empty UA
- [ ] T-064: Write unit tests for `IsValidSessionID()` — valid hex, too short, uppercase, non-hex chars

### Storage Layer — CRUD

- [x] T-070: Implement `internal/storage/events.go` — `InsertEvent`, `GetEventByIngestID`
- [x] T-071: Implement dashboard queries — `CountEvents`, `UniqueSessionCount`, `TopPages`, `TopReferrers`, `DailyEventCounts`, `DailyUniqueSessions`, `TopBrowsers`, `TopDeviceTypes`, `TopEventNames`, `TopUTMSources`, `BounceRate`, `AvgEventsPerSession`
- [x] T-072: Implement `internal/storage/sessions.go` — `UpsertSession` (INSERT OR REPLACE), `ListSessions`
- [x] T-073: Implement `internal/storage/funnels.go` — `CreateFunnel`, `FunnelByID`, `ListFunnels`, `AnalyzeFunnel`
- [x] T-074: Implement `internal/storage/db.go` — project, user, API key CRUD (`CreateProject`, `ProjectBySlug`, `EnsureProject`, `ListProjects`, `UpsertUser`, `UserByUsername`, `CreateAPIKey`, `ListAPIKeys`, `ValidAPIKeySHA256`, `TouchAPIKey`)
- [ ] T-075: Implement `DeleteSessionEvents(ctx, projectID, sessionHash)` — right-to-erasure (delete events + session row)
- [ ] T-076: Write integration tests for `InsertEvent` + `GetEventByIngestID` (idempotency)
- [ ] T-077: Write integration tests for `CountEvents`, `TopPages` — verify correct filtering by project and date range
- [ ] T-078: Write integration tests for `AnalyzeFunnel` — 3-step funnel with known data, assert conversion rates

### Authentication

- [x] T-080: Implement `internal/auth/auth.go` — `Authorizer` (static key + DB lookup + `WithDBLookup`)
- [x] T-081: Implement `UserAuthenticator` — bcrypt password comparison, `Enabled()`, `Valid()`
- [x] T-082: Implement `SessionManager` — HMAC-SHA256 signed tokens, `Create()`, `Valid()`, `SessionCookie()`, `CSRFCookie()`, `ClearSessionCookie()`, `ClearCSRFCookie()`, `CSRFToken()`
- [ ] T-083: Write unit tests for `Authorizer.ValidWithProject()` — static key match, DB key match, DB key miss, disabled auth (no key → allow all)
- [ ] T-084: Write unit tests for `SessionManager.Create()` + `Valid()` — valid token, expired token, bad signature, empty token
- [ ] T-085: Write unit tests for `CSRFToken()` — deterministic from same session token

### API Server

- [x] T-090: Implement `internal/api/server.go` — `Server`, CORS headers, `requireSession()` middleware
- [x] T-091: Implement `internal/api/health.go` — `GET /api/v1/health` → `{"status":"ok","time":"..."}`
- [x] T-092: Implement `internal/api/auth.go` — login, logout, me, list/create projects, list/create API keys
- [x] T-093: Implement `internal/api/events.go` — paginated event list with `limit`/`offset` query params
- [x] T-094: Implement `internal/api/sessions.go` — paginated session list
- [x] T-095: Implement `internal/api/dashboard.go` — aggregate stats endpoint
- [x] T-096: Implement `internal/api/funnels.go` — funnel CRUD + analysis
- [ ] T-097: Implement `internal/api/export.go` — CSV export with streaming response (`text/csv`, chunked)
- [ ] T-098: Implement `internal/api/erasure.go` — `DELETE /api/v1/projects/{id}/sessions/{hash}`
- [ ] T-099: Write unit tests for `handleLogin()` — valid credentials set cookie, invalid credentials 401, session manager nil → allow
- [ ] T-100: Write unit tests for `requireSession()` — valid cookie passes, missing cookie 401, expired token 401

### CLI Subcommands

- [x] T-105: Implement `funnelbarn user create --username --password` in `cmd/funnelbarn/main.go`
- [x] T-106: Implement `funnelbarn project create --name [--slug]` with slug generation
- [x] T-107: Implement `funnelbarn apikey create --project --name [--scope]` with random key generation + SHA256
- [x] T-108: Implement `funnelbarn worker-once` — replay unprocessed spool records into SQLite
- [x] T-109: Implement `funnelbarn version` — print version and build time
- [ ] T-110: Implement `funnelbarn alerts list` — list alert rules for a project
- [ ] T-111: Implement `funnelbarn alerts create --project --metric --condition --threshold --window` — create alert rule
- [ ] T-112: Implement `funnelbarn alerts delete --id` — delete alert rule

### BugBarn Self-Reporting

- [x] T-115: Wire `bb.Init()` + `bb.RecoverMiddleware()` when `FUNNELBARN_SELF_ENDPOINT` and `FUNNELBARN_SELF_API_KEY` are set
- [x] T-116: Call `bb.Shutdown()` on process exit
- [x] T-117: Set `ProjectSlug: "funnelbarn"` in `bb.Options`

### Structured Logging

- [x] T-120: Set default slog handler to JSON in `main()`
- [x] T-121: Log `funnelbarn starting` with addr and version on startup
- [x] T-122: Log `event enqueued` with ingest_id and project on each accepted ingest
- [x] T-123: Log `event stored` with ingest_id, event_id, project_id, event_name, session_id on persistence
- [x] T-124: Log worker errors with ingest_id and attempt count
- [ ] T-125: Log dead-letter writes at ERROR level
- [ ] T-126: Log startup summary: spool size, cursor offset, SQLite path, config source (env/file)

---

## Phase 1 — Web Dashboard

- [x] T-130: `web/package.json` — React 18, react-router-dom, Vite, TypeScript
- [x] T-131: `web/vite.config.ts` — proxy `/api` to `FUNNELBARN_API_URL || localhost:8080`
- [x] T-132: `web/index.html` — SPA entry point
- [x] T-133: `web/src/main.tsx` — React root mount
- [x] T-134: `web/src/App.tsx` — React Router with Dashboard and Funnels routes
- [x] T-135: `web/src/pages/Dashboard.tsx` — stats cards, top pages, top referrers, top events
- [x] T-136: `web/src/pages/Funnels.tsx` — funnel list, funnel analysis bar chart
- [ ] T-137: `web/src/pages/Login.tsx` — login form, POST /api/v1/login, set cookie
- [ ] T-138: `web/src/pages/Projects.tsx` — project list, create project form
- [ ] T-139: `web/src/components/TimeRangePicker.tsx` — last 7d / 30d / 90d / custom date picker
- [ ] T-140: `web/src/components/TimeSeriesChart.tsx` — SVG sparkline for daily events/sessions
- [ ] T-141: `web/src/pages/Events.tsx` — paginated event table with filter by name
- [ ] T-142: `web/src/pages/Sessions.tsx` — paginated session list
- [ ] T-143: `web/src/pages/APIKeys.tsx` — API key management (list, create, show key once)
- [ ] T-144: `web/src/pages/Attribution.tsx` — UTM source/medium/campaign breakdown table
- [ ] T-145: `web/src/pages/Alerts.tsx` — alert rules CRUD
- [ ] T-146: Add auth context and redirect to login when 401 is received on any API call
- [ ] T-147: Add project context — store selected project in URL and localStorage

---

## Phase 1 — JavaScript SDK

- [x] T-150: `sdks/js/src/index.ts` — `FunnelBarnClient` class with all public methods
- [x] T-151: `sdks/js/package.json` — `@funnelbarn/js` with ESM + CJS exports
- [x] T-152: `sdks/js/tsconfig.json` — base TypeScript config
- [ ] T-153: `sdks/js/tsconfig.esm.json` — ESM build config (`module: ES2020`, `outDir: dist/esm`)
- [ ] T-154: `sdks/js/tsconfig.cjs.json` — CJS build config (`module: CommonJS`, `outDir: dist/cjs`)
- [ ] T-155: Write unit tests for `FunnelBarnClient.page()` — URL/referrer auto-detection
- [ ] T-156: Write unit tests for UTM extraction in `FunnelBarnClient`
- [ ] T-157: Write unit tests for localStorage session ID — new session creation, expiry extension, idle timeout reset
- [ ] T-158: Write unit tests for event batching — flush on timer, flush on beforeunload
- [ ] T-159: Write unit tests for Node.js HTTP fallback transport
- [ ] T-160: Build ESM + CJS outputs and verify dist/ structure
- [ ] T-161: Publish to npm as `@funnelbarn/js`
- [ ] T-162: Create IIFE bundle for CDN script-tag usage

---

## Phase 1 — Go SDK

- [x] T-170: `sdks/go/funnelbarn.go` — `Init()`, `Track()`, `Page()`, `Flush()`, `Shutdown()`
- [x] T-171: `sdks/go/go.mod` — module `github.com/wiebe-xyz/funnelbarn-go`
- [x] T-172: `sdks/go/README.md` — installation and usage examples
- [ ] T-173: Write unit tests for `transport.send()` — mock HTTP server, verify headers
- [ ] T-174: Write unit tests for `transport.flush()` — timeout behavior, empty queue
- [ ] T-175: Write unit tests for `transport.shutdown()` — drain remaining events, timeout
- [ ] T-176: Write unit tests for concurrent `Track()` calls — no race conditions
- [ ] T-177: Publish to GitHub as `github.com/wiebe-xyz/funnelbarn-go`

---

## Phase 1 — Python SDK

- [x] T-180: `sdks/python/funnelbarn/__init__.py` — `FunnelBarnClient` with `page()`, `track()`, `identify()`, `flush()`, `shutdown()`
- [x] T-181: `sdks/python/pyproject.toml` — package name `funnelbarn`, Python 3.9+
- [x] T-182: `sdks/python/README.md` — installation and usage examples
- [ ] T-183: Write unit tests for `FunnelBarnClient.track()` — verify event enqueued
- [ ] T-184: Write unit tests for thread safety — concurrent `track()` from multiple threads
- [ ] T-185: Write unit tests for `flush()` — wait for queue drain within timeout
- [ ] T-186: Write unit tests for `shutdown()` — stop worker, drain remaining events
- [ ] T-187: Implement Django middleware helper — `FunnelBarnMiddleware` (auto `page_view` on request)
- [ ] T-188: Implement FastAPI dependency example
- [ ] T-189: Publish to PyPI as `funnelbarn`

---

## Phase 1 — Docker and Deployment

- [x] T-190: `deploy/docker/service.Dockerfile` — multi-stage Go build, Litestream binary, `funnelbarn` system user
- [x] T-191: `deploy/docker/web.Dockerfile` — Node build stage (Vite), nginx serve stage
- [x] T-192: `deploy/docker/entrypoint.sh` — conditional Litestream supervision
- [x] T-193: `deploy/docker/litestream.yml` — S3-compatible replica config with 1s sync interval
- [x] T-194: `deploy/docker/nginx.conf` — SPA routing, gzip
- [x] T-195: `docker-compose.yml` — service + web with `funnelbarn-data` volume
- [ ] T-196: Verify `docker compose up` starts cleanly and `GET /api/v1/health` returns 200
- [ ] T-197: Verify Litestream replication with a real S3-compatible endpoint (MinIO)
- [ ] T-198: Publish multi-arch images (`linux/amd64,linux/arm64`) to `ghcr.io/webwiebe/funnelbarn`
- [ ] T-199: Publish to Docker Hub `funnelbarn/service`

---

## Phase 1 — Kubernetes Manifests

- [x] T-200: Testing environment: namespace, pvc, deployment, web-deployment, service, web-service, ingress, kustomization
- [x] T-201: Staging environment: same structure, `funnelbarn-staging.wiebe.xyz` host
- [x] T-202: Production environment: same + Litestream env vars, `funnelbarn.wiebe.xyz` host
- [x] T-203: `.sops.yaml` — age key config for testing/staging/production secrets
- [ ] T-204: Create `deploy/k8s/testing/secret.yaml.example` — SOPS secret template with all `FUNNELBARN_*` secret keys
- [ ] T-205: Create `deploy/k8s/staging/secret.yaml.example`
- [ ] T-206: Create `deploy/k8s/production/secret.yaml.example`
- [ ] T-207: Test kustomize build for all three environments — `kubectl apply -k --dry-run`

---

## Phase 1 — CI/CD Workflows

- [x] T-210: `.github/workflows/build-and-test.yml` — build service + web images, run Go tests, deploy to testing k8s
- [x] T-211: `.github/workflows/binary-release.yml` — auto-tag, build .deb (amd64+arm64), build macOS tarballs (amd64+arm64), GitHub Release, APT dispatch, Homebrew tap update
- [x] T-212: `.github/workflows/deploy-production.yml` — manual trigger with version + confirmation, deploy to production k8s, BugBarn release marker
- [ ] T-213: Create GitHub repository `wiebe-xyz/funnelbarn` and push initial code
- [ ] T-214: Configure GitHub repository secrets: `SOPS_AGE_KEY_TESTING`, `SOPS_AGE_KEY_PRODUCTION`, `RAPID_ROOT_DISPATCH_TOKEN`, `TAP_GITHUB_TOKEN`, `MINIO_*`
- [ ] T-215: Create `webwiebe/homebrew-funnelbarn` GitHub tap repository
- [ ] T-216: First binary release — verify .deb installs on Debian and Homebrew formula works on macOS
- [ ] T-217: First deployment to testing environment

---

## Phase 1 — Packaging

- [x] T-220: `nfpm-funnelbarn.yaml` — .deb package config (binary, service, config example, directories)
- [x] T-221: `deploy/systemd/funnelbarn.service` — `EnvironmentFile=/etc/funnelbarn/funnelbarn.conf`
- [x] T-222: `deploy/deb/postinstall.sh` — create system user, directories, drop sample config, enable service
- [x] T-223: `deploy/deb/preremove.sh` — stop and disable service
- [x] T-224: `deploy/etc/funnelbarn.conf.example` — all `FUNNELBARN_*` env vars documented with defaults
- [ ] T-225: Test `.deb` install end-to-end on a clean `debian:bookworm` Docker container
- [ ] T-226: Test `systemctl start funnelbarn` → verify health check passes
- [ ] T-227: Test `apt remove funnelbarn` → verify service is stopped and disabled

---

## Phase 1 — Documentation

- [x] T-230: `README.md` — quick-start (Docker, Homebrew, APT), config table, API table, architecture diagram
- [x] T-231: `sdks/go/README.md` — installation, usage examples
- [x] T-232: `sdks/js/README.md` — installation, browser + Node.js usage, API reference
- [x] T-233: `sdks/python/README.md` — installation, usage, API reference
- [x] T-234: `site/src/pages/index.astro` — landing page (hero, features, quick-start)
- [x] T-235: `examples/basic-website/README.md` — tracking snippet + funnel example
- [ ] T-236: Full API reference in Astro docs site
- [ ] T-237: Self-hosting guide (binary, Docker Compose, k8s)
- [ ] T-238: Configuration reference (all env vars, defaults, examples)
- [ ] T-239: Privacy guide (what is collected, what is hashed, GDPR compliance statement)
- [ ] T-240: Upgrade guide (breaking changes between major versions)
- [ ] T-241: Deploy docs site to `funnelbarn.com`

---

## Phase 2 — Funnel Enhancements

- [ ] T-300: Implement funnel step property filter evaluation — `{"property":"plan","value":"pro"}` filter in `AnalyzeFunnel`
- [ ] T-301: Implement session-ordered funnel analysis — events must be completed in step order within a session
- [ ] T-302: Add `time_to_convert_median_seconds` and `time_to_convert_p95_seconds` to funnel analysis response
- [ ] T-303: Implement funnel comparison API — `GET /api/v1/projects/{id}/funnels/{fid}/compare?base_from=...&comp_from=...`
- [ ] T-304: Write integration tests for property filter evaluation
- [ ] T-305: Write integration tests for session-ordered funnel analysis
- [ ] T-306: Web UI — funnel builder with drag-and-drop step ordering (`@dnd-kit/sortable`)
- [ ] T-307: Web UI — funnel analysis visualization (conversion bars, drop-off %, time-to-convert histogram)
- [ ] T-308: Web UI — funnel create/edit modal with step management
- [ ] T-309: Web UI — funnel comparison view (two date ranges side-by-side)

---

## Phase 3 — Attribution and UTM

- [ ] T-400: Add `GET /api/v1/projects/{id}/attribution` endpoint — top sources/mediums/campaigns with event + session counts
- [ ] T-401: Add first-touch vs last-touch attribution model parameter to attribution endpoint
- [ ] T-402: Store first UTM in `sessions` table on `UpsertSession` (already done; expose in attribution API)
- [ ] T-403: Web UI — attribution report page with source/medium/campaign tables
- [ ] T-404: Web UI — campaign comparison view (select two campaigns, show conversion metrics side-by-side)
- [ ] T-405: Write integration tests for attribution queries

---

## Phase 4 — Alerts and Digests

- [ ] T-500: Design `alert_rules` table schema: `(id, project_id, name, metric, condition, threshold, window_minutes, delivery_type, delivery_config_json, active, created_at)`
- [ ] T-501: Add `alert_rules` table to schema.go
- [ ] T-502: Implement `storage/alerts.go` — CRUD: `CreateAlertRule`, `ListAlertRules`, `DeleteAlertRule`, `GetAlertRule`
- [ ] T-503: Design `alert_state` table: `(rule_id, fired_at, resolved_at, last_value)` — tracks active alerts
- [ ] T-504: Implement alert evaluation background job — runs every 60s, queries event counts, compares to thresholds
- [ ] T-505: Implement alert state machine — pending → active (fire notification) → resolved (fire recovery) → pending
- [ ] T-506: Implement alert deduplication — do not re-fire while state is `active`
- [ ] T-507: Implement SMTP email delivery — `net/smtp` with TLS, HTML email template
- [ ] T-508: Implement webhook delivery — `POST` JSON body to configured URL with `X-FunnelBarn-Signature` HMAC header
- [ ] T-509: Write unit tests for alert evaluation — threshold below, threshold above, no events
- [ ] T-510: Write unit tests for SMTP delivery — mock SMTP server
- [ ] T-511: Write unit tests for webhook delivery — mock HTTP server, verify signature
- [ ] T-512: Design weekly digest HTML email template — inline CSS, stats cards, top pages, top referrers
- [ ] T-513: Implement weekly digest generation — aggregate last 7 days of stats
- [ ] T-514: Implement weekly digest scheduler — fire on Monday 08:00 UTC using a cron-like tick
- [ ] T-515: Implement weekly digest email delivery
- [ ] T-516: Implement weekly digest webhook delivery
- [ ] T-517: Add `GET /api/v1/projects/{id}/alerts` endpoint
- [ ] T-518: Add `POST /api/v1/projects/{id}/alerts` endpoint
- [ ] T-519: Add `DELETE /api/v1/projects/{id}/alerts/{rid}` endpoint
- [ ] T-520: Web UI — alert rules management page (list, create form, delete button)

---

## Phase 5 — SDK Completion and Distribution

### JavaScript SDK Completion

- [ ] T-600: Implement ESM build pipeline (`tsconfig.esm.json`) — output to `dist/esm/`
- [ ] T-601: Implement CJS build pipeline (`tsconfig.cjs.json`) — output to `dist/cjs/`
- [ ] T-602: Write vitest tests for `FunnelBarnClient.page()` — auto URL/referrer detection
- [ ] T-603: Write vitest tests for UTM extraction from URL
- [ ] T-604: Write vitest tests for localStorage session ID — new session, expiry extension, timeout reset
- [ ] T-605: Write vitest tests for event batching and flush
- [ ] T-606: Write vitest tests for Node.js HTTP fallback transport
- [ ] T-607: Bundle IIFE version (`dist/iife/index.js`) for CDN `<script>` usage
- [ ] T-608: Publish `@funnelbarn/js` to npm
- [ ] T-609: Set up jsDelivr CDN link in README

### Go SDK Completion

- [ ] T-620: Write unit tests for `transport.send()` — mock HTTP server, verify `x-funnelbarn-api-key` header
- [ ] T-621: Write unit tests for `transport.flush()` — timeout, empty queue
- [ ] T-622: Write unit tests for `transport.shutdown()` — drain events, timeout
- [ ] T-623: Run `go test -race ./...` — verify no race conditions
- [ ] T-624: Publish `github.com/wiebe-xyz/funnelbarn-go` with semantic versioning

### Python SDK Completion

- [ ] T-630: Write unit tests for `FunnelBarnClient.track()` — verify event enqueued
- [ ] T-631: Write unit tests for thread safety — 10 concurrent goroutines calling `track()`
- [ ] T-632: Write unit tests for `flush()` — wait for queue drain
- [ ] T-633: Write unit tests for `shutdown()` — drain remaining events after shutdown event set
- [ ] T-634: Implement `FunnelBarnMiddleware` for Django — auto-track `page_view` with request URL and referrer
- [ ] T-635: Write Django middleware tests with `RequestFactory`
- [ ] T-636: Implement FastAPI dependency `FunnelBarnTracker` — injects tracking into request lifecycle
- [ ] T-637: Publish `funnelbarn` to PyPI

### APT Package Verification

- [ ] T-640: Test `.deb` end-to-end on `debian:bookworm` Docker container
- [ ] T-641: Verify `systemctl start funnelbarn` after install
- [ ] T-642: Verify `apt remove funnelbarn` stops and disables service
- [ ] T-643: Verify `apt upgrade funnelbarn` preserves existing config

### Homebrew Verification

- [ ] T-650: Create `webwiebe/homebrew-funnelbarn` GitHub repository
- [ ] T-651: Test `brew tap webwiebe/funnelbarn` on macOS arm64
- [ ] T-652: Test `brew install funnelbarn` produces working binary
- [ ] T-653: Verify formula auto-updates on new binary-release workflow run

### Docker Hub

- [ ] T-660: Publish multi-arch images (`linux/amd64`, `linux/arm64`) to `ghcr.io/webwiebe/funnelbarn`
- [ ] T-661: Publish to Docker Hub `funnelbarn/service`
- [ ] T-662: Add Docker Hub repository description and README

---

## Phase 6 — CI/CD + Infrastructure

- [ ] T-700: Create GitHub repository `wiebe-xyz/funnelbarn` and push initial commit
- [ ] T-701: Configure GitHub Actions secrets (SOPS age keys, RAPID_ROOT_DISPATCH_TOKEN, TAP_GITHUB_TOKEN, MINIO_*)
- [ ] T-702: Configure GitHub Actions variables (BUGBARN_ENDPOINT)
- [ ] T-703: Create SOPS secret template files for each environment
- [ ] T-704: Encrypt testing secret.yaml with age key and commit
- [ ] T-705: Encrypt staging secret.yaml with age key and commit
- [ ] T-706: Encrypt production secret.yaml with age key and commit
- [ ] T-707: First binary release — tag v0.1.0, verify .deb and Homebrew formula
- [ ] T-708: First deployment to `funnelbarn-testing.wiebe.xyz`
- [ ] T-709: First deployment to `funnelbarn-staging.wiebe.xyz`
- [ ] T-710: First deployment to `funnelbarn.wiebe.xyz`
- [ ] T-711: Verify Litestream replication to MinIO in production

---

## Phase 7 — Advanced Analytics

- [ ] T-800: Implement cohort analysis query — group sessions by signup week, track week-over-week retention
- [ ] T-801: Add `GET /api/v1/projects/{id}/cohorts` endpoint
- [ ] T-802: Web UI — cohort retention matrix heatmap
- [ ] T-803: Implement A/B test tracking — `variant` property filter in funnel analysis
- [ ] T-804: Add `?variant=` filter to funnel analysis endpoint
- [ ] T-805: Web UI — A/B test variant comparison in funnel view
- [ ] T-806: Implement retention curve calculation — day 1/7/30 return rate
- [ ] T-807: Add `GET /api/v1/projects/{id}/retention` endpoint
- [ ] T-808: Web UI — retention curve line chart
- [ ] T-809: Implement geo-IP lookup using MaxMind GeoLite2 (opt-in via `FUNNELBARN_GEOIP_DB`)
- [ ] T-810: Populate `country_code` on events and sessions when geo-IP is configured
- [ ] T-811: Add country breakdown to dashboard API
- [ ] T-812: Implement real-time event stream — `GET /api/v1/projects/{id}/stream` (SSE)
- [ ] T-813: Web UI — live event feed using SSE
- [ ] T-814: Implement CSV export streaming — `GET /api/v1/projects/{id}/export?format=csv`
- [ ] T-815: Implement keyset pagination on event list — `before_id` cursor parameter
- [ ] T-816: Implement rate limiting per API key — token bucket in SQLite (`rate_limit_tokens` table)
- [ ] T-817: Implement configurable data retention — nightly job deletes events older than `FUNNELBARN_RETENTION_DAYS`
- [ ] T-818: Implement right-to-erasure — `DELETE /api/v1/projects/{id}/sessions/{hash}` deletes all events and session row
- [ ] T-819: Implement custom dashboards — `dashboard_cards` table, user-defined metric cards
- [ ] T-820: Web UI — custom dashboard card builder

---

## Phase 8 — Documentation

- [ ] T-900: Set up Astro docs site at `site/` with basic layout and navigation
- [ ] T-901: Write getting started guide — install, create project, track first event
- [ ] T-902: Write self-hosting guide — binary, Docker Compose, k8s with Litestream
- [ ] T-903: Write configuration reference — all `FUNNELBARN_*` env vars with type, default, description
- [ ] T-904: Write OpenAPI specification for all API endpoints
- [ ] T-905: Embed Swagger UI in docs site for interactive API exploration
- [ ] T-906: Write JS SDK documentation with browser and Node.js examples
- [ ] T-907: Write Go SDK documentation with backend integration examples
- [ ] T-908: Write Python SDK documentation with Django/FastAPI examples
- [ ] T-909: Write funnel guide — step-by-step signup funnel example (events → funnel definition → analysis)
- [ ] T-910: Write privacy guide — data collected, data hashed, data never stored, GDPR compliance
- [ ] T-911: Write upgrade guide — breaking changes policy, v0.x migration notes
- [ ] T-912: Add CNAME for `funnelbarn.com` pointing to docs hosting
- [ ] T-913: Deploy Astro docs site to production
