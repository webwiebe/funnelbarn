---
title: Python SDK
description: Server-side page view tracking, custom events, and identify with the FunnelBarn Python SDK.
order: 6
---

# Python SDK

The FunnelBarn Python SDK provides a thread-safe client that sends analytics events from your Python applications. Events are buffered in a background thread so tracking calls return immediately without blocking your application.

## Installation

```bash
pip install funnelbarn
```

## Basic setup

```python
from funnelbarn import FunnelBarnClient

analytics = FunnelBarnClient(
    api_key="your-api-key",
    endpoint="https://funnelbarn.example.com",
    project_name="my-app",
)
```

### Constructor options

| Parameter | Type | Default | Description |
|---|---|---|---|
| `api_key` | `str` | required | Your API key |
| `endpoint` | `str` | required | Full URL of your FunnelBarn instance |
| `project_name` | `str` | `""` | Project identifier sent as `x-funnelbarn-project` header |
| `flush_interval` | `float` | `5.0` | Seconds between automatic flushes |
| `queue_size` | `int` | `256` | Max events buffered in memory |
| `timeout` | `float` | `5.0` | HTTP request timeout in seconds |

## Tracking page views

```python
analytics.page(
    url="https://example.com/pricing",
    referrer="https://google.com",
    properties={"variant": "b"},
)
```

All parameters except `url` are optional.

## Tracking custom events

```python
analytics.track("signup", {"plan": "pro", "source": "landing_page"})
analytics.track("purchase", {"amount": 49.99, "currency": "USD"})
analytics.track("api_request", {"route": "/users", "status": 200, "latency_ms": 42})
```

## Identifying users

```python
# Associate subsequent events with this user
analytics.identify("user-123")

analytics.track("dashboard_viewed")

# Clear the identity (e.g. after logout)
analytics.identify("")
```

User IDs are hashed server-side before storage. The raw user ID value is never written to the database.

## Flushing and shutdown

Call `flush()` to wait for all queued events to be sent, and `shutdown()` before your process exits:

```python
# Wait for all events to drain
analytics.flush(timeout=5.0)

# Flush and stop the background thread
analytics.shutdown(timeout=5.0)
```

## Django example

Create a module-level client and call `shutdown()` on application exit:

```python
# myapp/analytics.py
import os
import atexit
from funnelbarn import FunnelBarnClient

analytics = FunnelBarnClient(
    api_key=os.environ["FUNNELBARN_API_KEY"],
    endpoint=os.environ["FUNNELBARN_ENDPOINT"],
    project_name="my-django-app",
)

atexit.register(analytics.shutdown)
```

```python
# myapp/views.py
from django.http import JsonResponse
from .analytics import analytics

def checkout(request):
    # ... process order ...
    analytics.track("purchase", {"plan": "pro", "amount": 49.99})
    return JsonResponse({"ok": True})
```

## FastAPI example

```python
from contextlib import asynccontextmanager
from fastapi import FastAPI
from funnelbarn import FunnelBarnClient
import os

analytics = FunnelBarnClient(
    api_key=os.environ["FUNNELBARN_API_KEY"],
    endpoint=os.environ["FUNNELBARN_ENDPOINT"],
    project_name="my-fastapi-app",
)

@asynccontextmanager
async def lifespan(app: FastAPI):
    yield
    analytics.shutdown()

app = FastAPI(lifespan=lifespan)

@app.post("/signup")
async def signup(data: SignupRequest):
    # ... create user ...
    analytics.track("signup", {"plan": data.plan})
    return {"ok": True}
```

## Flask example

```python
from flask import Flask, request
from funnelbarn import FunnelBarnClient
import os

app = Flask(__name__)
analytics = FunnelBarnClient(
    api_key=os.environ["FUNNELBARN_API_KEY"],
    endpoint=os.environ["FUNNELBARN_ENDPOINT"],
)

@app.teardown_appcontext
def shutdown_analytics(exception=None):
    analytics.shutdown()

@app.route("/purchase", methods=["POST"])
def purchase():
    analytics.track("purchase", request.get_json())
    return {"ok": True}
```

## Thread safety

The client is fully thread-safe. You can share one instance across threads or create multiple instances for different projects. Each instance starts its own background worker thread.
