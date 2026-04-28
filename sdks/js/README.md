# @funnelbarn/js

Browser + Node.js SDK for [FunnelBarn](https://github.com/wiebe-xyz/funnelbarn) — self-hosted web analytics.

## Installation

```bash
npm install @funnelbarn/js
```

## Usage (browser)

```html
<script type="module">
  import { FunnelBarnClient } from '@funnelbarn/js';

  const analytics = new FunnelBarnClient({
    apiKey: 'your-api-key',
    endpoint: 'https://funnelbarn.example.com',
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
import { FunnelBarnClient } from '@funnelbarn/js';

const analytics = new FunnelBarnClient({
  apiKey: process.env.FUNNELBARN_API_KEY!,
  endpoint: process.env.FUNNELBARN_ENDPOINT!,
  projectName: 'my-api',
});

analytics.track('api_request', { route: '/users', status: 200 });
await analytics.flush();
```

## API

### `new FunnelBarnClient(options)`

| Option | Type | Default | Description |
|---|---|---|---|
| `apiKey` | `string` | required | API key |
| `endpoint` | `string` | required | FunnelBarn server URL |
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
