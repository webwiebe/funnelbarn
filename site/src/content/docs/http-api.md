---
title: HTTP API Reference
description: Complete reference for all FunnelBarn HTTP API endpoints.
order: 7
---

# HTTP API Reference

All API endpoints are prefixed with `/api/v1`. Dashboard endpoints require an active session cookie (obtained via `POST /api/v1/login`). The ingest endpoint requires an API key header. A subset of read-only endpoints also accepts a project-scoped API key, so a script or scheduled job can read a project's numbers without a browser session — see [Reading analytics with an API key](#reading-analytics-with-an-api-key).

## Authentication

### Session authentication (dashboard)

```bash
# Login
curl -X POST http://localhost:8080/api/v1/login \
  -H "Content-Type: application/json" \
  -c cookies.txt \
  -d '{"username": "admin", "password": "changeme"}'

# Subsequent requests — pass the session cookie
curl http://localhost:8080/api/v1/me -b cookies.txt
```

### API key authentication (ingest)

All ingest requests require the `x-funnelbarn-api-key` header:

```
x-funnelbarn-api-key: your-api-key
```

### API key scopes

Keys are issued per project from **Settings → API Keys**. The scope decides what the key may do:

| Scope | Grants |
|---|---|
| `ingest` | Send events. Cannot read or change anything. |
| `analytics:read` | Read this project's events, event counts, funnels and funnel-step conversion. Read-only. |
| `flags:read` | Read this project's feature flags. |
| `flags:write` | Read and update (not create or delete) this project's feature flags. |
| `full` | Everything, for this project. |

Scopes do not imply each other across features. `flags:write` implies `flags:read`, but an `analytics:read` key gets `403` on the flag routes and a `flags:*` key gets `403` on the analytics routes — a token that toggles an outbound-email gate has no business reading the event stream.

Two things are refused by design:

- **The instance-wide `FUNNELBARN_API_KEY`** resolves to no project, which would make it a master key for every project on the instance. Scoped routes answer `401` for it; use a project key from the settings page.
- **A key aimed at another project** answers `403`, even for a route the scope otherwise allows.

### Reading analytics with an API key

These endpoints accept either a session cookie or an `analytics:read` (or `full`) key in `x-funnelbarn-api-key`:

| Method | Path | Returns |
|---|---|---|
| `GET` | `/api/v1/projects` | Just the project the key is scoped to |
| `GET` | `/api/v1/projects/:id/dashboard` | Aggregate stats over a range |
| `GET` | `/api/v1/projects/:id/events` | Paginated raw events |
| `GET` | `/api/v1/projects/:id/event-names` | Distinct event names |
| `GET` | `/api/v1/projects/:id/event-counts` | Per-event counts over a range |
| `GET` | `/api/v1/projects/:id/funnels` | The project's funnels |
| `GET` | `/api/v1/projects/:id/funnels/:fid/analysis` | Per-step conversion |

A weekly readout, end to end:

```bash
KEY=your-analytics-read-key
BASE=https://funnelbarn.example.com

# Resolve the project the key belongs to — the list is narrowed to it.
PROJECT=$(curl -s -H "x-funnelbarn-api-key: $KEY" "$BASE/api/v1/projects" \
  | jq -r '.projects[0].id')

# Per-event counts for the last 7 days.
curl -s -H "x-funnelbarn-api-key: $KEY" \
  "$BASE/api/v1/projects/$PROJECT/event-counts?range=7d" | jq '.events'

# Step conversion for a funnel. Note this endpoint takes from/to only —
# the `range` shorthand is an event-counts and dashboard convenience.
FUNNEL=$(curl -s -H "x-funnelbarn-api-key: $KEY" \
  "$BASE/api/v1/projects/$PROJECT/funnels" | jq -r '.funnels[0].id')
FROM=$(date -u -d '7 days ago' +%Y-%m-%dT%H:%M:%SZ)   # BSD date: -v-7d
curl -s -H "x-funnelbarn-api-key: $KEY" \
  "$BASE/api/v1/projects/$PROJECT/funnels/$FUNNEL/analysis?from=$FROM"
```

