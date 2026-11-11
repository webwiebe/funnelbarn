#!/usr/bin/env python3
"""Asserts the shape of .github/workflows/govulncheck.yml (issue #514).

  - an unpinned scanner                          -> test_scanner_is_pinned
  - a Go version that differs from ci.yml        -> test_go_version_matches_ci
  - losing the daily run or the pull request run -> test_runs_on_prs_main_and_daily

Run with:  python3 -m unittest discover -s scripts -p 'test_*.py'
"""

from __future__ import annotations

import re
import unittest
from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parent.parent
WORKFLOW = ROOT / ".github" / "workflows" / "govulncheck.yml"
CI = ROOT / ".github" / "workflows" / "ci.yml"


class GovulncheckWorkflowTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.text = WORKFLOW.read_text()
        data = yaml.safe_load(cls.text)
        cls.triggers = data.get("on", data.get(True))

    def test_runs_on_prs_main_and_daily(self):
        for trigger in ("pull_request", "push", "schedule"):
            self.assertIn(trigger, self.triggers)

    def test_scanner_is_pinned(self):
        self.assertRegex(self.text, r"govulncheck@v\d+\.\d+\.\d+")
        self.assertNotIn("govulncheck@latest", self.text)

    def test_go_version_matches_ci(self):
        want = set(re.findall(r"go-version: '([\d.]+)'", CI.read_text()))
        got = set(re.findall(r"go-version: '([\d.]+)'", self.text))
        self.assertEqual(want, got)

    def test_the_step_fails_on_findings(self):
        self.assertNotIn("continue-on-error", self.text)
        self.assertIn("scripts/govulncheck.sh", self.text)


if __name__ == "__main__":
    unittest.main()
