# Feature Specification: FunnelBarn — Analytics & Funnel Tracking

**Feature Branch**: `001-analytics-funnel`
**Created**: 2026-04-28
**Status**: In Progress

---

## Problem Statement

### The Cost of Commercial Analytics

Product teams that want to understand their users face an uncomfortable choice:

1. **Financial cost at scale**: Mixpanel and Amplitude price per event volume. A high-traffic SaaS product can easily spend $500–$5,000/month on analytics alone — money that compounds year over year and scales with growth, not with value delivered.

2. **Privacy and compliance burden**: Sending user behavior data to US-hosted third-party servers conflicts directly with GDPR, Swiss DPA, and a growing number of national privacy regulations. The legal risk is real: EU regulators have fined companies for using Google Analytics without appropriate data processing agreements.

3. **Vendor lock-in**: Analytics data trapped in Mixpanel's data model cannot be migrated cleanly. A decade of behavioral data becomes a hostage.

4. **Trust erosion**: Privacy-conscious users — increasingly the majority in certain verticals — object to third-party tracking scripts. Ad blockers block analytics, leading to systematic data loss and biased decision-making.

Self-hosted alternatives exist (Umami, Matomo, PostHog) but carry their own problems: heavy infrastructure requirements (PostgreSQL, Redis, Kafka), complex maintenance burden, or feature sets too minimal for conversion funnel work.

---

## Solution

**FunnelBarn** is a self-hosted, single-binary analytics and funnel tracking server. The operator deploys one binary (or one Docker container), points a domain at it, and immediately owns all their analytics data on their own infrastructure.

FunnelBarn replaces Mixpanel, Amplitude, and Fathom for product teams who value data ownership over SaaS convenience.

### Design Principles

1. **Privacy-first**: No cross-site tracking. No cookies required for session tracking. No third-party script dependencies. No IP addresses stored. GDPR-compliant by default.

2. **Simple operations**: Single Go binary, SQLite database, optional Litestream replication to S3-compatible storage. No PostgreSQL, no Redis, no Kafka, no message brokers.

3. **High-throughput ingest**: A durable local spool decouples ingest latency from database writes. The HTTP handler returns in under 1ms; a background worker handles enrichment and persistence asynchronously.

4. **Developer-friendly distribution**: Available via Docker, APT, Homebrew, and a single binary download. SDKs for JavaScript, Go, and Python.

5. **Self-reporting**: FunnelBarn reports its own errors to BugBarn for operational visibility when configured.

---

## Target Users

### Primary Personas

**The Indie Hacker** builds and runs a SaaS product solo or with a tiny team. They want product analytics without a $200/month bill on $500 MRR. They have a VPS or k8s cluster already running.

**The Privacy Officer** at a European SaaS company is responsible for GDPR compliance. They need a documented, auditable trail of what data is collected, where it lives, and how long it's kept. US-hosted analytics tools create unacceptable legal risk.

**The SaaS Founder** has a product with $10k+ MRR and is tired of analytics bills that scale faster than revenue. They want funnel analysis, UTM attribution, and session tracking — but not at Mixpanel enterprise pricing.

**The Backend Developer** maintains a multi-service platform. They want to instrument server-side events (API calls, webhook deliveries, job completions) without integrating a heavyweight SaaS SDK.

**The Marketer** runs campaigns and needs to know which UTM sources convert. They don't care about the infrastructure, but they need reliable funnel data and attribution reports.

---

## User Stories

### Story 1 — As a developer, I want to self-host my analytics in under 5 minutes

**Scenario**: I discover FunnelBarn and want to evaluate it quickly.

**Acceptance criteria**:
- `docker run -e FUNNELBARN_API_KEY=secret -p 8080:8080 ghcr.io/webwiebe/funnelbarn/service` starts a working server in under 60 seconds
- `POST /api/v1/events` with a valid API key accepts a pageview event and returns `202 Accepted`
- `GET /api/v1/health` returns `{"status":"ok"}` without authentication
- The browser dashboard is accessible at port 3000 (web container) or port 8080 with SPA routing

### Story 2 — As a product manager, I want to see which pages drive conversions

**Scenario**: I want a daily summary of product engagement.

**Acceptance criteria**:
- Dashboard shows: total events, unique sessions, bounce rate, avg events/session
- Top pages table shows URL + view count, sorted by views descending
- Top referrers table shows referring domain + visit count
- UTM source/medium/campaign breakdown is available
- Time-series chart shows daily events and sessions over the selected date range
- Date range selector supports last 7d, 30d, 90d, and custom range

