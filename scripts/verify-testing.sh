#!/usr/bin/env bash
# Post-deploy probe for a FunnelBarn environment (issue #302).
#
# Usage: scripts/verify-testing.sh BASE_URL API_KEY FLAG_KEY
#
#   1. GET  $BASE_URL/api/v1/health must answer 200.
#   2. POST $BASE_URL/api/v1/evaluate once per second for PROBE_SECONDS
#      (default 180), sequentially. The p99 of the request time must stay
#      under P99_LIMIT_SECONDS (default 0.1) and every response must be 200.
#      The daily maintenance purge used to stall this endpoint for 4-6s.
#   3. When SPANBARN_URL and SPANBARN_TOKEN (a read-scoped SpanBarn API key
#      for project funnelbarn) are set, look for a recent
#      maintenance.purge trace in SpanBarn. A miss is a warning, because the
#      pass runs once a day and SpanBarn samples clean traces; set
#      SPANBARN_REQUIRE=1 to make it a failure.
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
  out="$(curl -sS -o /dev/null -w '%{http_code} %{time_total}' --max-time 10 \
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
p99="$(awk '{print $2}' "$times" | sort -n | awk '{a[NR]=$1} END {i=int(NR*0.99); if (i<NR*0.99) i++; if (i<1) i=1; print a[i]}')"
max="$(awk '{print $2}' "$times" | sort -n | tail -1)"
echo "samples=$total non200=$non200 p99=${p99}s max=${max}s limit=${P99_LIMIT_SECONDS}s"

if [ "$non200" -gt 0 ]; then
  echo "FAIL $non200 of $total evaluate responses were not 200" >&2
  awk '$1 != 200' "$times" | sort | uniq -c >&2
  fail=1
fi
if awk -v p="$p99" -v l="$P99_LIMIT_SECONDS" 'BEGIN {exit !(p > l)}'; then
  echo "FAIL evaluate p99 ${p99}s exceeds ${P99_LIMIT_SECONDS}s" >&2
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
