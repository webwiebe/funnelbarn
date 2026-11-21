#!/usr/bin/env bash
# One run for the per-environment project merges of funnelbarn#545.
#
# For each instance in the order testing, staging, production it:
#   1. runs every merge as a dry run and prints the counts,
#   2. asks before each real merge,
#   3. posts the counts of every real merge as a comment on #545.
#
# It prints counts only. Run it from a checkout of the repo on a machine whose
# kubectl can exec into the funnelbarn writer pods:
#
#   scripts/project-merge-run.sh
#
# Environment overrides:
#   CTX_TESTING (default: testing)   CTX_STAGING (default: staging)   CTX_PRODUCTION (default: production)
#   ISSUE_REPO  (default: webwiebe/funnelbarn)   ISSUE (default: 545)
#   WITH_GEOBARN=1   also merge geobarn into brandtrace. Off by default: it waits for the
#                    brandtrace lead's answer on the five flags that exist on both sides.
#   POST=0           do not comment on the issue (print only)
#
# A pair whose projects do not exist on an instance is skipped. The merge is one
# transaction per pair, can be re-run, and deletes nothing: the source project is
# archived and rows the target already holds stay behind.
set -euo pipefail

ISSUE_REPO="${ISSUE_REPO:-webwiebe/funnelbarn}"
ISSUE="${ISSUE:-545}"
POST="${POST:-1}"
WITH_GEOBARN="${WITH_GEOBARN:-0}"

# from  to  env-stamp-for-moved-keys (empty = none)
PAIRS=(
  "maatwerksoftware-production maatwerksoftwarenijmegen -"
  "professionals profotograaf -"
  "maatwerksoftware-staging maatwerksoftwarenijmegen staging"
  "maatwerksoftware-testing maatwerksoftwarenijmegen test"
  "bananasketch-staging bananasketch staging"
  "bananasketch-test bananasketch test"
  "bananasketch-dev bananasketch development"
  "videoslop-production videoslop production"
  "videoslop-staging videoslop staging"
)
if [ "$WITH_GEOBARN" = "1" ]; then
  PAIRS+=("geobarn brandtrace -")
fi

INSTANCES=(
  "testing ${CTX_TESTING:-testing} funnelbarn-testing"
  "staging ${CTX_STAGING:-staging} funnelbarn-staging"
  "production ${CTX_PRODUCTION:-production} funnelbarn-production"
)

writer_pod() { # ctx ns
  kubectl --context "$1" -n "$2" get pods -o name | grep -E 'pod/funnelbarn-[0-9a-f]+-[a-z0-9]+$' | head -1 | sed 's|pod/||'
}

run_merge() { # ctx ns pod from to env dry
  local args=(project merge --from "$4" --to "$5")
  [ "$6" != "-" ] && args+=(--env "$6")
  [ "$7" = "1" ] && args+=(--dry-run)
  kubectl --context "$1" -n "$2" exec "$3" -c funnelbarn -- funnelbarn "${args[@]}"
}

for inst in "${INSTANCES[@]}"; do
  read -r name ctx ns <<<"$inst"
  echo
  echo "=== $name ($ctx / $ns) ==="
  if ! pod=$(writer_pod "$ctx" "$ns") || [ -z "$pod" ]; then
    echo "no writer pod found, skipping $name"
    continue
  fi
  runnable=()
  for pair in "${PAIRS[@]}"; do
    read -r from to env <<<"$pair"
    echo
    echo "--- dry run: $from -> $to (env stamp: ${env/-/none}) ---"
    if out=$(run_merge "$ctx" "$ns" "$pod" "$from" "$to" "$env" 1 2>&1); then
      echo "$out"
      runnable+=("$pair")
    else
      echo "skipped: $(echo "$out" | tail -1)"
    fi
  done
  [ "${#runnable[@]}" -gt 0 ] || { echo "nothing to merge on $name"; continue; }
  for pair in "${runnable[@]}"; do
    read -r from to env <<<"$pair"
    printf '\nMerge %s -> %s on %s for real? [y/N] ' "$from" "$to" "$name"
    read -r answer
    [ "$answer" = "y" ] || { echo "left as is"; continue; }
    out=$(run_merge "$ctx" "$ns" "$pod" "$from" "$to" "$env" 0)
    echo "$out"
    if [ "$POST" = "1" ]; then
      body=$(printf '%s %s, seat run of scripts/project-merge-run.sh. Merged `%s` into `%s` on %s (env stamp: %s). Counts:\n\n```\n%s\n```\n' \
        "$(date '+%Y-%m-%d')" "$(date '+%H:%M %Z')" "$from" "$to" "$name" "${env/-/none}" "$out")
      gh issue comment "$ISSUE" --repo "$ISSUE_REPO" --body "$body" >/dev/null && echo "counts logged on #$ISSUE"
    fi
  done
done
echo
echo "done"
