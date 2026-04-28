# FunnelBarn — Required GitHub Actions Secrets

This document lists all secrets that must be configured in the GitHub repository settings
before workflows can deploy successfully.

## SOPS Age Keys

Used to decrypt Kubernetes secret manifests per environment.

| Secret | Used by | Description |
|--------|---------|-------------|
| `SOPS_AGE_KEY_TESTING` | build-and-test.yml | Age private key for decrypting `deploy/k8s/testing/secret.yaml` |
| `SOPS_AGE_KEY_STAGING` | deploy-staging.yml | Age private key for decrypting `deploy/k8s/staging/secret.yaml` |
| `SOPS_AGE_KEY_PRODUCTION` | deploy-production.yml | Age private key for decrypting `deploy/k8s/production/secret.yaml` |

## MinIO / S3 Storage

Used to publish Homebrew tarballs and APT packages to the shared MinIO bucket.

| Secret | Description |
|--------|-------------|
| `MINIO_ACCESS_KEY` | MinIO user access key (webwiebe-apt) |
| `MINIO_SECRET_KEY` | MinIO user secret key |
| `MINIO_ENDPOINT` | MinIO endpoint URL (e.g. https://s3.wiebe.xyz) |
| `MINIO_BUCKET` | MinIO bucket name (e.g. webwiebe-apt-repository) |

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
