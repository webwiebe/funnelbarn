---
title: Getting Started
description: Install FunnelBarn and track your first event in under 5 minutes.
order: 1
---

# Getting Started

Get FunnelBarn running and tracking events in under 5 minutes.

## Prerequisites

- Docker (recommended), or a Linux server with APT, or macOS with Homebrew.
- An ingest API key — you'll set this yourself on first run.

## 1. Start the server

The fastest way is Docker:

```bash
docker run -d \
  --name funnelbarn \
  -e FUNNELBARN_API_KEY=mysecret \
  -e FUNNELBARN_ADMIN_USERNAME=admin \
  -e FUNNELBARN_ADMIN_PASSWORD=changeme \
  -p 8080:8080 \
  -v funnelbarn-data:/var/lib/funnelbarn \
  ghcr.io/webwiebe/funnelbarn/service:latest
```

The dashboard will be available at [http://localhost:8080](http://localhost:8080).

Log in with the `FUNNELBARN_ADMIN_USERNAME` / `FUNNELBARN_ADMIN_PASSWORD` you set above.

## 2. Send your first event

Once the server is running, send a test event with `curl`:

```bash
curl -X POST http://localhost:8080/api/v1/events \
  -H "x-funnelbarn-api-key: mysecret" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "page_view",
    "url": "https://example.com/",
    "referrer": "https://google.com"
  }'
```

You should get back:

```json
{"accepted": true, "ingestId": "..."}
```

Refresh the dashboard — your event will appear in the live feed and be rolled into the metrics within a few seconds.

## 3. Add the browser SDK to your site

Add this single script tag to your HTML `<head>`:

```html
<script src="https://your-funnelbarn-host.example.com/sdk.js"
        data-api-key="your-ingest-api-key" defer></script>
```

The SDK auto-initialises from the `data-api-key` attribute, fires a `page_view` event on load, manages sessions in `localStorage`, and provides `funnelbarn.track()` for custom events.

## 4. Track a custom event

```javascript
// After the SDK script has loaded:
funnelbarn.track('signup', { plan: 'pro', source: 'hero_cta' });
```

## What's next?

- Read the [Architecture Overview](/docs/architecture) to understand the ingest pipeline.
- See the [Configuration Reference](/docs/configuration) for all available environment variables.
- Explore the SDK guides for [JavaScript](/docs/sdk-js), [Node.js](/docs/sdk-node), and [Python](/docs/sdk-python).
- Set up [Funnel Analysis](/docs/funnel-analysis) or [A/B Tests](/docs/ab-tests) to measure conversion.
