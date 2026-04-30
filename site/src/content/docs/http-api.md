---
title: HTTP API Reference
description: Complete reference for all FunnelBarn HTTP API endpoints.
order: 7
---

# HTTP API Reference

All API endpoints are prefixed with `/api/v1`. Dashboard endpoints require an active session cookie (obtained via `POST /api/v1/login`). The ingest endpoint requires an API key header.

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

Returns aggregate analytics for a project.

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

---

## Projects

### `GET /api/v1/projects`

List all projects.

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

List all funnels for a project.

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

Run funnel analysis. Returns conversion rates for each step.

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

Scopes: `ingest` (events only) or `full` (events + dashboard API).

### `DELETE /api/v1/apikeys/:kid`

Revoke an API key.