### Story 3 — As a privacy officer, I want analytics without storing PII or IP addresses

**Scenario**: I need to demonstrate GDPR compliance to our DPO.

**Acceptance criteria**:
- No IP addresses are stored in any database table
- Session fingerprinting uses a one-way hash of (IP prefix + User-Agent)
- IPv4 addresses are anonymized to /24 before hashing; IPv6 to /48
- User IDs provided by clients are SHA256-hashed before storage; the plaintext is never persisted
- No cookies are set by the server; session tracking is cookie-free by design
- No third-party scripts are loaded by the tracking snippet or dashboard
- A right-to-erasure endpoint (`DELETE /api/v1/projects/{id}/sessions/{hash}`) deletes all events for a session hash

### Story 4 — As a developer, I want to instrument my JavaScript app in one script tag

**Scenario**: I'm adding analytics to a static marketing site.

**Acceptance criteria**:
- `@funnelbarn/js` is installable via npm or usable via CDN script tag
- `FunnelBarnClient.page()` auto-detects URL and referrer from `window.location` and `document.referrer`
- Session IDs are persisted in localStorage with a 30-minute idle timeout
- Events are batched and flushed every 5 seconds or on `beforeunload`
- The SDK works in browsers and Node.js 18+ without polyfills
- Failed sends are silently discarded (best-effort) — the SDK never throws

### Story 5 — As a developer, I want to define multi-step conversion funnels

**Scenario**: I want to understand where users drop off in my signup flow.

**Acceptance criteria**:
- `POST /api/v1/projects/{id}/funnels` creates a funnel with ordered steps, each step matching an event name
- `GET /api/v1/projects/{id}/funnels/{fid}/analysis` returns per-step entry count, conversion rate, and drop-off percentage
- The web dashboard renders a visual funnel with horizontal progress bars and drop-off labels
- Funnel steps can optionally filter on event properties (e.g., only events where `plan == "pro"`)
- Analysis respects a configurable date range

### Story 6 — As a developer, I want to receive a weekly email digest of my analytics

**Scenario**: I don't log into the dashboard daily, but want a summary.

**Acceptance criteria**:
- A SMTP configuration (`FUNNELBARN_SMTP_*`) enables email delivery
- A weekly digest is sent every Monday morning with: total events, unique sessions, top 5 pages, top 3 referrers, top 3 UTM sources
- The digest is rendered as a clean HTML email
- A webhook alternative delivers the same summary as a JSON POST to a configured URL

### Story 7 — As a marketer, I want to compare UTM campaigns against each other

**Scenario**: I ran three campaigns last month and want to know which drove the most signups.

**Acceptance criteria**:
- The attribution report shows events and sessions broken down by utm_source, utm_medium, utm_campaign
- A campaign comparison view allows selecting two campaigns and showing conversion rate side-by-side
- First-touch and last-touch attribution models are both available

### Story 8 — As a founder, I want to install FunnelBarn via Homebrew on my Mac for local testing

**Scenario**: I want to run FunnelBarn locally without Docker.

**Acceptance criteria**:
- `brew tap webwiebe/funnelbarn && brew install funnelbarn` installs a working binary
- `funnelbarn version` prints the version
- `funnelbarn project create --name "Test"` creates a project with a local SQLite database
- The binary works on both Apple Silicon (arm64) and Intel (amd64) Macs

### Story 9 — As a sysadmin, I want to install FunnelBarn as a systemd service on Debian

**Scenario**: I manage a Debian server and want FunnelBarn running as a proper system service.

**Acceptance criteria**:
- `apt install funnelbarn` installs the binary, systemd service, and example config
- `systemctl enable --now funnelbarn` starts the service
- The service runs as the `funnelbarn` system user (not root)
- Configuration lives at `/etc/funnelbarn/funnelbarn.conf`
- Data lives at `/var/lib/funnelbarn/`

### Story 10 — As a developer, I want to instrument a Go backend service

**Scenario**: I have a Go API server and want to track server-side events.

**Acceptance criteria**:
- `go get github.com/wiebe-xyz/funnelbarn-go` installs the SDK
- `funnelbarn.Init(options)` initializes a background queue
- `funnelbarn.Track("api_request", map[string]any{"route": "/users"})` enqueues an event
- `funnelbarn.Shutdown(5 * time.Second)` flushes and stops the background goroutine
- The SDK is safe for concurrent use from multiple goroutines

