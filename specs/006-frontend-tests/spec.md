# Spec 006: Frontend Unit Tests

**Spec**: `specs/006-frontend-tests/`
**Status**: Shipped
**Last verified against the tree**: 2026-10-17 at `ea1b0e6`
**Issues**: [#196](https://github.com/webwiebe/funnelbarn/issues/196)
**Principles touched**: VII

## Purpose

The web app has unit tests that run under Vitest with React Testing Library, and CI fails when coverage drops below the pinned floors.

## Functional Requirements

- **FR-001**: The system MUST run the web unit tests with `pnpm test` (`vitest run`) in the jsdom environment with `web/src/test-setup.ts`. Issues: none. Tests: `web/src/lib/api.test.ts`.
- **FR-002**: The API client MUST have tests for request headers, error responses and JSON parsing. Issues: none. Tests: `web/src/lib/api.test.ts`, `web/src/lib/api-error.test.ts`.
- **FR-003**: The auth context MUST have tests for use outside the provider, loading the user on mount, `login()` and `logout()`. Issues: none. Tests: `web/src/lib/auth.test.tsx`.
- **FR-004**: The web tests MUST run in the `frontend-test` job of `.github/workflows/ci.yml` with coverage floors enforced by `web/vite.config.ts`. Issues: [#196](https://github.com/webwiebe/funnelbarn/issues/196). [inferred]
- **FR-005**: The per-file coverage gate MUST fail CI when a source file falls below its floor (`pnpm gate:coverage-perfile`). Issues: [#196](https://github.com/webwiebe/funnelbarn/issues/196). [inferred]

## Drift

| Claim | Source | Tree |
|---|---|---|
| The environment is `happy-dom` | Original task, vite.config.ts block | `web/vite.config.ts` sets `environment: 'jsdom'`; `happy-dom` is not a dependency |
| The setup file is `web/src/setupTests.ts` | Original task | The file is `web/src/test-setup.ts` |
| Vitest `^1.6.0`, jsdom `^24.0.0`, `@testing-library/user-event` | Original task | `web/package.json` has vitest `^4.1.5`, jsdom `^29.1.0`, and no `user-event` |
| `npm test` in `web/`, and a step `cd web && npm test` in `build-and-test.yml` | Original task | The repo is a pnpm workspace; tests run as `pnpm test:coverage` in the `frontend-test` job of `ci.yml` |
| At least 8 test cases | Acceptance criteria | 48 test files exist under `web/src` |
| Not stated | | Coverage floors are lines 51, statements 48, functions 42, branches 45 (`web/vite.config.ts`); the 80 percent target in spec 003 is not enforced |

## Proof contract

- [ ] `pnpm test:coverage` in `web/` passes and enforces the four floors (`web/vite.config.ts`).
- [ ] `pnpm gate:coverage-perfile` passes in the `frontend-test` job.
- [ ] `web/src/lib/auth.test.tsx` fails when `logout()` stops clearing the user.

## Sources

- `web/package.json`, `web/vite.config.ts`, `.github/workflows/ci.yml`
- `web/src/lib/api.test.ts`, `web/src/lib/auth.test.tsx`
