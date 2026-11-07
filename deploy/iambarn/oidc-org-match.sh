#!/usr/bin/env bash
# Compares an IAMBarn organization id with the one an environment's FunnelBarn
# trusts for OIDC sign-in (FUNNELBARN_OIDC_ORGANIZATION_ID in its SOPS file),
# and prints only match=yes or match=no.
#
# The iambarn job that creates the FunnelBarn member test account
# (iambarn#449) logs the id its organization slug resolves to on a dry run.
# The real run goes only after this prints match=yes for that id, because a
# member of another organization is also refused at sign-in, which would make
# the member smoke pass without testing the role check.
#
# It runs in CI only (oidc-org-match.yml), with the same SOPS_AGE_KEY_<ENV>
# the deploy uses. No seat runs it locally. It never prints the configured id.
#
# Required environment variables:
#   GIVEN_ORGANIZATION_ID  the id to check
#
# Usage: oidc-org-match.sh <sops secret file>
set -euo pipefail

FILE="${1:?usage: $0 <sops secret file>}"
: "${GIVEN_ORGANIZATION_ID:?GIVEN_ORGANIZATION_ID is required}"

configured="$(sops -d --extract '["stringData"]["FUNNELBARN_OIDC_ORGANIZATION_ID"]' "$FILE")"
if [[ -z "$configured" ]]; then
  echo "match=no (no organization id configured)"
  exit 1
fi

if [[ "$GIVEN_ORGANIZATION_ID" == "$configured" ]]; then
  echo "match=yes"
else
  echo "match=no"
  exit 1
fi
