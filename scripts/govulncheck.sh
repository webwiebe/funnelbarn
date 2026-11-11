#!/usr/bin/env bash
# Runs govulncheck over every Go module in the repo and fails on any finding
# reachable from the code, including findings in the standard library. A
# stdlib finding clears only when the toolchain pin moves (go.mod, the service
# Dockerfile and the setup-go versions), which is the point of the gate.
#
#   scripts/govulncheck.sh            all modules, with the toolchain on PATH
#   GOTOOLCHAIN=go1.27.3 scripts/govulncheck.sh   prove the gate on an older Go
set -uo pipefail

cd "$(dirname "$0")/.."
bin="$(command -v govulncheck || true)"
if [ -z "$bin" ]; then
  echo "govulncheck is not installed; the workflow installs the pinned version" >&2
  exit 2
fi

status=0
for mod in . sdks/go sdks/go-bugbarn; do
  echo "== govulncheck ${mod} ($(cd "$mod" && go version))"
  if ! (cd "$mod" && "$bin" ./...); then
    echo "govulncheck found a reachable vulnerability in ${mod}" >&2
    status=1
  fi
done
exit "$status"
