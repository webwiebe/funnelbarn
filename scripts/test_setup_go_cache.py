"""Gate: no setup-go step in a self-hosted job may turn its cache on.

The defect this locks down: `actions/setup-go` with `cache: true` tars the Go
build and module caches of the host and uploads them whenever the key misses.
On the self-hosted mac-minis those caches are ~25 GB and shared by every
runner, so the Post Set up Go step sat for 20+ minutes in every job, while the
host already keeps the caches warm between jobs.

The test reads the workflow text and runs nothing.
"""

from __future__ import annotations

import unittest
from pathlib import Path

import yaml

REPO_ROOT = Path(__file__).resolve().parent.parent
WORKFLOWS = sorted((REPO_ROOT / ".github" / "workflows").glob("*.y*ml"))


def setup_go_steps() -> list[tuple[str, str, dict]]:
    found = []
    for path in WORKFLOWS:
        data = yaml.safe_load(path.read_text())
        for job_name, job in (data.get("jobs") or {}).items():
            if "self-hosted" not in str(job.get("runs-on", "")):
                continue
            for step in job.get("steps") or []:
                if str(step.get("uses", "")).startswith("actions/setup-go"):
                    found.append((path.name, job_name, step))
    return found


class SetupGoCacheTest(unittest.TestCase):
    def test_there_are_setup_go_steps_to_check(self):
        self.assertTrue(setup_go_steps(), "no setup-go step found; update this test if they were removed")

    def test_self_hosted_setup_go_does_not_cache(self):
        for workflow, job, step in setup_go_steps():
            cache = (step.get("with") or {}).get("cache", True)
            self.assertIn(
                cache,
                (False, "false"),
                f"{workflow} job {job}: setup-go cache is on; the host cache is ~25 GB and the upload takes 20+ minutes",
            )


if __name__ == "__main__":
    unittest.main()
