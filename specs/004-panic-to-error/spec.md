# Spec 004: Replace Panics with Proper Error Propagation

**Spec**: `specs/004-panic-to-error/`
**Status**: Shipped
**Last verified against the tree**: 2026-10-26 at `54cc4a7`
**Issues**: [#376](https://github.com/webwiebe/funnelbarn/issues/376)
**Principles touched**: VI

## Purpose

A failure of `crypto/rand` must not crash the server. UUID generation returns an error, and callers pass it up instead of calling `panic()`.

## Functional Requirements

- **FR-001**: The system MUST NOT call `panic()` in a UUID generator. `generateUUID` in `internal/repository/helpers.go` and `generateUUIDLocal` in `internal/worker/uuid.go` return `(string, error)`. Issues: none. [inferred]
- **FR-002**: The worker MUST wrap a UUID failure and return it to its caller instead of continuing (`internal/worker/worker.go`). Issues: none. [inferred]
- **FR-003**: The ingest handler MUST keep accepting events when `crypto/rand` fails, using a timestamp ID and an error-level log (`generateIngestID` in `internal/ingest/handler.go`). Issues: none. [inferred]
- **FR-004**: The repository MUST return the UUID error from every create call (users, API keys, projects) so the HTTP handler answers 500. Issues: [#376](https://github.com/webwiebe/funnelbarn/issues/376). Tests: `internal/repository/uuid_create_test.go::TestCreateCalls_ReturnUUIDError`.

## Drift

| Claim | Source | Tree |
|---|---|---|
| The storage layer lives in `internal/storage/` with `helpers.go`, `db.go`, `events.go`, `sessions.go`, `abtests.go`, `funnels.go` | Original task | The package is `internal/repository/` |
| No `panic()` calls remain in either UUID function | Acceptance criteria | Holds; no non-test `panic(` call exists under `internal/` or `cmd/` |

## Proof contract

- [ ] `grep -rn 'panic(' internal cmd` over non-test Go files returns nothing.
- [ ] Every create call whose UUID generation fails returns the error and writes no row (`internal/repository/uuid_create_test.go::TestCreateCalls_ReturnUUIDError`).

## Sources

- `internal/repository/helpers.go`, `internal/repository/users.go`, `internal/repository/apikeys.go`, `internal/repository/projects.go`, `internal/worker/uuid.go`, `internal/worker/worker.go`, `internal/ingest/handler.go`
- The original task text of this spec, replaced by this version on 2026-10-17
