#!/usr/bin/env python3
"""Adversarial tests for scripts/quality-soak.py.

Until this file existed, no gate script in this repo was tested at all: nothing
verified that a violation actually fails the build. A gate you have never seen
fail is not a gate.

The four cases the ratchet contract requires, for every dimension:

  1. clean tree              -> passes
  2. violation present       -> fails
  3. violation baselined     -> passes
  4. baseline beatable       -> fails

Plus the case that turns a gate into decoration: a tool that did not run must
fail, never report zero findings.

Run with:  python3 scripts/test_quality_soak.py
"""

from __future__ import annotations

import contextlib
import importlib.util
import io
import tempfile
import unittest
from pathlib import Path

# The script is hyphenated (it is a CLI, not a library), so load it by path.
_SPEC = importlib.util.spec_from_file_location(
    "quality_soak", Path(__file__).resolve().parent / "quality-soak.py"
)
soak = importlib.util.module_from_spec(_SPEC)
_SPEC.loader.exec_module(soak)


class RatchetEngineTest(unittest.TestCase):
    """The four properties, exercised directly on the comparison engine."""

    def test_clean_tree_passes(self):
        self.assertEqual(soak.compare({}, {}), [])

    def test_baselined_violation_passes(self):
        self.assertEqual(soak.compare({"a.go": 700}, {"a.go": 700}), [])

    def test_new_violation_fails(self):
        failures = soak.compare({"a.go": 700}, {})
        self.assertEqual(len(failures), 1)
        self.assertIn("NEW", failures[0])
        self.assertIn("a.go", failures[0])

    def test_worsened_entry_fails(self):
        failures = soak.compare({"a.go": 900}, {"a.go": 700})
        self.assertEqual(len(failures), 1)
        self.assertIn("WORSE", failures[0])

    def test_beatable_baseline_fails_as_stale(self):
        failures = soak.compare({"a.go": 600}, {"a.go": 700})
        self.assertEqual(len(failures), 1)
        self.assertIn("STALE", failures[0])
        self.assertIn("improved", failures[0])

    def test_now_clean_entry_fails_as_stale(self):
        """Property 3: the one that makes it a ratchet and not a static floor."""
        failures = soak.compare({}, {"a.go": 700})
        self.assertEqual(len(failures), 1)
        self.assertIn("STALE", failures[0])
        self.assertIn("now clean", failures[0])

    def test_unrelated_files_are_independent(self):
        failures = soak.compare({"a.go": 700, "b.go": 800}, {"a.go": 700})
        self.assertEqual(len(failures), 1)
        self.assertIn("b.go", failures[0])


class BaselineFormatTest(unittest.TestCase):
    def test_roundtrip(self):
        measured = {"b.go": 2, "a.go": 700}
        text = soak.render_baseline(measured, "header line")
        self.assertEqual(soak.parse_baseline(text), measured)

    def test_comments_and_blanks_ignored(self):
        self.assertEqual(soak.parse_baseline("# note\n\n700 a.go\n"), {"a.go": 700})

    def test_malformed_line_raises(self):
        with self.assertRaises(ValueError):
            soak.parse_baseline("700\n")
        with self.assertRaises(ValueError):
            soak.parse_baseline("seven a.go\n")

    def test_entries_are_sorted_so_the_diff_is_reviewable(self):
        text = soak.render_baseline({"z.go": 1, "a.go": 1}, "h")
        body = [line for line in text.splitlines() if not line.startswith("#")]
        self.assertEqual(body, ["1 a.go", "1 z.go"])


