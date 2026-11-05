#!/usr/bin/env bash
# Set a new Cloudflare R2 token in the SOPS file of each environment, one token
# per environment. deploy/SECRETS.md says what each token may reach.
#
#   scripts/rotate-r2-token.sh                 # production, staging and testing
#   scripts/rotate-r2-token.sh testing         # one environment
#
# Run from a checkout, with the age key available to sops. It prompts for each
# access key id and secret with echo off, checks them all before it writes
# anything, and never puts a value in a command line or its output. It edits the
# files only: committing and deploying them is a separate step.
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"

envs=("$@")
if [ "${#envs[@]}" -eq 0 ]; then
  envs=(production staging testing)
fi

for env in "${envs[@]}"; do
  if [ ! -f "deploy/k8s/${env}/secret.yaml" ]; then
    echo "no SOPS file for environment: ${env}" >&2
    exit 1
  fi
done

key_ids=()
secrets=()
for env in "${envs[@]}"; do
  read -rs -p "${env} access key id: " key_id
  echo >&2
  read -rs -p "${env} secret access key: " secret
  echo >&2
  # R2 issues a 32 hex key id and a 64 hex secret, so a wrong paste stops here.
  if ! [[ "$key_id" =~ ^[0-9a-f]{32}$ ]]; then
    echo "${env}: the access key id is not 32 hex characters; nothing written" >&2
    exit 1
  fi
  if ! [[ "$secret" =~ ^[0-9a-f]{64}$ ]]; then
    echo "${env}: the secret is not 64 hex characters; nothing written" >&2
    exit 1
  fi
  for seen in ${key_ids[@]+"${key_ids[@]}"}; do
    if [ "$seen" = "$key_id" ]; then
      echo "${env}: this token is already used for another environment; nothing written" >&2
      exit 1
    fi
  done
  key_ids+=("$key_id")
  secrets+=("$secret")
done

set_value() {
  # sops reads a JSON string from stdin. Both values are hex, so no escaping.
  printf '"%s"' "$3" | sops set --value-stdin "$1" "[\"stringData\"][\"$2\"]"
}

for i in "${!envs[@]}"; do
  file="deploy/k8s/${envs[$i]}/secret.yaml"
  set_value "$file" FUNNELBARN_R2_ACCESS_KEY_ID "${key_ids[$i]}"
  set_value "$file" FUNNELBARN_R2_SECRET_ACCESS_KEY "${secrets[$i]}"
  echo "${envs[$i]}: set FUNNELBARN_R2_ACCESS_KEY_ID and FUNNELBARN_R2_SECRET_ACCESS_KEY"
done
