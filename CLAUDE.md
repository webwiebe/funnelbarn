# FunnelBarn

## CI/CD Pipeline

- **PR quality gates** (`ci.yml`): go fmt, go vet, staticcheck, tsc, eslint, vitest — runs on all PRs
- **Testing** (`build-and-test.yml`): auto-deploys every push to `main` → `funnelbarn-testing` namespace
- **Staging** (`build-and-test.yml`): auto-deploys every push to `main` → `funnelbarn-staging` namespace (runs after testing and E2E succeed), then `smoke-staging` (`scripts/smoke.sh`) requires staging to report ready on exactly that version
- **Production** (`deploy-production.yml`): the `promote` job in `build-and-test.yml` calls it once staging and its smoke check are green, with the commit SHA as version. No manual step. It verifies the images exist in GHCR (preflight), deploys to `funnelbarn-production`, smoke checks `funnelbarn.wiebe.xyz` on that version, rolls back on a failed rollout or smoke check (unless the release carries a migration), and posts the BugBarn release marker. Run it by hand (`workflow_dispatch`, version tag or SHA) only to roll back or redeploy a known version
- A newer push to `main` waits for a running pipeline instead of cancelling it, so a production rollout is never cut off
- Images are tagged by commit SHA in GHCR (`ghcr.io/webwiebe/funnelbarn/service` and `ghcr.io/webwiebe/funnelbarn/web`)
- All deploy jobs run on self-hosted runners; CI quality checks run on ubuntu

## Environments

- **Production**: `funnelbarn.wiebe.xyz` (Cloudflare → k3s)
- **Testing**: `funnelbarn.test.wiebe.xyz` (legacy alias: `funnelbarn-test.nijmegen.wiebe.xyz`, still served)
- **Staging**: `funnelbarn.staging.wiebe.xyz` (legacy alias: `funnelbarn-staging.nijmegen.wiebe.xyz`, still served)
- Non-production uses `*.nijmegen.wiebe.xyz` wildcard — Caddy handles TLS, no Cloudflare free-tier limits

## Workflow

- Always use PRs — never push directly to main
