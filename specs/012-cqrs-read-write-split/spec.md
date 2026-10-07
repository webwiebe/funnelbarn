# Feature Specification: CQRS Read/Write Split

**Spec**: `012-cqrs-read-write-split`
**Created**: 2026-10-07
**Status**: In progress (issue #302)

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

Phase 2 step 1 ships the `bookkeeping` queue with a standalone consume and an in-process fallback when a publish fails; the `ingest` and `recordings` queues arrive with the reader and writer modes.

After a failed publish, commands go straight to the in-process fallback for a cooldown (5 s), so an outage that times out instead of refusing connections costs one publish timeout, and evaluate does not wait two seconds on every call. The evaluation id is fixed before the publish, so a push that lands while its reply times out stores one row on both paths.

Exactly one process consumes a queue: `Recover` requeues the whole processing list at startup, so a second live consumer would re-apply the first one's in-flight item. Standalone runs one replica, and the writer Deployment uses `Recreate`. A command whose apply fails is logged at Warn and acked, as in phase 1: the write connection has a 5 s busy timeout, and a retrying consumer would hold up every command behind a bad one.

### Durability while the writer or Redis is down

Readers keep writing ingest to the existing on-disk spool and advance its cursor only after a successful `LPUSH`. With the writer down, items wait in Redis and land when it is back. With Redis down, the spool backs up and drains later. Bookkeeping commands that cannot reach Redis go to the reader's spool as well, so an outage delays them and loses none. As in phase 1, nothing is dropped until evaluate latency measurements call for it.

### How readers see the database

Reader and writer pods mount the same volume and open the same SQLite file in WAL mode, the readers through the read-only pool. Pod affinity keeps them on the writer's node, which is required for a `ReadWriteOnce` volume and for SQLite's shared-memory WAL index. Readers see each commit as soon as it is made, so a flag edit in the dashboard reaches `/api/v1/evaluate` with no lag beyond the flag's `cache_max_age_seconds`.

BugBarn's readers restore a Litestream copy instead, which adds replication lag and puts an object-store dependency on the read path. FunnelBarn does not do that. The cost of the shared volume is that readers cannot spread across nodes; every FunnelBarn environment runs on a single node today.

Async commands are eventually consistent: an evaluation row, a touch or an ingested event becomes visible once the writer has applied it, normally within a second. The dashboard already treats these as eventually consistent through the spool.

### Deployment

- Valkey Deployment and Service with AOF on, per environment.
- `FUNNELBARN_REDIS_QUEUE_URL` in each environment's SOPS secret. Empty means standalone.
- Reader and writer Deployments; the ingress points at the reader Service.

---

## Verification

- `make regress` (blocking in CI): golden API snapshots of every GET route and of flag evaluation, a golden dump of the bookkeeping tables after a scripted scenario, and the architecture rules. The goldens were recorded on the code before this split, so every later change must reproduce them.
- A contention gate holds the only write connection and sends evaluate requests, including one for an unknown key; every request must complete, and every queued row must land once the connection is released. It fails on any write or read the evaluate path still does on the write pool, and does not depend on machine speed.
- A latency test runs the maintenance purge on a large database while evaluate requests go in, and asserts p99 under 100 ms. It is opt-in (`FUNNELBARN_CONTENTION_TEST=1`) because a wall-clock bound flakes on the shared CI runners.
- In phase 2 the same goldens run against standalone and against an in-process reader and writer pair, and a durability test stops the writer, sends traffic to a reader, restarts the writer and checks that every row landed exactly once.
- After each deploy to testing, `scripts/verify-testing.sh` probes evaluate latency through the post-boot maintenance pass and checks that a `maintenance.purge` span reached SpanBarn.
