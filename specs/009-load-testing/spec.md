# Spec 009: Load Testing

**Spec**: `specs/009-load-testing/`
**Status**: Shipped
**Last verified against the tree**: 2026-10-17 at `ea1b0e6`
**Issues**: none found
**Principles touched**: II, V

## Purpose

`loadtest/` holds k6 scripts that check ingest throughput, rate limiting and funnel analysis under concurrent load. The scripts run by hand; no CI job runs them and no gate fails on a latency number. The 2026-04-30 run is recorded in [docs/load-tests/2026-04-30-results.md](../../docs/load-tests/2026-04-30-results.md).

## Functional Requirements

- **FR-001**: `loadtest/config.js` MUST export `BASE_URL`, `API_KEY` and `PROJECT_ID` from the `BASE_URL`, `API_KEY` and `PROJECT_ID` environment variables. Issues: none. [inferred]
- **FR-002**: `loadtest/scenarios/ingest.js` MUST post `page_view` events at a constant 200 per second for 30 seconds with thresholds of under 1 percent failures and p95 under 200 ms. Issues: none. [inferred]
- **FR-003**: `loadtest/scenarios/rate-limit.js` MUST send 30 login attempts from 10 virtual users and check that a share of them answer 429. Issues: none. [inferred]
- **FR-004**: `loadtest/scenarios/analysis.js` MUST request the funnel analysis endpoint with 20 virtual users for 30 seconds with p95 under 2000 ms. Issues: none. [inferred]
- **FR-005**: `loadtest/README.md` MUST cover prerequisites, the three scenarios, how to read results and a baseline table. Issues: none. [inferred]

## Drift

| Claim | Source | Tree |
|---|---|---|
| The ingest script sends `X-FunnelBarn-Project` and references `PROJECT_SLUG` | Original task | The script sends only `x-funnelbarn-api-key` and imports `PROJECT_ID` without using it |
| The README baseline table gets a row per release | Original task | The table holds only a placeholder row; results live in `docs/load-tests/2026-04-30-results.md` |
| The login test expects 429 after the burst | Original task | The default login limit is 20 per minute with burst 20 (`internal/config/config.go`); 30 iterations exceed it only when no earlier request used the bucket |
| Scripts are checked for syntax | Acceptance criteria | No CI step or test runs `k6` or parses `loadtest/` (not established whether a hook does) |
| The results file lives at `specs/010-load-test-results/results.md` | Previous index | Moved to `docs/load-tests/2026-04-30-results.md` on 2026-10-17; the number 010 is retired |

## Proof contract

- [ ] `k6 run loadtest/scenarios/ingest.js` against a local server exits 0 with the two thresholds met.
- [ ] `k6 run loadtest/scenarios/rate-limit.js` records 429 responses.
- [ ] `loadtest/README.md` has the six sections the original task listed.

## Sources

- `loadtest/config.js`, `loadtest/README.md`, `loadtest/scenarios/*.js`
- `docs/load-tests/2026-04-30-results.md`
