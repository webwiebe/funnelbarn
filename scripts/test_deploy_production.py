#!/usr/bin/env python3
"""Asserts the shape of .github/workflows/deploy-production.yml (issue #364).

Production is reached by the promote job in build-and-test.yml (workflow_call)
or by a manual dispatch kept for rollbacks. Each assertion fails if one
safeguard is removed:

  - dropping workflow_call                     -> test_callable_from_the_pipeline
  - moving the smoke check below the rollback  -> test_smoke_runs_before_the_rollback
  - the rollback losing its failure condition  -> test_rollback_runs_on_failure
  - a deploy that skips the GHCR preflight     -> test_deploy_needs_preflight
  - reading github.event.inputs, which is empty
    when the promote job calls the workflow    -> test_no_step_reads_dispatch_only_inputs

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

    def test_no_step_reads_dispatch_only_inputs(self):
        # Under workflow_call github.event is the caller's workflow_run event,
        # so github.event.inputs.* expands to an empty string there.
        self.assertFalse(
            "github.event.inputs" in WORKFLOW.read_text(),
            "use inputs.* here; github.event.inputs.* is empty under workflow_call",
        )

    def test_rollout_asserts_version_and_commit_in_the_pods(self):
        rollout = self.steps[self._index("Apply manifests and roll out")]["run"]
        self.assertEqual(rollout.count("deploy/assert-version.sh"), 2)
        self.assertLess(
            self._index("Apply manifests and roll out"), self._index("Rollback on failure")
        )

    def test_rollback_runs_on_failure(self):
        self.assertEqual(self.steps[self._index("Rollback on failure")]["if"], "failure()")

    def test_rollback_restores_the_revisions_recorded_before_the_apply(self):
        # Issue #396: a plain `rollout undo` steps past the running release
        # when the apply never landed.
        pre = self.steps[self._index("Record restore point and detect migrations")]["run"]
        rollback = self.steps[self._index("Rollback on failure")]["run"]
        self.assertLess(
            self._index("Record restore point and detect migrations"),
            self._index("Apply manifests and roll out"),
        )
        self.assertIn("restore-revision.sh record", pre)
        self.assertIn('echo "${w%%:*}_revision=${REV}" >> "$GITHUB_OUTPUT"', pre)
        for key, target in (
            ("writer", "deployment/funnelbarn"),
            ("web", "deployment/funnelbarn-web"),
            ("reader", "statefulset/funnelbarn-reader"),
        ):
            self.assertIn(f"{key}:{target}", pre)
            self.assertIn(f"restore ${{NAMESPACE}} {target} '${{{{ steps.pre.outputs.{key}_revision }}}}'", rollback)
        self.assertNotIn("rollout undo", rollback)

    def test_release_marker_is_not_posted_after_a_failure(self):
        marker = self.steps[self._index("Post release marker to BugBarn")]
        self.assertNotIn("always()", marker["if"])
        self.assertLess(
            self._index("Rollback on failure"), self._index("Post release marker to BugBarn")
        )

    def test_refused_rollback_turns_the_run_red(self):
        rollback = self.steps[self._index("Rollback on failure")]["run"]
        refusal = rollback[rollback.index("NOT rolling back"):rollback.index("\nfi\n")]
        self.assertIn("exit 1", refusal)
        self.assertNotIn("continue-on-error", WORKFLOW.read_text())

    def test_refused_rollback_reports_to_bugbarn(self):
        report = self.steps[self._index("Report refused rollback to BugBarn")]
        self.assertLess(
            self._index("Rollback on failure"), self._index("Report refused rollback to BugBarn")
        )
        self.assertEqual(self.steps[self._index("Apply manifests and roll out")]["id"], "apply")
        self.assertIn("failure()", report["if"])
        self.assertIn("steps.pre.outputs.carries_migrations == 'true'", report["if"])
        self.assertIn("steps.apply.outcome != 'skipped'", report["if"])
        self.assertIn("/api/v1/events", report["run"])
        self.assertIn("RollbackRefused", report["run"])
        self.assertIn("exit 1", report["run"])
        self.assertNotIn("|| true", report["run"])


if __name__ == "__main__":
    unittest.main(verbosity=2)
