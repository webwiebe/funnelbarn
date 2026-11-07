import { C } from './shared'

// ConfigCurrentValue shows what a config flag serves without a matching rule.
// A default variant the flag lacks is reported, never shown as a blank.
export function ConfigCurrentValue({ value, defaultVariant }: { value: unknown; defaultVariant: string }) {
  return (
    <div style={{
      background: C.bg, border: `1px solid ${C.border}`, borderRadius: 10,
      padding: '1rem 1.25rem', marginBottom: 16,
    }}>
      <div style={{ fontSize: 12, color: C.muted, fontWeight: 600, marginBottom: 6 }}>Current value</div>
      {value === undefined ? (
        <div data-testid="config-value-missing" style={{ color: C.error, fontSize: 14 }}>
          No value: the default variant &quot;{defaultVariant}&quot; does not exist on this flag. Edit the flag to set one.
        </div>
      ) : (
        <code style={{ fontSize: 20, color: C.amber, fontWeight: 700, overflowWrap: 'anywhere' }}>
          {JSON.stringify(value)}
        </code>
      )}
    </div>
  )
}
