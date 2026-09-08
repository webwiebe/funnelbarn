#!/usr/bin/env python3
"""Asserts the shape of the deploy graph in .github/workflows/build-and-test.yml.

The defect this locks down: the workflow triggered on `push: branches: [main]`
with no dependency on CI, so a red CI run did not stop a testing or staging
deploy. The sibling repo iambarn shipped that on 2026-09-07: CI run
34091298203 FAILED on e25d82c3 at 06:32:28 while deploy run 34091298267
SUCCEEDED at 06:32:29 on the same SHA.

A repair nobody can regress is worth more than a repair, so these are
assertions rather than a comment. Each one fails if the corresponding half of
the fix is removed:

  - re-adding a `push` trigger              -> test_no_push_trigger
  - dropping the conclusion == 'success' if -> test_build_requires_ci_success
  - checking out the branch head instead of
    the commit CI passed                    -> test_build_checks_out_the_sha_ci_passed
  - making deploy-staging a sibling of e2e
    again                                   -> test_staging_waits_for_e2e

Run with:  python3 -m unittest discover -s scripts -p 'test_*.py'
"""

from __future__ import annotations

import unittest
from pathlib import Path

import yaml

REPO_ROOT = Path(__file__).resolve().parent.parent
WORKFLOW = REPO_ROOT / ".github" / "workflows" / "build-and-test.yml"

HEAD_SHA = "github.event.workflow_run.head_sha"


def load_workflow() -> dict:
    data = yaml.safe_load(WORKFLOW.read_text())
    # PyYAML resolves the bare key `on:` to the boolean True (YAML 1.1).
    triggers = data.get("on", data.get(True))
    if triggers is None:
        raise AssertionError("workflow has no trigger block")
    data["__triggers__"] = triggers
    return data


class DeployGraphTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.wf = load_workflow()
        cls.triggers = cls.wf["__triggers__"]
        cls.jobs = cls.wf["jobs"]

    def test_no_push_trigger(self):
        """A push must not be able to start a deploy on its own."""
        self.assertNotIn(
            "push",
            self.triggers,
            "build-and-test must not trigger on push: that is the defect this repair removes",
        )

    def test_triggered_by_the_ci_workflow_completing(self):
        self.assertIn("workflow_run", self.triggers)
        run = self.triggers["workflow_run"]
        self.assertIn("CI", run["workflows"])
        self.assertIn("completed", run["types"])
        self.assertEqual(run["branches"], ["main"])

    def test_build_requires_ci_success(self):
        condition = self.jobs["build"]["if"]
        self.assertIn("github.event.workflow_run.conclusion == 'success'", condition)
        # workflow_dispatch is the only other way in, and it is deliberate.
        self.assertIn("github.event_name == 'workflow_dispatch'", condition)

    def test_build_checks_out_the_sha_ci_passed(self):
        """Without head_sha the deploy builds a different commit than CI passed."""
        checkout = self._checkout_step(self.jobs["build"])
        self.assertIn(HEAD_SHA, checkout["with"]["ref"])

    def test_every_job_builds_or_deploys_one_commit(self):
        """Downstream jobs take the SHA from build, never the branch head."""
        self.assertEqual(
            self.jobs["build"]["outputs"]["sha"], "${{ steps.commit.outputs.sha }}"
        )
        for name in ("deploy", "e2e", "deploy-staging"):
            with self.subTest(job=name):
                job = self.jobs[name]
                self.assertIn("build", job["needs"])
                checkout = self._checkout_step(job)
                self.assertEqual(
                    checkout["with"]["ref"], "${{ needs.build.outputs.sha }}"
                )

    def test_image_tag_is_the_gated_sha_not_github_sha(self):
        raw = WORKFLOW.read_text()
        self.assertNotIn(
            "${{ github.sha }}:",
            raw,
            "image tags must come from the CI-passed SHA",
        )
        for job_name in ("deploy", "deploy-staging"):
            self.assertEqual(
                self.jobs[job_name]["env"]["IMAGE_TAG"],
                "${{ needs.build.outputs.sha }}",
            )

    def test_staging_waits_for_e2e(self):
        """specs/003-engineering-standards/spec.md: E2E failures block promotion to staging."""
        self.assertIn("e2e", self.jobs["deploy-staging"]["needs"])
        self.assertIn("deploy", self.jobs["e2e"]["needs"])

    def test_no_job_swallows_a_failure(self):
        raw = WORKFLOW.read_text()
        for lineno, line in enumerate(raw.splitlines(), start=1):
            stripped = line.strip()
            if stripped.startswith("#"):
                continue
            if "|| true" in stripped and "BugBarn release marker" not in stripped:
                # The BugBarn release marker is explicitly best-effort; a rollout,
                # a secret apply or a test run is not.
                self.assertIn(
                    "echo",
                    stripped,
                    f"{WORKFLOW.name}:{lineno} swallows a failure: {stripped}",
                )
            self.assertNotIn(
                "continue-on-error",
                stripped,
                f"{WORKFLOW.name}:{lineno} uses continue-on-error",
            )

    def _checkout_step(self, job: dict) -> dict:
        for step in job["steps"]:
            if str(step.get("uses", "")).startswith("actions/checkout"):
                return step
        raise AssertionError("job has no checkout step")


class SpecMatchesMechanismTest(unittest.TestCase):
    """The spec asserted a pipeline shape the workflow did not implement."""

    def test_spec_still_claims_e2e_blocks_staging(self):
        spec = (REPO_ROOT / "specs" / "003-engineering-standards" / "spec.md").read_text()
        self.assertIn("E2E failures block promotion to staging", spec)


if __name__ == "__main__":
    unittest.main(verbosity=2)
