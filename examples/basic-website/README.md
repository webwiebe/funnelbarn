# Basic Website Example

This example shows how to add FunnelBarn analytics to a static website.

## Setup

1. Deploy FunnelBarn (see root README)
2. Create a project and API key:
   ```bash
   funnelbarn project create --name "My Website"
   funnelbarn apikey create --project my-website --name frontend --scope ingest
   ```
3. Add the tracking snippet to your HTML

## Tracking Snippet

```html
<!DOCTYPE html>
<html>
<head>
  <!-- Sends a page view on load and exposes a global `funnelbarn`. -->
  <script src="https://funnelbarn.yourdomain.com/sdk.js"
    data-api-key="YOUR_INGEST_API_KEY"
    data-project-name="my-website"
    defer></script>
  <script>
    // Track custom events
    document.addEventListener('DOMContentLoaded', () => {
      document.querySelector('#cta-button')?.addEventListener('click', () => {
        funnelbarn.track('cta_click', { location: 'hero' });
      });
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
curl -X POST https://funnelbarn.yourdomain.com/api/v1/projects/PROJECT_ID/funnels \
  -H "x-funnelbarn-api-key: YOUR_FULL_API_KEY" \
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
// Landing page: the script tag already sent the page_view on load.

// When user clicks signup button
signupBtn.addEventListener('click', () => {
  funnelbarn.track('signup_click');
});

// After successful signup
funnelbarn.track('signup_complete', { plan: 'free' });
```
