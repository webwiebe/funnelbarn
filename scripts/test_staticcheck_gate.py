#!/usr/bin/env python3
"""Tests for scripts/staticcheck-gate.py: the floor's decision on recorded
output, then the real staticcheck over throwaway modules. A planted unused
function must fail the gate, and a module with no package must trip the floor.

Run with:  python3 -m unittest discover -s scripts -p 'test_*.py'
"""

from __future__ import annotations

import importlib.util
import io
import tempfile
import unittest
from pathlib import Path

# The script is hyphenated (it is a CLI, not a library), so load it by path.
_SPEC = importlib.util.spec_from_file_location(
    "staticcheck_gate", Path(__file__).resolve().parent / "staticcheck-gate.py"
)
gate = importlib.util.module_from_spec(_SPEC)
_SPEC.loader.exec_module(gate)

MEASURE = "\n".join(
    [
        "U1000\tgithub.com/wiebe-xyz/funnelbarn/internal/api\t1200",
        "U1000\tgithub.com/wiebe-xyz/funnelbarn/internal/api [github.com/wiebe-xyz/funnelbarn/internal/api.test]\t900",
        "U1000\tgithub.com/wiebe-xyz/funnelbarn/internal/api\t300",
        "SA5012\tgithub.com/wiebe-xyz/funnelbarn/internal/spool\t800",
        "inspect\tfmt\t40",
    ]
)

GO_MOD = "module example.com/planted\n\ngo 1.26\n"


class FloorDecisionTest(unittest.TestCase):
    def test_counts_distinct_packages_of_the_dead_code_analyzer(self):
        self.assertEqual(gate.analysed_packages(MEASURE), 2)

    def test_empty_or_foreign_output_counts_zero(self):
        self.assertEqual(gate.analysed_packages(""), 0)
        self.assertEqual(gate.analysed_packages("inspect\tfmt\t40\n"), 0)
        self.assertEqual(gate.analysed_packages("U1000 fmt 40\n"), 0)

    def test_at_the_floor_passes_below_it_fails(self):
        self.assertIsNone(gate.check_floor(MEASURE, 2))
        self.assertRegex(gate.check_floor(MEASURE, 3), r"ran U1000 on 2 package\(s\), expected at least 3")
        self.assertRegex(gate.check_floor("", gate.MIN_PACKAGES), r"nothing was measured")

    def test_the_floor_sits_near_the_tree(self):
        self.assertGreaterEqual(gate.MIN_PACKAGES, 80)


class RealStaticcheckTest(unittest.TestCase):
    def module(self, files: dict[str, str]) -> Path:
        root = Path(self.enterContext(tempfile.TemporaryDirectory()))
        for name, body in files.items():
            Path(root, name).write_text(body)
        return root

    def gate(self, root: Path, min_packages: int) -> tuple[int, str]:
        out, err = io.StringIO(), io.StringIO()
        code = gate.run(root, min_packages, out=out, err=err)
        return code, out.getvalue() + err.getvalue()

    def test_a_planted_unused_function_fails(self):
        root = self.module({"go.mod": GO_MOD, "planted.go": "package planted\n\nfunc plantedUnused() {}\n"})
        code, output = self.gate(root, 1)
        self.assertEqual(code, 1, output)
        self.assertIn("plantedUnused is unused (U1000)", output)

    def test_the_same_module_without_it_passes(self):
        root = self.module({"go.mod": GO_MOD, "planted.go": "package planted\n\nfunc Used() {}\n"})
        code, output = self.gate(root, 1)
        self.assertEqual(code, 0, output)

    def test_a_second_run_still_counts_the_packages(self):
        # staticcheck measures nothing for a package it has cached; the gate's
        # own cache per run is what keeps the count up.
        root = self.module({"go.mod": GO_MOD, "planted.go": "package planted\n\nfunc Used() {}\n"})
        for _ in range(2):
            code, output = self.gate(root, 1)
            self.assertEqual(code, 0, output)

    def test_a_module_with_no_package_trips_the_floor(self):
        root = self.module({"go.mod": GO_MOD})
        code, output = self.gate(root, 1)
        self.assertEqual(code, 1, output)
        self.assertIn("ran U1000 on 0 package(s)", output)


if __name__ == "__main__":
    unittest.main()
