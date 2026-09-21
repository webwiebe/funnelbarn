#!/usr/bin/env bash
# Registers FunnelBarn's MCP endpoint as an RFC 8707 resource server on the
# IAMBarn OAuth client FunnelBarn uses for dashboard OIDC login, for one
# environment. IAMBarn validates an OAuth `resource` parameter by finding a
# client row whose resource_identifier exactly equals it, so without this the
# MCP token exchange never gets a resource-scoped token.
#
# It sends a PUT carrying only resource_identifier. IAMBarn's client update
# keeps every field the body omits, so name, redirect URIs, scopes and the
# rest are untouched, and it answers with the updated client, which this
# script checks. Setting the same value again is harmless, so it is safe to
# run on every deploy. It never prints the admin token or a client secret; the
# PUT response does not include secrets.
#
# Required environment variables:
#   IAMBARN_ADMIN_TOKEN  PAT or M2M token with scope admin:clients:write, for
#                         the IAMBarn organization that owns the FunnelBarn
#                         OAuth clients.
#   IAMBARN_URL           IAMBarn issuer base URL, e.g. https://iam.staging.wiebe.xyz
#   CLIENT_ID             FunnelBarn's OIDC client_id for this environment
#                         (FUNNELBARN_OIDC_CLIENT_ID).
#   RESOURCE_URL          The MCP resource URL to register, e.g.
#                         https://funnelbarn.staging.wiebe.xyz/api/v1/mcp
#                         (must match FUNNELBARN_MCP_RESOURCE_URL in the
#                         env's deployment.yaml).
#
# Usage: register-mcp-resource.sh <env>
# <env> (e.g. "staging", "production") is used only to label log output.
set -euo pipefail

ENV="${1:-}"
if [[ -z "$ENV" ]]; then
  echo "usage: $0 <env>" >&2
  exit 2
fi

: "${IAMBARN_ADMIN_TOKEN:?IAMBARN_ADMIN_TOKEN is required (PAT or M2M token with admin:clients:write)}"
: "${IAMBARN_URL:?IAMBARN_URL is required (the IAMBarn issuer base URL)}"
: "${CLIENT_ID:?CLIENT_ID is required (the FunnelBarn OIDC client_id for this environment)}"
: "${RESOURCE_URL:?RESOURCE_URL is required (the MCP resource URL to register)}"

command -v curl >/dev/null || { echo "curl is required" >&2; exit 1; }
command -v jq >/dev/null || { echo "jq is required" >&2; exit 1; }

IAM="${IAMBARN_URL%/}"

echo "[$ENV] setting resource_identifier on IAMBarn client $CLIENT_ID"

body="$(jq -n --arg resource_identifier "$RESOURCE_URL" '{resource_identifier: $resource_identifier}')"

response_file="$(mktemp)"
trap 'rm -f "$response_file"' EXIT

status="$(curl -sS -o "$response_file" -w '%{http_code}' -X PUT \
  -H "Authorization: Bearer ${IAMBARN_ADMIN_TOKEN}" \
  -H "Content-Type: application/json" \
  -d "$body" \
  "$IAM/api/v1/admin/clients/${CLIENT_ID}")"

if [[ "$status" != "200" ]]; then
  # The error body is a short JSON message such as {"error":"insufficient_scope"}.
  echo "[$ENV] FAIL: PUT returned HTTP $status: $(jq -r '.error // empty' "$response_file" 2>/dev/null)" >&2
  exit 1
fi

after="$(jq -r '.resource_identifier // ""' "$response_file")"
if [[ "$after" != "$RESOURCE_URL" ]]; then
  echo "[$ENV] FAIL: resource_identifier after PUT is not RESOURCE_URL" >&2
  exit 1
fi

echo "[$ENV] OK: resource_identifier registered"
