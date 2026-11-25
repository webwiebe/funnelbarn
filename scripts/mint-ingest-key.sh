#!/usr/bin/env bash
# Mint an ingest key for a FunnelBarn project, stamp its environment and, when asked,
# store the value in a SOPS file. The value is never printed: the output holds the key
# id, project, name and environment only.
#
#   scripts/mint-ingest-key.sh --project leadgen --env testing \
#       --sops-file ~/webwiebe/leadgen/deploy/secrets/test/secrets.yaml \
#       --sops-path '["stringData"]["funnelbarn-api-key"]'
#
# Without --sops-file the value is discarded and only the id is printed (a key whose
# value nobody holds is useless, so pass --sops-file for real use).
#
# Options:
#   --project SLUG     project slug (created if missing, as `funnelbarn apikey create` does)
#   --env NAME         testing | staging | production | e2e | development (key environment)
#   --name NAME        key name (default: <project>-<env>)
#   --sops-file PATH   SOPS file that receives the value, written with `sops set`
#   --sops-path PATH   SOPS key path (default: '["stringData"]["funnelbarn-api-key"]')
#   --context CTX      kubectl context (default: production)
#   --namespace NS     namespace (default: funnelbarn-production)
#
# Run it on a machine whose kubectl can exec into the funnelbarn writer pod, under the
# access rules of the seat or owner running it. Log the printed id on the requesting issue.
set -euo pipefail

project="" env_name="" name="" sops_file="" sops_path='["stringData"]["funnelbarn-api-key"]'
ctx="production" ns="funnelbarn-production"
while [ $# -gt 0 ]; do
  case "$1" in
    --project) project=$2; shift 2 ;;
    --env) env_name=$2; shift 2 ;;
    --name) name=$2; shift 2 ;;
    --sops-file) sops_file=$2; shift 2 ;;
    --sops-path) sops_path=$2; shift 2 ;;
    --context) ctx=$2; shift 2 ;;
    --namespace) ns=$2; shift 2 ;;
    *) echo "unknown option: $1" >&2; exit 2 ;;
  esac
done
[ -n "$project" ] && [ -n "$env_name" ] || { echo "usage: $0 --project SLUG --env NAME [--name NAME] [--sops-file PATH]" >&2; exit 2; }
name=${name:-$project-$env_name}

K=(kubectl --context "$ctx" -n "$ns")
pod=$("${K[@]}" get pods -o name | grep -E 'pod/funnelbarn-[0-9a-f]+-[a-z0-9]+$' | head -1 | sed 's|pod/||')
[ -n "$pod" ] || { echo "no writer pod found in $ns" >&2; exit 1; }

out=$("${K[@]}" exec "$pod" -c funnelbarn -- funnelbarn apikey create --project="$project" --name="$name" --scope=ingest 2>&1)
id=$(printf '%s\n' "$out" | sed -n 's/.*API key created (id=\([^,]*\),.*/\1/p')
val=$(printf '%s\n' "$out" | sed -n 's/^Key (shown once[^:]*): //p')
if [ -z "$id" ] || [ -z "$val" ]; then
  echo "mint failed: $(printf '%s\n' "$out" | tail -1 | cut -c1-120)" >&2
  exit 1
fi
"${K[@]}" exec "$pod" -c funnelbarn -- funnelbarn apikey set-environment --id="$id" --environment="$env_name" >/dev/null 2>&1 \
  || echo "warning: key $id minted but its environment was not set" >&2
if [ -n "$sops_file" ]; then
  sops set "$sops_file" "$sops_path" "\"$val\""
  echo "value stored in $sops_file"
fi
echo "minted key id=$id project=$project name=$name environment=$env_name"