class FileLengthScanTest(unittest.TestCase):
    """End-to-end over a synthetic tree: the measurement itself, not just the engine."""

    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.root = Path(self.tmp.name)
        self.addCleanup(self.tmp.cleanup)

    def write(self, rel: str, lines: int):
        path = self.root / rel
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text("x\n" * lines)

    def test_clean_tree_measures_nothing(self):
        self.write("internal/small.go", soak.FILE_LINE_LIMIT)
        self.assertEqual(soak.measure_file_length(self.root), {})

    def test_over_limit_file_is_found(self):
        self.write("internal/big.go", soak.FILE_LINE_LIMIT + 1)
        self.assertEqual(
            soak.measure_file_length(self.root),
            {"internal/big.go": soak.FILE_LINE_LIMIT + 1},
        )

    def test_sdk_files_are_in_scope(self):
        """The repo's largest file lives in sdks/ and used to be outside every scan."""
        self.write("sdks/js/src/index.ts", 1076)
        self.write("sdks/python/funnelbarn/__init__.py", 900)
        measured = soak.measure_file_length(self.root)
        self.assertEqual(measured["sdks/js/src/index.ts"], 1076)
        self.assertEqual(measured["sdks/python/funnelbarn/__init__.py"], 900)

    def test_web_files_are_in_scope(self):
        self.write("web/src/pages/Big.tsx", 900)
        self.assertIn("web/src/pages/Big.tsx", soak.measure_file_length(self.root))

    def test_tests_generated_and_vendored_are_excluded(self):
        self.write("internal/thing_test.go", 900)
        self.write("internal/repository/sqlcgen/models.go", 900)
        self.write("internal/service/mock/store.go", 900)
        self.write("web/src/pages/Big.test.tsx", 900)
        self.write("sdks/js/node_modules/dep/index.ts", 900)
        self.write("sdks/js/dist/index.ts", 900)
        self.write("sdks/js/test/client.ts", 900)
        self.assertEqual(soak.measure_file_length(self.root), {})

    def test_unscanned_languages_are_ignored(self):
        self.write("internal/README.md", 900)
        self.assertEqual(soak.measure_file_length(self.root), {})


class DidNotRunTest(unittest.TestCase):
    """"Tool missing" must be distinguishable from "tool found nothing"."""

    def test_missing_binary_raises(self):
        with self.assertRaises(soak.MeasurementError) as ctx:
            soak._run(["funnelbarn-no-such-tool"], cwd=Path.cwd(), allowed_returncodes=(0,))
        self.assertIn("not on PATH", str(ctx.exception))

    def test_unexpected_exit_code_raises(self):
        with self.assertRaises(soak.MeasurementError) as ctx:
            soak._run(["sh", "-c", "exit 42"], cwd=Path.cwd(), allowed_returncodes=(0, 1))
        self.assertIn("exited 42", str(ctx.exception))

    def test_expected_exit_code_is_allowed(self):
        proc = soak._run(["sh", "-c", "exit 1"], cwd=Path.cwd(), allowed_returncodes=(0, 1))
        self.assertEqual(proc.returncode, 1)

    def test_uninstalled_web_tree_raises_instead_of_reporting_zero(self):
        with tempfile.TemporaryDirectory() as tmp:
            (Path(tmp) / "web").mkdir()
            with self.assertRaises(soak.MeasurementError) as ctx:
                soak.measure_web_complexity(Path(tmp))
        self.assertIn("node_modules", str(ctx.exception))


class DimensionWiringTest(unittest.TestCase):
    """Every dimension carries a baseline, and every baseline parses."""

    def test_every_dimension_has_a_committed_baseline(self):
        for dimension in soak.DIMENSIONS:
            path = soak.baseline_path(dimension)
            self.assertTrue(path.exists(), f"{dimension} has no committed baseline at {path}")
            soak.parse_baseline(path.read_text())

    def test_every_dimension_states_its_exit_criterion(self):
        for dimension, spec in soak.DIMENSIONS.items():
            self.assertIn("ENFORCE AT", spec["header"], f"{dimension} has no written exit criterion")

    def test_no_environment_variable_can_move_a_threshold(self):
        """A threshold overridable at run time leaves no trace in a diff."""
        source = (Path(soak.__file__)).read_text()
        self.assertNotIn("os.environ.get(", source)
        self.assertNotIn("os.getenv(", source)


class MissingBaselineTest(unittest.TestCase):
    def test_absent_baseline_fails_rather_than_passing_vacuously(self):
        original = soak.BASELINE_DIR
        with tempfile.TemporaryDirectory() as tmp:
            soak.BASELINE_DIR = Path(tmp)
            try:
                err = io.StringIO()
                with contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(err):
                    status = soak.run_dimension("file-length", update=False)
            finally:
                soak.BASELINE_DIR = original
        self.assertEqual(status, 1)
        self.assertIn("no baseline", err.getvalue())


if __name__ == "__main__":
    unittest.main(verbosity=2)
