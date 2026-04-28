# Data Model: Trailpost SQLite Schema

## Tables Overview

| Table | Purpose |
|-------|---------|
| `projects` | Tracked websites/applications |
| `api_keys` | Authentication tokens per project |
| `users` | Admin dashboard users |
| `sessions_http` | HTTP session tokens for dashboard login |
| `events` | Analytics events (core data) |
| `sessions` | Aggregated visitor sessions |
| `funnels` | Named conversion funnels |
| `funnel_steps` | Ordered steps within a funnel |

---

## `projects`

Represents one tracked website or application. All events and funnels belong to a project.

| Column | Type | Description |
|--------|------|-------------|
| `id` | TEXT PK | UUID v4 |
| `name` | TEXT | Display name (e.g. "My Website") |
| `slug` | TEXT UNIQUE | URL-safe identifier (e.g. "my-website") |
| `created_at` | DATETIME | Creation timestamp (UTC) |

---

## `api_keys`

Authentication tokens used by SDKs and integrations. Keys are hashed before storage.

| Column | Type | Description |
|--------|------|-------------|
| `id` | TEXT PK | UUID v4 |
| `project_id` | TEXT FK → projects | Owning project |
| `name` | TEXT | Human label (e.g. "production-sdk") |
| `key_hash` | TEXT UNIQUE | SHA256 hex of the plaintext key |
| `scope` | TEXT | `full` (all API access) or `ingest` (events only) |
| `last_used_at` | DATETIME | Last successful authentication |
| `created_at` | DATETIME | Creation timestamp |

**Security note**: Plaintext API keys are shown once at creation and never stored. Only the SHA256 hash is persisted.

---

## `users`

Admin users who log into the web dashboard.

| Column | Type | Description |
|--------|------|-------------|
| `id` | TEXT PK | UUID v4 |
| `username` | TEXT UNIQUE | Login username |
| `password_hash` | TEXT | bcrypt hash of password |
| `created_at` | DATETIME | Creation timestamp |

---

## `sessions_http`

Server-side session tokens for dashboard login (alternative to JWT-only approach).

| Column | Type | Description |
|--------|------|-------------|
| `token_hash` | TEXT PK | SHA256 of session token |
| `user_id` | TEXT FK → users | Authenticated user |
| `expires_at` | DATETIME | Expiry timestamp |

---

## `events`

The core analytics table. Each row is one tracked event occurrence.

| Column | Type | Description |
|--------|------|-------------|
| `id` | TEXT PK | UUID v4 |
| `project_id` | TEXT FK → projects | Owning project |
| `session_id` | TEXT | Anonymous session fingerprint hash (32 hex chars) |
| `user_id_hash` | TEXT | SHA256 of provided user ID (if any) |
| `name` | TEXT | Event name (e.g. `page_view`, `signup`, `purchase`) |
| `url` | TEXT | Full URL of the page/action |
| `referrer` | TEXT | Full referrer URL |
| `referrer_domain` | TEXT | Extracted domain (e.g. `google.com`) |
| `utm_source` | TEXT | UTM source parameter |
| `utm_medium` | TEXT | UTM medium parameter |
| `utm_campaign` | TEXT | UTM campaign parameter |
| `utm_term` | TEXT | UTM term parameter |
| `utm_content` | TEXT | UTM content parameter |
| `properties` | TEXT | Arbitrary JSON blob with event properties |
| `user_agent` | TEXT | Raw User-Agent string |
| `browser` | TEXT | Parsed browser name (e.g. `Chrome`, `Firefox`) |
| `os` | TEXT | Parsed OS name (e.g. `macOS`, `Windows`, `Android`) |
| `device_type` | TEXT | `desktop`, `mobile`, `tablet`, or `bot` |
| `country_code` | TEXT | ISO 3166-1 alpha-2 (reserved for geo-IP, v2) |
| `ingest_id` | TEXT | Unique ingest record ID (for idempotency) |
| `occurred_at` | DATETIME | Event timestamp (from payload or server receive time) |
| `created_at` | DATETIME | Storage timestamp |

**Indexes**:
- `idx_events_project_occurred (project_id, occurred_at DESC)` — primary query index
- `idx_events_session (session_id)` — session lookups
- `idx_events_name (project_id, name)` — funnel step matching

---

## `sessions`

Aggregated session records. One row per unique fingerprint + project combination. Updated on each event.

| Column | Type | Description |
|--------|------|-------------|
| `id` | TEXT PK | Session fingerprint hash (same as `events.session_id`) |
| `project_id` | TEXT FK → projects | Owning project |
| `first_seen_at` | DATETIME | Timestamp of first event in this session |
| `last_seen_at` | DATETIME | Timestamp of most recent event |
| `event_count` | INTEGER | Total events in this session |
| `entry_url` | TEXT | First URL visited |
| `exit_url` | TEXT | Last URL visited |
| `referrer` | TEXT | Referrer from first event |
| `utm_source` | TEXT | UTM source from first event |
| `utm_medium` | TEXT | UTM medium from first event |
| `utm_campaign` | TEXT | UTM campaign from first event |
| `device_type` | TEXT | Device type from first event |
| `country_code` | TEXT | Country from first event (reserved) |

**Index**: `idx_sessions_project (project_id, last_seen_at DESC)`

---

## `funnels`

Named conversion funnels belonging to a project.

| Column | Type | Description |
|--------|------|-------------|
| `id` | TEXT PK | UUID v4 |
| `project_id` | TEXT FK → projects | Owning project |
| `name` | TEXT | Display name (e.g. "Signup Flow") |
| `description` | TEXT | Optional description |
| `created_at` | DATETIME | Creation timestamp |

---

## `funnel_steps`

Ordered steps within a funnel. Each step matches events by name (and optionally property filters).

| Column | Type | Description |
|--------|------|-------------|
| `id` | TEXT PK | UUID v4 |
| `funnel_id` | TEXT FK → funnels | Owning funnel (CASCADE DELETE) |
| `step_order` | INTEGER | Position in funnel (1-indexed) |
| `event_name` | TEXT | Event name to match (e.g. `page_view`, `purchase`) |
| `filters` | TEXT | JSON array: `[{"property":"plan","value":"pro"}]` |

**Index**: `idx_funnel_steps_funnel (funnel_id, step_order)`

---

## Session Fingerprinting Algorithm

Session IDs are computed server-side using:

```
session_id = SHA256(normalized_ip + "|" + user_agent)[:32 hex chars]
```

IP normalization:
- IPv4: Zero the last octet (`192.168.1.100` → `192.168.1.0`)
- IPv6: Zero bytes 7–16 (`2001:db8:85a3::8a2e:370:7334` → `2001:db8:85a3::`)

This provides k-anonymity within a /24 or /48 network while keeping sessions stable for a typical household or office.

Client-side session IDs (from the JS SDK) use localStorage with a 30-minute idle timeout and take precedence over server fingerprints when provided.

---

## Privacy Design Notes

1. **No IP addresses stored**: The IP is used only to compute the session fingerprint hash. The raw IP never touches the database.
2. **User IDs hashed**: `user_id_hash = SHA256(user_id)`. One-way only.
3. **No cross-site tracking**: All data is scoped to one Trailpost instance. No beacon CDN, no shared domain.
4. **Properties are user-controlled**: Operators choose what goes in `properties`. Sensitive fields should be omitted or pre-hashed by the SDK before sending.
