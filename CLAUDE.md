# FunnelBarn

## Specs And Constitution

Work here is spec-driven. Before changing behaviour, read:

- `.specify/memory/constitution.md`: the principles CI and the tests enforce, each with its source.
- `specs/README.md`: the index of feature specs (one per product area) and the workflow.
- The `specs/NNN-<area>/spec.md` for the area you are changing.

A new feature starts as a spec or a spec change referenced from its issue. A PR
that changes behaviour updates the spec it implements in the same PR, including
its Drift section. When a spec, the docs and the code disagree, the code is the
fact; fix the document that is wrong.

## CI/CD Pipeline

- **PR quality gates** (`ci.yml`): jobs `go-quality` (gofmt, vet, `make regress`, staticcheck, sqlc up to date, complexity, file-length and duplication ratchets, gate script tests, golangci soak), `go-test` (race detector, coverage ratchet, and diff coverage on PRs, report only until 2026-10-31), `go-sdk`, `frontend-lint` (tsc, eslint, knip) and `frontend-test` (vitest with coverage floors, per-file ratchet, jscpd) and `secret-scan` (gitleaks 8.30.1 over history or the PR range via `scripts/secret-scan.sh`; self-test plants a fake secret; report-only until 2026-10-31, known history hits are fingerprints in `.gitleaksignore` and the list may only shrink). Runs on all PRs and on pushes to `main`.
- **Testing** (`build-and-test.yml`): starts when `CI` completes on `main` (`workflow_run`) and builds only when that run succeeded, from `workflow_run.head_sha`. It deploys `funnelbarn-testing`, then runs E2E and post-deploy checks.
- **Staging** (`build-and-test.yml`): `deploy-staging` needs the E2E job, so a red E2E run leaves staging unchanged. It deploys `funnelbarn-staging`.
- **Staging smoke** (`build-and-test.yml`): `smoke-staging` after `deploy-staging` runs `scripts/smoke.sh` and requires staging to report ready on exactly that commit.
- **Production** (`deploy-production.yml`): the `promote` job in `build-and-test.yml` calls it once staging and its smoke check are green, with the commit SHA as version. No manual step. Preflight verifies the images exist in GHCR and resolves them to digests; the deploy pins them by digest (`pin-release.sh`) to `funnelbarn-production`, asserts the running version and commit with `deploy/assert-version.sh` on the writer and a reader, smoke checks `funnelbarn.wiebe.xyz` on that version, rolls back on a failed rollout or smoke check (unless the release carries a migration), and posts the BugBarn release marker. Run it by hand (`workflow_dispatch`, version tag or SHA) only to roll back or redeploy a known version.
- A newer push to `main` waits for a running pipeline instead of cancelling it, so a production rollout is never cut off
- Images are tagged by commit SHA in GHCR (`ghcr.io/webwiebe/funnelbarn/service` and `ghcr.io/webwiebe/funnelbarn/web`)
- Every job runs on self-hosted runners (`[self-hosted, build]`).

## Environments

- **Production**: `funnelbarn.wiebe.xyz` (Cloudflare → k3s)
- **Testing**: `funnelbarn.test.wiebe.xyz` (legacy alias: `funnelbarn-test.nijmegen.wiebe.xyz`, still served)
- **Staging**: `funnelbarn.staging.wiebe.xyz` (legacy alias: `funnelbarn-staging.nijmegen.wiebe.xyz`, still served)
- Non-production uses `*.nijmegen.wiebe.xyz` wildcard, Caddy handles TLS, no Cloudflare free-tier limits

## Workflow

- Always use PRs, never push directly to main
