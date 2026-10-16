#!/usr/bin/env python3
"""scripts/smoke.sh is the gate between staging and production, so each of its
failure modes is proven here against a local stub server: a healthy staging of
the right version passes; an unready one, a stale version and an unreachable
host all fail.

Run with:  python3 -m unittest discover -s scripts -p 'test_*.py'
"""

from __future__ import annotations

import json
import subprocess
import threading
import unittest
from http.server import BaseHTTPRequestHandler, HTTPServer
from pathlib import Path

SMOKE = Path(__file__).resolve().parent / "smoke.sh"


def serve(status: int, body: dict):
    class Handler(BaseHTTPRequestHandler):
        def do_GET(self):  # noqa: N802
            payload = json.dumps(body).encode()
            self.send_response(status)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(payload)))
            self.end_headers()
            self.wfile.write(payload)

        def log_message(self, *args):
            pass

    server = HTTPServer(("127.0.0.1", 0), Handler)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    return server


def run_smoke(base: str, version: str) -> subprocess.CompletedProcess:
    env = {"PATH": "/usr/bin:/bin:/usr/local/bin:/opt/homebrew/bin", "SMOKE_ATTEMPTS": "2", "SMOKE_DELAY": "0"}
    return subprocess.run(
        ["bash", str(SMOKE), base, version], capture_output=True, text=True, env=env, timeout=60
    )


class SmokeTest(unittest.TestCase):
    def check(self, status: int, body: dict, version: str = "abc123") -> subprocess.CompletedProcess:
        server = serve(status, body)
        self.addCleanup(server.server_close)
        self.addCleanup(server.shutdown)
        return run_smoke(f"http://127.0.0.1:{server.server_port}", version)

    def test_passes_when_ready_on_the_expected_version(self):
        result = self.check(200, {"status": "ok", "version": "abc123"})
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_fails_when_not_ready(self):
        result = self.check(503, {"status": "not_ready", "reason": "migrating"})
        self.assertEqual(result.returncode, 1)

    def test_fails_on_a_stale_version(self):
        """The pods answering are still the previous release."""
        result = self.check(200, {"status": "ok", "version": "old999"})
        self.assertEqual(result.returncode, 1)
        self.assertIn("abc123", result.stderr)

    def test_fails_when_status_is_not_ok(self):
        result = self.check(200, {"status": "degraded", "version": "abc123"})
        self.assertEqual(result.returncode, 1)

    def test_fails_when_nothing_answers(self):
        result = run_smoke("http://127.0.0.1:9", "abc123")
        self.assertEqual(result.returncode, 1)

    def test_usage_error_without_arguments(self):
        result = subprocess.run(["bash", str(SMOKE)], capture_output=True, text=True, timeout=10)
        self.assertEqual(result.returncode, 2)


if __name__ == "__main__":
    unittest.main(verbosity=2)
