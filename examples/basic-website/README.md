# Basic Website Example

This example shows how to add Trailpost analytics to a static website.

## Setup

1. Deploy Trailpost (see root README)
2. Create a project and API key:
   ```bash
   trailpost project create --name "My Website"
   trailpost apikey create --project my-website --name frontend --scope ingest
   ```
3. Add the tracking snippet to your HTML

## Tracking Snippet

```html
<!DOCTYPE html>
<html>
<head>
  <script type="module">
    import { TrailpostClient } from 'https://cdn.jsdelivr.net/npm/@trailpost/js/dist/esm/index.js';

    const analytics = new TrailpostClient({
      apiKey: 'YOUR_INGEST_API_KEY',
      endpoint: 'https://analytics.yourdomain.com',
      projectName: 'my-website',
    });

    // Auto-track page view on load
    analytics.page();

    // Track custom events
    document.querySelector('#cta-button')?.addEventListener('click', () => {
      analytics.track('cta_click', { location: 'hero' });
    });
  </script>
</head>
<body>
  <h1>My Website</h1>
  <button id="cta-button">Get Started</button>
</body>
</html>
```

## Funnel Example

Create a signup funnel to track conversion:

```bash
curl -X POST https://analytics.yourdomain.com/api/v1/projects/PROJECT_ID/funnels \
  -H "x-trailpost-api-key: YOUR_FULL_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Signup Funnel",
    "steps": [
      { "event_name": "page_view", "step_order": 1 },
      { "event_name": "signup_click", "step_order": 2 },
      { "event_name": "signup_complete", "step_order": 3 }
    ]
  }'
```

Then track the events:

```javascript
// Landing page
analytics.page();

// When user clicks signup button
signupBtn.addEventListener('click', () => {
  analytics.track('signup_click');
});

// After successful signup
analytics.track('signup_complete', { plan: 'free' });
```
