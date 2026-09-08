#!/usr/bin/env python3
"""Report-only quality soaks with committed, ratcheting baselines.

A SOAK is not a gate. It runs a check at the estate target threshold, records
every pre-existing violation in a committed baseline, and does not fail the
build on that recorded debt. What it does fail on is drift:

  1. NEW      a path that violates and is not in the baseline
  2. WORSE    a baselined path whose count/value rose
  3. STALE    a baselined path that improved or became clean

Rule 3 is the one that makes this a ratchet rather than a floor that never
rises. Without it the baseline goes stale the first time someone improves a
file, and silently re-opens room for a regression.

The only escape hatch is --update-baseline, which is auditable in the PR diff.
There is no environment-variable override and no skip label, on purpose: a
threshold that can be changed at run time leaves no trace in a diff.

"Did not run" must never read as "clean". Every measurement below fails loudly
when its tool is missing, when the tool exits in a way that is not "ran and
reported", or when its output cannot be parsed.

Dimensions, their thresholds, and their written exit criteria:

  file-length     files over 500 lines across internal/, cmd/, web/src/ and
                  sdks/. 16 files today (largest: sdks/js/src/index.ts, 1076).
                  ENFORCE AT 0: when the baseline is empty, set
                  MAX_FILE_LINES=500 in scripts/go-quality-gates.sh, extend that
                  scan to sdks/, set eslint max-lines to 500, and delete this
                  dimension. Interim walk-down of the blocking limits:
                  848/845 -> 700 -> 600 -> 500.

  go-complexity   Go functions over cyclomatic 12 in internal/, cmd/, sdks/go.
                  27 functions in 21 files today. The blocking limit in
                  scripts/go-quality-gates.sh is still 49.
                  ENFORCE AT 0: when the baseline is empty, set MAX_CYCLO=12
                  there and delete this dimension. Interim walk-down of the
                  blocking limit: 49 -> 30 -> 20 -> 15 -> 12.

  web-complexity  web/src functions over cyclomatic 12. 22 violations in 17
                  files today. The blocking eslint limit is still 55.
                  ENFORCE AT 0: when the baseline is empty, set
                  complexity: ['error', 12] in web/eslint.config.js, delete
                  web/eslint.soak.config.js and this dimension. Interim
                  walk-down: 55 -> 30 -> 20 -> 15 -> 12.

  golangci        golangci-lint run ./... with the repo's own .golangci.yml
                  (errcheck, govet, staticcheck, unused, ineffassign, misspell,
                  goimports). 120 findings in 57 files today: 88 errcheck,
                  27 goimports, 5 staticcheck. Nothing had ever run this config
                  and it did not even load.
                  ENFORCE AT 0: when the baseline is empty, replace this
                  dimension with a blocking `golangci-lint run ./...` step in
                  ci.yml's go-quality job. The 27 goimports findings clear with
                  `golangci-lint fmt`; errcheck is the real work.

Usage:
    python3 scripts/quality-soak.py <dimension> [--update-baseline]
    python3 scripts/quality-soak.py all
"""

from __future__ import annotations

import argparse
import json
import os
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent
BASELINE_DIR = REPO_ROOT / "scripts" / "soak-baselines"

# Pinned tool versions. A soak that fetches @latest is a check whose behaviour
# can change without a commit; that is how a green run stops meaning anything.
GOCYCLO_VERSION = "v0.6.0"
GOLANGCI_VERSION = "v2.13.2"

# Estate targets. These are the thresholds the soaks measure against; they are
# NOT the blocking limits, which are walked down separately.
FILE_LINE_LIMIT = 500
CYCLO_LIMIT = 12


class MeasurementError(RuntimeError):
    """A tool did not run, or ran and produced something unusable."""


# --------------------------------------------------------------------------
# ratchet engine
# --------------------------------------------------------------------------


def parse_baseline(text: str) -> dict[str, int]:
    out: dict[str, int] = {}
    for lineno, raw in enumerate(text.splitlines(), start=1):
        line = raw.strip()
        if not line or line.startswith("#"):
            continue
        parts = line.split(None, 1)
        if len(parts) != 2:
            raise ValueError(f"baseline line {lineno}: expected '<value> <path>', got {raw!r}")
        value, path = parts
        try:
            out[path] = int(value)
        except ValueError as exc:
            raise ValueError(f"baseline line {lineno}: {value!r} is not an integer") from exc
    return out


