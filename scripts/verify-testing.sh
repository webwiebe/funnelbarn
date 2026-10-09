#!/usr/bin/env bash
# Post-deploy probe for a FunnelBarn environment (issue #302).
#
# Usage: scripts/verify-testing.sh BASE_URL API_KEY FLAG_KEY
#
#   1. GET  $BASE_URL/api/v1/health must answer 200.
#   2. POST $BASE_URL/api/v1/evaluate once per second for PROBE_SECONDS
#      (default 180), sequentially. Every response must be 200 and carry a
#      Server-Timing app;dur=<ms> header. The p99 of that server-side time
#      must stay under P99_LIMIT_SECONDS (default 0.1). It covers the whole
#      handler, pool waits included, so the daily maintenance purge that used
#      to stall this endpoint for 4-6s still fails the gate. The time curl
#      measures is printed for information only: it includes the load of the
#      CI host running this script, which swung the old gate between 30ms
#      and 166ms on identical builds.
#   3. When SPANBARN_URL and SPANBARN_TOKEN (a read-scoped SpanBarn API key
#      for project funnelbarn) are set, look for a recent
#      maintenance.purge trace in SpanBarn. A miss is a warning unless
#      SPANBARN_REQUIRE=1, which makes it a failure. The pass runs daily and
#      2 minutes after each boot, and SpanBarn keeps rare root operations
#      since spanbarn#270.
#
# Every network call and the probe loop itself are bounded by timeouts.
set -euo pipefail

if [ "$#" -ne 3 ]; then
  echo "usage: $0 BASE_URL API_KEY FLAG_KEY" >&2
  exit 2
fi

BASE_URL="${1%/}"
API_KEY="$2"
FLAG_KEY="$3"
PROBE_SECONDS="${PROBE_SECONDS:-180}"
P99_LIMIT_SECONDS="${P99_LIMIT_SECONDS:-0.1}"
SPANBARN_LOOKBACK_HOURS="${SPANBARN_LOOKBACK_HOURS:-48}"

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
times="$work/times"
: >"$times"

fail=0

echo "== health: $BASE_URL/api/v1/health"
code="$(curl -sS -o /dev/null -w '%{http_code}' --max-time 10 "$BASE_URL/api/v1/health")"
if [ "$code" != "200" ]; then
  echo "FAIL health returned $code" >&2
  exit 1
fi
echo "ok"

echo "== evaluate probe: ${PROBE_SECONDS}s at 1 req/s against flag '$FLAG_KEY'"
body="$(printf '{"flag_key":"%s","default_value":false,"context":{"targeting_key":"verify-probe","session_id":"verify-probe"}}' "$FLAG_KEY")"

# Bounded by a wall-clock deadline: each curl is capped at 10s, so the loop
# ends at most 10s after PROBE_SECONDS even if the host stalls.
deadline=$(($(date +%s) + PROBE_SECONDS))
while [ "$(date +%s)" -lt "$deadline" ]; do
  started="$(date +%s)"
  out="$(curl -sS -o /dev/null -w '%{http_code} %{time_total} %header{server-timing}' --max-time 10 \
    -X POST "$BASE_URL/api/v1/evaluate" \
    -H 'Content-Type: application/json' \
    -H "x-funnelbarn-api-key: $API_KEY" \
    -d "$body" || echo "000 10")"
  echo "$out" >>"$times"
  if [ "$(date +%s)" -le "$started" ]; then
    sleep 1
  fi
done

total="$(wc -l <"$times" | tr -d ' ')"
non200="$(awk '$1 != 200' "$times" | wc -l | tr -d ' ')"
if [ "$total" -eq 0 ]; then
  echo "FAIL no probe samples were taken" >&2
  exit 1
fi
pct99() { sort -n | awk '{a[NR]=$1} END {i=int(NR*0.99); if (i<NR*0.99) i++; if (i<1) i=1; print a[i]}'; }
# Column 3 is the Server-Timing value, e.g. "app;dur=4.2" (milliseconds).
server="$work/server"
awk '$1 == 200 && $3 ~ /^app;dur=[0-9.]+$/ {sub(/^app;dur=/, "", $3); print $3 / 1000}' "$times" >"$server"
untimed="$(awk '$1 == 200 && $3 !~ /^app;dur=[0-9.]+$/' "$times" | wc -l | tr -d ' ')"
client_p99="$(awk '{print $2}' "$times" | pct99)"
echo "samples=$total non200=$non200 untimed=$untimed client_p99=${client_p99}s (informational)"

p99=""
if [ -s "$server" ]; then
  p99="$(pct99 <"$server")"
  max="$(sort -n "$server" | tail -1)"
  echo "server_p99=${p99}s server_max=${max}s limit=${P99_LIMIT_SECONDS}s"
fi

if [ "$non200" -gt 0 ]; then
  echo "FAIL $non200 of $total evaluate responses were not 200" >&2
  awk '$1 != 200' "$times" | sort | uniq -c >&2
  fail=1
fi
if [ "$untimed" -gt 0 ]; then
  echo "FAIL $untimed evaluate responses had no Server-Timing app;dur header" >&2
  fail=1
fi
if [ -n "$p99" ] && awk -v p="$p99" -v l="$P99_LIMIT_SECONDS" 'BEGIN {exit !(p > l)}'; then
  echo "FAIL evaluate server p99 ${p99}s exceeds ${P99_LIMIT_SECONDS}s" >&2
  fail=1
fi

if [ -n "${SPANBARN_URL:-}" ] && [ -n "${SPANBARN_TOKEN:-}" ]; then
  echo "== spanbarn: maintenance.purge within the last ${SPANBARN_LOOKBACK_HOURS}h"
  from="$(($(date +%s) - SPANBARN_LOOKBACK_HOURS * 3600))"
  resp="$(curl -sS --max-time 20 -G "${SPANBARN_URL%/}/api/v1/traces" \
    -H "X-SpanBarn-Api-Key: $SPANBARN_TOKEN" \
    --data-urlencode "operation=maintenance.purge" \
    --data-urlencode "root_only=true" \
    --data-urlencode "from=$from" \
    --data-urlencode "limit=5" || echo "")"
  if printf '%s' "$resp" | grep -q 'maintenance.purge'; then
    echo "ok maintenance.purge trace found"
  elif [ "${SPANBARN_REQUIRE:-0}" = "1" ]; then
    echo "FAIL no maintenance.purge trace in SpanBarn" >&2
    fail=1
  else
    echo "WARN no maintenance.purge trace in SpanBarn (pass runs daily, clean traces are sampled)" >&2
  fi
fi

if [ "$fail" -ne 0 ]; then
  exit 1
fi
echo "verify-testing: ok"
