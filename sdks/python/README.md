# funnelbarn-python

Python SDK for [FunnelBarn](https://github.com/wiebe-xyz/funnelbarn) — self-hosted web analytics.

## Installation

> **Not yet published to PyPI.** The package metadata is ready (`name = "funnelbarn"`, `version = "1.0.0"`) but no release has been pushed. Until publishing is wired in, install from source:

```bash
pip install git+https://github.com/webwiebe/funnelbarn.git#subdirectory=sdks/python
```

Or vendor `sdks/python/funnelbarn/` into your project and import directly.

## Usage

```python
from funnelbarn import FunnelBarnClient

analytics = FunnelBarnClient(
    api_key="your-api-key",
    endpoint="https://funnelbarn.example.com",
    project_name="my-app",
)

analytics.page("https://example.com/pricing", referrer="https://google.com")
analytics.track("signup", {"plan": "pro"})
analytics.identify("user_123")

# On shutdown
analytics.shutdown()
```

## API

### `FunnelBarnClient(api_key, endpoint, project_name="")`

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