def render_baseline(measured: dict[str, int], header: str) -> str:
    lines = [f"# {h}" for h in header.strip().splitlines()]
    lines.append(f"# entries: {len(measured)}  total: {sum(measured.values())}")
    lines.append("#")
    for path in sorted(measured):
        lines.append(f"{measured[path]} {path}")
    return "\n".join(lines) + "\n"


def compare(measured: dict[str, int], baseline: dict[str, int]) -> list[str]:
    """Return a list of failure lines. Empty means the soak passes."""
    failures: list[str] = []

    for path in sorted(measured):
        if path not in baseline:
            failures.append(f"NEW    {path}: {measured[path]} (not in baseline)")
        elif measured[path] > baseline[path]:
            failures.append(f"WORSE  {path}: {measured[path]} (baseline {baseline[path]})")

    for path in sorted(baseline):
        if path not in measured:
            failures.append(f"STALE  {path}: now clean, baseline still records {baseline[path]}")
        elif measured[path] < baseline[path]:
            failures.append(
                f"STALE  {path}: improved to {measured[path]}, baseline still records {baseline[path]}"
            )

    return failures


# --------------------------------------------------------------------------
# measurements
# --------------------------------------------------------------------------

SCAN_ROOTS = ("internal", "cmd", "web/src", "sdks")
SCAN_SUFFIXES = (".go", ".ts", ".tsx", ".py")
SKIP_DIR_NAMES = {
    "node_modules",
    "dist",
    "build",
    ".venv",
    ".git",
    ".claude",
    "sqlcgen",
    "mock",
    "mocks",
    "__pycache__",
    "coverage",
}


def _is_test_path(rel: str) -> bool:
    name = os.path.basename(rel)
    if name.endswith("_test.go") or name.endswith("_test.py"):
        return True
    for suffix in (".test.ts", ".test.tsx", ".spec.ts", ".spec.tsx"):
        if name.endswith(suffix):
            return True
    parts = rel.split("/")
    return "test" in parts or "tests" in parts or "__tests__" in parts


def source_files(root: Path = REPO_ROOT) -> list[str]:
    """Repo-relative first-party source paths, sorted."""
    found: list[str] = []
    for scan_root in SCAN_ROOTS:
        base = root / scan_root
        if not base.is_dir():
            continue
        for dirpath, dirnames, filenames in os.walk(base):
            dirnames[:] = sorted(d for d in dirnames if d not in SKIP_DIR_NAMES and not d.startswith("."))
            for filename in sorted(filenames):
                if not filename.endswith(SCAN_SUFFIXES):
                    continue
                rel = os.path.relpath(os.path.join(dirpath, filename), root)
                if _is_test_path(rel):
                    continue
                found.append(rel)
    return sorted(found)


def measure_file_length(root: Path = REPO_ROOT) -> dict[str, int]:
    measured: dict[str, int] = {}
    for rel in source_files(root):
        with open(root / rel, "rb") as handle:
            lines = sum(1 for _ in handle)
        if lines > FILE_LINE_LIMIT:
            measured[rel] = lines
    return measured


def _run(cmd: list[str], cwd: Path, allowed_returncodes: tuple[int, ...]) -> subprocess.CompletedProcess:
    if shutil.which(cmd[0]) is None and not os.path.isabs(cmd[0]):
        raise MeasurementError(
            f"{cmd[0]} is not on PATH. A missing tool must fail this soak, never read as zero findings."
        )
    proc = subprocess.run(cmd, cwd=cwd, capture_output=True, text=True)
    if proc.returncode not in allowed_returncodes:
        raise MeasurementError(
            f"{' '.join(cmd)} exited {proc.returncode}\n--- stdout ---\n{proc.stdout}\n--- stderr ---\n{proc.stderr}"
        )
    return proc


def _ensure_gocyclo() -> str:
    """Return a path to a pinned gocyclo, installing it if needed."""
    gobin = Path(
        subprocess.run(
            ["go", "env", "GOPATH"], capture_output=True, text=True, check=True
        ).stdout.strip()
    ) / "bin"
    binary = gobin / f"gocyclo-{GOCYCLO_VERSION}"
    if not binary.exists():
        env = dict(os.environ, GOBIN=str(gobin))
        staged = gobin / "gocyclo"
        subprocess.run(
            ["go", "install", f"github.com/fzipp/gocyclo/cmd/gocyclo@{GOCYCLO_VERSION}"],
            env=env,
            check=True,
        )
        shutil.move(str(staged), str(binary))
    return str(binary)


