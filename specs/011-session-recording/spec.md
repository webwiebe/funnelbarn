# Feature Specification: Session Recording

**Spec**: `011-session-recording`
**Created**: 2026-06-07
**Status**: Planned

---

## Problem Statement

FunnelBarn tells you *what* users do — which pages they visit, which funnels they complete, which flags they encountered. It does not tell you *how* they do it: where they hesitate, what they click that doesn't work, how they navigate through a confusing form.

Tools like Hotjar and FullStory fill this gap with session recording: the ability to watch a user's browser session play back like a video. The problem is the same as with analytics tools — third-party SaaS, data leaving your infrastructure, per-session pricing that scales with traffic.

FunnelBarn should own this too.

---

## Solution

Add opt-in session recording to FunnelBarn using **rrweb** for DOM capture and **Cloudflare R2** (S3-compatible) for chunk storage. The dashboard gains a Sessions page with a replay player. Recordings are integrated with existing features: funnel steps link to sessions that dropped off there, flow nodes link to sessions that passed through that page, and flag evaluations are visible as markers on the recording timeline.

### Design Principles

1. **Opt-in, not opt-out**: Recording is disabled unless `record: true` is explicitly passed to SDK init. Sites that only want analytics remain unaffected by the bundle size increase.

2. **Privacy by default**: Password fields are masked in all recordings. Any element with the CSS class `fb-block` is redacted. No other defaults are changed without explicit configuration.

3. **Cheap storage, not DB bloat**: rrweb event chunks are gzip-compressed and stored in Cloudflare R2. SQLite stores only metadata (recording ID, session ID, chunk count, duration). At ~200KB per session compressed, 10,000 sessions/month = ~2GB in R2.

4. **First-class cross-feature integration**: Recordings are not a standalone feature. Every existing feature that surfaces session-level data gets a "Watch recordings" path.

---

## Architecture

### Data Flow

```
Browser (rrweb) → SDK chunks every 10s
  → POST /api/v1/recordings/chunk   [API key auth]
  → Go: gzip compress → R2: recordings/{project}/{recording_id}/{index:05d}.json.gz
  → SQLite: upsert recordings row

Dashboard → GET /api/v1/projects/{id}/recordings
  → recording list (metadata only, no R2 reads)

Dashboard → GET /api/v1/projects/{id}/recordings/{rid}/chunks/{index}
  → Go proxies from R2 → decompressed rrweb event JSON
  → rrweb Replayer plays back
```

### Storage Layout in R2

```
recordings/
  {project_slug}/
    {recording_id}/
      00000.json.gz   ← chunk 0 (0–10s)
      00001.json.gz   ← chunk 1 (10–20s)
      ...
```

Each chunk file is the gzip-compressed JSON array of rrweb `eventWithTime` objects for that interval.

---

## SDK Changes

### New init options

```typescript
funnelbarn.init({
  apiKey: 'fb_...',
  // existing options...

  // NEW:
  recording: true,            // enable session recording (default: false)
  recordingChunkMs: 10_000,  // flush a chunk every N ms (default: 10000)
})
```

### Recording ID

