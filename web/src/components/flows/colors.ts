export const C = {
  bg: '#0f1117',
  surface: '#1a1d27',
  border: '#2a2d3a',
  amber: '#f59e0b',
  amberDim: 'rgba(245,158,11,0.15)',
  text: '#e2e8f0',
  muted: '#94a3b8',
  blue: '#3b82f6',
  green: '#10b981',
  red: '#ef4444',
  indigo: '#6366f1',
} as const

export function nodeColor(type: string, isFocused: boolean): string {
  if (isFocused) return C.amber
  if (type === 'referrer') return C.blue
  if (type === 'exit') return C.red
  return C.indigo
}

export function shortLabel(label: string, maxLen = 28): string {
  if (label.length <= maxLen) return label
  return '…' + label.slice(-(maxLen - 1))
}
