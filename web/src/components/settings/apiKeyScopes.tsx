import { C } from '../../lib/theme'

// scopeTone colours a badge by how much the key can do: amber for
// send-or-read-only, green for anything that can change state. The point is
// that "this token can turn the outbound-email gate off" reads differently at
// a glance from "this token sends page views".
export function scopeTone(scope: string): 'read' | 'write' {
  return scope === 'full' || scope === 'flags:write' ? 'write' : 'read'
}

// scopeHint explains, in one line, what a key with this scope may do.
export function scopeHint(scope: string): string {
  switch (scope) {
    case 'ingest': return 'Send events. Cannot read or change anything.'
    case 'analytics:read': return "Read this project's events, funnels and conversion numbers. Cannot change anything."
    case 'flags:read': return "Read this project's flags. Cannot change them."
    case 'flags:write': return "Read and update this project's flags. Cannot create or delete them."
    case 'full': return 'Full API access to this project.'
    default: return 'Unknown scope — this key grants nothing.'
  }
}

export function ScopeBadge({ scope }: { scope: string }) {
  const write = scopeTone(scope) === 'write'
  return (
    <span title={scopeHint(scope)} style={{
      fontSize: 12,
      fontWeight: 600,
      background: write ? 'rgba(16,185,129,0.1)' : 'rgba(245,158,11,0.1)',
      color: write ? C.success : C.amber,
      border: `1px solid ${write ? 'rgba(16,185,129,0.3)' : 'rgba(245,158,11,0.3)'}`,
      borderRadius: 99,
      padding: '0.15rem 0.6rem',
      whiteSpace: 'nowrap',
    }}>
      {scope}
    </span>
  )
}

// formatLastUsed distinguishes a token something depends on from one that was
// issued and forgotten — the question you actually have before revoking.
export function formatLastUsed(iso?: string): string {
  if (!iso) return 'never'
  const d = new Date(iso)
  return isNaN(d.getTime()) ? 'never' : d.toLocaleString()
}
