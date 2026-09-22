# FunnelBarn — Required GitHub Actions Secrets

This document lists all secrets that must be configured in the GitHub repository settings
before workflows can deploy successfully.

## SOPS Age Keys

Used to decrypt Kubernetes secret manifests per environment.

| Secret | Used by | Description |
|--------|---------|-------------|
| `SOPS_AGE_KEY_TESTING` | build-and-test.yml | Age private key for decrypting `deploy/k8s/testing/secret.yaml` |
| `SOPS_AGE_KEY_STAGING` | build-and-test.yml (`deploy-staging` job) | Age private key for decrypting `deploy/k8s/staging/secret.yaml` |
| `SOPS_AGE_KEY_PRODUCTION` | deploy-production.yml | Age private key for decrypting `deploy/k8s/production/secret.yaml` |

## Cloudflare R2 (package repository)

Used by `binary-release.yml` to publish Homebrew tarballs into the shared
package-repository bucket, under the `brew/` prefix.

| Secret | Description |
|--------|-------------|
| `R2_ACCESS_KEY` | Cloudflare R2 S3 access key |
| `R2_SECRET_KEY` | Cloudflare R2 S3 secret key |
| `R2_ENDPOINT` | R2 account S3 endpoint URL |
| `R2_BUCKET` | Bucket name (`webwiebe-apt-repository-production`) |

The `brew/` prefix is shared with other publishers, so this repo only ever
writes and prunes its own `funnelbarn-darwin-*` keys. See
`wiebe-xyz/rapid-root#2799` for how the shared package repository is set up.

## Infrastructure SSH

Used to SSH into the k3s cluster and apply Kubernetes manifests.

| Secret | Description |
|--------|-------------|
| `K3S_SSH_KEY` | SSH private key for the `deployer` user on `layer7.wiebe.xyz` |

## BugBarn Integration

Used to post release markers and upload source maps to BugBarn for error tracking.

| Secret | Used by | Description |
|--------|---------|-------------|
| `FUNNELBARN_BUGBARN_API_KEY` | deploy-production.yml, binary-release.yml | BugBarn API key for the `funnelbarn` project |

## IAMBarn MCP Resource Registration

Used by `deploy/iambarn/register-mcp-resource.sh`, run from the staging and
production deploy jobs, to register FunnelBarn's MCP endpoint as an RFC 8707
resource server on the IAMBarn OAuth client FunnelBarn uses for dashboard
OIDC login. Not used in testing, which has no OIDC configured.

The IAMBarn "Create API token" form always creates a PAT with no scopes, and a
PAT without `admin:clients:write` gets `403 insufficient_scope`. Create it from
the browser console while signed in to the right IAMBarn instance:

```js
const { csrf_token } = await (await fetch('/api/v1/csrf')).json()
const res = await fetch('/api/v1/me/tokens', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrf_token },
  body: JSON.stringify({ name: 'funnelbarn mcp registration', scopes: ['admin:clients:write'] }),
})
console.log(await res.json())
```

To replace a token without writing it to disk:

```sh
sops set deploy/k8s/staging/secret.yaml '["stringData"]["IAMBARN_ADMIN_TOKEN"]' '"iampat_..."'
```

| Value | Used by | Description |
|--------|---------|-------------|
| `IAMBARN_ADMIN_TOKEN` | build-and-test.yml (`deploy-staging` job), deploy-production.yml | IAMBarn PAT with scope `admin:clients:write`, for the organization that owns the FunnelBarn OAuth clients. Stored in SOPS at `deploy/k8s/<env>/secret.yaml` (key `stringData.IAMBARN_ADMIN_TOKEN`). It lands in the app Secret, but no container references it, so it never reaches a pod's environment. Staging and production are separate IAMBarn instances, so each file holds its own PAT. |

## APT Repository Dispatch

| Secret | Description |
|--------|-------------|
| `RAPID_ROOT_DISPATCH_TOKEN` | GitHub PAT with `repo` scope on `wiebe-xyz/rapid-root` for APT publishing |

## Homebrew Tap

| Secret | Description |
|--------|-------------|
| `TAP_GITHUB_TOKEN` | GitHub PAT with `repo` scope on `webwiebe/homebrew-funnelbarn` for formula updates |

## Secret Template Encryption

The YAML files in `deploy/k8s/*/secret.yaml` are SOPS-encrypted templates.
After filling in real values, encrypt each with the appropriate age key:

```bash
# Example: encrypt testing secret
sops --age <AGE_PUBLIC_KEY_TESTING> -e -i deploy/k8s/testing/secret.yaml

# Example: encrypt production secret
sops --age <AGE_PUBLIC_KEY_PRODUCTION> -e -i deploy/k8s/production/secret.yaml
```
