#!/usr/bin/env bash
# Post-rollout smoke check for a FunnelBarn environment (issue #364).
#
# Usage: scripts/smoke.sh BASE_URL EXPECTED_VERSION
#
# Passes when, within SMOKE_ATTEMPTS tries (default 12, SMOKE_DELAY seconds
# apart, default 5):
#   1. GET $BASE_URL/api/v1/ready answers 200 with status "ok", and
#   2. its "version" equals EXPECTED_VERSION, so the pods that answer are the
#      release that was just rolled out and not the previous one.
# Exits 1 otherwise. Every request is bounded by --max-time.
set -euo pipefail

if [ "$#" -ne 2 ]; then
  echo "usage: $0 BASE_URL EXPECTED_VERSION" >&2
  exit 2
fi

base="${1%/}"
expected="$2"
attempts="${SMOKE_ATTEMPTS:-12}"
delay="${SMOKE_DELAY:-5}"

for i in $(seq 1 "$attempts"); do
  body="$(curl -sS --max-time 10 -w '\n%{http_code}' "$base/api/v1/ready" 2>&1)" || body=""
  code="${body##*$'\n'}"
  json="${body%$'\n'*}"
  if [ "$code" = "200" ]; then
    status="$(printf '%s' "$json" | python3 -c 'import json,sys; print(json.load(sys.stdin).get("status",""))' 2>/dev/null || true)"
    version="$(printf '%s' "$json" | python3 -c 'import json,sys; print(json.load(sys.stdin).get("version",""))' 2>/dev/null || true)"
    if [ "$status" = "ok" ] && [ "$version" = "$expected" ]; then
      echo "ok: $base is ready on version $version (attempt $i)"
      exit 0
    fi
    echo "attempt $i/$attempts: ready 200 but status='$status' version='$version', want version '$expected'"
  else
    echo "attempt $i/$attempts: ready returned '${code:-no response}'"
  fi
  [ "$i" -lt "$attempts" ] && sleep "$delay"
done

echo "FAIL: $base did not report version '$expected' as ready after $attempts attempts" >&2
exit 1
