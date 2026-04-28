"""
Trailpost Python SDK — self-hosted web analytics client.
"""

from __future__ import annotations

import hashlib
import json
import threading
import time
from datetime import datetime, timezone
from queue import Empty, Queue
from typing import Any, Optional
from urllib.request import Request, urlopen
from urllib.error import URLError


class TrailpostClient:
    """
    Thread-safe client for sending analytics events to a Trailpost server.

    Usage::

        from trailpost import TrailpostClient

        analytics = TrailpostClient(
            api_key="your-api-key",
            endpoint="https://analytics.example.com",
            project_name="my-app",
        )
        analytics.page("https://example.com/pricing")
        analytics.track("signup", {"plan": "pro"})
        analytics.flush()
        analytics.shutdown()
    """

    def __init__(
        self,
        api_key: str,
        endpoint: str,
        project_name: str = "",
        flush_interval: float = 5.0,
        queue_size: int = 256,
        timeout: float = 5.0,
    ) -> None:
        self._api_key = api_key
        self._endpoint = endpoint.rstrip("/")
        self._project_name = project_name
        self._flush_interval = flush_interval
        self._timeout = timeout
        self._queue: Queue[dict[str, Any]] = Queue(maxsize=queue_size)
        self._user_id: Optional[str] = None
        self._shutdown_event = threading.Event()

        self._worker = threading.Thread(target=self._run, daemon=True)
        self._worker.start()

    def page(self, url: str, referrer: str = "", properties: Optional[dict] = None) -> None:
        """Track a page view."""
        self._enqueue({
            "name": "page_view",
            "url": url,
            "referrer": referrer,
            "properties": properties or {},
            "user_id": self._user_id or "",
            "timestamp": _now(),
        })

    def track(self, name: str, properties: Optional[dict] = None) -> None:
        """Track a custom event."""
        if not name:
            return
        self._enqueue({
            "name": name,
            "properties": properties or {},
            "user_id": self._user_id or "",
            "timestamp": _now(),
        })

    def identify(self, user_id: str) -> None:
        """Associate subsequent events with a user ID."""
        self._user_id = user_id or None

    def flush(self, timeout: float = 5.0) -> None:
        """Wait for all queued events to be sent."""
        deadline = time.monotonic() + timeout
        while not self._queue.empty() and time.monotonic() < deadline:
            time.sleep(0.05)

    def shutdown(self, timeout: float = 5.0) -> None:
        """Flush and stop the background worker."""
        self.flush(timeout)
        self._shutdown_event.set()
        self._worker.join(timeout=timeout)

    # --------------------------------------------------------------------------

    def _enqueue(self, event: dict[str, Any]) -> None:
        try:
            self._queue.put_nowait(event)
        except Exception:
            # Drop on full queue — best effort.
            pass

    def _run(self) -> None:
        while not self._shutdown_event.is_set():
            try:
                event = self._queue.get(timeout=self._flush_interval)
                self._send(event)
                self._queue.task_done()
            except Empty:
                continue
            except Exception:
                # Never crash the background thread.
                pass

        # Drain remaining events on shutdown.
        while True:
            try:
                event = self._queue.get_nowait()
                self._send(event)
                self._queue.task_done()
            except Empty:
                break
            except Exception:
                break

    def _send(self, event: dict[str, Any]) -> None:
        url = f"{self._endpoint}/api/v1/events"
        body = json.dumps(event).encode()
        headers = {
            "Content-Type": "application/json",
            "x-trailpost-api-key": self._api_key,
        }
        if self._project_name:
            headers["x-trailpost-project"] = self._project_name

        req = Request(url, data=body, headers=headers, method="POST")
        try:
            with urlopen(req, timeout=self._timeout) as resp:
                _ = resp.read()
        except URLError:
            # Best-effort: silently discard on network error.
            pass


def _now() -> str:
    return datetime.now(tz=timezone.utc).isoformat()


def _hash_user_id(user_id: str) -> str:
    if not user_id:
        return ""
    return hashlib.sha256(user_id.encode()).hexdigest()


__all__ = ["TrailpostClient"]