A new `recording_id` is generated at SDK init (same mechanism as `session_id` — 16-byte random hex). This is stable for the duration of the page load but does not persist across navigations (a new recording starts on each page load, matching rrweb's full-snapshot behavior).

### Chunk payload

```typescript
// POST /api/v1/recordings/chunk
{
  recording_id: string,   // stable per page load
  session_id:   string,   // existing session ID
  chunk_index:  number,   // 0, 1, 2, ...
  events:       rrweb.eventWithTime[],
  started_at:   string,   // ISO 8601, timestamp of recording start
  duration_ms:  number,   // elapsed ms since recording start
}
```

Chunks are sent on a fixed interval (`recordingChunkMs`) and also on `beforeunload` alongside the existing event flush.

### Flag evaluation markers

When recording is active and `evaluate()` is called, the SDK emits a custom rrweb event:

```typescript
{
  type: EventType.Custom,
  data: { tag: 'fb_flag', payload: { flag: flagName, variant: result } },
  timestamp: Date.now(),
}
```

These events are stored in the normal chunk stream and surfaced as timeline markers during replay — no extra backend calls required.

### Privacy

- `maskInputOptions: { password: true }` — password fields replaced with `***`
- `blockClass: 'fb-block'` — any element with this class is fully redacted from the recording
- No cookies set; uses existing session ID from localStorage

### Bundle

rrweb is bundled into the existing `funnelbarn.js` IIFE. When `recording: false` (default), the rrweb `record()` function is never called — no DOM observers are registered, no overhead.

---

## Backend Changes

### Config

Four new environment variables:

| Variable | Description |
|----------|-------------|
| `FUNNELBARN_R2_ACCOUNT_ID` | Cloudflare account ID |
| `FUNNELBARN_R2_ACCESS_KEY_ID` | R2 API token access key |
| `FUNNELBARN_R2_SECRET_ACCESS_KEY` | R2 API token secret |
| `FUNNELBARN_R2_BUCKET` | R2 bucket name |

When any of these are unset, `POST /api/v1/recordings/chunk` returns `503 Service Unavailable` with a clear error body.

### Database Schema

New migration `00018_recordings.sql`:

```sql
-- +goose Up
CREATE TABLE recordings (
    id          TEXT PRIMARY KEY,
    project_id  TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    session_id  TEXT NOT NULL,
    environment TEXT NOT NULL DEFAULT '',
    chunk_count INTEGER NOT NULL DEFAULT 0,
    duration_ms INTEGER NOT NULL DEFAULT 0,
    started_at  DATETIME NOT NULL,
    ended_at    DATETIME,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_recordings_project     ON recordings(project_id, started_at DESC);
CREATE INDEX idx_recordings_session     ON recordings(session_id);
CREATE INDEX idx_recordings_project_env ON recordings(project_id, environment, started_at DESC);

-- +goose Down
DROP TABLE IF EXISTS recordings;
```

### New packages / files

| Path | Role |
|------|------|
| `internal/storage/r2.go` | Thin R2 client wrapper (aws-sdk-go-v2/service/s3) |
| `internal/repository/recordings.go` | Repository methods: `UpsertRecording`, `ListRecordings`, `GetRecording`, `ListRecordingsBySessionIDs` |
| `internal/service/recordings.go` | Service: `IngestChunk`, `ListRecordings`, `GetChunk` |
| `internal/api/recordings.go` | HTTP handlers for all recording endpoints |

### New API Endpoints

#### Ingest (API key auth, ingest rate limiter)

```
POST /api/v1/recordings/chunk
Headers: x-funnelbarn-api-key, x-funnelbarn-project
Body: ChunkPayload (see SDK section)
Response: 202 {"accepted": true}
```

#### Dashboard (session cookie auth)

```
GET /api/v1/projects/{id}/recordings
  Query: limit, offset, environment, session_ids (comma-sep), flag, variant
  Response: { recordings: [Recording], limit, offset }

GET /api/v1/projects/{id}/recordings/{rid}/chunks/{index}
  Response: application/json — decompressed rrweb event array
  Headers: Cache-Control: private, max-age=3600

GET /api/v1/projects/{id}/recordings/{rid}/flags
  Response: { evaluations: [{flag_name, variant, evaluated_at}] }
```

The chunk proxy endpoint reads from R2 and returns decompressed JSON. This keeps R2 private (no presigned URLs) and enforces session auth on replay.

---

## Cross-Feature Integration

### Funnels → Recordings

**New endpoint:**
```
GET /api/v1/projects/{id}/funnels/{fid}/steps/{step}/sessions
  Query: from, to, limit (default 100)
  Response: { session_ids: string[] }
```

Returns the session IDs that reached step N but did not reach step N+1 (i.e. dropped off at that step). Also works for the final step (sessions that fully converted).

**SQL approach**: For each step, find `DISTINCT session_id` in `events` where `name = step.event_name` and `project_id = ?` and `occurred_at BETWEEN from AND to`. Intersect/except with the next step to determine converters vs drop-offs.

**Dashboard change** (`Funnels.tsx`): Small `<Video>` icon button next to each step's user count. Clicking navigates to `/sessions/{projectId}?funnel={fid}&step={n}`. The Sessions page resolves this to a `session_ids` filter.

---

### Flows → Recordings

**New endpoint:**
```
GET /api/v1/projects/{id}/flows/sessions
  Query: page (URL), from, to, limit (default 100)
  Response: { session_ids: string[] }
```

Returns `DISTINCT session_id` from `events` where `url = page` in the time range.

**Dashboard change** (`SankeyChart.tsx`): For `type: 'page'` nodes, add a secondary action (tooltip button on hover). Left-click continues to drill into the flow as today. The new button navigates to `/sessions/{projectId}?page={encodedURL}`.

---

### Feature Flags → Recordings

**Existing data**: The `flag_evaluations` table already stores `session_id`. No schema change needed.

**New repository query** (added to `internal/repository/flags.go`):
```sql
SELECT fe.id, f.name AS flag_name, fe.variant, fe.created_at
FROM flag_evaluations fe
JOIN flags f ON f.id = fe.flag_id
WHERE fe.session_id = ? AND fe.project_id = ?
ORDER BY fe.created_at ASC
```

**New endpoint** (added to `internal/api/recordings.go`):
```
GET /api/v1/projects/{id}/recordings/{rid}/flags
```

**Dashboard replay player**: Custom rrweb events tagged `fb_flag` (embedded by the SDK during `evaluate()`) are detected in the loaded event stream and rendered as colored dot markers on the timeline scrubber. A sidebar panel lists all flag evaluations with their timestamps.

**Sessions list**: Each recording row in the list shows a flag badge for each unique flag evaluated during that session (fetched on list load using the flags endpoint).

---

### Unified Session Filter

The `GET /api/v1/projects/{id}/recordings` endpoint accepts these filter params:

| Param | Source | Behavior |
|-------|--------|----------|
| `session_ids` | Funnel / Flow drilldown | `WHERE session_id IN (...)` |
| `flag` + `variant` | Flag page filter | JOIN flag_evaluations WHERE flag_id=? AND variant=? |
| `session_id` | Direct link | Exact match |
| `environment` | Environment picker | Existing filter |

The Sessions page (`/sessions/:projectId`) reads these from `useSearchParams()`.

---

## Frontend Changes

### New page: `/sessions/:projectId`

Route added to `App.tsx` (same pattern as all other pages).

Navigation: `Video` icon added to `navLinks` in `Shell.tsx` between Live and Settings.

**Layout:**
- Time range picker (reuse existing component)
- Environment filter
- Recordings table:
  - Started at (relative: "3 hours ago")
  - Duration (formatted: "2m 14s")
  - Pages visited (chunk count as a proxy)
  - Environment badge
  - Device type icon
  - Flag badges (if any evaluations in this session)
  - Session ID (truncated, monospace)
- Click any row → opens full-screen replay modal

**Pagination**: limit 50, offset-based (same as Sessions handler).

**Filter banner**: When arriving from a funnel/flow drilldown, show a dismissible banner: "Showing recordings from users who dropped off at step 2 of [Funnel Name]".

### New components: `web/src/components/sessions/`

**`SessionReplay.tsx`**: Full-screen modal overlay.
- Fetches all chunks (0 to `chunk_count - 1`) sequentially
- Feeds all events to `new rrweb.Replayer(events, { root: containerEl })`
- Controls: play/pause button, progress scrubber, playback speed (1×/2×/4×)
- Flag markers: scans events for `EventType.Custom` with `tag: 'fb_flag'`, renders as amber dots on the scrubber (matching FunnelBarn's primary color `C.amber`)
- Sidebar: flag evaluation list with timestamps
- Loading state: progress bar while chunks download

**Styling**: All inline styles using `C` color palette. Dark theme. No external CSS (the rrweb Replayer renders into an iframe — its internal styles are self-contained).

---

## New Dependencies

### Go (`go.mod`)
```
github.com/aws/aws-sdk-go-v2
github.com/aws/aws-sdk-go-v2/config
github.com/aws/aws-sdk-go-v2/credentials
github.com/aws/aws-sdk-go-v2/service/s3
```

### JS SDK (`sdks/js/package.json`)
```
"rrweb": "^2.0.0"   (production dependency)
```

### Frontend (`web/package.json`)
```
"rrweb": "^2.0.0"   (for Replayer class — same version as SDK)
```

Note: `rrweb-player` (the pre-built UI widget) is NOT used in favor of a custom player that matches FunnelBarn's dark theme.

---

## Out of Scope

- **Heatmaps**: Click/scroll heatmaps are a separate feature and not part of this spec.
- **Network requests capture**: rrweb-plugin-network is not included — no request/response logging.
- **Console capture**: rrweb-plugin-console is not included.
- **Mobile apps**: Recording is browser-only via the JS SDK.
- **Retention / cleanup**: Old recordings in R2 are not auto-purged in this iteration. A separate retention job (similar to `PurgeOldEvents`) is planned but out of scope here.
- **Recording sampling**: All sessions with `record: true` are recorded in full. Percentage-based sampling (e.g. "record 10% of sessions") is not in this spec.

---

## Verification Checklist

- [ ] Set R2 env vars, start server with `go run ./cmd/funnelbarn`
- [ ] Instrument a test page with `record: true`, navigate several pages, type in a password field, trigger a flag `evaluate()` call
- [ ] Confirm chunks appear in R2 bucket at the expected key path
- [ ] Open `/sessions` dashboard page — recording row appears with correct duration and flag badge
- [ ] Click recording — player fetches all chunks, session replays correctly
- [ ] Password field in replay shows `***`
- [ ] Flag evaluation appears as amber dot on timeline scrubber + in sidebar list
- [ ] Go to Funnels page, click "Watch recordings" on a step — Sessions page loads filtered to those sessions
- [ ] Go to Flows page, hover a page node, click secondary action — Sessions page loads filtered to sessions that visited that page
- [ ] `POST /api/v1/recordings/chunk` returns 503 when R2 env vars are unset
- [ ] `go fmt ./...` and `go vet ./...` produce zero output
- [ ] `npm run build` in `sdks/js/` succeeds, IIFE bundle includes rrweb
- [ ] `npm run build` and `npm run lint` in `web/` succeed
- [ ] CI gates pass on PR (go fmt, go vet, staticcheck, tsc, eslint, vitest)
