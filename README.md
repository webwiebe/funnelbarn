# FunnelBarn

Self-hosted web analytics and conversion funnel tracking. Single binary. SQLite. Privacy-first.

FunnelBarn replaces Mixpanel, Amplitude, or Fathom for teams who want to own their analytics data. Deploy one binary, point a domain at it, and keep everything on your own server.

## 🐣 Tamagotchi

[![FunnelBarn Tamagotchi](https://tamagotchi.nijmegen.wiebe.xyz/api/v1/pets/webwiebe/funnelbarn/badge.svg)](https://tamagotchi.nijmegen.wiebe.xyz/pets/webwiebe/funnelbarn)

## Features

- **Event ingestion**: Track pageviews and custom events with properties
- **Session tracking**: Anonymous session fingerprinting — no cookies required
- **Funnel analysis**: Define multi-step funnels, see conversion rates and drop-off
- **Dashboard**: Time-series charts, top pages, referrers, UTM attribution
- **Privacy-first**: No cross-site tracking, GDPR-friendly, all data stays on your server
- **Simple ops**: Single Go binary, SQLite storage, Litestream replication

## Quick Start

### Docker

```bash
docker run -d \
  --name funnelbarn \
  -e FUNNELBARN_API_KEY=your-secret-ingest-key \
  -e FUNNELBARN_ADMIN_USERNAME=admin \
  -e FUNNELBARN_ADMIN_PASSWORD=changeme \
  -p 8080:8080 \
  -v funnelbarn-data:/var/lib/funnelbarn \
  ghcr.io/webwiebe/funnelbarn/service:latest
```

### Docker Compose

```bash
git clone https://github.com/wiebe-xyz/funnelbarn
cd funnelbarn
FUNNELBARN_API_KEY=secret docker compose up
```

### Homebrew (macOS)

```bash
brew tap webwiebe/funnelbarn
brew install funnelbarn
```

### APT (Debian/Ubuntu)

```bash
curl -fsSL https://webwiebe.nl/apt/funnelbarn-archive-keyring.gpg | sudo tee /etc/apt/keyrings/funnelbarn.gpg > /dev/null
echo "deb [signed-by=/etc/apt/keyrings/funnelbarn.gpg] https://webwiebe.nl/apt/ stable main" | sudo tee /etc/apt/sources.list.d/funnelbarn.list
sudo apt update && sudo apt install funnelbarn
```

## Track Your First Event

```bash
curl -X POST http://localhost:8080/api/v1/events \
  -H "x-funnelbarn-api-key: your-secret-ingest-key" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "page_view",
    "url": "https://example.com/pricing",
    "referrer": "https://google.com",
    "utm_source": "newsletter"
  }'
```

## CLI Setup

```bash
# Create a project
funnelbarn project create --name "My Website" --slug my-website

# Create an ingest API key
funnelbarn apikey create --project my-website --name frontend --scope ingest

# Create an admin user
funnelbarn user create --username admin --password yourpassword
```

## JavaScript SDK

```html
<script type="module">
  import { FunnelBarnClient } from 'https://cdn.jsdelivr.net/npm/@funnelbarn/js';

  const analytics = new FunnelBarnClient({
    apiKey: 'your-ingest-key',
    endpoint: 'https://funnelbarn.yourdomain.com',
    projectName: 'my-website',
  });

  analytics.page(); // auto-detect URL + referrer
  analytics.track('signup', { plan: 'pro' });
</script>
```

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `FUNNELBARN_ADDR` | `:8080` | Listen address |
| `FUNNELBARN_API_KEY` | — | Master ingest API key |
| `FUNNELBARN_API_KEY_SHA256` | — | Alternative pre-hashed key |
| `FUNNELBARN_DB_PATH` | `.data/funnelbarn.db` | SQLite database path |
| `FUNNELBARN_SPOOL_DIR` | `.data/spool` | Event spool directory |
| `FUNNELBARN_MAX_BODY_BYTES` | `1048576` | Max request body (1 MiB) |
| `FUNNELBARN_MAX_SPOOL_BYTES` | unlimited | Spool size cap |
| `FUNNELBARN_ADMIN_USERNAME` | — | Admin username |
| `FUNNELBARN_ADMIN_PASSWORD` | — | Admin password (plaintext) |
| `FUNNELBARN_ADMIN_PASSWORD_BCRYPT` | — | Admin password (bcrypt hash) |
| `FUNNELBARN_SESSION_SECRET` | random | HMAC secret for session tokens |
| `FUNNELBARN_SESSION_TTL_SECONDS` | `43200` | Session TTL (12 hours) |
| `FUNNELBARN_ALLOWED_ORIGINS` | — | CORS origins (CSV) |
| `FUNNELBARN_PUBLIC_URL` | — | Public server URL |
| `FUNNELBARN_SELF_ENDPOINT` | — | BugBarn endpoint for self-reporting |
| `FUNNELBARN_SELF_API_KEY` | — | BugBarn API key for self-reporting |
| `FUNNELBARN_LOG_LEVEL` | `info` | Log verbosity: `debug`, `info`, `warn`, `error` |
| `FUNNELBARN_LOGIN_RATE_PER_MINUTE` | `20` | Login endpoint rate limit (requests/min) |
| `FUNNELBARN_LOGIN_RATE_BURST` | `20` | Login endpoint burst capacity |
| `FUNNELBARN_API_RATE_PER_MINUTE` | `300` | Rate limit for authenticated API endpoints (requests/min) |
| `FUNNELBARN_API_RATE_BURST` | `60` | Authenticated API burst capacity |
| `FUNNELBARN_INGEST_RATE_PER_MINUTE` | `500` | Ingest endpoint rate limit (requests/min) |
| `FUNNELBARN_INGEST_RATE_BURST` | `100` | Ingest endpoint burst capacity |
| `FUNNELBARN_METRICS_TOKEN` | — | Optional Bearer token to protect `/metrics` endpoint |
| `FUNNELBARN_EVENT_RETENTION_DAYS` | `90` | Days to retain events (0 = disabled) |

## API Endpoints

Authentication: session cookie (obtained via `POST /api/v1/login`) or API key (`X-Api-Key` header). All project-scoped routes are under `/api/v1/projects/{id}/`. `/api/v1/health` returns DB connectivity status and server version.

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/api/v1/health` | — | Health check + version |
| `POST` | `/api/v1/events` | API key | Ingest event |
| `POST` | `/api/v1/login` | — | Dashboard login |
| `POST` | `/api/v1/logout` | Session | Logout |
| `GET` | `/api/v1/me` | Session | Current user |
| `GET` | `/api/v1/projects` | Session | List projects |
| `POST` | `/api/v1/projects` | Session | Create project |
| `GET` | `/api/v1/projects/{id}/dashboard` | Session | Dashboard stats |
| `GET` | `/api/v1/projects/{id}/events` | Session | Paginated events |
| `GET` | `/api/v1/projects/{id}/sessions` | Session | Paginated sessions |
| `GET` | `/api/v1/projects/{id}/funnels` | Session | List funnels |
| `POST` | `/api/v1/projects/{id}/funnels` | Session | Create funnel |
| `GET` | `/api/v1/projects/{id}/funnels/{fid}/analysis` | Session | Funnel analysis |
| `GET` | `/api/v1/apikeys` | Session | List API keys |
| `POST` | `/api/v1/apikeys` | Session | Create API key |

## Architecture

FunnelBarn uses a layered structure with clear separation of concerns:

```
HTTP handler → service (business logic, validation) → repository (SQL via sqlc) → SQLite
```

Key packages:

- `internal/api/` — HTTP handlers only; maps service errors to HTTP status codes
- `internal/service/` — business logic, input validation, domain error wrapping
- `internal/repository/` — data access; uses sqlc-generated type-safe query methods
- `internal/domain/` — domain errors (`ErrNotFound`, `ErrConflict`, `ErrValidation`)
- `internal/worker/` — background spool processor
- `internal/spool/` — durable NDJSON event queue
- `internal/ingest/` — event ingestion handler

### Ingest pipeline

FunnelBarn uses a durable spool pattern for high-throughput ingest:

```
SDK/Browser → POST /api/v1/events
                 │
                 ▼
           In-memory queue (32k cap)
                 │  5ms batch flush
                 ▼
           NDJSON spool file (append-only)
                 │  Background worker (1s tick)
                 ▼
           SQLite database
```

The HTTP handler never writes to the database. Events are appended to a durable NDJSON spool file and processed asynchronously. This keeps ingest latency below 1ms regardless of database pressure.

## Security

- Rate limiting on login, ingest, and authenticated API endpoints (configurable via env vars above)
- Security headers applied automatically: `X-Frame-Options`, `Content-Security-Policy`, `HSTS`, etc.
- `/metrics` endpoint can be protected with `FUNNELBARN_METRICS_TOKEN` (Bearer token)
- Input validation at the service layer; errors map to appropriate HTTP codes (404/409/422)
- No sensitive fields (password hashes, API key hashes) are included in API responses

## Development

```bash
# Install dependencies
make setup

# Run all tests (Go + frontend)
make test

# Go tests only (~70% coverage on core packages)
go test ./...

# Frontend tests only (60 Vitest tests)
cd web && npm test

# Lint (golangci-lint + go vet + frontend lint)
make lint

# Build binary
make build

# Start with Docker Compose
make dev
```

## License

MIT
