# @funnelbarn/js

Browser + Node.js SDK for [FunnelBarn](https://github.com/wiebe-xyz/funnelbarn) — self-hosted web analytics.

## Installation

```bash
npm install https://github.com/webwiebe/funnelbarn/releases/download/funnelbarn-js-v1.1.0/funnelbarn-js-1.1.0.tgz
```

The package is not on the npm registry. Each version is released as a
tarball on a GitHub release tagged `funnelbarn-js-vX.Y.Z`; swap the version
in the URL for a newer one from the
[releases page](https://github.com/webwiebe/funnelbarn/releases).

Or load the CDN script tag — auto-tracks page views on init, no install step:

```html
<script src="https://funnelbarn.wiebe.xyz/sdk.js"
  data-api-key="<your-key>"
  data-project-name="<your-slug>"
  defer></script>
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
| `sessionMaxAge` | `number` | `86400000` | Hard cap on session lifetime in ms, regardless of activity |

### `analytics.page(properties?)`

Track a page view. Auto-detects URL and referrer from `window.location` in browsers.

### `analytics.track(name, properties?)`

Track a custom event.

### `analytics.identify(userId)`

Associate events with a user ID (hashed server-side).

### `analytics.flush()`

Flush all queued events immediately. Returns a Promise.
