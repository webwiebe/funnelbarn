# Feature Specification: FunnelBarn, Analytics & Funnel Tracking

**Spec**: `specs/001-analytics-funnel/`
**Status**: Partial
**Last verified against the tree**: 2026-10-17 at `ea1b0e6`
**Issues**: [#226](https://github.com/webwiebe/funnelbarn/issues/226), [#326](https://github.com/webwiebe/funnelbarn/issues/326)
**Principles touched**: II, III, IV, V
**Created**: 2026-04-28

---

## Problem Statement

### The Cost of Commercial Analytics

Product teams that want to understand their users face an uncomfortable choice:

1. **Financial cost at scale**: Mixpanel and Amplitude price per event volume. A high-traffic SaaS product can spend $500 to $5,000 per month on analytics alone.

2. **Privacy and compliance burden**: Sending user behavior data to US-hosted third-party servers conflicts with GDPR, the Swiss DPA, and a growing number of national privacy regulations.

3. **Vendor lock-in**: Analytics data trapped in a vendor's data model cannot be migrated cleanly.

4. **Trust erosion**: Privacy-conscious users object to third-party tracking scripts. Ad blockers block analytics, which causes systematic data loss.

Self-hosted alternatives exist (Umami, Matomo, PostHog) but carry heavy infrastructure requirements, a large maintenance burden, or feature sets too small for conversion funnel work.

---

## Solution

**FunnelBarn** is a self-hosted analytics and funnel tracking server. The operator deploys one Go binary (or one container), points a domain at it, and owns all analytics data on their own infrastructure. Production and staging split the process into a writer, readers and a Valkey queue (spec 012); testing and the Debian package run one process.

### Design Principles

1. **Privacy-first**: No cross-site tracking. No cookies for session tracking. No third-party script dependencies. Session identifiers derive from a one-way hash.
2. **Simple operations**: Go binary, SQLite database, no PostgreSQL.
3. **High-throughput ingest**: A durable local spool decouples ingest latency from database writes. The HTTP handler returns after enqueueing; a background worker enriches and persists.
4. **Developer-friendly distribution**: Docker images, a Debian package, a Homebrew formula and SDKs for JavaScript, Go and Python.
5. **Self-reporting**: FunnelBarn reports its own errors to BugBarn when configured.

---

## Target Users

- **The Indie Hacker** runs a SaaS product solo and wants product analytics without a large bill.
- **The Privacy Officer** at a European SaaS company needs an auditable record of what data is collected and where it lives.
- **The SaaS Founder** wants funnel analysis and UTM attribution without enterprise pricing.
- **The Backend Developer** instruments server-side events from a multi-service platform.
- **The Marketer** needs to know which UTM sources convert.

---

## User Stories

Each story lists the requirements that implement it. Stories whose behaviour the tree lacks appear under Drift.

1. **Self-host in minutes** (FR-001, FR-002, FR-018). A container started with an API key accepts a pageview at `POST /api/v1/events` with `202 Accepted`, and `GET /api/v1/health` answers without authentication.
2. **See which pages drive conversions** (FR-009, FR-010). The dashboard shows event and session counts, bounce rate, top pages, top referrers, top browsers, UTM breakdowns and a time series over a selectable range.
3. **Analytics without direct identifiers** (FR-004, FR-005, FR-006). Session identifiers derive from a one-way hash; client user IDs are hashed before storage.
4. **Instrument a site with one script tag** (FR-016). The JavaScript SDK batches events and flushes every 5 seconds or on `beforeunload`.
5. **Define multi-step funnels** (FR-011, FR-012, FR-013). Funnels have ordered steps, optional property filters and a conversion window; analysis returns per-step counts.
6. **Install through Docker, Debian or Homebrew** (FR-018).
7. **Instrument a Go or Python backend** (FR-016).
8. **Set a data retention policy** (FR-015, see spec 005).

---

## Functional Requirements

### Ingest and processing

- **FR-001**: The system MUST accept events at `POST /api/v1/events`, authenticated by an API key, and answer `202 Accepted` after the event is enqueued. Issues: none. Tests: `internal/ingest/handler_test.go::TestServeHTTP_Accepted`, `internal/ingest/handler_test.go::TestServeHTTP_Unauthorized`, `internal/ingest/handler_test.go::TestServeHTTP_BodyTooLarge`.
- **FR-002**: The ingest handler MUST NOT write to the database in the request path; it appends to the on-disk spool (or the Valkey ingest queue in reader mode). Issues: none. Tests: `internal/spool/spool_test.go::TestAppend_ReadRecords`, `cmd/funnelbarn/mode_test.go::TestReaderPublishesAndTheWriterApplies`.
- **FR-003**: A background worker MUST read spool records from a persisted cursor and write them to SQLite, and a malformed record MUST advance the cursor. Issues: none. Tests: `internal/spool/spool_test.go::TestCursor_RoundTrip`, `cmd/funnelbarn/worker_spool_test.go::TestSkipMalformedAdvancesCursor`.
- **FR-004**: An event without a valid client session ID MUST receive a session ID that is the first 32 hex characters of SHA-256 over the client IP truncated to /24 (IPv4) or /48 (IPv6), the user agent, the project slug, the environment and a 30-minute time window. Issues: [#226](https://github.com/webwiebe/funnelbarn/issues/226). Tests: `internal/session/fingerprint_test.go::TestFingerprint_IPv4AnonymizationSlash24`, `internal/session/fingerprint_test.go::TestFingerprint_IPv6AnonymizationSlash48`, `internal/session/fingerprint_test.go::TestFingerprint_RotatesAcrossWindows`, `internal/worker/worker_test.go::TestProcessRecord_SessionFingerprintFallback`.
- **FR-005**: A client-supplied user ID MUST be SHA-256 hashed before storage. Issues: none. Tests: `internal/enrich/enrich_test.go::TestHashUserID`, `internal/worker/worker_test.go::TestProcessRecord_UserIDHashed`.
- **FR-006**: The worker MUST extract UTM parameters from the event URL, and UTM values in the payload MUST override those from the URL. Issues: none. Tests: `internal/worker/worker_test.go::TestProcessRecord_UTMFromURL`, `internal/worker/worker_test.go::TestProcessRecord_UTMPayloadOverridesURL`, `internal/enrich/enrich_test.go::TestExtractUTM`.
- **FR-007**: The worker MUST parse the user agent into browser, operating system and device type. Issues: none. Tests: `internal/enrich/enrich_test.go::TestParseUA_Browsers`, `internal/enrich/enrich_test.go::TestParseUA_OS`, `internal/enrich/enrich_test.go::TestParseUA_DeviceTypes`.
- **FR-008**: The worker MUST reduce a referrer URL to its domain. Issues: none. Tests: `internal/enrich/enrich_test.go::TestExtractReferrerDomain`.
- **FR-014**: The ingest handler MUST answer `429` when its in-memory queue (32768 records) is full. Issues: none. [inferred]
- **FR-015**: Events older than the retention window (default 90 days, `FUNNELBARN_EVENT_RETENTION_DAYS`) MUST be deleted by the maintenance pass. Issues: none. Tests: `internal/repository/store_test.go::TestStore_PurgeOldEvents` (details in spec 005).
- **FR-019**: A replayed event with an already stored ingest ID MUST NOT be stored twice. Issues: none. Tests: `cmd/funnelbarn/ingest_apply_test.go::TestIngestApply_StoresOnceOnRedelivery`.

### Dashboard and funnels

- **FR-009**: The dashboard endpoint `GET /api/v1/projects/{id}/dashboard` MUST return event count, session count, top pages, top referrers, a time series and bounce rate for the requested range. Issues: none. Tests: `internal/api/handlers_test.go::TestHandleDashboard_Success`, `internal/api/handlers_test.go::TestHandleDashboard_TimeRanges`, `internal/repository/store_test.go::TestStore_BounceRate`, `internal/repository/store_test.go::TestStore_TopPages`.
- **FR-010**: The dashboard MUST report top browsers and top UTM sources, mediums and campaigns. Issues: none. Tests: `internal/repository/store_test.go::TestStore_TopBrowsers`, `internal/repository/store_test.go::TestStore_TopUTMSources`, `internal/repository/store_test.go::TestStore_TopUTMMediums`, `internal/repository/store_test.go::TestStore_TopUTMCampaigns`.
- **FR-011**: The system MUST offer funnel create, list, update and delete under `/api/v1/projects/{id}/funnels`, with ordered steps that each match an event name. Issues: none. Tests: `internal/api/funnels_test.go::TestHandleUpdateFunnel_NoSteps_Returns422`, `internal/api/funnels_test.go::TestHandleUpdateFunnel_EmptyStepEventName_Returns422`, `internal/repository/store_test.go::TestStore_CreateFunnel_WithSteps`.
- **FR-012**: `GET /api/v1/projects/{id}/funnels/{fid}/analysis` MUST return per-step counts for the requested date range, with steps completed in order within the funnel's conversion window. Issues: none. Tests: `internal/repository/funnels_user_scope_test.go::TestAnalyzeFunnel_UserScopeOrderedWithinWindow`, `internal/repository/events_test.go::TestAnalyzeFunnel_EmptyReturnsNoError`.
- **FR-013**: A funnel step MUST be able to filter on event properties, and segment queries MUST be scoped to the project. Issues: none. Tests: `internal/repository/events_test.go::TestAnalyzeFunnel_WithEqSegment`, `internal/repository/events_test.go::TestAnalyzeFunnel_SegmentJoinIsProjectScoped`.

### Auth, distribution and operations

- **FR-020**: Admin login with username and password MUST set a signed session cookie, reject a wrong password, and protect state-changing requests with a CSRF token keyed by the session secret. Issues: none. Tests: `internal/api/api_test.go::TestHandleLogin_Success`, `internal/api/api_test.go::TestHandleLogin_WrongPassword`, `internal/auth/auth_test.go::TestCSRFToken_KeyedBySessionSecret`.
- **FR-021**: API keys MUST be created with a scope from `full`, `ingest`, `analytics:read`, `flags:read` and `flags:write`, and an invalid scope MUST be refused. Issues: none. Tests: `internal/api/api_test.go::TestHandleCreateAndDeleteAPIKey`, `internal/api/api_test.go::TestHandleCreateAPIKey_InvalidScope`, `internal/api/analytics_tokens_test.go::TestCreateAPIKey_AcceptsAnalyticsReadScope`.
- **FR-022**: `GET /api/v1/health` MUST answer without authentication and report a database failure. Issues: none. Tests: `internal/api/api_test.go::TestHandleHealth`, `internal/api/api_test.go::TestHandleHealth_DBDown`.
- **FR-016**: The repository MUST ship SDKs for JavaScript (`sdks/js`), Go (`sdks/go`) and Python (`sdks/python`); the JavaScript SDK MUST flush its batch every 5 seconds by default. Issues: [#326](https://github.com/webwiebe/funnelbarn/issues/326). [inferred]
- **FR-017**: The binary MUST offer the subcommands `version`, `worker-once`, `replay-dead-letter`, `user`, `project` and `apikey`. Issues: none. [inferred]
- **FR-018**: Distribution MUST cover container images (`deploy/docker`), a Debian package with a post-install script (`deploy/deb`), a Homebrew formula and per-environment Kubernetes manifests (`deploy/k8s/{testing,staging,production}`). Issues: none. [inferred]
- **FR-023**: The server MUST report its own errors to BugBarn when a self-reporting endpoint and key are configured. Issues: none. [inferred]

---

## Non-Functional Requirements

| ID | Category | Requirement |
|----|----------|-------------|
| NFR-001 | Durability | No accepted event (202) is lost on a crash; the spool and cursor persist it |
| NFR-002 | Security | API keys are stored as hashes; the plaintext is shown once at creation |
| NFR-003 | Ops | Configuration through environment variables (`FUNNELBARN_*`) |
| NFR-004 | Ops | Structured JSON logging through `slog` |

The performance, memory and availability targets of the first draft (ingest p99 under 5 ms, 256 MB RAM, dashboard under 500 ms at 10M events) have no test or gate in the tree and are not claims of this spec.

---

## Non-Goals

- A paid hosted cloud version.
- Ad network conversion API integrations.
- Native mobile SDKs (iOS, Android).
- Horizontal write scaling: SQLite has one writer.

## Drift

| Claim | Source | Tree |
|---|---|---|
| No IP address is stored in any table | Design Principles, Story 3, NFR-006 | `sessions` has an `ip` column (`internal/repository/migrations/00010_session_geo.sql`, kept by `00034_sessions_composite_key.sql`) with geo fields, and `internal/api/anonymize.go` erases them on request; the anonymized /24 prefix feeds only the session hash |
| Session ID is a hash of IP prefix and user agent | Story 3 | The hash also includes project slug, environment and a 30-minute window (`internal/session/fingerprint.go`) |
| `DELETE /api/v1/projects/{id}/sessions/{hash}` erases a session's events | Story 3, FR-035 | No such route in `internal/api/server.go`; `anonymize.go` clears geo and IP fields of sessions instead |
| Weekly email digest, SMTP delivery and threshold alert rules | Stories 6 and 15, FR-036 to FR-039 | No SMTP, digest or alert-rule code under `internal/` or `cmd/` |
| CSV export `GET /api/v1/projects/{id}/export` | Story 14, FR-034 | No such route and no `text/csv` response in the tree |
| UTM attribution report with first-touch and last-touch models and campaign comparison | Story 7, FR-031 | The dashboard lists top UTM sources, mediums and campaigns; no attribution model or comparison endpoint |
| Cohort and retention curves, real-time SSE stream | FR-051, FR-052, FR-054 | Not present; the only `text/event-stream` use is the MCP test |
| Rate limit per API key | FR-058 | Limiters are per client IP and route group (`FUNNELBARN_LOGIN_RATE_*`, `FUNNELBARN_API_RATE_*`; spec 013) |
| Django middleware helper | Story 11, FR-060 | No Django code in `sdks/python` |
| Session tokens are HMAC-signed | NFR-010 | HMAC is used for the CSRF token and webhook signatures (`internal/auth/auth.go`, `internal/ghwebhook`); session cookie signing not established |
| `FUNNELBARN_RETENTION_DAYS` configures retention | Story 13 | The variable is `FUNNELBARN_EVENT_RETENTION_DAYS`, default 90 (`internal/config/config.go`) |
| Geo-IP lookup is a future item | FR-050 | `internal/geoip` exists and enriches sessions |
| Multi-user roles and OpenAPI | FR-056, FR-057 | OIDC login exists (`internal/api/oidc.go`); OpenAPI contract tests exist (`internal/api/contracts_test.go`); role model not established |
| Promised "single binary, no Redis" | Design Principles | Production and staging use a Valkey queue between readers and the writer (spec 012) |
| Litestream replication gives under 1 s data loss | NFR-008 | `deploy/docker/litestream.yml` exists; the replication window is not established |
| Features outside this spec | n/a | The tree also holds feature flags, A/B tests, session recording, an MCP server, agents, canonical overview funnels and branded ingest hosts, which no spec in this folder covers except 011 (recording) |

## Proof contract

- [ ] A pageview posted to a running server returns `202` and appears in the dashboard counts after the worker runs.
- [ ] Two events from the same IP prefix, user agent, project and environment inside one 30-minute window share a session ID (`internal/session/fingerprint_test.go::TestFingerprint_StableWithinWindow`).
- [ ] A user ID sent by a client never appears in plaintext in the `events` table (`internal/worker/worker_test.go::TestProcessRecord_UserIDHashed`).
- [ ] A funnel with three steps returns non-increasing step counts for a seeded dataset.
- [ ] `GET /api/v1/health` answers `200` without credentials (`internal/api/api_test.go::TestHandleHealth`).

## Sources

- `internal/ingest/handler.go`, `internal/worker/worker.go`, `internal/session/fingerprint.go`, `internal/enrich/enrich.go`
- `internal/api/server.go`, `internal/api/funnels.go`, `internal/api/dashboard.go`, `internal/api/anonymize.go`
- `internal/repository/migrations/00010_session_geo.sql`, `internal/config/config.go`
- `sdks/js/src/index.ts`, `deploy/`
