# FunnelBarn

## CI/CD Pipeline

- **PR quality gates** (`ci.yml`): go fmt, go vet, staticcheck, tsc, eslint, vitest — runs on all PRs
- **Testing** (`build-and-test.yml`): auto-deploys every push to `main` → `funnelbarn-testing` namespace
- **Staging** (`build-and-test.yml`): auto-deploys every push to `main` → `funnelbarn-staging` namespace (runs after testing succeeds)
- **Production** (`deploy-production.yml`): manual workflow_dispatch — provide a git tag (e.g. `v0.1.0`), it resolves the tag to a commit SHA, verifies images exist in GHCR, then deploys to `funnelbarn` namespace
- Images are tagged by commit SHA in GHCR (`ghcr.io/webwiebe/funnelbarn/service` and `ghcr.io/webwiebe/funnelbarn/web`)
- All deploy jobs run on self-hosted runners; CI quality checks run on ubuntu

## Environments

- **Production**: `funnelbarn.wiebe.xyz`
- **Testing**: `funnelbarn-test.wiebe.xyz`
- **Staging**: `funnelbarn-staging.wiebe.xyz`
- Subdomains use hyphens (`app-env.wiebe.xyz`), never dots (`app.env.wiebe.xyz`) — free Cloudflare SSL only covers `*.wiebe.xyz`

## Workflow

- Always use PRs — never push directly to main