def measure_go_complexity(root: Path = REPO_ROOT) -> dict[str, int]:
    gocyclo = _ensure_gocyclo()
    targets = [d for d in ("internal", "cmd", "sdks/go") if (root / d).is_dir()]
    if not targets:
        raise MeasurementError("no Go source roots found; refusing to report zero findings")
    proc = _run(
        [gocyclo, "-over", str(CYCLO_LIMIT), "-ignore", r"_test|sqlcgen|/mock/", *targets],
        cwd=root,
        allowed_returncodes=(0, 1),
    )
    if proc.stderr.strip():
        raise MeasurementError(f"gocyclo wrote to stderr:\n{proc.stderr}")
    if proc.returncode == 1 and not proc.stdout.strip():
        raise MeasurementError("gocyclo exited 1 with no findings; treating that as a tool failure")

    measured: dict[str, int] = {}
    for line in proc.stdout.splitlines():
        # <complexity> <package> <func> <file>:<line>:<col>
        fields = line.split()
        if len(fields) < 4:
            raise MeasurementError(f"unparseable gocyclo line: {line!r}")
        path = fields[-1].rsplit(":", 2)[0]
        measured[path] = measured.get(path, 0) + 1
    return measured


def measure_golangci(root: Path = REPO_ROOT) -> dict[str, int]:
    # A config that does not load is the failure mode this soak exists to close:
    # .golangci.yml sat in this repo for months declaring version "2" while using
    # the v1 schema, so it would have errored out on first contact.
    _run(["golangci-lint", "config", "verify"], cwd=root, allowed_returncodes=(0,))

    with tempfile.TemporaryDirectory() as tmp:
        report = Path(tmp) / "golangci.json"
        _run(
            ["golangci-lint", "run", f"--output.json.path={report}", "./..."],
            cwd=root,
            allowed_returncodes=(0, 1),
        )
        if not report.exists():
            raise MeasurementError("golangci-lint produced no JSON report")
        try:
            payload = json.loads(report.read_text())
        except json.JSONDecodeError as exc:
            raise MeasurementError(f"golangci-lint report is not JSON: {exc}") from exc

    if payload.get("Report", {}).get("Error"):
        raise MeasurementError(f"golangci-lint reported an error: {payload['Report']['Error']}")

    measured: dict[str, int] = {}
    for issue in payload.get("Issues") or []:
        path = issue["Pos"]["Filename"]
        measured[path] = measured.get(path, 0) + 1
    return measured


def measure_web_complexity(root: Path = REPO_ROOT) -> dict[str, int]:
    web = root / "web"
    if not (web / "node_modules").is_dir():
        raise MeasurementError(
            "web/node_modules is missing; run `npm ci` in web/ first. "
            "An uninstalled tree must fail this soak, never read as zero findings."
        )
    with tempfile.TemporaryDirectory() as tmp:
        report = Path(tmp) / "eslint.json"
        _run(
            [
                "npx",
                "eslint",
                "src/",
                "--config",
                "eslint.soak.config.js",
                "-f",
                "json",
                "-o",
                str(report),
            ],
            cwd=web,
            allowed_returncodes=(0, 1),
        )
        if not report.exists():
            raise MeasurementError("eslint produced no JSON report")
        try:
            payload = json.loads(report.read_text())
        except json.JSONDecodeError as exc:
            raise MeasurementError(f"eslint report is not JSON: {exc}") from exc

    if not payload:
        raise MeasurementError("eslint linted zero files; refusing to report zero findings")

    measured: dict[str, int] = {}
    for file_result in payload:
        rel = os.path.relpath(file_result["filePath"], root)
        for message in file_result.get("messages", []):
            if message.get("fatal"):
                raise MeasurementError(f"eslint could not parse {rel}: {message.get('message')}")
            if message.get("ruleId") == "complexity":
                measured[rel] = measured.get(rel, 0) + 1
    return measured


# --------------------------------------------------------------------------
# dimensions
# --------------------------------------------------------------------------

