#!/usr/bin/env python3
"""Pods that Traefik routes to drain before they stop.

A production deploy on 2026-10-21 replaced the readers while Traefik still
routed to the old pods. They refused connections, Traefik answered 502 without
CORS headers, and a browser on another origin reported CORS blocks on
/api/v1/events and /api/v1/recordings/chunk.

  - every reader and web container sleeps in preStop, so it keeps serving while
    it leaves the endpoints, and the sleep plus the server's 5s shutdown fits
    in the grace period
  - the reader StatefulSet waits minReadySeconds before it stops the next reader

Run with:  python3 -m unittest discover -s scripts -p 'test_*.py'
"""

from __future__ import annotations

import unittest
from pathlib import Path

import yaml

REPO_ROOT = Path(__file__).resolve().parent.parent
ENVS = ("testing", "staging", "production")
ROUTED = ("reader-statefulset.yaml", "web-deployment.yaml")

# cmd/funnelbarn/shutdown.go gives server.Shutdown 5 seconds.
SERVER_SHUTDOWN_SECONDS = 5
DEFAULT_GRACE_SECONDS = 30


def load(env: str, name: str) -> dict:
    return yaml.safe_load((REPO_ROOT / "deploy" / "k8s" / env / name).read_text())


def prestop_sleep(container: dict) -> int:
    command = container.get("lifecycle", {}).get("preStop", {}).get("exec", {}).get("command", [])
    if len(command) != 2 or command[0] != "sleep":
        return 0
    return int(command[1])


class RolloutDrainTest(unittest.TestCase):
    def test_routed_containers_sleep_before_they_stop(self):
        for env in ENVS:
            for name in ROUTED:
                spec = load(env, name)["spec"]["template"]["spec"]
                grace = spec.get("terminationGracePeriodSeconds", DEFAULT_GRACE_SECONDS)
                for container in spec["containers"]:
                    with self.subTest(env=env, manifest=name, container=container["name"]):
                        sleep = prestop_sleep(container)
                        self.assertGreaterEqual(sleep, 3, "preStop must sleep while Traefik drops the pod")
                        self.assertLess(sleep + SERVER_SHUTDOWN_SECONDS, grace)

    def test_readers_roll_one_at_a_time_with_a_ready_margin(self):
        for env in ENVS:
            with self.subTest(env=env):
                spec = load(env, "reader-statefulset.yaml")["spec"]
                self.assertGreaterEqual(spec["replicas"], 2)
                self.assertEqual(spec["updateStrategy"]["type"], "RollingUpdate")
                self.assertGreaterEqual(spec.get("minReadySeconds", 0), 5)


if __name__ == "__main__":
    unittest.main()
