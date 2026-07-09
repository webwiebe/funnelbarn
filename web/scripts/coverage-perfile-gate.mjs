#!/usr/bin/env node
// Frontend per-file coverage ratchet: fail if any src file that is NOT in the
// grandfathered baseline has 0% line coverage. This blocks NEW untested files
// from being added (they can't hide behind coverage added elsewhere), while the
// existing 0% debt in coverage-uncovered-baseline.txt is tracked to burn down.
//
// Run AFTER `vitest run --coverage` (needs coverage/coverage-summary.json).
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, resolve } from 'node:path'

const webDir = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const summaryPath = resolve(webDir, 'coverage/coverage-summary.json')
const baselinePath = resolve(webDir, 'coverage-uncovered-baseline.txt')

let summary
try {
  summary = JSON.parse(readFileSync(summaryPath, 'utf8'))
} catch {
  console.error(`ERROR: ${summaryPath} not found. Run "npm run test:coverage" first.`)
  process.exit(1)
}

const grandfathered = new Set(
  readFileSync(baselinePath, 'utf8')
    .split('\n')
    .map((l) => l.trim())
    .filter((l) => l && !l.startsWith('#')),
)

const rel = (p) => p.replace(webDir + '/', '')
const zeroFiles = []
const recoveredBaselineFiles = []

for (const [file, cov] of Object.entries(summary)) {
  if (file === 'total') continue
  const r = rel(file)
  const isZero = cov.lines.pct === 0
  if (isZero && !grandfathered.has(r)) zeroFiles.push(r)
  if (!isZero && grandfathered.has(r)) recoveredBaselineFiles.push(r)
}

if (recoveredBaselineFiles.length) {
  console.log(
    `note: ${recoveredBaselineFiles.length} grandfathered file(s) now have coverage — ` +
      `remove them from coverage-uncovered-baseline.txt to tighten the gate:`,
  )
  recoveredBaselineFiles.forEach((f) => console.log(`  ${f}`))
}

if (zeroFiles.length) {
  console.error(`\nFAIL: ${zeroFiles.length} NEW file(s) have 0% coverage (add tests):`)
  zeroFiles.forEach((f) => console.error(`  ${f}`))
  process.exit(1)
}

console.log('per-file coverage gate passed (no new uncovered files).')