DIMENSIONS = {
    "file-length": {
        "measure": measure_file_length,
        "unit": "lines",
        "header": (
            f"SOAK BASELINE: source files over {FILE_LINE_LIMIT} lines.\n"
            "Scope: internal/, cmd/, web/src/, sdks/ (.go .ts .tsx .py), excluding tests,\n"
            "generated code, mocks and vendored trees.\n"
            "Value is the file's line count. This baseline may only shrink.\n"
            "ENFORCE AT 0 entries: set MAX_FILE_LINES=500 in scripts/go-quality-gates.sh,\n"
            "extend that scan to sdks/, set eslint max-lines to 500, drop this dimension.\n"
            "Refresh with: python3 scripts/quality-soak.py file-length --update-baseline"
        ),
    },
    "go-complexity": {
        "measure": measure_go_complexity,
        "unit": "functions",
        "header": (
            f"SOAK BASELINE: Go functions over cyclomatic complexity {CYCLO_LIMIT}.\n"
            "Scope: internal/, cmd/, sdks/go. Value is the count of offending functions\n"
            "in that file, which stays stable when code moves inside the file.\n"
            "The BLOCKING limit in scripts/go-quality-gates.sh is still 49.\n"
            "ENFORCE AT 0 entries: set MAX_CYCLO=12 there and drop this dimension.\n"
            "Refresh with: python3 scripts/quality-soak.py go-complexity --update-baseline"
        ),
    },
    "web-complexity": {
        "measure": measure_web_complexity,
        "unit": "functions",
        "header": (
            f"SOAK BASELINE: web/src functions over cyclomatic complexity {CYCLO_LIMIT}.\n"
            "Measured with web/eslint.soak.config.js, which is the real eslint config with\n"
            "complexity tightened to the estate target. The BLOCKING limit in\n"
            "web/eslint.config.js is still 55.\n"
            "ENFORCE AT 0 entries: set complexity: ['error', 12] in web/eslint.config.js,\n"
            "delete web/eslint.soak.config.js, drop this dimension.\n"
            "Refresh with: python3 scripts/quality-soak.py web-complexity --update-baseline"
        ),
    },
    "golangci": {
        "measure": measure_golangci,
        "unit": "findings",
        "header": (
            "SOAK BASELINE: golangci-lint findings per file, using the repo's .golangci.yml\n"
            "(errcheck, govet, staticcheck, unused, ineffassign, misspell, goimports).\n"
            "This config existed for months and was invoked by nothing, while README.md and\n"
            "specs/003-engineering-standards/spec.md both listed it as a required check.\n"
            "ENFORCE AT 0 entries: replace this dimension with a blocking\n"
            "`golangci-lint run ./...` step in ci.yml's go-quality job.\n"
            "Refresh with: python3 scripts/quality-soak.py golangci --update-baseline"
        ),
    },
}


def baseline_path(dimension: str) -> Path:
    return BASELINE_DIR / f"{dimension}.txt"


def _display(path: Path) -> str:
    try:
        return str(path.relative_to(REPO_ROOT))
    except ValueError:
        return str(path)


def run_dimension(dimension: str, update: bool) -> int:
    spec = DIMENSIONS[dimension]
    unit = spec["unit"]
    print(f"==> soak: {dimension} (report-only, baselined)")

    measured = spec["measure"]()
    total = sum(measured.values())
    print(f"    measured: {total} {unit} across {len(measured)} file(s)")

    path = baseline_path(dimension)
    if update:
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(render_baseline(measured, spec["header"]))
        print(f"    baseline written: {_display(path)}")
        return 0

    if not path.exists():
        print(f"    FAIL: no baseline at {_display(path)}", file=sys.stderr)
        print(
            f"    create it with: python3 scripts/quality-soak.py {dimension} --update-baseline",
            file=sys.stderr,
        )
        return 1

    baseline = parse_baseline(path.read_text())
    print(f"    baseline: {sum(baseline.values())} {unit} across {len(baseline)} file(s)")

    failures = compare(measured, baseline)
    if not failures:
        print("    ok: matches baseline exactly (soak does not block on baselined debt)")
        return 0

    print(f"    FAIL: {len(failures)} baseline discrepancy/discrepancies", file=sys.stderr)
    for line in failures:
        print(f"      {line}", file=sys.stderr)
    print("", file=sys.stderr)
    print(
        "    NEW/WORSE means this change added debt: fix it, do not baseline it.\n"
        "    STALE means the baseline is beatable and must be tightened. Refresh with:\n"
        f"      python3 scripts/quality-soak.py {dimension} --update-baseline",
        file=sys.stderr,
    )
    return 1


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("dimension", choices=[*DIMENSIONS, "all"])
    parser.add_argument(
        "--update-baseline",
        action="store_true",
        help="rewrite the baseline from the current measurement (the only escape hatch)",
    )
    args = parser.parse_args(argv)

    dimensions = list(DIMENSIONS) if args.dimension == "all" else [args.dimension]
    status = 0
    for dimension in dimensions:
        try:
            status |= run_dimension(dimension, args.update_baseline)
        except MeasurementError as exc:
            print(f"    FAIL: {dimension} could not be measured: {exc}", file=sys.stderr)
            status |= 1
    return status


if __name__ == "__main__":
    sys.exit(main())
