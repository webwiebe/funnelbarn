# Feature Specification: Analytics & Funnel Tracking Foundation

**Feature Branch**: `001-analytics-funnel`
**Created**: 2026-04-28
**Status**: In Progress

## Problem Statement

Commercial analytics tools (Mixpanel, Amplitude, Fathom, Plausible) impose two significant costs:

1. **Financial cost**: Pricing scales with event volume. A high-traffic site can easily spend $500–5,000/month.
2. **Privacy cost**: User behavior data is sent to third-party infrastructure. This conflicts with GDPR, user trust, and data sovereignty goals.

Self-hosted options exist (Umami, Matomo, PostHog) but are either too minimal, require heavy infrastructure (PostgreSQL, Redis, Kafka), or are difficult to operate on modest hardware.

## Solution

Trailpost is a single-binary, self-hostable analytics and funnel tracking server. The operator deploys one binary (or Docker container), points a domain at it, and keeps all their data locally. It replaces Mixpanel/Amplitude for product teams who value data ownership over SaaS convenience.

### Design Principles

- **Privacy-first**: No cross-site tracking, no third-party calls, no cookies required for session tracking.
- **Simple operations**: Single binary, SQLite, Litestream replication. No PostgreSQL, no Redis, no Kafka.
- **High-throughput ingest**: Durable local spool decouples ingest latency from database writes.
- **Self-reporting**: Reports its own errors to BugBarn for operational visibility.

## User Stories

### Story 1 — As a developer, I want to self-host my analytics in 5 minutes

**Acceptance criteria:**
- `docker run -e TRAILPOST_API_KEY=secret -p 8080:8080 ghcr.io/webwiebe/trailpost/service` starts a working server
- `POST /api/v1/events` with API key accepts a pageview event
- `GET /api/v1/health` returns `{"status":"ok"}`

### Story 2 — As a product manager, I want to see which pages drive conversions

**Acceptance criteria:**
- Dashboard shows top pages, top referrers, UTM attribution breakdown
- Time-series charts show daily event and session trends
- Funnel analysis shows conversion rates and drop-off per step

### Story 3 — As a privacy-conscious company, I want analytics without storing PII

**Acceptance criteria:**
- Session fingerprinting uses IP + UA hash with IP prefix anonymization
- User IDs are SHA256-hashed before storage
- No cookies are set by the server
- No third-party scripts are loaded

### Story 4 — As a developer, I want to instrument my JavaScript app in one script tag

**Acceptance criteria:**
- `@trailpost/js` auto-detects URL and referrer from `window.location`
- Generates a session ID in localStorage with 30-min idle timeout
- Batches events and flushes on a 5-second timer or `beforeunload`
- Works in browsers and Node.js 18+

### Story 5 — As a developer, I want to define conversion funnels

**Acceptance criteria:**
- `POST /api/v1/projects/{id}/funnels` creates a funnel with named steps
- `GET /api/v1/projects/{id}/funnels/{fid}/analysis` returns step-by-step conversion rates
- The dashboard shows a visual funnel with percentage bars and drop-off labels

## Functional Requirements

- **FR-001**: Authenticated ingest endpoint at `POST /api/v1/events`
- **FR-002**: Ingest MUST durably enqueue before responding (no DB writes in request path)
- **FR-003**: Background worker processes spool into SQLite
- **FR-004**: Session fingerprinting: IP prefix + UA hash, no cookies
- **FR-005**: User IDs hashed (SHA256) before storage
- **FR-006**: UTM parameters extracted from event URL or payload
- **FR-007**: User-agent parsed into browser, OS, device type
- **FR-008**: Referrer domain extracted from full referrer URL
- **FR-009**: Dashboard API returns: event count, session count, top pages, top referrers, time series
- **FR-010**: Funnel CRUD + analysis endpoints
- **FR-011**: Admin login via username/password (bcrypt)
- **FR-012**: API keys with `full` and `ingest` scopes
- **FR-013**: Docker image + Litestream replication
- **FR-014**: APT package and Homebrew formula
- **FR-015**: JavaScript SDK (browser + Node)
- **FR-016**: Go SDK
- **FR-017**: Python SDK stub
- **FR-018**: K8s manifests for testing/staging/production
- **FR-019**: GitHub Actions CI/CD (build+test, binary release, production deploy)

## Non-Functional Requirements

- **NFR-001**: Ingest p95 < 10ms on modest hardware under normal load
- **NFR-002**: Single-node deployment fits in 256MB RAM
- **NFR-003**: No external runtime dependencies beyond Go stdlib + sqlite3
- **NFR-004**: Explicit backpressure (429) when spool is full — no unbounded memory growth
- **NFR-005**: Operable without any SaaS dependencies

## Key Entities

- **Project**: A tracked website/application with its own API keys and data
- **API Key**: Secret token (`full` or `ingest` scope) for authenticated access
- **Event**: One analytics occurrence (pageview or custom) with properties
- **Session**: Group of events from the same anonymous fingerprint within 30 min
- **Funnel**: Ordered sequence of event names; used to analyze conversion
- **Funnel Step**: One event name (with optional property filters) in a funnel
- **User**: Admin human who logs into the dashboard

## Success Criteria

- **SC-001**: 10,000 events ingested without measurable latency increase
- **SC-002**: Dashboard loads with correct stats for a seeded project
- **SC-003**: Funnel analysis shows correct conversion rates in integration test
- **SC-004**: Docker Compose stack starts with one command
- **SC-005**: APT and Homebrew installs produce a working binary
- **SC-006**: JavaScript SDK auto-tracks page views in a minimal HTML page

## Assumptions

- Single-tenant for v1 (one admin user, multiple projects)
- Cookie-free session tracking is a hard requirement
- SQLite is sufficient for up to ~10M events/month per instance
- Geo-IP lookup is out of scope for v1 (country_code field reserved for later)
