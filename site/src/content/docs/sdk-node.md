---
title: Node.js SDK
description: Server-side event tracking with the FunnelBarn JavaScript SDK in Node.js.
order: 5
---

# Node.js SDK

The same `@funnelbarn/js` package works server-side in Node.js. It buffers events in memory and flushes them to FunnelBarn in the background so tracking calls never block your request handlers.

## Installation

```bash
npm install @funnelbarn/js
```

## Basic setup

```typescript
import { FunnelBarnClient } from '@funnelbarn/js';

export const analytics = new FunnelBarnClient({
  apiKey: process.env.FUNNELBARN_API_KEY!,
  endpoint: process.env.FUNNELBARN_ENDPOINT!,
  projectName: 'my-api',
  flushInterval: 2000,   // flush every 2 seconds (default 5s)
});
```

Create a single shared instance and import it wherever you need it.

## Tracking server-side page views

```typescript
// Track when your API renders a page or handles a server-side request
analytics.page({ url: 'https://example.com/dashboard', userId: 'user-123' });
```

In a Node.js context `window.location` is not available, so pass the `url` explicitly as a property or call the lower-level method:

```typescript
// Using the track method (works the same way)
analytics.track('page_view', {
  url: req.url,
  referrer: req.headers.referer ?? '',
});
```

## Tracking custom events

```typescript
// Express middleware example
app.post('/api/checkout', async (req, res) => {
  const order = await processOrder(req.body);

  analytics.track('purchase', {
    order_id: order.id,
    amount: order.total,
    plan: order.plan,
  });

  res.json({ ok: true });
});
```

## Identifying users

```typescript
analytics.identify('user-123');

// All subsequent track() calls carry this user ID
analytics.track('settings_saved');
```

## Flushing on shutdown

In a server process, always flush before exit so queued events are not lost:

```typescript
process.on('SIGTERM', async () => {
  await analytics.flush();
  process.exit(0);
});
```

Or use the constructor's built-in timer — events will be sent within `flushInterval` ms.

## Environment variables

We recommend reading the API key and endpoint from environment variables rather than hard-coding them:

```typescript
const analytics = new FunnelBarnClient({
  apiKey: process.env.FUNNELBARN_API_KEY ?? '',
  endpoint: process.env.FUNNELBARN_ENDPOINT ?? 'http://localhost:8080',
  projectName: process.env.FUNNELBARN_PROJECT ?? 'my-app',
});
```

## Next.js / Vercel example

```typescript
// lib/analytics.ts
import { FunnelBarnClient } from '@funnelbarn/js';

let client: FunnelBarnClient | null = null;

export function getAnalytics(): FunnelBarnClient {
  if (!client) {
    client = new FunnelBarnClient({
      apiKey: process.env.FUNNELBARN_API_KEY!,
      endpoint: process.env.FUNNELBARN_ENDPOINT!,
      projectName: 'my-nextjs-app',
    });
  }
  return client;
}
```

```typescript
// app/api/route.ts (Next.js App Router)
import { getAnalytics } from '@/lib/analytics';

export async function POST(req: Request) {
  const analytics = getAnalytics();
  analytics.track('api_called', { route: '/api/route' });
  // ...
}
```

## Node.js < 18 compatibility

The SDK detects whether the global `fetch` API is available. On Node.js versions older than 18, it automatically falls back to the built-in `http`/`https` module. No extra configuration is needed.