### Story 11 — As a developer, I want to instrument a Python/Django application

**Scenario**: I have a Django application and want to track server-side events.

**Acceptance criteria**:
- `pip install funnelbarn` installs the SDK
- `FunnelBarnClient` is thread-safe and initializes a background worker
- A Django middleware helper automatically tracks page requests
- A FastAPI dependency helper is provided as an example

### Story 12 — As a developer, I want to track A/B test variants in my analytics

**Scenario**: I'm running an A/B test and want to see conversion rates per variant.

**Acceptance criteria**:
- Events can include a `variant` property in their properties object
- The dashboard can filter events by property value
- The funnel analysis endpoint accepts a property filter parameter for variant comparison

### Story 13 — As a developer, I want to set a data retention policy

**Scenario**: I don't want events older than 1 year to accumulate in my database.

**Acceptance criteria**:
- `FUNNELBARN_RETENTION_DAYS` configures automatic deletion of events older than N days
- Deletion runs as a background job (nightly)
- The log records how many events were deleted in each run

### Story 14 — As a developer, I want to export my analytics data as CSV

**Scenario**: I want to run ad-hoc analysis in a spreadsheet or Python notebook.

**Acceptance criteria**:
- `GET /api/v1/projects/{id}/export?format=csv&from=...&to=...` downloads a CSV of events
- The CSV includes all event fields: name, url, referrer, utm fields, properties, occurred_at
- The export is streamed (not buffered in memory)

### Story 15 — As a developer, I want to configure threshold alerts

**Scenario**: I want to be notified if events drop to zero, indicating a tracking outage.

**Acceptance criteria**:
- Alert rules can be defined: "if event count for project X is below N in the last M minutes, fire alert"
- Alert rules can also trigger on upper bounds: "if event count exceeds N/hour"
- Alerts are delivered via email (if SMTP configured) or webhook
- Alert state is tracked: alerts do not re-fire while already active (no notification spam)

---

## Functional Requirements

### Must Have (MVP)

| ID | Requirement |
|----|-------------|
| FR-001 | Authenticated ingest endpoint `POST /api/v1/events` |
| FR-002 | Ingest MUST durably enqueue before responding — no database writes in request path |
| FR-003 | Background worker processes spool NDJSON into SQLite |
| FR-004 | Session fingerprinting: IP prefix + UA hash, no cookies |
| FR-005 | User IDs SHA256-hashed before storage |
| FR-006 | UTM parameters extracted from event URL or payload properties |
| FR-007 | User-agent parsed into browser, OS, device type |
| FR-008 | Referrer domain extracted from full referrer URL |
| FR-009 | Dashboard API: event count, session count, top pages, top referrers, time series, bounce rate |
| FR-010 | Funnel CRUD + analysis endpoints |
| FR-011 | Admin login via username/password (bcrypt, session cookie + CSRF) |
| FR-012 | API keys with `full` and `ingest` scopes, SHA256-hashed |
| FR-013 | Docker image with Litestream replication support |
| FR-014 | APT package (.deb) with systemd service and postinstall script |
| FR-015 | Homebrew formula |
| FR-016 | JavaScript SDK (browser + Node.js 18+) |
| FR-017 | Go SDK |
| FR-018 | Python SDK |
| FR-019 | K8s manifests for testing/staging/production environments |
| FR-020 | GitHub Actions CI/CD (build+test, binary release, production deploy) |
| FR-021 | BugBarn self-error-reporting integration |
| FR-022 | CLI subcommands: user create, project create, apikey create, worker-once |
| FR-023 | Explicit backpressure (429) when ingest queue is saturated |

### Should Have (Near-term)

| ID | Requirement |
|----|-------------|
| FR-030 | Funnel step property filters (filter sessions by event property values) |
| FR-031 | UTM attribution report (by source/medium/campaign) |
| FR-032 | Session-ordered funnel analysis (steps must be completed in order) |
| FR-033 | Configurable data retention period with automatic deletion |
| FR-034 | CSV export of events |
| FR-035 | Right-to-erasure endpoint (delete all events for a session hash) |
| FR-036 | Alert rules: threshold (below/above N events in window) |
| FR-037 | Alert delivery: SMTP email |
| FR-038 | Alert delivery: webhook (POST JSON) |
| FR-039 | Weekly digest: email + webhook |
| FR-040 | Top browsers and device type breakdown in dashboard |
| FR-041 | Top UTM sources in dashboard |

