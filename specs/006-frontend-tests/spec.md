# Spec 006: Frontend Unit Tests

## Goal
The web/ directory has zero unit tests. Add Vitest + React Testing Library and write tests for the most critical logic: the API client, auth context, and key components.

## Files to modify / create
- `web/package.json` — add devDependencies and test script
- `web/vite.config.ts` — add Vitest config block
- `web/src/setupTests.ts` (new) — global test setup
- `web/src/lib/api.test.ts` (new) — API client tests
- `web/src/lib/auth.test.tsx` (new) — auth context tests
- `web/src/components/` — test files alongside components (at least one component)

## Dependencies to add
```json
"devDependencies": {
  "vitest": "^1.6.0",
  "@vitest/ui": "^1.6.0",
  "@testing-library/react": "^16.0.0",
  "@testing-library/user-event": "^14.5.0",
  "@testing-library/jest-dom": "^6.4.0",
  "jsdom": "^24.0.0",
  "happy-dom": "^14.0.0"
}
```

## vite.config.ts — add test block
```ts
/// <reference types="vitest" />
// Inside defineConfig:
test: {
  globals: true,
  environment: 'happy-dom',
  setupFiles: ['./src/setupTests.ts'],
}
```

## package.json scripts
```json
"test": "vitest run",
"test:watch": "vitest",
"test:ui": "vitest --ui"
```

## Test coverage targets

### web/src/lib/api.test.ts
Test the API client functions (fetch wrappers). Mock `globalThis.fetch`.
- Test that requests include correct headers
- Test error response handling (4xx, 5xx)
- Test JSON parsing

### web/src/lib/auth.test.tsx
Test the AuthContext:
- `useAuth()` outside provider throws or returns null
- `login()` calls correct API endpoint
- `logout()` clears state

### web/src/components/ — pick 1-2 components
Look at what's in `web/src/components/` and write a smoke test for the simplest one (renders without crashing, key props show up in DOM).

## Acceptance criteria
- `npm test` in web/ runs and passes
- At least 8 test cases total across the files
- No snapshots — prefer explicit assertions
- CI: add `cd web && npm test` step to `.github/workflows/build-and-test.yml` in the test job
