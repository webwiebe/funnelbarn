# Engineering Standards: Architecture Patterns & Code Quality

**Spec**: `003-engineering-standards`
**Created**: 2026-04-30
**Status**: In Progress

---

## Goals

Establish consistent, enforced engineering standards across the FunnelBarn codebase so that:

- Code is easy to navigate, test, and extend without understanding the full system
- Responsibilities are clearly separated and no single file mixes concerns
- Tests catch regressions before they reach production
- CI/CD enforces quality mechanically, not by convention

---

## Backend Architecture (Go)

### Layered Structure

All request handling follows a strict three-layer pipeline:

```
HTTP handler / CLI command / ingest event
        │
        ▼
  internal/service/
    Pure business logic. Coordinates between entities.
    Calls repository functions. Returns domain errors.
        │
        ▼
  internal/repository/
    Data access only. One file per entity/aggregate.
    Executes SQL. Returns typed structs or errors.
        │
        ▼
  SQLite (via sqlc-generated queries)
```

**Rules:**
- Handlers (`internal/api/`, `internal/ingest/`, `cmd/`) may only call services — never repositories directly.
- Services contain all business rules and validation.
- Repositories contain only data access: no business logic, no HTTP concepts.
- No circular imports between layers.

### Package Layout

```
internal/
  api/           # HTTP handlers only — parse request, call service, write response
  ingest/        # Event ingest handler — same rules as api/
  service/       # Business logic
    projects.go
    events.go
    funnels.go
    abtests.go
    sessions.go
    auth.go
  repository/    # Data access (replaces storage/)
    projects.go
    events.go
    funnels.go
    abtests.go
    sessions.go
    apikeys.go
    users.go
  db/
    migrations/  # Versioned goose migration files (*.sql)
    queries/     # sqlc input query files (*.sql)
    sqlc.yaml    # sqlc config
    generated/   # sqlc output (committed, not edited by hand)
  config/
  enrich/
  session/
  spool/
  worker/
  auth/
  bblog/
```

### Migration Tooling

Use **goose** (`github.com/pressly/goose/v3`) for schema migrations:

- Migration files live in `internal/db/migrations/`
- Naming: `000001_initial_schema.sql`, `000002_add_sessions_index.sql`, etc.
- Migrations run automatically at server startup (before serving requests)
- `goose up` / `goose down` available via CLI subcommand
- The existing hardcoded DDL in `storage/schema.go` becomes the first migration file

### Query Layer

Use **sqlc** (`github.com/sqlc-dev/sqlc`) to generate type-safe Go from SQL:

- Query files in `internal/db/queries/*.sql`, one file per entity
- Generated code in `internal/db/generated/` (committed)
- Repository functions wrap generated code, returning domain types
- Repository files are thin: no logic beyond mapping generated structs to domain types

### Style

- Functional over OOP: prefer free functions over method receivers where possible. Use struct receivers only for the repository and service types that hold a DB handle.
- No global state outside of `main`.
- Errors are returned, never panicked (except truly unrecoverable startup failures).
- Each file has a single clear purpose. If a file grows past ~200 lines or handles two distinct concerns, split it.

### Unit Tests

- Every service function has a unit test in `internal/service/*_test.go`
- Every repository function has a unit test in `internal/repository/*_test.go` using an in-memory SQLite instance (`:memory:`)
- Test files sit alongside the code they test (Go standard)
- Use `testing` stdlib + `github.com/stretchr/testify` for assertions
- Table-driven tests for functions with multiple input variants
- No mocking of the database — use real SQLite in-memory instances for repository tests

### Integration Tests

- `internal/api/` handlers tested via `httptest.NewServer` + real in-memory DB
- Cover the full request-response cycle per endpoint
- Located in `internal/api/*_test.go`

---

## Frontend Architecture (React + TypeScript)

### Component Namespacing

Components are organized into namespaced directories. The directory name is the namespace; the filename is the component name. No flat `components/` dumping ground.

```
src/
  pages/            # Route-level page components (one per route)
    Dashboard.tsx
    Funnels.tsx
    ABTests.tsx
    Live.tsx
    Settings.tsx
    Login.tsx
    Landing.tsx
  components/
    shell/          # App shell, nav, layout chrome
      Shell.tsx
      NavLink.tsx
      ProjectSwitcher.tsx
    charts/         # Chart/visualization wrappers
      FunnelChart.tsx
      LiveChart.tsx
    ui/             # Generic reusable UI primitives (buttons, modals, etc.)
      Button.tsx
      Modal.tsx
      EmptyState.tsx
    wizards/        # Multi-step setup flows
      FirstRunWizard.tsx
  lib/
    api.ts          # HTTP client — one function per API endpoint
    auth.tsx        # Auth context + hook
    projects.tsx    # Project context + hook
    hooks/          # Custom hooks (one per concern)
```

**Rules:**
- Pages import from `components/` and `lib/`, never from other pages.
- Components in `ui/` must be completely generic (no domain knowledge).
- `lib/api.ts` is the only file that calls `fetch`. No ad-hoc API calls in components.
- Each component file exports exactly one component (the default export).
- No mixing of data-fetching and rendering in the same component; use a container/presenter split or React Query when relevant.

### Unit Tests

Use **Vitest** + **@testing-library/react** for all frontend tests:

- Test files sit alongside components: `Shell.test.tsx` next to `Shell.tsx`
- Test every non-trivial util function in `lib/`
- Test every component's key rendering states (loading, empty, populated, error)
- `npm test` runs all unit tests; `npm run test:coverage` enforces 80% line coverage

---

## CI/CD Quality Gates

Every push to any branch must pass all of the following before merging:

### Backend

| Check | Tool | Command |
|-------|------|---------|
| Formatting | `gofmt` | `gofmt -l .` (fail if output non-empty) |
| Linting | `golangci-lint` | `golangci-lint run ./...` |
| Vet | `go vet` | `go vet ./...` |
| Unit tests | `go test` | `go test ./... -race -count=1` |
| Test coverage | `go test` | Fail if coverage < 70% on `service/` and `repository/` packages |
| Migration check | `goose` | Verify all migrations apply cleanly on a fresh DB |
| sqlc check | `sqlc` | `sqlc vet` — fail if generated code is out of date |

### Frontend

| Check | Tool | Command |
|-------|------|---------|
| Type checking | `tsc` | `tsc --noEmit` |
| Linting | `eslint` | `eslint src/` |
| Unit tests | `vitest` | `vitest run` |
| Test coverage | `vitest` | Fail if coverage < 70% |
| Build | `vite` | `vite build` — fail on any build error |

### E2E

- Playwright tests run after deploy to the testing environment (existing behavior, preserved)
- E2E failures block promotion to staging

### Pipeline Structure

```
push/PR
  ├── backend-quality  (format, lint, vet, unit tests, coverage, migration check)
  ├── frontend-quality (type-check, lint, unit tests, coverage, build)
  └── [on success of both] → build → deploy-testing → e2e → deploy-staging
```

All quality jobs run in parallel. Build and deploy only proceed when all quality jobs pass.

---

## Non-Goals

- This spec does not introduce a new database engine. SQLite remains.
- This spec does not change the ingest spool architecture.
- This spec does not add a REST framework or replace `net/http`.
- This spec does not change the deployment setup (K8s, Docker, litestream).
- Full 100% test coverage is not required; the thresholds above are minimums.
