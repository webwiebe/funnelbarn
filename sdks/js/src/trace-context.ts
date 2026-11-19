// --------------------------------------------------------------------------
// W3C trace context helpers
// --------------------------------------------------------------------------

/**
 * parseTraceparent validates a W3C `traceparent` header and extracts the
 * trace + span ids. Returns null for malformed or all-zero (invalid) ids.
 * Format: `00-<32 hex trace-id>-<16 hex span-id>-<2 hex flags>`.
 */
export function parseTraceparent(value: string): { traceId: string; spanId: string } | null {
  if (!value) return null;
  const m = /^00-([0-9a-f]{32})-([0-9a-f]{16})-[0-9a-f]{2}$/i.exec(value.trim());
  if (!m) return null;
  const traceId = m[1].toLowerCase();
  const spanId = m[2].toLowerCase();
  if (traceId === '0'.repeat(32) || spanId === '0'.repeat(16)) return null;
  return { traceId, spanId };
}

/** buildTraceparent assembles a sampled (flags=01) W3C traceparent header. */
export function buildTraceparent(traceId: string, spanId: string): string {
  return `00-${traceId}-${spanId}-01`;
}

/** randomHex returns `bytes` cryptographically-random bytes as a lowercase hex string. */
export function randomHex(bytes: number): string {
  const arr = new Uint8Array(bytes);
  if (typeof crypto !== 'undefined' && crypto.getRandomValues) {
    crypto.getRandomValues(arr);
  } else {
    for (let i = 0; i < arr.length; i++) arr[i] = Math.floor(Math.random() * 256);
  }
  return Array.from(arr).map((b) => b.toString(16).padStart(2, '0')).join('');
}
