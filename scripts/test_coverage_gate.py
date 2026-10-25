#!/usr/bin/env python3
"""Tests for scripts/coverage-gate.py: a failed `go test` run must say why.

The gate used to send go test output to DEVNULL, so a red gate on a CI runner
named no test and no cause (PR #391, 2026-10-23).

Run with:  python3 scripts/test_coverage_gate.py
"""

from __future__ import annotations

import contextlib
import importlib.util
import io
import subprocess
import unittest
from pathlib import Path
from unittest import mock

# The script is hyphenated (it is a CLI, not a library), so load it by path.
_SPEC = importlib.util.spec_from_file_location(
    "coverage_gate", Path(__file__).resolve().parent / "coverage-gate.py"
)
gate = importlib.util.module_from_spec(_SPEC)
_SPEC.loader.exec_module(gate)

FAILED_RUN = "\n".join(
    [f"ok  \tgithub.com/wiebe-xyz/funnelbarn/internal/p{i}\t1.0s\tcoverage: 50.0% of statements in ./..." for i in range(100)]
    + [
        "--- FAIL: TestSomething (0.01s)",
        "    something_test.go:42: expected 404, got 500",
        "FAIL",
        "FAIL\tgithub.com/wiebe-xyz/funnelbarn/internal/api\t3.1s",
        "open /var/folders/x/T/go-build123/b001/_testmain.go: no space left on device",
    ]
    + [f"ok  \tgithub.com/wiebe-xyz/funnelbarn/internal/q{i}\t1.0s" for i in range(100)]
)


class FailureExcerptTest(unittest.TestCase):
    def test_names_the_cause_even_when_it_is_far_from_the_end(self):
        out = gate.failure_excerpt(FAILED_RUN)
        for want in (
            "--- FAIL: TestSomething",
            "something_test.go:42: expected 404, got 500",
            "FAIL\tgithub.com/wiebe-xyz/funnelbarn/internal/api",
            "no space left on device",
        ):
            self.assertIn(want, out)
        self.assertNotIn("internal/p0\t", out, "passing packages are not failure lines")

    def test_keeps_the_tail(self):
        out = gate.failure_excerpt(FAILED_RUN, tail=3)
        self.assertTrue(out.endswith("internal/q99\t1.0s"))
        self.assertIn("last 3 lines:", out)

    def test_output_without_failure_lines_still_shows_the_tail(self):
        out = gate.failure_excerpt("go: downloading x\nexit status 2")
        self.assertNotIn("failure lines:", out)
        self.assertIn("exit status 2", out)


class MeasureTest(unittest.TestCase):
    def test_failed_go_test_prints_its_output_and_exits(self):
        done = subprocess.CompletedProcess(args=["go"], returncode=1, stdout=FAILED_RUN)
        stderr = io.StringIO()
        with mock.patch.object(gate.subprocess, "run", return_value=done), \
                contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(stderr):
            with self.assertRaises(SystemExit) as exc:
                gate.measure()
        self.assertEqual(exc.exception.code, 1)
        self.assertIn("go test` failed", stderr.getvalue())
        self.assertIn("--- FAIL: TestSomething", stderr.getvalue())
        self.assertIn("no space left on device", stderr.getvalue())


if __name__ == "__main__":
    unittest.main()
