# @trailpost/js

Browser + Node.js SDK for [Trailpost](https://github.com/wiebe-xyz/trailpost) — self-hosted web analytics.

## Installation

```bash
npm install @trailpost/js
```

## Usage (browser)

```html
<script type="module">
  import { TrailpostClient } from '@trailpost/js';

  const analytics = new TrailpostClient({
    apiKey: 'your-api-key',
    endpoint: 'https://analytics.example.com',
    projectName: 'my-website',
  });

  // Auto-detect URL and referrer from window.location
  analytics.page();

  // Track custom events
  document.querySelector('#signup-btn').addEventListener('click', () => {
    analytics.track('signup_click', { plan: 'pro' });
  });
</script>
```

## Usage (Node.js)

```typescript
import { TrailpostClient } from '@trailpost/js';

const analytics = new TrailpostClient({
  apiKey: process.env.TRAILPOST_API_KEY!,
  endpoint: process.env.TRAILPOST_ENDPOINT!,
  projectName: 'my-api',
});

analytics.track('api_request', { route: '/users', status: 200 });
await analytics.flush();
```

## API

### `new TrailpostClient(options)`

| Option | Type | Default | Description |
|---|---|---|---|
| `apiKey` | `string` | required | API key |
| `endpoint` | `string` | required | Trailpost server URL |
| `projectName` | `string` | — | Project identifier |
| `flushInterval` | `number` | `5000` | Flush interval in ms |
| `sessionTimeout` | `number` | `1800000` | Session idle timeout in ms |

### `analytics.page(properties?)`

Track a page view. Auto-detects URL and referrer from `window.location` in browsers.

### `analytics.track(name, properties?)`

Track a custom event.

### `analytics.identify(userId)`

Associate events with a user ID (hashed server-side).

### `analytics.flush()`

Flush all queued events immediately. Returns a Promise.
