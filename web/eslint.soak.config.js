// Soak config: the real eslint config with cyclomatic complexity tightened from
// the pinned-at-current-worst 55 down to the estate target of 12.
//
// Nothing in CI lints with this config directly. It is read by
// scripts/quality-soak.py (dimension: web-complexity), which counts the
// violations per file and compares them against
// scripts/soak-baselines/web-complexity.txt. The soak does not block on the
// violations that already exist; it blocks on a new one, on a file that got
// worse, and on a baseline that has become beatable.
//
// max-lines is turned off here because file length is measured across the whole
// repo (Go, web and sdks) by the file-length soak instead, and the SDK is
// outside eslint's reach.
//
// Exit criterion: when scripts/soak-baselines/web-complexity.txt is empty, move
// complexity: ['error', 12] into eslint.config.js and delete this file.
import base from './eslint.config.js'

export default [
  ...base,
  {
    files: ['**/*.{ts,tsx}'],
    rules: {
      complexity: ['error', 12],
      'max-lines': 'off',
    },
  },
]
