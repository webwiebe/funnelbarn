#!/usr/bin/env bash
#
# Go static quality ratchets. Every threshold below is PINNED AT THE CURRENT
# WORST VALUE — the gate passes today and fails on any regression. When you
# legitimately improve a metric, tighten the matching number in the same PR
# (the numbers may only ratchet DOWN, never up).
#
# Covers: cyclomatic complexity, file length, and code duplication.
# First-party code only (internal/, cmd/; complexity also covers sdks/go, file length also covers sdks/); generated (sqlcgen), mocks, and
# _test.go files are excluded.
set -euo pipefail

# ---- pinned thresholds -------------------------------------------------------
MAX_CYCLO=12        # gocyclo: no function may exceed this complexity (estate target)
MAX_FILE_LINES=500  # no non-test/non-generated source file may exceed this (estate target)
MAX_DUPL_HITS=2     # dupl -plumbing lines (2 = one known clone group)
DUPL_TOKENS=150     # dupl clone-detection sensitivity (tokens)
# Pinned tool versions: a gate whose tool version can change without a commit is
# a gate whose green run stops meaning anything.
GOCYCLO_VERSION=v0.6.0
DUPL_VERSION=v1.1.0
# -----------------------------------------------------------------------------

GB="$(go env GOPATH)/bin"
fail=0

# First-party, non-test, non-generated Go files.
# Portable read loop: the self-hosted macOS runners ship bash 3.2, which has no mapfile.
SRC=()
while IFS= read -r f; do SRC+=("$f"); done < <(find internal cmd -name '*.go' \
  -not -name '*_test.go' -not -path '*/sqlcgen/*' -not -path '*/mock/*' | sort)

echo "==> cyclomatic complexity (gocyclo, max ${MAX_CYCLO})"
# A binary already on disk may be any version, so check the build info and
# (re)install the pin when it differs.
tool_version() { go version -m "$1" 2>/dev/null | awk '$1 == "mod" { print $3; exit }'; }
if [ "$(tool_version "${GB}/gocyclo")" != "${GOCYCLO_VERSION}" ]; then
  go install "github.com/fzipp/gocyclo/cmd/gocyclo@${GOCYCLO_VERSION}"
fi
if "${GB}/gocyclo" -over "${MAX_CYCLO}" -ignore '_test|sqlcgen|/mock/' internal cmd sdks/go; then
  echo "    ok: no function over ${MAX_CYCLO}"
else
  echo "    FAIL: function(s) above cyclomatic ${MAX_CYCLO} (listed above)"; fail=1
fi

echo "==> file length (max ${MAX_FILE_LINES} lines, non-test/non-generated)"
overlen=0
# SDK sources (Go, TypeScript, Python) count too; installed and built trees do not.
LEN=("${SRC[@]}")
while IFS= read -r f; do LEN+=("$f"); done < <(find sdks \( -name node_modules -o -name dist -o -name vendor -o -name .venv \) -prune -o \
  -type f \( -name '*.go' -o -name '*.ts' -o -name '*.tsx' -o -name '*.py' \) \
  -not -name '*_test.go' -not -name '*.test.*' -not -path '*/test/*' -not -path '*/tests/*' -print | sort)
for f in "${LEN[@]}"; do
  n=$(wc -l < "$f")
  if [ "$n" -gt "$MAX_FILE_LINES" ]; then echo "    FAIL: $f has $n lines (> $MAX_FILE_LINES)"; overlen=1; fi
done
if [ "$overlen" -eq 0 ]; then echo "    ok: no file over ${MAX_FILE_LINES} lines"; else fail=1; fi

echo "==> code duplication (dupl, ${DUPL_TOKENS} tokens, max ${MAX_DUPL_HITS} hits)"
if [ "$(tool_version "${GB}/dupl")" != "${DUPL_VERSION}" ]; then
  go install "github.com/mibk/dupl@${DUPL_VERSION}"
fi
hits=$("${GB}/dupl" -plumbing -t "${DUPL_TOKENS}" "${SRC[@]}" 2>/dev/null | wc -l | tr -d ' ')
if [ "$hits" -gt "$MAX_DUPL_HITS" ]; then
  echo "    FAIL: dupl found $hits clone lines (> $MAX_DUPL_HITS):"
  "${GB}/dupl" -plumbing -t "${DUPL_TOKENS}" "${SRC[@]}" 2>/dev/null | sed 's/^/      /'
  fail=1
else
  echo "    ok: dupl clone lines = $hits (<= $MAX_DUPL_HITS)"
fi

exit $fail
