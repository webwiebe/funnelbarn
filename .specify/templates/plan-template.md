# Implementation Plan: [FEATURE]

**Spec**: [link to spec.md] | **Issue**: #[n] | **Date**: [DATE]

<!--
Filled during refinement. The issue's "Implementation Details" become Technical
Context and Project Structure; its "Execution Plan" becomes Approach.
-->

## Summary

[One paragraph: the technical approach and why.]

## Technical Context

**Packages touched**: [e.g. internal/repository, internal/ports, internal/api, web/src]
**Languages**: [Go / TypeScript (web, sdks/js) / Python (sdks/python)]
**Storage**: [SQLite tables; recordings in R2; Valkey queues; the spool]
**New migrations**: [yes/no; number in internal/repository/migrations; sqlc regenerated]
**Process roles**: [standalone, reader, writer: which ones change]
**Testing**: [go test -race, make regress, golden files, vitest, Playwright]

## Constitution Check

Answer each line for this change, or mark it not applicable with a reason.

- [ ] I. Handlers and services reach storage through `internal/ports`; no `sqlcgen` import outside the repository
- [ ] II. Ingest still answers from the in-memory queue, sheds with 429, and refuses unattributed events
- [ ] III. A query path holds no write handle; new bookkeeping writes are commands; a reader forwards or publishes every write
- [ ] IV. IPs are truncated before hashing and user IDs are hashed; any stored IP stays behind GeoIP and can be anonymized
- [ ] V. New endpoints carry the right auth, a rate limiter where public, security headers, CSRF for session mutations, and a test
- [ ] VI. Error-level logs go through `slog`; goroutines start through `bblog.Go`
- [ ] VII. Coverage, complexity, file length and duplication do not worsen; baselines move only toward better
- [ ] VIII. Ships through CI, testing, E2E and staging; the version smoke still passes on writer and reader
- [ ] IX. Schema change is a goose migration, `sqlc generate` is committed, and golden files are updated and reviewed

## Approach

1. [Step, from the issue's Execution Plan]

## Project Structure

```text
[files to add or change]
```

## Risk Log

| Risk | Impact | Mitigation |
|------|--------|------------|
| [risk] | [impact] | [mitigation] |
