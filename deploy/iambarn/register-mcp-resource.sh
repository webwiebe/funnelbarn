#!/usr/bin/env bash
# Registers FunnelBarn's MCP endpoint as an RFC 8707 resource server on the
# IAMBarn OAuth client FunnelBarn uses for dashboard OIDC login, for one
# environment. IAMBarn validates an OAuth `resource` parameter by finding a
# client row whose resource_identifier exactly equals it, so without this the
# MCP token exchange never gets a resource-scoped token.
#
# Idempotent: exits 0 without writing anything when the client's
# resource_identifier already equals RESOURCE_URL. Otherwise it re-sends the
# client's own existing fields (name, redirect_uris, post_logout_uris, scopes,
# website_url) alongside the new resource_identifier. IAMBarn's PUT treats an
# empty/omitted field as "leave unchanged", but resending the current values
# keeps this call from ever silently depending on that and makes the intent
# explicit. It never prints the admin token or a client secret; GET
# /api/v1/admin/clients does not return secrets, and this script does not
# request a rotation.
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

echo "[$ENV] looking up IAMBarn client $CLIENT_ID"

clients="$(curl -fsS \
  -H "Authorization: Bearer ${IAMBARN_ADMIN_TOKEN}" \
  "$IAM/api/v1/admin/clients")"

client="$(jq -ce --arg id "$CLIENT_ID" '.clients[] | select(.client_id == $id)' <<<"$clients")" \
  || { echo "[$ENV] FAIL: client $CLIENT_ID was not found in the IAMBarn admin clients list" >&2; exit 1; }

current="$(jq -r '.resource_identifier // ""' <<<"$client")"
if [[ "$current" == "$RESOURCE_URL" ]]; then
  echo "[$ENV] OK: resource_identifier already equals RESOURCE_URL, nothing to do"
  exit 0
fi

echo "[$ENV] resource_identifier is not yet set to RESOURCE_URL - updating the client"

existing_name="$(jq -r '.name // ""' <<<"$client")"
existing_redirect_uris="$(jq -c '.redirect_uris // []' <<<"$client")"
existing_post_logout_uris="$(jq -c '.post_logout_uris // []' <<<"$client")"
existing_scopes="$(jq -c '.scopes // []' <<<"$client")"
existing_website_url="$(jq -r '.website_url // ""' <<<"$client")"

body="$(jq -n \
  --arg name "$existing_name" \
  --argjson redirect_uris "$existing_redirect_uris" \
  --argjson post_logout_uris "$existing_post_logout_uris" \
  --argjson scopes "$existing_scopes" \
  --arg website_url "$existing_website_url" \
  --arg resource_identifier "$RESOURCE_URL" \
  '{
    name: $name,
    redirect_uris: $redirect_uris,
    post_logout_uris: $post_logout_uris,
    scopes: $scopes,
    website_url: $website_url,
    resource_identifier: $resource_identifier
  }')"

curl -fsS -X PUT \
  -H "Authorization: Bearer ${IAMBARN_ADMIN_TOKEN}" \
  -H "Content-Type: application/json" \
  -d "$body" \
  "$IAM/api/v1/admin/clients/${CLIENT_ID}" >/dev/null

clients_after="$(curl -fsS \
  -H "Authorization: Bearer ${IAMBARN_ADMIN_TOKEN}" \
  "$IAM/api/v1/admin/clients")"
after="$(jq -r --arg id "$CLIENT_ID" '.clients[] | select(.client_id == $id) | .resource_identifier // ""' <<<"$clients_after")"

if [[ "$after" != "$RESOURCE_URL" ]]; then
  echo "[$ENV] FAIL: resource_identifier after PUT is not RESOURCE_URL" >&2
  exit 1
fi

echo "[$ENV] OK: resource_identifier registered"
