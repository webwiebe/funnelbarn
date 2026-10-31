# Engineering Standards: Architecture Patterns & Code Quality

**Spec**: `specs/003-engineering-standards/`
**Status**: Partial
**Last verified against the tree**: 2026-10-17 at `ea1b0e6`
**Issues**: [#196](https://github.com/webwiebe/funnelbarn/issues/196), [#273](https://github.com/webwiebe/funnelbarn/issues/273), [#275](https://github.com/webwiebe/funnelbarn/issues/275)
**Principles touched**: I, VII, VIII, IX
**Created**: 2026-04-30

---

## Purpose

CI enforces the architecture and quality rules of the codebase mechanically. The enforced rules are listed under Functional Requirements; the targets the tree does not meet are listed under Drift. The constitution at `.specify/memory/constitution.md` holds the principles that have a gate or test behind them.

## Functional Requirements

### Backend architecture

- **FR-001**: `internal/api`, `internal/service` and `internal/mcp` MUST NOT import `internal/repository/sqlcgen`. Issues: [#275](https://github.com/webwiebe/funnelbarn/issues/275). Tests: `internal/archtest/archtest_test.go::TestArchitecture`.
- **FR-002**: `internal/repository` MUST NOT import `internal/service`, `internal/api` or `internal/ports`; `internal/domain` MUST NOT import `internal/repository`; `internal/ports` MUST NOT import `internal/service`, `internal/api` or `internal/command`. Issues: [#275](https://github.com/webwiebe/funnelbarn/issues/275). Tests: `internal/archtest/archtest_test.go::TestArchitecture`.
- **FR-003**: Schema changes MUST be goose migrations in `internal/repository/migrations`, applied at start-up, and the embedded schema version MUST equal the newest migration. Issues: none. Tests: `internal/repository/readonly_test.go::TestEmbeddedSchemaVersionIsNewestMigration`.
- **FR-004**: The generated query code in `internal/repository/sqlcgen` MUST equal the output of `sqlc generate`; CI fails on any difference. Issues: none. [inferred]
- **FR-005**: Repository tests MUST use a real SQLite database. Issues: none. Tests: `internal/repository/store_test.go::TestStore_CreateFunnel_WithSteps`.
- **FR-006**: API handlers MUST be tested through the HTTP stack with a real store. Issues: none. Tests: `internal/api/golden_test.go::TestGoldenAPI`.

### Frontend architecture

- **FR-007**: Components MUST sit in namespace directories under `web/src/components` (`shell`, `charts`, `ui`, `wizards` and others), and pages in `web/src/pages`. Issues: none. [inferred]

### CI quality gates

- **FR-008**: CI MUST fail on unformatted Go code, `go vet` findings and `staticcheck` findings. Issues: none. [inferred]
- **FR-009**: CI MUST run the Go tests with `-race` and fail when per-package or total coverage falls below the floors in `scripts/coverage-baseline.txt` (`internal/service` 74, `internal/repository` 74, total 64). Issues: none. [inferred]
- **FR-010**: CI MUST fail on any golangci-lint finding. Issues: [#273](https://github.com/webwiebe/funnelbarn/issues/273). Tests: `scripts/test_quality_soak.py`.
- **FR-011**: CI MUST run `tsc --noEmit`, `eslint` and `vitest` with coverage floors for the web app. Issues: [#196](https://github.com/webwiebe/funnelbarn/issues/196). [inferred]
- **FR-012**: The gate scripts MUST have tests that run in CI. Issues: none. Tests: `scripts/test_quality_soak.py`, `scripts/test_deploy_graph.py`.
- **FR-013**: The testing deploy MUST build only after a green CI run, and E2E failures block promotion to staging. Issues: none. Tests: `scripts/test_deploy_graph.py::DeployGraphTest.test_build_requires_ci_success`, `scripts/test_deploy_graph.py::DeployGraphTest.test_staging_waits_for_e2e`, `scripts/test_deploy_graph.py::SpecMatchesMechanismTest.test_spec_still_claims_e2e_blocks_staging` (this spec must keep the sentence "E2E failures block promotion to staging").

---

## Non-Goals

- This spec does not introduce a new database engine. SQLite remains.
- This spec does not change the ingest spool architecture.
- This spec does not add a REST framework or replace `net/http`.
- Full 100% test coverage is not required; the floors are minimums.

## Drift

| Claim | Source | Tree |
|---|---|---|
| Handlers (`internal/api`, `internal/ingest`, `cmd`) call only services | Layered Structure rules | `internal/api` imports `internal/repository` in 20 files, `internal/service` in 6 and `internal/ports` in 1; the only enforced rule is the `sqlcgen` ban (`internal/archtest/archtest.go`) |
| Migrations, queries and generated code live in `internal/db/{migrations,queries,generated}` | Package Layout | They live in `internal/repository/{migrations,queries,sqlcgen}`, with `sqlc.yaml` in `internal/repository` |
| Migration files are named `000001_initial_schema.sql` | Migration Tooling | Names have five digits, for example `00040_funnel_conversion_window.sql` |
| `goose up` and `goose down` are available as a CLI subcommand | Migration Tooling | `goose.Up` runs in `internal/repository/db.go` at start-up; no `goose` subcommand exists in `cmd/` |
| Repository tests use `:memory:` SQLite | Unit Tests | Tests open a temp-file database (`repository.Open(filepath.Join(t.TempDir(), "test.db"))`); the write and read pools of an in-memory database alias each other (spec 012) |
| Every service and repository function has a unit test | Unit Tests | not established |
| Files stay under about 200 lines | Style | `MAX_FILE_LINES=848` in `scripts/go-quality-gates.sh`; the estate target of 500 is report-only (`scripts/soak-baselines/file-length.txt`) |
| `pages/ABTests.tsx` exists, `lib/hooks/` holds custom hooks | Frontend layout | Neither exists |
| `lib/api.ts` is the only file that calls `fetch` | Frontend rules | `fetch(` appears in `App.tsx`, `lib/bugbarn.ts`, `lib/projects.tsx`, `lib/analytics.ts` and `pages/Live.tsx` as well |
| Each component file exports exactly one default component | Frontend rules | not established |
| Vitest coverage fails under 80 percent (and 70 percent in the CI table) | Unit Tests and CI table | `web/vite.config.ts` pins lines 51, statements 48, functions 42, branches 45; the per-file gate adds a floor per source file |
| Lint is `golangci-lint run ./...`, with 120 findings in the soak | CI table | Findings reached zero and `golangci-lint run ./...` is a blocking step in `go-quality` (#273) |
| Coverage fails below 70 percent on `service/` and `repository/` | CI table | Per-package floors are 74 and 74 in `scripts/coverage-baseline.txt`, plus a total floor of 64 |
| `sqlc vet` checks the generated code | CI table | CI runs `sqlc generate` and `git diff --exit-code` on `internal/repository/sqlcgen` |
| `vite build` runs in the frontend quality job | CI table | `ci.yml` does not build the web app; `deploy/docker/web.Dockerfile` runs the build when the image is made |
| Quality jobs `backend-quality` and `frontend-quality` run in parallel | Pipeline Structure | `ci.yml` has `go-quality`, `go-test`, `go-sdk`, `frontend-lint` and `frontend-test`; `build-and-test.yml` starts on a successful CI run (`workflow_run`) |
| Deployment setup includes litestream | Non-Goals | Litestream is still in `deploy/docker/` and the writer manifests |

## Proof contract

- [ ] `make regress` passes `TestArchitecture` on the current tree, and the rule engine reports a violation for a synthetic bad import (`internal/archtest/archtest_test.go::TestRuleEngine`).
- [ ] The CI run of a pull request that worsens a coverage floor, a complexity limit or a file length limit is red.
- [ ] A commit to `main` whose CI run is red produces no testing deploy (`scripts/test_deploy_graph.py::DeployGraphTest.test_build_requires_ci_success`).
- [ ] A red E2E run leaves staging unchanged (`scripts/test_deploy_graph.py::DeployGraphTest.test_staging_waits_for_e2e`).

## Sources

- `internal/archtest/archtest.go`, `scripts/go-quality-gates.sh`, `scripts/coverage-baseline.txt`, `scripts/quality-soak.py`, `scripts/soak-baselines/`
- `.github/workflows/ci.yml`, `.github/workflows/build-and-test.yml`, `web/vite.config.ts`
- `internal/repository/db.go`, `internal/repository/sqlc.yaml`
