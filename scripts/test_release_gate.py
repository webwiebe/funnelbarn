"""Gate: binary-release.yml releases only a commit whose CI run succeeded.

The defect this locks down: the workflow ran on every push to main beside CI and
nothing consumed CI's result, so a red CI run still tagged the commit and
published apt, brew, npm and PyPI artifacts. build-and-test.yml already gates on
CI through workflow_run; this workflow does the same, from the commit CI passed.

The test reads the workflow text and runs nothing.
"""

from __future__ import annotations

import unittest
from pathlib import Path

import yaml

REPO_ROOT = Path(__file__).resolve().parent.parent
WORKFLOW = REPO_ROOT / ".github" / "workflows" / "binary-release.yml"
HEAD_SHA = "github.event.workflow_run.head_sha"


def load() -> dict:
    data = yaml.safe_load(WORKFLOW.read_text())
    # PyYAML reads the bare key `on` as the boolean True.
    data["on"] = data.get("on", data.get(True))
    return data


class ReleaseGateTest(unittest.TestCase):
    def setUp(self):
        self.data = load()
        self.jobs = self.data["jobs"]

    def test_triggers_on_ci_completing_not_on_push(self):
        triggers = self.data["on"]
        self.assertNotIn("push", triggers, "a push trigger releases without waiting for CI")
        run = triggers["workflow_run"]
        self.assertEqual(run["workflows"], ["CI"])
        self.assertEqual(run["types"], ["completed"])
        self.assertEqual(run["branches"], ["main"])
        self.assertIn("workflow_dispatch", triggers)

    def test_tag_job_needs_a_green_ci_run(self):
        condition = str(self.jobs["tag"].get("if", ""))
        self.assertIn("workflow_run.conclusion == 'success'", condition)

    def test_every_other_job_depends_on_the_tag_job(self):
        # A skipped tag job (red CI) then skips the whole workflow.
        for name, job in self.jobs.items():
            if name == "tag":
                continue
            needs = job.get("needs") or []
            needs = [needs] if isinstance(needs, str) else needs
            self.assertIn("tag", needs, f"job {name} runs without the CI gate")

    def test_release_sha_is_the_commit_ci_passed(self):
        sha = str(self.data["env"]["RELEASE_SHA"])
        self.assertIn(HEAD_SHA, sha)

    def test_every_checkout_uses_that_commit(self):
        for name, job in self.jobs.items():
            for step in job.get("steps") or []:
                if str(step.get("uses", "")).startswith("actions/checkout"):
                    ref = str((step.get("with") or {}).get("ref", ""))
                    self.assertIn("RELEASE_SHA", ref, f"job {name} checks out the branch head")

    def test_no_step_reads_the_branch_head_sha(self):
        text = WORKFLOW.read_text()
        self.assertNotIn("GITHUB_SHA", text)
        self.assertNotIn("github.sha }}", text.replace("|| github.sha }}", ""))


if __name__ == "__main__":
    unittest.main()
