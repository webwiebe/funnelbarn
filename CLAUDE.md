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

- **PR quality gates** (`ci.yml`): jobs `go-quality` (gofmt, vet, `make regress`, staticcheck, sqlc up to date, complexity, file-length and duplication ratchets, gate script tests, golangci soak), `go-test` (race detector and coverage ratchet), `go-sdk`, `frontend-lint` (tsc, eslint, knip) and `frontend-test` (vitest with coverage floors, per-file ratchet, jscpd). Runs on all PRs and on pushes to `main`.
- **Testing** (`build-and-test.yml`): starts when `CI` completes on `main` (`workflow_run`) and builds only when that run succeeded, from `workflow_run.head_sha`. It deploys `funnelbarn-testing`, then runs E2E and post-deploy checks.
- **Staging** (`build-and-test.yml`): `deploy-staging` needs the E2E job, so a red E2E run leaves staging unchanged. It deploys `funnelbarn-staging`.
- **Production** (`deploy-production.yml`): manual `workflow_dispatch` with the input `production_version` (a tag such as `v0.1.0`) and a `confirmed` checkbox. Preflight resolves the tag to a commit SHA and verifies the images exist in GHCR; the deploy asserts the running version with `deploy/assert-version.sh` on the writer and a reader, and rolls back with `kubectl rollout undo` when the smoke fails.
- Images are tagged by commit SHA in GHCR (`ghcr.io/webwiebe/funnelbarn/service` and `ghcr.io/webwiebe/funnelbarn/web`)
- Every job runs on self-hosted runners (`[self-hosted, build]`).

## Environments

- **Production**: `funnelbarn.wiebe.xyz` (Cloudflare → k3s)
- **Testing**: `funnelbarn.test.wiebe.xyz` (legacy alias: `funnelbarn-test.nijmegen.wiebe.xyz`, still served)
- **Staging**: `funnelbarn.staging.wiebe.xyz` (legacy alias: `funnelbarn-staging.nijmegen.wiebe.xyz`, still served)
- Non-production uses `*.nijmegen.wiebe.xyz` wildcard, Caddy handles TLS, no Cloudflare free-tier limits

## Workflow

- Always use PRs, never push directly to main
