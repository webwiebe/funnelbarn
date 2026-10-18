# Spec 005: Event Data Retention + Schema Versioning

**Spec**: `specs/005-data-retention/`
**Status**: Shipped
**Last verified against the tree**: 2026-10-17 at `ea1b0e6`
**Issues**: none found
**Principles touched**: III, IX

## Purpose

Events and flag evaluations expire after a configurable number of days, and the schema carries a version so migrations apply in order.

## Functional Requirements

- **FR-001**: The system MUST read `FUNNELBARN_EVENT_RETENTION_DAYS` into `Config.EventRetentionDays` with a default of 90, and treat 0 as disabled. Issues: none. [inferred]
- **FR-002**: The system MUST delete events with `occurred_at` before the cutoff and return the deleted count, in bounded batches that stop on a cancelled context. Issues: none. Tests: `internal/repository/store_test.go::TestStore_PurgeOldEvents`, `internal/repository/purge_batch_test.go::TestPurgeStopsOnCancelledContext`, `internal/repository/purge_golden_test.go::TestPurgeRetention`.
- **FR-003**: The system MUST delete flag evaluations older than the same cutoff. Issues: none. Tests: `internal/repository/purge_evaluations_test.go::TestStore_PurgeOldEvaluations`.
- **FR-004**: The background worker MUST run a maintenance pass 2 minutes after start and then every 24 hours, with each step independent and failure-tolerant. Issues: none. Tests: `cmd/funnelbarn/maintenance_test.go::TestRunMaintenance_EmitsRootAndStepSpans`, `cmd/funnelbarn/maintenance_test.go::TestRunMaintenance_SkippedStepsEmitNoSpans`.
- **FR-005**: The system MUST skip the event and evaluation purge when retention is 0. Issues: none. Tests: `cmd/funnelbarn/maintenance_test.go::TestRunMaintenance_SkippedStepsEmitNoSpans`.
- **FR-006**: The system MUST apply schema changes as goose migrations at start-up, and the embedded schema version MUST equal the newest migration. Issues: none. Tests: `internal/repository/readonly_test.go::TestEmbeddedSchemaVersionIsNewestMigration`.

## Drift

| Claim | Source | Tree |
|---|---|---|
| A `schema_migrations` table with `INSERT OR IGNORE (version) VALUES (1)` tracks the schema | Original task, "Schema changes" | Versions live in the goose table `goose_db_version`; migrations are the numbered files in `internal/repository/migrations` (00001 to 00040) |
| Code lives in `internal/storage/schema.go` and `internal/storage/db.go` | Original task | The package is `internal/repository` (`schema.go`, `purge.go`) |
| `PurgeOldEvents` is one `DELETE FROM events WHERE occurred_at < ?` | Original task, "Storage method" | `purgeOlderThan` in `internal/repository/purge.go` deletes in batches |
| The purge runs from a 24 hour ticker in `runBackgroundWorker()` and logs through `slog.Error` | Original task | `runBackgroundWorker` in `cmd/funnelbarn/worker.go` calls `runMaintenance` (`cmd/funnelbarn/maintenance.go`) on a 24 hour ticker and once 2 minutes after start; the same pass also purges web sessions, stale auto flags, recordings and orphaned rows |
| Retention covers events | Original task | Flag evaluations are purged with the same cutoff |

## Proof contract

- [ ] Rows strictly older than the cutoff are deleted, rows at or after it remain, and a second run deletes nothing (`internal/repository/purge_golden_test.go::TestPurgeRetention`).
- [ ] With retention set to 0, a maintenance pass runs no events or evaluations step (`cmd/funnelbarn/maintenance_test.go::TestRunMaintenance_SkippedStepsEmitNoSpans`).
- [ ] A fresh database reports its newest goose version (`internal/repository/readonly_test.go::TestEmbeddedSchemaVersionIsNewestMigration`).

## Sources

- `internal/config/config.go`, `internal/repository/purge.go`, `internal/repository/schema.go`, `cmd/funnelbarn/maintenance.go`, `cmd/funnelbarn/worker.go`
- Test files named above
