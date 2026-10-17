#!/usr/bin/env python3
"""Asserts the shape of .github/workflows/deploy-production.yml (issue #364).

Production is reached by the promote job in build-and-test.yml (workflow_call)
or by a manual dispatch kept for rollbacks. Each assertion fails if one
safeguard is removed:

  - dropping workflow_call                     -> test_callable_from_the_pipeline
  - moving the smoke check below the rollback  -> test_smoke_runs_before_the_rollback
  - the rollback losing its failure condition  -> test_rollback_runs_on_failure
  - a deploy that skips the GHCR preflight     -> test_deploy_needs_preflight

Run with:  python3 -m unittest discover -s scripts -p 'test_*.py'
"""

from __future__ import annotations

import unittest
from pathlib import Path

import yaml

WORKFLOW = Path(__file__).resolve().parent.parent / ".github" / "workflows" / "deploy-production.yml"


class DeployProductionTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        data = yaml.safe_load(WORKFLOW.read_text())
        cls.triggers = data.get("on", data.get(True))
        cls.jobs = data["jobs"]
        cls.steps = cls.jobs["deploy"]["steps"]

    def _index(self, name: str) -> int:
        for i, step in enumerate(self.steps):
            if step.get("name") == name:
                return i
        raise AssertionError(f"no step named {name!r}")

    def test_callable_from_the_pipeline(self):
        self.assertIn("workflow_call", self.triggers)
        self.assertIn("production_version", self.triggers["workflow_call"]["inputs"])

    def test_manual_dispatch_stays_as_the_rollback_path(self):
        self.assertIn("workflow_dispatch", self.triggers)

    def test_deploy_needs_preflight(self):
        self.assertIn("preflight", self.jobs["deploy"]["needs"])

    def test_smoke_runs_before_the_rollback(self):
        self.assertLess(
            self._index("Smoke check production"), self._index("Rollback on failure")
        )

    def test_smoke_checks_the_released_version(self):
        smoke = self.steps[self._index("Smoke check production")]
        self.assertIn("scripts/smoke.sh", smoke["run"])
        self.assertIn("https://funnelbarn.wiebe.xyz", smoke["run"])
        self.assertIn("inputs.production_version", smoke["run"])

    def test_preflight_resolves_digests_and_deploy_pins_them(self):
        outputs = self.jobs["preflight"]["outputs"]
        self.assertIn("service_digest", outputs)
        self.assertIn("web_digest", outputs)
        pin = self.steps[self._index("Pin manifests to this release")]["run"]
        self.assertIn("needs.preflight.outputs.service_digest", pin)
        self.assertIn("needs.preflight.outputs.web_digest", pin)

    def test_rollback_reads_the_sha_from_the_annotation_not_the_image_string(self):
        pre = self.steps[self._index("Record restore point and detect migrations")]["run"]
        self.assertIn("deploy/deployed-sha.sh", pre)
        self.assertIn("annotations.funnelbarn", pre)
        self.assertIn("release-sha", pre)
        self.assertNotIn("sed 's/.*://'", pre)

    def test_release_sha_is_recorded_after_the_smoke_check(self):
        self.assertLess(
            self._index("Smoke check production"), self._index("Record released SHA")
        )
        self.assertLess(
            self._index("Record released SHA"), self._index("Rollback on failure")
        )

    def test_rollback_runs_on_failure(self):
        self.assertEqual(self.steps[self._index("Rollback on failure")]["if"], "failure()")

    def test_release_marker_is_not_posted_after_a_failure(self):
        marker = self.steps[self._index("Post release marker to BugBarn")]
        self.assertNotIn("always()", marker["if"])
        self.assertLess(
            self._index("Rollback on failure"), self._index("Post release marker to BugBarn")
        )


if __name__ == "__main__":
    unittest.main(verbosity=2)
