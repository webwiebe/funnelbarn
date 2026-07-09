#!/usr/bin/env python3
"""
Go coverage ratchet — per-file, per-package, and global gates.

Enforces three rules against first-party code (internal/, cmd/), excluding
generated (sqlcgen), mocks, and _test.go:

  1. NO first-party file may be 0% covered (zero exemptions). This is the strict
     gate: every source file must be exercised by at least one test.
  2. Each package must meet its committed floor (scripts/coverage-baseline.txt).
  3. Global first-party coverage must meet its committed floor.

Floors are PINNED AT CURRENT coverage (truncated to whole percent). They may
only ratchet UP: when coverage improves, raise the floor in the same PR.

Cross-package coverage under -race is not bit-reproducible across machines:
timing-dependent branches (rate limits, context deadlines, full buffers on the
ingest hot path) execute or not depending on goroutine scheduling, so the same
tree measures a couple of points differently on a laptop vs the CI runner. The
percentage floors (Rules 2 & 3) therefore allow a small TOLERANCE_PP noise band
below the pinned floor — enough to absorb that jitter, far too small to hide a
real regression. Rule 1 (no 0% file) is deterministic and stays strict.

Coverage is measured cross-package (-coverpkg=./...) so a file counts as covered
when ANY test exercises it, then block spans are de-duplicated (a block that
appears in several test binaries is counted once, covered if any binary hit it).

Usage:
  coverage-gate.py --update   # measure and (re)write the baseline
  coverage-gate.py            # measure and enforce the baseline (CI mode)
"""
import os
import re
import subprocess
import sys
import collections

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
BASELINE = os.path.join(ROOT, "scripts", "coverage-baseline.txt")
PROFILE = os.path.join(ROOT, "coverage-gate.out")
LINE_RE = re.compile(r"^(.*\.go:\d+\.\d+,\d+\.\d+) (\d+) (\d+)$")

# Noise band for the percentage floors (Rules 2 & 3), in percentage points.
# Cross-package -race coverage jitters ~2pp between the laptop the baseline was
# pinned on and the CI runner; a package/global only FAILS when it drops more
# than this below its floor. Real regressions are far larger than the band.
TOLERANCE_PP = 2.0


def first_party(path: str) -> bool:
    p = path.replace("github.com/wiebe-xyz/funnelbarn/", "")
    return (p.startswith("internal/") or p.startswith("cmd/")) \
        and "sqlcgen" not in p and "/mock/" not in p


def short(path: str) -> str:
    return path.replace("github.com/wiebe-xyz/funnelbarn/", "")


def measure():
    print("measuring cross-package coverage (go test -coverpkg=./... ./...) ...", flush=True)
    r = subprocess.run(
        ["go", "test", "-coverpkg=./...", "-covermode=atomic",
         f"-coverprofile={PROFILE}", "./..."],
        cwd=ROOT, stdout=subprocess.DEVNULL, stderr=subprocess.STDOUT,
    )
    if r.returncode != 0:
        print("ERROR: `go test` failed; fix tests before the coverage gate.", file=sys.stderr)
        sys.exit(1)

    # De-duplicate blocks: keep max count per unique block span.
    blocks = {}
    with open(PROFILE) as f:
        next(f)  # mode line
        for line in f:
            m = LINE_RE.match(line.strip())
            if not m:
                continue
            span, nstmt, count = m.group(1), int(m.group(2)), int(m.group(3))
            prev = blocks.get(span)
            blocks[span] = (nstmt, count if prev is None else max(count, prev[1]))

    files = collections.defaultdict(lambda: [0, 0])   # path -> [covered, total]
    pkgs = collections.defaultdict(lambda: [0, 0])
    gc = gt = 0
    for span, (nstmt, count) in blocks.items():
        path = short(span.split(":")[0])
        if not first_party(path):
            continue
        cov = nstmt if count > 0 else 0
        files[path][0] += cov
        files[path][1] += nstmt
        pkg = os.path.dirname(path)
        pkgs[pkg][0] += cov
        pkgs[pkg][1] += nstmt
        gc += cov
        gt += nstmt
    return files, pkgs, (gc, gt)


def pct(cov, tot):
    return 100.0 * cov / tot if tot else 100.0


def update(files, pkgs, glob):
    lines = ["# Go coverage ratchet floors — whole-percent, pinned at current.",
             "# Raise (never lower) these as coverage improves. Regenerate with",
             "#   python3 scripts/coverage-gate.py --update",
             f"TOTAL\t{int(pct(*glob))}"]
    for pkg in sorted(pkgs):
        lines.append(f"{pkg}\t{int(pct(*pkgs[pkg]))}")
    with open(BASELINE, "w") as f:
        f.write("\n".join(lines) + "\n")
    print(f"wrote {BASELINE} (global floor {int(pct(*glob))}%, {len(pkgs)} packages)")


def check(files, pkgs, glob):
    floors = {}
    with open(BASELINE) as f:
        for line in f:
            line = line.strip()
            if not line or line.startswith("#"):
                continue
            name, val = line.split("\t")
            floors[name] = int(val)

    failed = False

    # Rule 1: no first-party file at 0%.
    zero = sorted(p for p, (c, t) in files.items() if t > 0 and c == 0)
    if zero:
        failed = True
        print(f"\nFAIL: {len(zero)} first-party file(s) have 0% coverage (must be >0%):")
        for p in zero:
            print(f"  {p}")

    # Rule 2: per-package floors.
    print("\nper-package coverage (floor):")
    for pkg in sorted(pkgs):
        cov, tot = pkgs[pkg]
        got = pct(cov, tot)
        floor = floors.get(pkg, 0)
        bad = got + TOLERANCE_PP + 1e-9 < floor
        print(f"  {'FAIL' if bad else 'ok  '} {got:6.2f}%  (floor {floor}%)  {pkg}")
        if bad:
            failed = True
    # New package with no floor yet → must be added to baseline.
    for pkg in sorted(pkgs):
        if pkg not in floors:
            print(f"  FAIL: package {pkg} missing from baseline — run --update")
            failed = True

    # Rule 3: global floor.
    g = pct(*glob)
    gf = floors.get("TOTAL", 0)
    gbad = g + TOLERANCE_PP + 1e-9 < gf
    print(f"\nglobal first-party coverage: {g:.2f}%  (floor {gf}%)  [{'FAIL' if gbad else 'ok'}]")
    if gbad:
        failed = True

    if failed:
        print("\nCOVERAGE GATE FAILED — coverage regressed below the committed floor,\n"
              "or a file/package was left uncovered. Add tests, or (if you intentionally\n"
              "raised coverage) run `python3 scripts/coverage-gate.py --update`.")
        sys.exit(1)
    print("\ncoverage gate passed.")


def main():
    files, pkgs, glob = measure()
    if "--update" in sys.argv:
        update(files, pkgs, glob)
    else:
        check(files, pkgs, glob)


if __name__ == "__main__":
    main()
