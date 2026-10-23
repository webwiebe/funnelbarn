# Feature Specification: CQRS Read/Write Split

**Spec**: `specs/012-cqrs-read-write-split/`
**Status**: Shipped
**Last verified against the tree**: 2026-10-17 at `ea1b0e6`
**Issues**: [#302](https://github.com/webwiebe/funnelbarn/issues/302), [#307](https://github.com/webwiebe/funnelbarn/issues/307), [#337](https://github.com/webwiebe/funnelbarn/issues/337), [#346](https://github.com/webwiebe/funnelbarn/issues/346)
**Principles touched**: I, II, III, IX
**Created**: 2026-10-07

---

## Functional Requirements

- **FR-001**: A query adapter MUST open the database read-only, so a write routed to it fails. Issues: [#302](https://github.com/webwiebe/funnelbarn/issues/302). Tests: `internal/repository/readonly_test.go::TestOpenReadOnlyRefusesWrites`, `internal/repository/readpool_test.go::TestReadPool_RejectsWritesAndSeesCommits`.
- **FR-002**: `*repository.ReadStore` MUST implement every query port and hold no write handle. Issues: [#302](https://github.com/webwiebe/funnelbarn/issues/302). Tests: `internal/ports/assert_test.go` (compile-time assertions).
- **FR-003**: `internal/ports` MUST NOT import `internal/command`, and `internal/api`, `internal/service` and `internal/mcp` MUST NOT import `internal/repository/sqlcgen`. Issues: [#302](https://github.com/webwiebe/funnelbarn/issues/302). Tests: `internal/archtest/archtest_test.go::TestArchitecture`.
- **FR-004**: The write pool MUST hold one connection and the read pool four; in-memory databases alias the two. Issues: [#302](https://github.com/webwiebe/funnelbarn/issues/302). Tests: `internal/repository/readpool_test.go::TestReadPool_AliasesWritePoolInMemory`.
- **FR-005**: A full command buffer MUST block the submitter and never drop a command. Issues: [#302](https://github.com/webwiebe/funnelbarn/issues/302). Tests: `internal/command/dispatcher_test.go::TestBackpressureBlocksAndNeverDrops`.
- **FR-006**: `Flush` MUST wait for every submitted command, and shutdown MUST drain with a timeout. Issues: [#302](https://github.com/webwiebe/funnelbarn/issues/302). Tests: `internal/command/dispatcher_test.go::TestFIFOAndFlush`, `internal/command/dispatcher_test.go::TestCloseDrainsAndTimesOut`.
- **FR-007**: A failing or panicking command MUST NOT stop the consumer. Issues: [#302](https://github.com/webwiebe/funnelbarn/issues/302). Tests: `internal/command/dispatcher_test.go::TestApplyErrorAndPanicDoNotStopConsumer`.
- **FR-008**: `EnsureAutoFlag` MUST re-check the auto-flag cap when the consumer applies it and be idempotent per key. Issues: [#302](https://github.com/webwiebe/funnelbarn/issues/302). Tests: `internal/command/dispatcher_test.go::TestEnsureAutoFlagRechecksCap`, `internal/repository/flags_autoregister_test.go::TestStore_EnsureAutoFlag_Idempotent`.
- **FR-009**: The evaluate path MUST complete while the write connection is held, and every queued row MUST land after release. Issues: [#302](https://github.com/webwiebe/funnelbarn/issues/302). Tests: `internal/api/contention_test.go::TestContention`, `internal/api/contention_test.go::TestEvaluateWithWriterHeld`.
- **FR-010**: Retention deletes MUST run in batches of 500 and stop on a cancelled context. Issues: [#302](https://github.com/webwiebe/funnelbarn/issues/302). Tests: `internal/repository/purge_batch_test.go::TestPurgeStopsOnCancelledContext`, `internal/repository/purge_golden_test.go::TestPurgeRetention`.
- **FR-011**: `FUNNELBARN_MODE` MUST default to `standalone`, and reader and writer modes MUST refuse to start without the Redis queue URL and the internal token (a reader also without the writer URL). Issues: [#302](https://github.com/webwiebe/funnelbarn/issues/302). Tests: `internal/config/mode_test.go::TestLoad_ModeDefaultsToStandalone`, `internal/config/mode_test.go::TestValidateMode`, `internal/config/mode_test.go::TestLoad_WriterURLAndInternalToken`.
- **FR-012**: A reader MUST forward every write to the writer, carry the client address and the internal token, and answer 502 while the writer is down. Issues: [#302](https://github.com/webwiebe/funnelbarn/issues/302). Tests: `internal/api/forwarder_test.go::TestForwardsToWriter`, `internal/api/forwarder_test.go::TestForwardCarriesTheClientAndTheWritersAnswer`, `internal/api/forwarder_test.go::TestForwardAnswers502WhileTheWriterIsDown`, `internal/api/forwarder_test.go::TestReaderForwardsASessionThatNeedsAWrite`.
- **FR-013**: The writer MUST believe the forwarded client headers only when the token matches. Issues: [#302](https://github.com/webwiebe/funnelbarn/issues/302). Tests: `internal/api/forwarder_test.go::TestWriterTrustsForwardedHeadersWithTheToken`.
- **FR-014**: A reader MUST serve reads itself, and every golden case MUST give the same answer in reader mode as in standalone mode. Issues: [#302](https://github.com/webwiebe/funnelbarn/issues/302). Tests: `internal/api/forwarder_test.go::TestReaderServesReadsItself`, `internal/api/golden_modes_test.go::TestGoldenReaderMode`.
- **FR-015**: `GET /api/v1/ready` MUST answer 503 with its reason until the database reaches the newest migration. Issues: [#302](https://github.com/webwiebe/funnelbarn/issues/302). Tests: `internal/api/ready_test.go::TestReady_GateFailsWithItsReason`, `internal/api/ready_test.go::TestReady_PassesThroughWhenTheGateHolds`.
- **FR-016**: After a failed publish the bus MUST use the in-process fallback for a cooldown, and one evaluation MUST keep one ID across both paths. Issues: [#302](https://github.com/webwiebe/funnelbarn/issues/302). Tests: `internal/command/redisbus_test.go::TestRedisBusPublishFailureUsesFallback`, `internal/command/redisbus_test.go::TestRedisBusCooldownSkipsRedis`, `internal/command/redisbus_test.go::TestRedisBusAmbiguousPublishKeepsOneEvaluationID`.
- **FR-017**: At start-up the consumer MUST requeue what its processing list holds, and a message that fails to decode MUST be acked and skipped. Issues: [#302](https://github.com/webwiebe/funnelbarn/issues/302). Tests: `internal/command/redisbus_test.go::TestRedisBusRecoverReappliesUnacked`, `internal/command/redisbus_test.go::TestRedisBusGarbagePayloadIsAckedAndSkipped`.
- **FR-018**: Commands that cannot reach Redis MUST go to a file fallback that survives a restart, forwards after the outage and stops at a 64 MiB cap. Issues: [#302](https://github.com/webwiebe/funnelbarn/issues/302). Tests: `internal/command/spoolfallback_test.go::TestSpoolFallbackForwardsAfterAnOutage`, `internal/command/spoolfallback_test.go::TestSpoolFallbackSurvivesARestart`, `internal/command/spoolfallback_test.go::TestSpoolFallbackStopsAtItsCap`.
- **FR-019**: The reader worker MUST forward its spool in order, advance the cursor only after a publish, and keep records when a publish fails. Issues: [#302](https://github.com/webwiebe/funnelbarn/issues/302). Tests: `cmd/funnelbarn/spool_forwarder_test.go::TestForwardPublishesInOrderAndMovesCursor`, `cmd/funnelbarn/spool_forwarder_test.go::TestForwardKeepsRecordsInSpoolWhenPublishFails`.
- **FR-020**: Traffic sent to a reader while the writer is stopped MUST land exactly once after the writer returns, including after a Valkey outage and a crash between apply and ack. Issues: [#302](https://github.com/webwiebe/funnelbarn/issues/302). Tests: `cmd/funnelbarn/durability_test.go::TestReaderTrafficSurvivesAStoppedWriter`.
- **FR-021**: Each maintenance step MUST emit a child span under `maintenance.purge`. Issues: [#302](https://github.com/webwiebe/funnelbarn/issues/302). Tests: `cmd/funnelbarn/maintenance_test.go::TestRunMaintenance_EmitsRootAndStepSpans`.
- **FR-022**: The post-deploy check on testing MUST probe evaluate latency and look for a `maintenance.purge` span. Issues: [#307](https://github.com/webwiebe/funnelbarn/issues/307). Tests: `scripts/verify-testing.sh` (a script; no test file).
- **FR-023**: The evaluate path MUST hand the evaluation row and the `last_evaluated_at` touch to a bounded worker pool (`command.Async`) and wait only for a buffer slot; a full buffer blocks and drops nothing, and shutdown hands the queue to the bus before the bus drains. `EnsureAutoFlag` stays on the bus, so a new flag is published before its touch is queued. Issues: [#337](https://github.com/webwiebe/funnelbarn/issues/337). Tests: `internal/command/async_test.go::TestAsyncSubmitDoesNotWaitForTheInnerBus`, `internal/command/async_test.go::TestAsyncBackpressureBlocksAndNeverDrops`, `internal/command/async_test.go::TestAsyncFlushWaitsForEveryWorker`, `internal/command/async_test.go::TestAsyncCloseDrainsAndLeavesTheInnerBusOpen`, `internal/service/flags_async_internal_test.go::TestEvaluate_BookkeepingGoesToTheBackgroundBus`.
- **FR-024**: A failed Valkey publish or receive that a fallback absorbs MUST log at Warn when the run of failures starts and at Error once only when it has lasted `command.DefaultOutageAfter` (30s); recovery logs at Info with how long it lasted. The spool forwarder logs Warn only, because the spool stall check reports an outage that lasts. Shared Valkey stalls for a few seconds several times a day, and BugBarn opens an issue for every Error. Issues: [#346](https://github.com/webwiebe/funnelbarn/issues/346), [#347](https://github.com/webwiebe/funnelbarn/issues/347), [#384](https://github.com/webwiebe/funnelbarn/issues/384), [#389](https://github.com/webwiebe/funnelbarn/issues/389). Tests: `internal/command/outage_test.go::TestOutageLevels`, `internal/command/redisbus_test.go::TestRedisBusLastingPublishOutageLogsErrorOnce`, `internal/command/redisbus_faults_test.go::TestRedisBusReceiveErrorsBackOffAndRecover`, `cmd/funnelbarn/recordings_queue_test.go::TestRecordingsQueuePublishFailureLevels`, `cmd/funnelbarn/spool_forwarder_test.go::TestForwardPublishFailureLogsWarnOnce`.

---

## Problem Statement

FunnelBarn runs every database operation through one SQLite connection (`repository.Open`, `SetMaxOpenConns(1)`). Reads, writes, the `/api/v1/health` ping and the daily maintenance pass all queue for it. When one statement holds the connection for a few seconds, the whole process stalls.

On 2026-10-07 at 13:50 UTC in production, `PurgeOldEvents` (one unbatched `DELETE FROM events`) followed by `CountOrphanedRows` held the connection for about 11 seconds. Every `POST /api/v1/evaluate` in that window waited 4.3 to 6.5 s before its first DB span, although its own work took about 15 ms. Three BrandTrace callers hit their 5 s read timeout, and `/api/v1/health` took over a second.

The evaluate path makes this worse. It is a query (which variant does this context get?) that writes on every call: `TouchAPIKey` during auth, `RecordEvaluation`, `TouchFlagEvaluated`, `MarkProjectHealthFlagsEvaluated`, and `EnsureAutoFlag` for unknown keys. Each write needs the one connection.

There is no command/query separation: `repository.Querier` is one interface of about 100 methods mixing reads and writes, the per-aggregate ports in `internal/ports` mix both, and one process role does ingest, dashboard reads, flag evaluation, the worker and maintenance.

---

## Solution

Adopt the estate CQRS shape that BugBarn (specs 006 and 007) and SpanBarn (`SPANBARN_MODE`) already run: commands change state, queries return state, query adapters hold only a read-only connection, and one writer applies every command.

### Design Principles

1. **Queries cannot write.** A query adapter holds a connection opened with `mode=ro` and `PRAGMA query_only`. A write routed to it fails loudly, in tests and in production.
2. **Requests that read do not wait for writes.** Bookkeeping writes caused by a query (evaluation rows, touches, health marks, auto-registration) become commands handed to an asynchronous dispatcher.
3. **One writer.** All writes go through one write connection (`MaxOpenConns(1)`), which serialises every statement. Writes that must happen together use a transaction, or run on the single command consumer (the `EnsureAutoFlag` cap check). A separate write mutex would only duplicate the connection pool's queue, so there is none.
4. **No long write transactions.** Maintenance deletes in bounded batches and gives the write connection back between batches.
5. **Unset means today.** With `FUNNELBARN_MODE` and `FUNNELBARN_REDIS_QUEUE_URL` unset, FunnelBarn runs as one standalone process with an in-process dispatcher. That is also the rollback path.

---

## Phase 1: logical split in one process

### Two pools

`repository.Open` opens:

| Pool | DSN | Size | Used by |
|---|---|---|---|
| write | `<path>?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)` (unchanged) | 1 | command adapters, migrations, the dispatcher consumer, worker, maintenance deletes |
| read | `file:<abs path>?mode=ro&_pragma=query_only(1)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)` | 4 | query adapters, `/api/v1/health`, `CountOrphanedRows` |

WAL is on, so readers see the last committed state and do not block on a running write. An in-memory database (`:memory:`) cannot be shared between pools, so there the read pool aliases the write pool. Test suites use temp-file databases so the read-only pool is real in tests.

Each pool reports `otelsql` connection-pool metrics with a `db.pool` attribute (`read` or `write`).

### Async command dispatcher

`internal/command` holds typed commands and a dispatcher:

- `RecordEvaluation`, `TouchAPIKey`, `TouchFlagEvaluated`, `MarkFlagsEvaluated`, `EnsureAutoFlag`.
- Phase 1: a buffered channel drained by one goroutine on the write pool. The queue depth is exported as a gauge.
- A full buffer applies backpressure: submitting blocks until there is room, independent of the request context, so a caller that disconnects does not lose its row. Commands are never dropped, so A/B exposure rows stay complete. The time a submit waited is recorded on the request span, and an evaluate whose total time, or whose wait to submit, crosses a threshold logs at Error level, which opens a BugBarn issue. Dropping bookkeeping under pressure is deferred until that measurement shows evaluate actually slowing down.
- `Flush(ctx)` waits until every submitted command is applied; tests use it, and shutdown drains with a timeout before the store closes.
- Phase 2 replaces the channel with Redis/Valkey without touching the handlers.

`EvaluateOrRegisterFlag` for an unknown key checks the auto-flag cap on the read pool, submits `EnsureAutoFlag`, and returns the caller's default with reason `DISABLED`, which is what the endpoint returns for a freshly auto-registered flag today. Concurrent first evaluations of different new keys can all pass that read, so `EnsureAutoFlag` checks the cap again when the consumer applies it and skips a new key past the cap. It is idempotent per key.

### Maintenance

`PurgeOldEvents` and `PurgeOldEvaluations` delete in batches of 500 (`DELETE ... WHERE rowid IN (SELECT rowid ... LIMIT 500)`) and check the context between batches. `CountOrphanedRows` runs on the read pool. Each step gets a child span under `maintenance.purge`.

### Ports

`repository.Querier` is removed. Each aggregate has one query port and one command port (`FlagQueries` / `FlagCommands`, `EventQueries` / `EventCommands`, ...). `ports.XRepo` stays as the composition of the two, so a service that both reads and writes takes one value; it declares no methods of its own. `repository.ReadStore` holds only the read pool and implements every query port; `repository.Store` embeds it and adds the write pool and the command methods. A query adapter therefore has no write handle to misuse.

Enforcement (blocking in CI):

- `internal/ports/assert_test.go` asserts at compile time that `*repository.ReadStore` implements every query port, so a write method added to a query port, or a query method left on `*Store`, fails the build.
- `internal/archtest`: `internal/ports` does not import `internal/command`; `internal/api`, `internal/service` and `internal/mcp` do not import the generated SQL (`internal/repository/sqlcgen`). The domain types still live in `internal/repository`, so `internal/api` imports that package for types; moving them out is not part of this split.

---

## Phase 2: process split and a Redis/Valkey command bus

### Modes

`FUNNELBARN_MODE`:

| Mode | Replicas | Database | Does |
|---|---|---|---|
| `standalone` (default) | 1 | read + write | everything, as phase 1 |
| `reader` | N, rolling updates | read-only `ReadStore`, no write handle | serves queries, ingest and evaluate; publishes async commands to Redis; forwards synchronous dashboard mutations to the writer over internal HTTP |
| `writer` | 1, Recreate | the only write connection | drains the queues, runs the spool worker, retention and maintenance, serves forwarded mutations |

### Two kinds of command

- **Async, high volume, no response needed**: event ingest, recording chunks, session upserts and signals, `RecordEvaluation`, `TouchAPIKey`, `TouchFlagEvaluated`, `MarkProjectHealth*`, `EnsureAutoFlag`. These go through the queue.
- **Sync, low volume, response needed**: dashboard CRUD for projects, funnels, segments, flags, A/B tests, widgets, API keys and settings. Readers forward these to the writer over HTTP, as BugBarn's `WriteForwarder` does.

### Queues

One Redis list per kind (`ingest`, `recordings`, `bookkeeping`), so an ingest backlog does not delay evaluation bookkeeping and the other way round. The envelope follows BugBarn's `QueueItem`: `{kind, project_id, received_at, payload}`.

Delivery is at-least-once: the consumer moves an item to a per-queue processing list with `BLMOVE`, applies it, then removes it; on startup the consumer requeues anything left in its processing list. Every consumer is idempotent: events dedupe on `ingest_id`, evaluation rows carry an id inserted with `INSERT OR IGNORE`, touches and health marks are idempotent by nature.

The `bookkeeping` queue shipped first, with a standalone consume and an in-process fallback when a publish fails; the `ingest` and `recordings` queues followed with the reader and writer modes.

After a failed publish, commands go straight to the in-process fallback for a cooldown (5 s), so an outage that times out instead of refusing connections costs one publish timeout, and evaluate does not wait two seconds on every call. The evaluation id is fixed before the publish, so a push that lands while its reply times out stores one row on both paths.

A publish to the shared Valkey is synchronous and took 60 to 200+ ms at times in production (#337, FUN-48), while the evaluation itself takes under a millisecond. The evaluation row and the `last_evaluated_at` touch therefore go through `command.Async`: four workers publish from a 1024-slot buffer, and the request waits only for the hand-off. Neither command depends on the order of others, so the workers may publish them out of order. `EnsureAutoFlag` is published on the request, before the new flag's touch is queued. The API key touch and the health mark were moved off the request path earlier (#353).

Exactly one process consumes a queue: `Recover` requeues the whole processing list at startup, so a second live consumer would re-apply the first one's in-flight item. Standalone runs one replica, and the writer Deployment uses `Recreate`. A command whose apply fails is logged at Warn and acked, as in phase 1: the write connection has a 5 s busy timeout, and a retrying consumer would hold up every command behind a bad one.

### Durability while the writer or Redis is down

Readers keep writing ingest to the existing on-disk spool and advance its cursor only after a successful `LPUSH`. With the writer down, items wait in Redis and land when it is back. With Redis down, the spool backs up and drains later. Bookkeeping commands and recording chunk metadata that cannot reach Redis go to files next to the reader's spool (`bookkeeping-fallback.ndjson`, `recordings-fallback.ndjson`, each with a cursor file). The reader's worker publishes them once a second and advances the cursor after each published batch, so an outage delays them and loses none. Each file is capped at 64 MiB; past the cap a command is dropped with an Error log. A reader has no write connection, so it never applies a command itself.

Project health marks (`setup_called`, `events_received`, `recordings_received`) travel as one `mark_project_health` command naming the field.

### Forwarding to the writer

A reader with `FUNNELBARN_WRITER_URL` set hands every request that writes the database to the writer over HTTP and copies the answer back:

- every non-GET request except `POST /api/v1/events`, `/recordings/chunk` and `/evaluate`, which publish to the queues, and `/telemetry` and `/client-errors`, which write nothing;
- `GET /api/v1/oidc/callback` (creates a session), `GET /api/v1/auth/oidc/logged-out` (deletes one) and `GET /api/v1/setup/{slug}` (creates a pending project and upserts its ingest key);
- the MCP endpoint in every method, because its Streamable HTTP sessions live in one process and its setup tool writes;
- any session-authenticated request whose session row needs a write: past its absolute expiry (the row is deleted) or due for an OIDC refresh (the rotated tokens are stored). A reader that refreshed itself would burn the single-use refresh token and then fail to store the new one.

The reader sends the client address, scheme and host it resolved as `X-Forwarded-For`, `-Proto` and `-Host`, plus `X-Funnelbarn-Internal-Token` set to `FUNNELBARN_INTERNAL_TOKEN`. The writer believes those headers only when the token matches, so its rate limiters and Secure cookies see the real client. Redirects are passed back to the browser unfollowed, and the reader keeps its own CORS headers. A writer that cannot be reached answers 502. Reader and writer modes both refuse to start without the token, and a reader refuses to start without the writer URL.

`TestGoldenReaderMode` runs every golden case against a reader on a read-only store in front of a writer and compares with the same files as the standalone run. Any write left on a reader path fails it with a read-only database error.

A reader serves `GET /api/v1/ready` with 503 until the database's goose version reaches the binary's newest migration. During a rollout the new readers wait for the new writer to migrate; `/api/v1/health` stays a liveness check.

### How readers see the database

Reader and writer pods mount the same volume and open the same SQLite file in WAL mode, the readers through the read-only pool. A `nodeSelector` on the same hostname keeps them on the writer's node, which is required for a `ReadWriteOnce` volume and for SQLite's shared-memory WAL index. Readers see each commit as soon as it is made, so a flag edit in the dashboard reaches `/api/v1/evaluate` with no lag beyond the flag's `cache_max_age_seconds`.

BugBarn's readers restore a Litestream copy instead, which adds replication lag and puts an object-store dependency on the read path. FunnelBarn does not do that. The cost of the shared volume is that readers cannot spread across nodes; every FunnelBarn environment runs on a single node today.

Async commands are eventually consistent: an evaluation row, a touch or an ingested event becomes visible once the writer has applied it, normally within a second. The dashboard already treats these as eventually consistent through the spool.

### Deployment

- A shared Valkey per environment, run from k3s-infra (`valkey-primary.shared-<env>.svc`), with a `funnelbarn` ACL user.
- `FUNNELBARN_REDIS_QUEUE_URL` in each environment's SOPS secret. Empty means standalone.
- A writer Deployment (`Recreate`, one replica) and a reader StatefulSet; the ingress points at the reader Service.

---

## Verification

- `make regress` (blocking in CI): golden API snapshots of every GET route and of flag evaluation, a golden dump of the bookkeeping tables after a scripted scenario, and the architecture rules. The goldens were recorded on the code before this split, so every later change must reproduce them.
- A contention gate holds the only write connection and sends evaluate requests, including one for an unknown key; every request must complete, and every queued row must land once the connection is released. It fails on any write or read the evaluate path still does on the write pool, and does not depend on machine speed.
- A latency test runs the maintenance purge on a large database while evaluate requests go in, and asserts p99 under 100 ms. It is opt-in (`FUNNELBARN_CONTENTION_TEST=1`) because a wall-clock bound flakes on the shared CI runners.
- In phase 2 the same goldens run against standalone and against an in-process reader and writer pair, and a durability test stops the writer, sends traffic to a reader, restarts the writer and checks that every row landed exactly once. Both are part of `make regress`. The durability test (`TestReaderTrafficSurvivesAStoppedWriter`) covers events, recording chunks, evaluations and health marks, with a Valkey outage while the writer is down. It then puts every message back on the processing lists, as a writer that crashed between apply and ack leaves them, and checks that a second writer adds no rows.
- After each deploy to testing, `scripts/verify-testing.sh` probes evaluate latency through the post-boot maintenance pass and checks that a `maintenance.purge` span reached SpanBarn.

---

## Open Questions

- [#337](https://github.com/webwiebe/funnelbarn/issues/337) Evaluate requests wait up to 740 ms on Valkey publishes in production.

## Drift

| Claim | Source | Tree |
|---|---|---|
| Status is in progress | Original header | Issue #302 is closed; both phases are in the tree and run in testing, staging and production |
| Valkey runs as a Deployment and Service with AOF per environment | Deployment section | Valkey is the shared instance from k3s-infra (`valkey-primary.shared-<env>.svc`); no manifest in `deploy/k8s` creates it (`deploy/SECRETS.md`). The AOF setting is not established from this repo |
| Reader and writer Deployments | Deployment section | The writer is a Deployment with `Recreate`; readers are a StatefulSet with 2 replicas in each environment (`deploy/k8s/*/reader-statefulset.yaml`) |
| Pod affinity keeps readers on the writer's node | How readers see the database | Both manifests use `nodeSelector` on `kubernetes.io/hostname` |
| Reader and writer modes refuse to start without the token | Forwarding section | They also refuse to start without `FUNNELBARN_REDIS_QUEUE_URL` (`internal/config/mode_test.go::TestValidateMode`) |
| Requests that read do not wait for writes | Design principle 2 | Evaluate waits for a buffer slot only (FR-023); `EnsureAutoFlag` for an unknown key still publishes on the request. Production latency after the change is measured on issue #337 |
| Evaluate logs at error level when slow | Async command dispatcher | `internal/api/flags_slow.go` logs `flag evaluate slow` at error level; issue #362 tracks the resulting BugBarn reports |
| The latency test is opt-in through `FUNNELBARN_CONTENTION_TEST=1` | Verification | Holds (`internal/api/contention_test.go`) |

## Proof contract

- [ ] A write sent to a read-only store fails (`internal/repository/readonly_test.go::TestOpenReadOnlyRefusesWrites`).
- [ ] `make regress` passes: `TestGoldenReaderMode` returns the same snapshots in reader mode as in standalone mode.
- [ ] With the writer stopped, reader traffic lands exactly once after the writer returns (`cmd/funnelbarn/durability_test.go::TestReaderTrafficSurvivesAStoppedWriter`).
- [ ] After a deploy to testing, `scripts/verify-testing.sh` exits 0 and finds a `maintenance.purge` span in SpanBarn.

## Sources

- `internal/command/`, `internal/ports/`, `internal/archtest/`, `internal/repository/db.go`, `internal/config/`, `internal/api/forwarder.go`, `cmd/funnelbarn/`
- `deploy/k8s/*/deployment.yaml`, `deploy/k8s/*/reader-statefulset.yaml`, `deploy/SECRETS.md`, `deploy/RUNBOOK.md`, `scripts/verify-testing.sh`
- Issue titles from the repository issue list: #302, #307, #337, #346, #362
