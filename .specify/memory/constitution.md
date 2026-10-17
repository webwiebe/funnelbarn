# FunnelBarn Constitution

These are the rules the codebase already enforces. Each principle names the
test, gate or issue that backs it. A principle with no source in the tree does
not belong here.

FunnelBarn is a self-hosted analytics and feature-flag server. The sites it
measures must never wait for it, and its single SQLite writer must never be
asked to do two jobs at once, so the first three principles protect those two
things.

## Core Principles

### I. Callers reach storage through ports

HTTP handlers, services and the MCP server MUST reach the database through the
query and command interfaces in `internal/ports`. The generated SQL in
`internal/repository/sqlcgen` MUST stay inside `internal/repository`. The
repository MUST NOT import services, handlers or ports. A query port MUST be
satisfied by `*repository.ReadStore`, which holds no write handle.

Sources:
- `internal/archtest/archtest_test.go::TestArchitecture` (rules in `internal/archtest/archtest.go`), run by `make regress` in the `go-quality` job of `.github/workflows/ci.yml`
- `internal/ports/assert_test.go` (compile-time assertions that `*repository.ReadStore` implements every query port)
- [specs/012-cqrs-read-write-split/spec.md](../../specs/012-cqrs-read-write-split/spec.md); issue [#302](https://github.com/webwiebe/funnelbarn/issues/302)

### II. Ingest answers fast and sheds load with a 429

`POST /api/v1/events` MUST answer after queueing the record in memory and MUST
NOT write to the database in the request. A full queue MUST answer 429 with
`Retry-After`. A stop MUST flush the queue to the on-disk spool before the
process exits. An event that names no project, or a project that no longer exists,
MUST be refused at the door instead of being dead-lettered later.

Sources:
- `internal/ingest/handler_test.go::TestServeHTTP_Accepted`, `::TestStart_DrainOnCancel`, `::TestStartBackground_StopWaitsForTheFinalFlush`, `::TestServeHTTP_RejectsUnattributedEvent`, `::TestServeHTTP_KeyOfDeletedProjectIsRejected`
- `internal/spool/spool_test.go::TestSizeLimit_ErrFull`
- `cmd/funnelbarn/durability_test.go::TestReaderTrafficSurvivesAStoppedWriter`, run by `make regress`
- Issues [#209](https://github.com/webwiebe/funnelbarn/issues/209), [#257](https://github.com/webwiebe/funnelbarn/issues/257)

### III. Queries cannot write and one connection writes

A query adapter MUST open the database read-only, so a write routed to it
fails. All writes MUST go through the one write connection or through the
command bus, and a request that only reads MUST NOT wait for a write.
Bookkeeping writes caused by a read (evaluation rows, touches, health marks)
MUST be commands. Commands MUST NOT be dropped under backpressure. Maintenance
deletes MUST run in bounded batches. A reader MUST refuse to start without the
writer URL and the internal token.

Sources:
- `internal/repository/readonly_test.go::TestOpenReadOnlyRefusesWrites`, `internal/repository/readpool_test.go::TestReadPool_RejectsWritesAndSeesCommits`
- `internal/api/contention_test.go::TestContention`, `::TestEvaluateWithWriterHeld`
- `internal/api/golden_modes_test.go::TestGoldenReaderMode`
- `internal/command/dispatcher_test.go::TestBackpressureBlocksAndNeverDrops`
- `internal/repository/purge_golden_test.go::TestPurgeRetention`, run by `make regress`
- `internal/config/mode_test.go::TestValidateMode`
- [specs/012-cqrs-read-write-split/spec.md](../../specs/012-cqrs-read-write-split/spec.md); issue [#302](https://github.com/webwiebe/funnelbarn/issues/302)

### IV. Visitor identity is hashed before it is stored

A fallback session ID MUST be a hash of the IP truncated to /24 (IPv4) or /48
(IPv6), the user agent, the project, the environment and a 30 minute window.
A client-supplied user ID MUST be SHA256 hashed before storage. The fallback
session MUST be scoped to its project and environment. Full IPs MAY be stored
only when a GeoIP database is configured, and an admin MUST be able to
anonymize them.

Sources:
- `internal/session/fingerprint_test.go::TestFingerprint_IPv4AnonymizationSlash24`, `::TestFingerprint_IPv6AnonymizationSlash48`, `::TestFingerprint_DifferentProjectsDifferentOutput`, `::TestFingerprint_RotatesAcrossWindows`
- `internal/enrich/enrich_test.go::TestHashUserID`
- `internal/worker/worker_test.go::TestProcessRecord_UserIDHashed`
- Issues [#225](https://github.com/webwiebe/funnelbarn/issues/225), [#226](https://github.com/webwiebe/funnelbarn/issues/226)

### V. Credentials are hashed and the HTTP edge is defended

Dashboard sessions MUST be opaque tokens stored hashed on the server, and
mutating session requests MUST carry a CSRF token. Every response MUST carry
the security headers. Login, ingest, setup and API routes MUST be rate limited
per client IP, and the client IP MUST honour `X-Forwarded-For` only from a
trusted proxy when proxies are configured. A recording chunk MUST be refused
for a global ingest key.

Sources:
- `internal/auth/auth_test.go::TestHashSessionToken`, `::TestSessionManager_CreateOpaqueToken`, `::TestCSRFToken_KeyedBySessionSecret`
- `internal/api/csrf_test.go::TestCSRF_MutatingRequestWithoutToken_Blocked`
- `internal/api/security_test.go::TestSecurityHeaders_Present`, `::TestSecurityHeaders_OnErrorResponse`, `::TestRateLimit_ResponseFormat`
- `internal/api/ratelimit_test.go::TestRateLimiter_BlocksOverBurst`, `::TestRateLimiter_MiddlewareReturns429`
- `internal/api/recordings_globalkey_test.go::TestIngestRecordingChunk_GlobalKeyRejected`
- [specs/013-security-middleware/spec.md](../../specs/013-security-middleware/spec.md)

### VI. Error-level logs reach BugBarn

Error-level log paths MUST use `slog` so the BugBarn handler captures them.
The handler MUST forward error records and MUST NOT forward warn or below.
Background goroutines MUST be started through `bblog.Go` so a panic is logged
and does not stop the process.

Sources:
- `internal/bblog/delivery_test.go::TestHandler_ForwardsErrorWithMessageAndAttributes`, `::TestHandler_DoesNotForwardWarnOrBelow`
- `internal/bblog/goroutine_test.go::TestGo_RecoversPanicAndLogs`
- `internal/worker/worker_test.go::TestSafeProcess_RecoversPanic`

### VII. Quality metrics only move toward better

Go complexity, file length and duplication are pinned in
`scripts/go-quality-gates.sh`, and the numbers may only go down. Go coverage
floors per package and in total are pinned in `scripts/coverage-baseline.txt`
and may only go up. Web coverage floors are pinned in `web/vite.config.ts`,
per-file coverage and duplication have their own gates, and the dead-code
ratchet has a test. The golangci-lint soak blocks a new finding, a worsened
file and a baseline that became beatable. The gate scripts MUST have tests.

Sources:
- `scripts/go-quality-gates.sh`, `scripts/coverage-gate.py`, `scripts/coverage-baseline.txt`, run in the `go-quality` and `go-test` jobs of `.github/workflows/ci.yml`
- `web/vite.config.ts` (coverage thresholds), `pnpm gate:coverage-perfile`, `pnpm gate:duplication`, `pnpm lint:knip-test`, run in the `frontend-lint` and `frontend-test` jobs
- `scripts/quality-soak.py` against `scripts/soak-baselines/`, tested by `scripts/test_quality_soak.py`
- `python3 -m unittest discover -s scripts -p 'test_*.py'` ("Gate script tests" step of `ci.yml`)
- Issue [#196](https://github.com/webwiebe/funnelbarn/issues/196)

### VIII. Deploys follow green CI and assert the version

Testing builds and deploys only the commit a green CI run passed, and every
job in that workflow MUST use that one SHA. Staging MUST wait for the E2E run
against testing. Every environment MUST assert the running version and commit
through `/api/v1/version` on the writer and on a reader after rollout. A
production deploy MUST resolve its tag to a SHA, find both images in GHCR and
roll back automatically when the rollout fails.

Sources:
- `.github/workflows/build-and-test.yml`, `.github/workflows/deploy-production.yml`
- `scripts/test_deploy_graph.py::DeployGraphTest.test_build_requires_ci_success`, `::test_every_job_builds_or_deploys_one_commit`, `::test_staging_waits_for_e2e`, `::test_no_job_swallows_a_failure`
- `scripts/test_version_smoke.py::DeployWiringTest.test_every_environment_runs_the_smoke_on_writer_and_reader`
- `scripts/test_runner_scope.py` (reclaim steps touch only their own workspace)
- Issues [#262](https://github.com/webwiebe/funnelbarn/issues/262)

### IX. The schema is migrations, and generated code matches them

Schema changes MUST be goose migrations in `internal/repository/migrations`,
applied at start-up. The embedded schema version MUST equal the newest
migration. `internal/repository/sqlcgen` MUST equal the output of `sqlc
generate`. Every GET route and every flag evaluation MUST be covered by a
golden API snapshot.

Sources:
- `internal/repository/readonly_test.go::TestEmbeddedSchemaVersionIsNewestMigration`
- the "sqlc generate is up to date" step of `.github/workflows/ci.yml`
- `internal/api/golden_coverage_test.go::TestGoldenRouteCoverage`, `internal/api/golden_test.go::TestGoldenAPI`, run by `make regress`

## Not ratified

Candidates checked against the tree and left out because nothing enforces them:

- A production deploy goes out on its own after staging. Production runs only
  from a manual `workflow_dispatch` with a confirmation input
  (`.github/workflows/deploy-production.yml`). Issue
  [#364](https://github.com/webwiebe/funnelbarn/issues/364) is open.
- Files stay under about 200 lines
  ([specs/003-engineering-standards/spec.md](../../specs/003-engineering-standards/spec.md)).
  The pinned limit is 848 lines (`MAX_FILE_LINES` in `scripts/go-quality-gates.sh`); the 500 line target is report-only (`scripts/soak-baselines/file-length.txt`).
- Services and handlers stay under the estate complexity target of 12. The
  blocking limit is 49 (`MAX_CYCLO`); 12 is report-only
  (`scripts/soak-baselines/go-complexity.txt`).
- golangci-lint blocks on any finding. It blocks only on a new one while
  `scripts/soak-baselines/golangci.txt` records the existing ones. Issues
  [#273](https://github.com/webwiebe/funnelbarn/issues/273) and
  [#275](https://github.com/webwiebe/funnelbarn/issues/275) are open.
- 70 percent Go coverage on `service/` and `repository/`, and 80 percent web
  line coverage ([specs/003-engineering-standards/spec.md](../../specs/003-engineering-standards/spec.md)).
  The floors are the current values in `scripts/coverage-baseline.txt` and
  `web/vite.config.ts`.
- Ingest p99 under 5 ms and dashboard queries under 500 ms for 10 million
  events ([specs/001-analytics-funnel/spec.md](../../specs/001-analytics-funnel/spec.md), NFR-001 and NFR-002).
  The k6 scripts in `loadtest/` run by hand and no gate fails on a latency number.

## Governance

- This file changes through a pull request that edits the principle and names
  the gate, test, ADR or issue that now backs the change. A principle whose
  source is removed from the tree is removed or amended in the same PR.
- A principle is tightened by adding a gate or test. It is weakened only by an
  ADR that says why.
- Every spec under `specs/` lists the principles it touches in its
  "Principles touched" header line, and every plan answers the Constitution
  Check in `.specify/templates/plan-template.md`.
- Where this file and the agent rule (`AGENTS.md`, `CLAUDE.md`) disagree, fix
  whichever one is wrong against the tree in the same PR.

**Version**: 1.0.0 | **Ratified**: 2026-10-17 | **Last Amended**: 2026-10-17
