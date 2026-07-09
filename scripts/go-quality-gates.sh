#!/usr/bin/env bash
#
# Go static quality ratchets. Every threshold below is PINNED AT THE CURRENT
# WORST VALUE — the gate passes today and fails on any regression. When you
# legitimately improve a metric, tighten the matching number in the same PR
# (the numbers may only ratchet DOWN, never up).
#
# Covers: cyclomatic complexity, file length, and code duplication.
# First-party code only (internal/, cmd/); generated (sqlcgen), mocks, and
# _test.go files are excluded.
set -euo pipefail

# ---- pinned thresholds -------------------------------------------------------
MAX_CYCLO=49        # gocyclo: no function may exceed this complexity
MAX_FILE_LINES=848  # no non-test/non-generated .go file may exceed this
MAX_DUPL_HITS=2     # dupl -plumbing lines (2 = one known clone group)
DUPL_TOKENS=150     # dupl clone-detection sensitivity (tokens)
# -----------------------------------------------------------------------------

GB="$(go env GOPATH)/bin"
fail=0

# First-party, non-test, non-generated Go files.
mapfile -t SRC < <(find internal cmd -name '*.go' \
  -not -name '*_test.go' -not -path '*/sqlcgen/*' -not -path '*/mock/*' | sort)

echo "==> cyclomatic complexity (gocyclo, max ${MAX_CYCLO})"
if ! command -v "${GB}/gocyclo" >/dev/null 2>&1; then
  go install github.com/fzipp/gocyclo/cmd/gocyclo@latest
fi
if "${GB}/gocyclo" -over "${MAX_CYCLO}" -ignore '_test|sqlcgen|/mock/' internal cmd; then
  echo "    ok: no function over ${MAX_CYCLO}"
else
  echo "    FAIL: function(s) above cyclomatic ${MAX_CYCLO} (listed above)"; fail=1
fi

echo "==> file length (max ${MAX_FILE_LINES} lines, non-test/non-generated)"
overlen=0
for f in "${SRC[@]}"; do
  n=$(wc -l < "$f")
  if [ "$n" -gt "$MAX_FILE_LINES" ]; then echo "    FAIL: $f has $n lines (> $MAX_FILE_LINES)"; overlen=1; fi
done
if [ "$overlen" -eq 0 ]; then echo "    ok: no file over ${MAX_FILE_LINES} lines"; else fail=1; fi

echo "==> code duplication (dupl, ${DUPL_TOKENS} tokens, max ${MAX_DUPL_HITS} hits)"
if ! command -v "${GB}/dupl" >/dev/null 2>&1; then
  go install github.com/mibk/dupl@latest
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
