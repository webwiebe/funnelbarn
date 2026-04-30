---
title: Configuration Reference
description: All FUNNELBARN_* environment variables and config file options.
order: 3
---

# Configuration Reference

FunnelBarn is configured entirely through environment variables. There are no required config files.

## Environment variables

| Variable | Default | Description |
|---|---|---|
| `FUNNELBARN_ADDR` | `:8080` | TCP address to bind. Use `0.0.0.0:8080` to listen on all interfaces. |
| `FUNNELBARN_API_KEY` | _(none)_ | Plain-text ingest API key. Clients send this in the `x-funnelbarn-api-key` header. Use this **or** `FUNNELBARN_API_KEY_SHA256`, not both. |
| `FUNNELBARN_API_KEY_SHA256` | _(none)_ | SHA-256 hash of the API key (hex-encoded). Use instead of `FUNNELBARN_API_KEY` to avoid storing the raw secret in the environment. |
| `FUNNELBARN_ADMIN_USERNAME` | _(none)_ | Dashboard login username. |
| `FUNNELBARN_ADMIN_PASSWORD` | _(none)_ | Dashboard login password (plain text). Use this **or** `FUNNELBARN_ADMIN_PASSWORD_BCRYPT`. |
| `FUNNELBARN_ADMIN_PASSWORD_BCRYPT` | _(none)_ | bcrypt hash of the admin password. Safer than the plain-text variant. |
| `FUNNELBARN_SESSION_SECRET` | _(none)_ | Secret used to sign dashboard session cookies. Set to a long random string. |
| `FUNNELBARN_SESSION_TTL_SECONDS` | `43200` (12 h) | Dashboard session lifetime in seconds. |
| `FUNNELBARN_DB_PATH` | `.data/funnelbarn.db` | Path to the SQLite database file. The directory must exist and be writable. |
| `FUNNELBARN_SPOOL_DIR` | `.data/spool` | Directory where raw event spool files are written before processing. Must be writable. |
| `FUNNELBARN_ALLOWED_ORIGINS` | _(allow all)_ | Comma-separated list of allowed CORS origins for the ingest endpoint, e.g. `https://example.com,https://www.example.com`. Leave unset to allow `*`. |
| `FUNNELBARN_MAX_BODY_BYTES` | `1048576` (1 MiB) | Maximum request body size for ingest events, in bytes. |
| `FUNNELBARN_MAX_SPOOL_BYTES` | _(none)_ | Optional cap on total spool directory size in bytes. New events are rejected with `429` when the limit is reached. |
| `FUNNELBARN_PUBLIC_URL` | _(none)_ | Public URL of this FunnelBarn instance. Used to build absolute URLs in emails and setup links. |
| `FUNNELBARN_SELF_ENDPOINT` | _(none)_ | Endpoint for FunnelBarn to report its own analytics to (BugBarn self-reporting). |
| `FUNNELBARN_SELF_API_KEY` | _(none)_ | API key used for self-reporting. |
| `FUNNELBARN_ENVIRONMENT` | `production` | Environment label attached to self-reported events (e.g. `staging`, `production`). |

## Config files

In addition to environment variables, FunnelBarn reads `KEY=VALUE` config files from two locations:

1. `/etc/funnelbarn/funnelbarn.conf` — system-wide (Linux).
2. `~/.config/funnelbarn/funnelbarn.conf` — current user (Linux and macOS).

Lines beginning with `#` and blank lines are ignored. Values may be wrapped in single or double quotes. **Environment variables always win over config file values.**

Example `/etc/funnelbarn/funnelbarn.conf`:

```ini
# FunnelBarn configuration
FUNNELBARN_ADDR=:8080
FUNNELBARN_DB_PATH=/var/lib/funnelbarn/funnelbarn.db
FUNNELBARN_SPOOL_DIR=/var/lib/funnelbarn/spool
FUNNELBARN_API_KEY=a-long-random-secret
FUNNELBARN_ADMIN_USERNAME=admin
FUNNELBARN_ADMIN_PASSWORD_BCRYPT=$2b$12$...
FUNNELBARN_SESSION_SECRET=another-long-random-secret
FUNNELBARN_ALLOWED_ORIGINS=https://mysite.example.com
```

## API key scopes

When creating per-project API keys through the dashboard, each key has one of two scopes:

- **`ingest`** — can only send events to `POST /api/v1/events`. Safe to embed in browser-side JavaScript.
- **`full`** — can send events and access all dashboard API endpoints. Keep this server-side.

The global `FUNNELBARN_API_KEY` / `FUNNELBARN_API_KEY_SHA256` environment variable always has `full` scope.

## Docker example

```bash
docker run -d \
  --name funnelbarn \
  -e FUNNELBARN_ADDR=:8080 \
  -e FUNNELBARN_API_KEY=your-ingest-key \
  -e FUNNELBARN_ADMIN_USERNAME=admin \
  -e FUNNELBARN_ADMIN_PASSWORD=changeme \
  -e FUNNELBARN_SESSION_SECRET=$(openssl rand -hex 32) \
  -e FUNNELBARN_ALLOWED_ORIGINS=https://yoursite.example.com \
  -p 8080:8080 \
  -v funnelbarn-data:/var/lib/funnelbarn \
  ghcr.io/webwiebe/funnelbarn/service:latest
```