### Could Have (Future)

| ID | Requirement |
|----|-------------|
| FR-050 | Geo-IP lookup (MaxMind GeoLite2, opt-in) |
| FR-051 | Real-time event stream (Server-Sent Events) |
| FR-052 | Cohort analysis (week-over-week retention matrix) |
| FR-053 | A/B test variant comparison in funnel analysis |
| FR-054 | Retention curves (day 1/7/30 retention calculation) |
| FR-055 | Custom dashboards (user-defined metric cards) |
| FR-056 | Multi-user dashboard access (roles: admin, viewer) |
| FR-057 | OpenAPI specification for all endpoints |
| FR-058 | Rate limiting per API key (configurable requests/minute) |
| FR-059 | Pagination cursor on event list (keyset pagination) |
| FR-060 | Django middleware for auto page-view tracking |

### Won't Have (Out of Scope for v1)

| ID | Non-requirement |
|----|----------------|
| FR-100 | Paid hosted SaaS version — FunnelBarn is self-host only |
| FR-101 | Real-time collaboration features |
| FR-102 | Ad network integration (Google Ads conversion import, Facebook CAPI) |
| FR-103 | Horizontal write scaling — SQLite is single-writer by design |
| FR-104 | Heatmaps and session recordings |
| FR-105 | Mobile SDK (iOS, Android) |

---

## Non-Functional Requirements

| ID | Category | Requirement |
|----|----------|-------------|
| NFR-001 | Performance | Ingest p99 < 5ms on modest hardware (2 vCPU, 2GB RAM) under sustained 100 req/s |
| NFR-002 | Performance | Dashboard API responds in < 500ms for projects with up to 10M events |
| NFR-003 | Availability | Single-node deployment; acceptable downtime for restarts (< 30s) |
| NFR-004 | Memory | Single-node deployment fits in 256MB RAM |
| NFR-005 | Dependencies | No external runtime dependencies beyond Go stdlib + sqlite3 (CGO) |
| NFR-006 | Privacy | No IP addresses stored anywhere; fingerprinting is one-way |
| NFR-007 | Durability | No events lost on crash after they have been accepted (202 Accepted) |
| NFR-008 | Durability | Litestream replication provides < 1s data loss window when configured |
| NFR-009 | Security | API keys are stored as SHA256 hashes only; plaintext never persisted |
| NFR-010 | Security | Session tokens are HMAC-signed; not JWTs — no algorithm confusion attacks |
| NFR-011 | GDPR | IP prefix anonymization enabled by default |
| NFR-012 | GDPR | User IDs are one-way hashed; no reversal possible |
| NFR-013 | GDPR | No cross-site tracking; no shared identifiers across instances |
| NFR-014 | Ops | Configuration via environment variables or config file (`funnelbarn.conf`) |
| NFR-015 | Ops | Structured JSON logging throughout (slog) |

---

## Acceptance Criteria by Feature Area

### Ingest

- 10,000 events ingested sequentially without measurable latency regression
- 429 response returned when the in-memory queue is saturated (capacity: 32k records)
- Events survive a server restart without loss (spool file + cursor)
- Duplicate detection via IngestID prevents double-counting on replay

### Dashboard

- Dashboard loads with correct stats for a project with ≥1,000 seeded events
- Date range filtering correctly limits all aggregate queries
- Time series accurately reflects daily event and session counts

### Funnels

- Funnel analysis returns correct step-level conversion rates in an integration test
- Multi-step drop-off percentages sum correctly (each step ≤ previous step count)
- Funnel analysis respects the date range parameter

### Auth

- Admin login with correct credentials sets a signed session cookie
- An expired session returns 401 on protected endpoints
- API key with `ingest` scope is rejected on non-ingest endpoints

### Distribution

- `docker compose up` starts the full stack (service + web) with one command
- `apt install funnelbarn` on Debian/Ubuntu installs a working binary and systemd service
- `brew install funnelbarn` on macOS produces a working binary
- `funnelbarn version` prints the version string in all distribution formats

---

## Out of Scope

- Real-time multi-user collaboration on dashboards
- Paid hosted cloud version
- Ad network conversion API integrations
- Mobile native SDKs (iOS, Android)
- Session recording or heatmaps
- Horizontal write scaling (SQLite is single-writer; that is a feature, not a bug)
