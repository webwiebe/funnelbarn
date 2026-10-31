#!/usr/bin/env python3
"""Asserts that every job in a pull request workflow runs for branches in this repository only.

Each assertion fails if one condition goes:

  - a job without the condition, or with another one -> test_jobs_run_for_this_repository_only
  - the check finding no workflow to look at         -> test_a_workflow_runs_on_pull_requests

Run with:  python3 -m unittest discover -s scripts -p 'test_*.py'
"""

from __future__ import annotations

import pathlib
import unittest

import yaml

REPO_ROOT = pathlib.Path(__file__).resolve().parent.parent
SAME_REPO = "github.event_name != 'pull_request' || github.event.pull_request.head.repo.full_name == github.repository"


def on_pull_requests():
    found = []
    for path in sorted((REPO_ROOT / ".github" / "workflows").glob("*.y*ml")):
        workflow = yaml.safe_load(path.read_text())
        # YAML 1.1 reads the bare key `on` as True.
        on = workflow.get(True, workflow.get("on"))
        if "pull_request" in ([on] if isinstance(on, str) else list(on)):
            found.append((path.name, workflow))
    return found


class SameRepoCi(unittest.TestCase):
    def test_a_workflow_runs_on_pull_requests(self):
        self.assertTrue(on_pull_requests())

    def test_jobs_run_for_this_repository_only(self):
        for name, workflow in on_pull_requests():
            for job_name, job in workflow["jobs"].items():
                self.assertEqual(job.get("if"), SAME_REPO, f"{name}: {job_name}")


if __name__ == "__main__":
    unittest.main()
