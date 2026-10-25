"""Gate: the version smoke, and the deploy wiring it depends on.

assert-version.sh proves the deployed release equals what the pods report. It
is only worth anything if it fails on a mismatch and every deploy runs it, so
this test:

  - runs the script against a stub kubectl: a match passes, a wrong version, a
    wrong commit and an empty reply each fail
  - requires each deploy workflow to run the smoke against the writer and a reader
  - requires the image build to pass the commit to the binary

The workflows are read as text; nothing is deployed.
"""

from __future__ import annotations

import os
import stat
import subprocess
import tempfile
import unittest
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent
SCRIPT = REPO_ROOT / "deploy" / "assert-version.sh"
SHA = "0123456789abcdef0123456789abcdef01234567"


def run_script(reply: str, version: str = "v1.2.3", commit: str = SHA) -> subprocess.CompletedProcess:
    with tempfile.TemporaryDirectory() as tmp:
        stub = Path(tmp) / "kubectl"
        stub.write_text(f"#!/bin/sh\nprintf '%s' '{reply}'\n")
        stub.chmod(stub.stat().st_mode | stat.S_IXUSR)
        env = dict(os.environ, PATH=f"{tmp}:{os.environ['PATH']}")
        return subprocess.run(
            ["sh", str(SCRIPT), "ns", "deploy/funnelbarn", "funnelbarn", version, commit],
            capture_output=True, text=True, env=env,
        )


class AssertVersionScriptTest(unittest.TestCase):
    def test_matching_version_and_commit_pass(self):
        reply = f'{{"version":"v1.2.3","commit":"{SHA}","built":"2026-10-17T12:00:00Z"}}'
        self.assertEqual(run_script(reply).returncode, 0)

    def test_wrong_version_fails(self):
        reply = f'{{"version":"v1.2.2","commit":"{SHA}","built":"x"}}'
        result = run_script(reply)
        self.assertEqual(result.returncode, 1)
        self.assertIn("wanted version v1.2.3", result.stderr)

    def test_wrong_commit_fails(self):
        reply = '{"version":"v1.2.3","commit":"ffffffffffffffffffffffffffffffffffffffff","built":"x"}'
        result = run_script(reply)
        self.assertEqual(result.returncode, 1)
        self.assertIn("wanted commit", result.stderr)

    def test_empty_reply_fails(self):
        self.assertEqual(run_script("").returncode, 1)

    def test_wrong_argument_count_is_a_usage_error(self):
        result = subprocess.run(["sh", str(SCRIPT)], capture_output=True, text=True)
        self.assertEqual(result.returncode, 2)


class DeployWiringTest(unittest.TestCase):
    def workflow(self, name: str) -> str:
        return (REPO_ROOT / ".github" / "workflows" / name).read_text()

    def test_every_environment_runs_the_smoke_on_writer_and_reader(self):
        # testing and staging deploy from build-and-test.yml, production from deploy-production.yml.
        self.assertEqual(self.workflow("build-and-test.yml").count("deploy/funnelbarn funnelbarn"), 2)
        self.assertEqual(self.workflow("build-and-test.yml").count("pod/funnelbarn-reader-0 funnelbarn"), 2)
        self.assertEqual(self.workflow("deploy-production.yml").count("deploy/funnelbarn funnelbarn"), 1)
        self.assertEqual(self.workflow("deploy-production.yml").count("pod/funnelbarn-reader-0 funnelbarn"), 1)

    def test_image_build_passes_the_commit_to_the_binary(self):
        self.assertIn("--build-arg COMMIT=", self.workflow("build-and-test.yml"))
        dockerfile = (REPO_ROOT / "deploy" / "docker" / "service.Dockerfile").read_text()
        self.assertIn("-X main.Commit=${COMMIT}", dockerfile)


if __name__ == "__main__":
    unittest.main()
