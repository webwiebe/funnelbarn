# Trailpost

Self-hosted web analytics and conversion funnel tracking. Single binary. SQLite. Privacy-first.

Trailpost replaces Mixpanel, Amplitude, or Fathom for teams who want to own their analytics data. Deploy one binary, point a domain at it, and keep everything on your own server.

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
  --name trailpost \
  -e TRAILPOST_API_KEY=your-secret-ingest-key \
  -e TRAILPOST_ADMIN_USERNAME=admin \
  -e TRAILPOST_ADMIN_PASSWORD=changeme \
  -p 8080:8080 \
  -v trailpost-data:/var/lib/trailpost \
  ghcr.io/webwiebe/trailpost/service:latest
```

### Docker Compose

```bash
git clone https://github.com/wiebe-xyz/trailpost
cd trailpost
TRAILPOST_API_KEY=secret docker compose up
```

### Homebrew (macOS)

```bash
brew tap webwiebe/trailpost
brew install trailpost
```

### APT (Debian/Ubuntu)

```bash
curl -fsSL https://webwiebe.nl/apt/trailpost-archive-keyring.gpg | sudo tee /etc/apt/keyrings/trailpost.gpg > /dev/null
echo "deb [signed-by=/etc/apt/keyrings/trailpost.gpg] https://webwiebe.nl/apt/ stable main" | sudo tee /etc/apt/sources.list.d/trailpost.list
sudo apt update && sudo apt install trailpost
```

## Track Your First Event

```bash
curl -X POST http://localhost:8080/api/v1/events \
  -H "x-trailpost-api-key: your-secret-ingest-key" \
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
trailpost project create --name "My Website" --slug my-website

# Create an ingest API key
trailpost apikey create --project my-website --name frontend --scope ingest

# Create an admin user
trailpost user create --username admin --password yourpassword
```

## JavaScript SDK

```html
<script type="module">
  import { TrailpostClient } from 'https://cdn.jsdelivr.net/npm/@trailpost/js';

  const analytics = new TrailpostClient({
    apiKey: 'your-ingest-key',
    endpoint: 'https://analytics.yourdomain.com',
    projectName: 'my-website',
  });

  analytics.page(); // auto-detect URL + referrer
  analytics.track('signup', { plan: 'pro' });
</script>
```

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `TRAILPOST_ADDR` | `:8080` | Listen address |
| `TRAILPOST_API_KEY` | — | Master ingest API key |
| `TRAILPOST_API_KEY_SHA256` | — | Alternative pre-hashed key |
| `TRAILPOST_DB_PATH` | `.data/trailpost.db` | SQLite database path |
| `TRAILPOST_SPOOL_DIR` | `.data/spool` | Event spool directory |
| `TRAILPOST_MAX_BODY_BYTES` | `1048576` | Max request body (1 MiB) |
| `TRAILPOST_MAX_SPOOL_BYTES` | unlimited | Spool size cap |
| `TRAILPOST_ADMIN_USERNAME` | — | Admin username |
| `TRAILPOST_ADMIN_PASSWORD` | — | Admin password (plaintext) |
| `TRAILPOST_ADMIN_PASSWORD_BCRYPT` | — | Admin password (bcrypt hash) |
| `TRAILPOST_SESSION_SECRET` | random | HMAC secret for session tokens |
| `TRAILPOST_SESSION_TTL_SECONDS` | `43200` | Session TTL (12 hours) |
| `TRAILPOST_ALLOWED_ORIGINS` | — | CORS origins (CSV) |
| `TRAILPOST_PUBLIC_URL` | — | Public server URL |
| `TRAILPOST_SELF_ENDPOINT` | — | BugBarn endpoint for self-reporting |
| `TRAILPOST_SELF_API_KEY` | — | BugBarn API key for self-reporting |

## API Endpoints

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/api/v1/health` | — | Health check |
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

Trailpost uses a durable spool pattern for high-throughput ingest:

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

## Development

```bash
# Install dependencies
make setup

# Run all tests
make test

# Build binary
make build

# Start with Docker Compose
make dev
```

## License

MIT