---

## Health

### `GET /api/v1/health`

Returns `200 OK` when the server is running. No authentication required.

```json
{"ok": true}
```

---

## Ingest

### `POST /api/v1/events`

Ingests a single analytics event. Returns `202 Accepted` immediately — the event is queued for processing.

**Headers:**

| Header | Required | Description |
|---|---|---|
| `x-funnelbarn-api-key` | Yes | Ingest or full-scope API key |
| `x-funnelbarn-project` | No | Project slug override (defaults to the key's project) |
| `Content-Type` | Yes | Must be `application/json` |

**Request body:**

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | string | Yes | Event name, e.g. `page_view`, `signup`, `purchase` |
| `url` | string | No | Full URL where the event occurred |
| `referrer` | string | No | HTTP Referer value |
| `session_id` | string | No | Client-managed session ID |
| `user_id` | string | No | User identifier — hashed server-side |
| `user_agent` | string | No | Browser or client user agent string |
| `timestamp` | string | No | ISO 8601 timestamp (server time if omitted) |
| `properties` | object | No | Arbitrary JSON key-value pairs |
| `utm_source` | string | No | UTM source (also auto-extracted from `url`) |
| `utm_medium` | string | No | UTM medium |
| `utm_campaign` | string | No | UTM campaign |
| `utm_term` | string | No | UTM term |
| `utm_content` | string | No | UTM content |

**Example:**

```bash
curl -X POST http://localhost:8080/api/v1/events \
  -H "x-funnelbarn-api-key: mysecret" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "purchase",
    "url": "https://example.com/checkout",
    "user_id": "user-123",
    "properties": {
      "plan": "pro",
      "amount": 49.99,
      "currency": "USD"
    }
  }'
```

**Response:**

```json
{"accepted": true, "ingestId": "a3f2c1..."}
```

---

## Dashboard

### `GET /api/v1/projects/:id/dashboard`

Returns aggregate analytics for a project. Accepts a session or an `analytics:read` key.

**Query parameters:**

| Parameter | Values | Description |
|---|---|---|
| `range` | `24h`, `7d`, `30d` | Preset time range (default: `30d`) |
| `from` | RFC3339 timestamp | Custom start time (overrides `range`) |
| `to` | RFC3339 timestamp | Custom end time (overrides `range`) |

**Response fields:**

| Field | Description |
|---|---|
| `total_events` | Total event count |
| `unique_sessions` | Unique session count |
| `bounce_rate` | Fraction of sessions with exactly one event |
| `avg_events_per_session` | Average number of events per session |
| `top_pages` | Top 10 pages by event count |
| `top_referrers` | Top 10 referrers |
| `top_browsers` | Top 5 browsers |
| `device_types` | Distribution across desktop / mobile / tablet |
| `top_event_names` | Top 10 event names |
| `top_utm_sources` | Top 5 UTM sources |
| `events_time_series` | Daily event counts |
| `sessions_time_series` | Daily unique session counts |

### `GET /api/v1/projects/:id/events`

Returns a paginated list of raw events.

**Query parameters:**

| Parameter | Default | Description |
|---|---|---|
| `limit` | `50` | Max events to return (1–500) |
| `offset` | `0` | Pagination offset |

### `GET /api/v1/projects/:id/event-counts`

Returns a count per event name over a date range — the whole catalog, not the dashboard's top ten, and without the dozen other aggregates wrapped around it. Accepts an `analytics:read` key.

**Query parameters:**

| Parameter | Default | Description |
|---|---|---|
| `range` | `30d` | Preset time range: `24h`, `7d`, `30d` |
| `from` | 30 days ago | RFC3339 start time (overrides `range`) |
| `to` | now | RFC3339 end time (overrides `range`) |
| `limit` | `100` | Max distinct event names to return (1–500) |
| `environment` | all | Restrict to one environment |

Unlike the dashboard endpoint, unparseable input is rejected with `400` rather than silently answered with the default window — a readout that asked for last week and quietly got last month reports the wrong number with no way to notice.

```json
{
  "project_id": "proj-123",
  "from": "2026-08-29T09:00:00Z",
  "to": "2026-09-05T09:00:00Z",
  "environment": "",
  "limit": 100,
  "total_events": 1284,
  "events": [
    {"name": "page_view", "count": 941},
    {"name": "first_run_started", "count": 287},
    {"name": "first_run_completed", "count": 56}
  ]
}
```

---

## Projects

### `GET /api/v1/projects`

List all projects. With an `analytics:read` key instead of a session, the list is narrowed to the one project that key is scoped to.

### `POST /api/v1/projects`

Create a project.

```json
{"name": "My Website", "slug": "my-website"}
```

### `PUT /api/v1/projects/:id`

Update a project's name or slug.

### `DELETE /api/v1/projects/:id`

Delete a project and all its data.

### `POST /api/v1/projects/:id/approve`

Approve a pending project (admin action after self-service setup).

---

## Funnels

### `GET /api/v1/projects/:id/funnels`

List all funnels for a project. Accepts a session or an `analytics:read` key.

### `POST /api/v1/projects/:id/funnels`

Create a funnel.

```json
{
  "name": "Signup Funnel",
  "description": "From landing page to activation",
  "steps": [
    {"event_name": "page_view", "filters": {"url": "https://example.com/"}},
    {"event_name": "signup_click"},
    {"event_name": "signup_completed"}
  ]
}
```

### `PUT /api/v1/projects/:id/funnels/:fid`

Update a funnel's name, description, or steps.

### `DELETE /api/v1/projects/:id/funnels/:fid`

Delete a funnel.

### `GET /api/v1/projects/:id/funnels/:fid/analysis`

Run funnel analysis. Returns conversion rates for each step. Accepts a session or an `analytics:read` key.

**Query parameters:**

| Parameter | Description |
|---|---|
| `from` | RFC3339 start time (default: 30 days ago) |
| `to` | RFC3339 end time (default: now) |
| `segment` | Preset segment: `all`, `mobile`, `desktop`, `tablet`, `logged_in`, `not_logged_in`, `new_visitor`, `returning` |

### `GET /api/v1/projects/:id/funnels/:fid/segments`

Returns available segment dimension values for a project (browsers, device types, UTM sources, etc.).

---

## A/B Tests

### `GET /api/v1/projects/:id/abtests`

List all A/B tests for a project.

### `POST /api/v1/projects/:id/abtests`

Create an A/B test.

```json
{
  "name": "Hero CTA colour",
  "control_filter": "variant=control",
  "variant_filter": "variant=blue-cta",
  "conversion_event": "signup_completed"
}
```

### `GET /api/v1/projects/:id/abtests/:abid/analysis`

Run A/B test analysis. Returns sample sizes, conversion counts and rates, z-score, and a `significant` boolean (two-proportion z-test at 95% confidence).

**Query parameters:**

| Parameter | Values | Description |
|---|---|---|
| `range` | `24h`, `7d`, `30d` | Preset time range (default: `30d`) |
| `from` | RFC3339 | Custom start time |
| `to` | RFC3339 | Custom end time |

---

## Sessions

### `GET /api/v1/projects/:id/sessions`

List sessions with pagination.

### `GET /api/v1/projects/:id/sessions/active`

Returns the count of sessions active in the last 5 minutes.

---

## API Keys

### `GET /api/v1/apikeys`

List all API keys.

### `POST /api/v1/apikeys`

Create an API key.

```json
{"name": "Browser SDK key", "scope": "ingest", "project_id": "proj-123"}
```

Scopes: `ingest`, `analytics:read`, `flags:read`, `flags:write` or `full` — see [API key scopes](#api-key-scopes). Anything else is refused rather than stored and silently treated as no access.

The plaintext key is returned once, in the create response, and never again.

### `DELETE /api/v1/apikeys/:kid`

Revoke an API key.
