"""Gate: every artifact binary-release.yml uploads outlives a next-day re-run.

The defect this locks down: the .deb and macOS tarball uploads kept their
artifacts for 1 day. On 2026-10-18 the attach and Homebrew jobs of run
39058503372 were re-run the day after the build, found no artifacts and
failed, so v0.6.109 shipped without tarballs, debs or a brew update. A full
re-run is no way out, since it cuts a new tag.

The test reads the workflow text and runs nothing.
"""

from __future__ import annotations

import unittest
from pathlib import Path

import yaml

REPO_ROOT = Path(__file__).resolve().parent.parent
WORKFLOW = REPO_ROOT / ".github" / "workflows" / "binary-release.yml"
MIN_RETENTION_DAYS = 3


def upload_steps() -> list[tuple[str, dict]]:
    data = yaml.safe_load(WORKFLOW.read_text())
    found = []
    for job_name, job in (data.get("jobs") or {}).items():
        for step in job.get("steps") or []:
            if str(step.get("uses", "")).startswith("actions/upload-artifact"):
                found.append((job_name, step))
    return found


class ReleaseArtifactRetentionTest(unittest.TestCase):
    def test_there_are_uploads_to_check(self):
        self.assertTrue(upload_steps(), "no upload-artifact step found; update this test if they were removed")

    def test_uploads_survive_a_next_day_rerun(self):
        for job, step in upload_steps():
            days = (step.get("with") or {}).get("retention-days")
            # Without retention-days the repository default (90 days) applies.
            if days is None:
                continue
            self.assertGreaterEqual(
                int(days),
                MIN_RETENTION_DAYS,
                f"{job}: retention-days {days} expires before a next-day re-run of the attach jobs",
            )


if __name__ == "__main__":
    unittest.main()
