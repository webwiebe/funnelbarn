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

It sends a PUT with only `resource_identifier`; IAMBarn keeps every field the
body omits and returns the updated client. Setting the same value again is
harmless, so it runs on every deploy (IAMBarn records an `oauth.client.updated`
audit event each time). It fails loudly (exit non-zero) on a missing input, a
non-200 response, or a mismatch in the returned client. It never prints the
admin token or a client secret.

## Required token

`IAMBARN_ADMIN_TOKEN` needs the `admin:clients:write` scope on the IAMBarn
organization that owns the FunnelBarn OAuth clients, and its user must be an
admin of that organization. The pipeline reads it from SOPS at
`deploy/iambarn/secrets/<env>.yaml`. See `deploy/SECRETS.md`.
