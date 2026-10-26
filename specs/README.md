# Specs

One spec per product area. Each spec states the behaviour FunnelBarn ships, links every functional requirement to the issue that asked for it and the test that proves it, and records in its `## Drift` section where a document or issue disagrees with the code. The code is the fact.

The principles every spec answers to are in the [constitution](../.specify/memory/constitution.md). The templates are in [`.specify/templates/`](../.specify/templates/).

All specs were last verified against the tree on 2026-10-17 at `ea1b0e6`.

## Index

| Spec | Status | What it covers |
|---|---|---|
| [001 Analytics and funnels](./001-analytics-funnel/spec.md) | Partial | Ingest, spool and worker, session fingerprinting, enrichment, dashboard statistics, funnels, login and API keys, SDKs and distribution. Also holds the [plan](./001-analytics-funnel/plan.md), [data model](./001-analytics-funnel/data-model.md), [tasks](./001-analytics-funnel/tasks.md) and the [ingest contract](./001-analytics-funnel/contracts/ingest-api.yaml). |
| [002 UI overhaul](./002-ui-overhaul/spec.md) | Partial | The React dashboard: landing page, login, shell, dashboard, live page, funnel pages and settings. Also holds the [design system](./002-ui-overhaul/design-system.md) and [tasks](./002-ui-overhaul/tasks.md). A/B test pages, the setup wizard, export and share links are not built. |
| [003 Engineering standards](./003-engineering-standards/spec.md) | Partial | Architecture rules enforced by `archtest`, the CI quality gates and ratchets, the testing deploy chain. |
| [004 Panic to error](./004-panic-to-error/spec.md) | Shipped | UUID generation returns its error; the worker, ingest and repository create calls handle it. |
| [005 Data retention](./005-data-retention/spec.md) | Shipped | Event and evaluation purge, the maintenance pass, the migration version check. |
| [006 Frontend tests](./006-frontend-tests/spec.md) | Shipped | Vitest with jsdom, coverage floors, per-file gate. |
| [007 Infra and ops](./007-infra-ops/spec.md) | Shipped | Kubernetes resource limits and probes per environment, and the operations runbook. |
| [008 Prometheus metrics](./008-prometheus-metrics/spec.md) | Partial | The `/metrics` endpoint and its instruments; no test reads the endpoint. |
| [009 Load testing](./009-load-testing/spec.md) | Shipped | The k6 scripts in `loadtest/` and the manual runs. |
| 010 | Retired | The load-test results moved to [docs/load-tests/](../docs/load-tests/2026-04-30-results.md). The number is not reused. |
| [011 Session recording](./011-session-recording/spec.md) | Partial | rrweb capture, chunk ingest, storage, replay in the dashboard, retention, project settings. |
| [012 CQRS read and write split](./012-cqrs-read-write-split/spec.md) | Shipped | Writer, readers and the Valkey queues; command bus, forwarder, readiness gate, modes. |
| [013 Security middleware](./013-security-middleware/spec.md) | Shipped | Security headers, CORS, rate limiting, request logging. Renumbered from a second 003 on 2026-10-17. |

Areas with code and no spec yet: feature flags, A/B tests (API only), the MCP server, agents, canonical overview funnels, branded ingest hosts.

## Workflow

1. **A feature starts as a spec.** Before code, write a new `specs/NNN-<feature>/spec.md` from the spec template, or change the spec of the area it belongs to. The issue links to that spec. The issue's "What" and "Why" become the Purpose, and its acceptance criteria become FR lines.
2. **Refinement fills the plan and the tasks.** Implementation details go into `plan.md`, including the Constitution Check. The checklist goes into `tasks.md`, one task per line, each naming the FR it serves. Small changes can skip `plan.md` and `tasks.md` when the issue already holds them.
3. **A PR that changes behaviour updates its spec.** The same PR adds or edits the FR lines, links the new tests, moves resolved items out of Open Questions, and adds or clears Drift rows.
4. **Unbacked statements are marked.** A requirement with no issue and no test carries `[inferred]` until one of them exists. A requirement the code does not meet carries `[gap]` and an issue.
5. **Drift is recorded, then fixed.** When a document and the code disagree, add a Drift row and treat the code as the fact. Fix the document, or open an issue when the code is the thing that is wrong.

## Related documents

- [docs/load-tests/](../docs/load-tests/2026-04-30-results.md): load-test results
- [deploy/RUNBOOK.md](../deploy/RUNBOOK.md): operating the deployments
- [AGENTS.md](../AGENTS.md) and [CLAUDE.md](../CLAUDE.md): working rules for this repository
