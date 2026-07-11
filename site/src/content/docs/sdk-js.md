---
title: JavaScript SDK (Browser)
description: Auto page views, custom events, and identify in the browser with the FunnelBarn JS SDK.
order: 4
---

# JavaScript SDK — Browser

The `@funnelbarn/js` package works in all modern browsers (and Node.js — see the [Node.js guide](/docs/sdk-node)). It auto-tracks page views, manages sessions in `localStorage`, and provides a simple API for custom events and user identification.

## Installation

### npm / yarn / pnpm

```bash
npm install @funnelbarn/js
```

### Script tag (no bundler)

If you are not using a bundler, load the SDK from your FunnelBarn instance's built-in CDN path:

```html
<script src="https://funnelbarn.example.com/sdk.js"
        data-api-key="your-ingest-api-key" defer></script>
```

The SDK auto-initialises from `data-api-key`, infers the endpoint from the script's `src` URL, and exposes a global `funnelbarn` object with `track()`, `page()`, and `identify()` methods.

## Usage with a bundler

```typescript
import { FunnelBarnClient } from '@funnelbarn/js';

const analytics = new FunnelBarnClient({
  apiKey: 'your-ingest-api-key',
  endpoint: 'https://funnelbarn.example.com',
  projectName: 'my-website',   // optional but recommended
});

// Track current page view (auto-reads window.location and document.referrer)
analytics.page();
```

### Options

| Option | Type | Default | Description |
|---|---|---|---|
| `apiKey` | `string` | required | Your ingest API key (see [API keys](#api-keys)) |
| `endpoint` | `string` | required | Full URL of your FunnelBarn instance |
| `projectName` | `string` | — | Project identifier sent as `x-funnelbarn-project` header (events only; not used for recording) |
| `flushInterval` | `number` | `5000` | How often (ms) the event queue is flushed |
| `sessionTimeout` | `number` | `1800000` | Session idle timeout in ms (default 30 min) |
| `recording` | `boolean` | `false` | Enable session recording immediately on init (server config can also enable it) |
| `recordingChunkMs` | `number` | `10000` | How often (ms) a recording chunk is flushed to the server |
| `recordInclude` | `string[]` | — | URL path glob patterns to always record (e.g. `['/checkout/**']`) |
| `recordExclude` | `string[]` | — | URL path glob patterns to never record (e.g. `['/admin/**']`) |

## Tracking page views

```javascript
// Auto-detect URL and referrer from window.location
analytics.page();

// Pass extra properties
analytics.page({ variant: 'dark-theme' });
```

The SDK extracts UTM parameters from the current URL automatically (`utm_source`, `utm_medium`, `utm_campaign`, `utm_term`, `utm_content`).

## Tracking custom events

```javascript
analytics.track('signup_click', { plan: 'pro', source: 'hero_cta' });
analytics.track('checkout_started', { cart_value: 99.00 });
analytics.track('video_played', { video_id: 'intro-tour', duration_seconds: 120 });
```

## Identifying users

Call `identify` with a user ID before tracking events to associate them with a specific user. The ID is hashed server-side before storage — the raw value is never persisted.

```javascript
// After login
analytics.identify('user-123');

// All subsequent track() and page() calls carry this user ID
analytics.track('dashboard_viewed');
```

To clear the identity (e.g. on logout):

```javascript
analytics.identify('');
```

## Sessions

The SDK generates a random session ID and stores it in `localStorage` under `funnelbarn_sid`. Sessions expire after the `sessionTimeout` idle period. A new session ID is created when the previous one expires.

The session ID is sent with every event so FunnelBarn can group page views and custom events into sessions without requiring cookies.

## Flushing manually

The queue is flushed automatically every `flushInterval` ms and on `beforeunload`. To force a flush (e.g. before a redirect):

```javascript
await analytics.flush();
```

## Single-page applications (SPA)

For client-side routers (React Router, Next.js, Vue Router, etc.), call `analytics.page()` each time the route changes:

```javascript
// React Router example
import { useEffect } from 'react';
import { useLocation } from 'react-router-dom';

function Analytics() {
  const location = useLocation();
  useEffect(() => {
    analytics.page();
  }, [location]);
  return null;
}
```

## Session Recording

FunnelBarn can record user sessions with rrweb and replay them in the dashboard.

### API keys

> **Important:** session recording requires a **project-scoped API key** — the
> key shown on your project's settings page. The global `FUNNELBARN_API_KEY`
> (the env-var key used for self-hosting) is not bound to any project and will
> be rejected with a `401` when the SDK tries to send recording chunks.
>
> If you are integrating two barns (e.g. bugbarn sending recordings to
> funnelbarn), set the cross-barn env var (`BUGBARN_FUNNELBARN_API_KEY` etc.)
> to the **project-scoped key** from the funnelbarn project settings page, not
> the global `FUNNELBARN_API_KEY`.

### Enabling recording

Recording is off by default. Turn it on in one of two ways:

**Option A — enable in the dashboard** (recommended): Go to your project's Settings → Session Recording and toggle recording on. The SDK fetches this setting on init via `GET /api/v1/recording-config` and starts recording automatically.

**Option B — enable in the SDK init:**

```javascript
const analytics = new FunnelBarnClient({
  apiKey: 'your-project-scoped-api-key',
  endpoint: 'https://funnelbarn.example.com',
  recording: true,
});
```

### Script-tag usage with recording

```html
<script src="https://funnelbarn.example.com/sdk.js"
        data-api-key="your-project-scoped-api-key" defer></script>
```

Recording is controlled server-side when using the script tag. Enable it from the project settings in the dashboard.

### Filtering which pages are recorded

```javascript
const analytics = new FunnelBarnClient({
  apiKey: 'your-project-scoped-api-key',
  endpoint: 'https://funnelbarn.example.com',
  recording: true,
  recordInclude: ['/checkout/**', '/signup'],
  recordExclude: ['/admin/**', '/account/password'],
});
```

`recordInclude` and `recordExclude` are checked first. Server-side rules (set from the dashboard) are applied next. If no rule matches, the page is recorded by default.

To mask sensitive input fields, add the `fb-block` CSS class to any element and its contents will be hidden in the recording.

## TypeScript

The package ships full TypeScript types. The main types you'll use are:

```typescript
import type { FunnelBarnOptions, EventProperties } from '@funnelbarn/js';
```
