#!/usr/bin/env node
// Frontend per-file coverage ratchet: fail if ANY src file has 0% line coverage.
// This matches the Go bar (scripts/coverage-gate.py Rule 1): every first-party
// file must be exercised by at least one test — no exemptions. The grandfathered
// baseline that briefly carried the pre-existing 0% debt has been burned down to
// empty and removed; there is nothing left to exempt.
//
// Run AFTER `vitest run --coverage` (needs coverage/coverage-summary.json).
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, resolve } from 'node:path'

const webDir = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const summaryPath = resolve(webDir, 'coverage/coverage-summary.json')

let summary
try {
  summary = JSON.parse(readFileSync(summaryPath, 'utf8'))
} catch {
  console.error(`ERROR: ${summaryPath} not found. Run "npm run test:coverage" first.`)
  process.exit(1)
}

const rel = (p) => p.replace(webDir + '/', '')
const zeroFiles = []

for (const [file, cov] of Object.entries(summary)) {
  if (file === 'total') continue
  if (cov.lines.pct === 0) zeroFiles.push(rel(file))
}

if (zeroFiles.length) {
  console.error(`\nFAIL: ${zeroFiles.length} file(s) have 0% coverage (add tests):`)
  zeroFiles.forEach((f) => console.error(`  ${f}`))
  process.exit(1)
}

console.log('per-file coverage gate passed (every src file has >0% coverage).')
