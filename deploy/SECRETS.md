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

These replace the former `MINIO_ACCESS_KEY` / `MINIO_SECRET_KEY` /
`MINIO_ENDPOINT` / `MINIO_BUCKET` secrets. The self-hosted MinIO on layer7
(`s3.wiebe.xyz`) was wound down in favour of R2 — see
`wiebe-xyz/rapid-root#2799` for the migration, and note that the `brew/` prefix
is shared with other publishers, so this repo only ever writes and prunes its
own `funnelbarn-darwin-*` keys.

The old `MINIO_*` secrets can be deleted from this repository once this
workflow has published a release to R2 successfully.

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
