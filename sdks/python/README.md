# trailpost-python

Python SDK for [Trailpost](https://github.com/wiebe-xyz/trailpost) — self-hosted web analytics.

## Installation

```bash
pip install trailpost
```

## Usage

```python
from trailpost import TrailpostClient

analytics = TrailpostClient(
    api_key="your-api-key",
    endpoint="https://analytics.example.com",
    project_name="my-app",
)

analytics.page("https://example.com/pricing", referrer="https://google.com")
analytics.track("signup", {"plan": "pro"})
analytics.identify("user_123")

# On shutdown
analytics.shutdown()
```

## API

### `TrailpostClient(api_key, endpoint, project_name="")`

### `client.page(url, referrer="", properties=None)`

Track a page view.

### `client.track(name, properties=None)`

Track a custom event.

### `client.identify(user_id)`

Associate events with a user ID (hashed server-side).

### `client.flush(timeout=5.0)`

Wait for all queued events to be sent.

### `client.shutdown(timeout=5.0)`

Flush and stop the background worker.
