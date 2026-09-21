# IAMBarn resource registration

`register-mcp-resource.sh` registers FunnelBarn's MCP endpoint as an RFC 8707
resource server on the IAMBarn OAuth client FunnelBarn uses for dashboard OIDC
login. IAMBarn only issues a resource-scoped access token for a `resource`
value that exactly matches a client's `resource_identifier`, so this step is
what lets an MCP client complete the OAuth token exchange against
`FUNNELBARN_MCP_RESOURCE_URL`.

It runs from the staging and production deploy jobs in
`.github/workflows/build-and-test.yml` and `.github/workflows/deploy-production.yml`,
before the rollout, so the route works as soon as the new pod is live. Testing
has no OIDC configured, so it does not run there.

## Usage

```sh
IAMBARN_ADMIN_TOKEN=$(sops -d --extract '["stringData"]["IAMBARN_ADMIN_TOKEN"]' deploy/iambarn/secrets/staging.yaml) \
IAMBARN_URL=https://iam.staging.wiebe.xyz \
CLIENT_ID=<FUNNELBARN_OIDC_CLIENT_ID for the environment> \
RESOURCE_URL=https://funnelbarn.staging.wiebe.xyz/api/v1/mcp \
  deploy/iambarn/register-mcp-resource.sh staging
```

It is idempotent: it exits 0 without writing when the client's
`resource_identifier` already equals `RESOURCE_URL`, and fails loudly (exit
non-zero) on a missing input, a client that cannot be found, or a mismatch
after the update. It never prints the admin token or a client secret.

## Required token

`IAMBARN_ADMIN_TOKEN` needs the `admin:clients:read` scope (the script lists
clients to read the current fields) and `admin:clients:write` (the update), on
the IAMBarn organization that owns the FunnelBarn OAuth clients. IAMBarn checks
scopes by exact match, so write does not imply read. The pipeline reads it from
SOPS at `deploy/iambarn/secrets/<env>.yaml`. See `deploy/SECRETS.md`.
