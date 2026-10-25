// The server rejects a recording chunk over 10 MiB with 413 (#349), and fetch
// resolves on a 413, so the events in it were gone. A flush splits its events
// into requests that stay under this budget, which leaves room for the rest of
// the body. One event over the budget still goes alone.
export const RECORDING_CHUNK_BUDGET_BYTES = 8 * 1024 * 1024;

// utf8Length is the UTF-8 size of s, which is what the server counts.
function utf8Length(s: string): number {
  let n = 0;
  for (let i = 0; i < s.length; i++) {
    const c = s.charCodeAt(i);
    if (c < 0x80) n += 1;
    else if (c < 0x800) n += 2;
    else if (c >= 0xd800 && c <= 0xdbff) { n += 4; i++; }
    else n += 3;
  }
  return n;
}

// splitByBudget groups events in order so each group's JSON stays within
// budget bytes. A group always holds at least one event.
export function splitByBudget(events: unknown[], budget: number): unknown[][] {
  const groups: unknown[][] = [];
  let group: unknown[] = [];
  let size = 0;
  for (const event of events) {
    const n = utf8Length(JSON.stringify(event)) + 1; // the comma
    if (group.length && size + n > budget) {
      groups.push(group);
      group = [];
      size = 0;
    }
    group.push(event);
    size += n;
  }
  if (group.length) groups.push(group);
  return groups;
}

// sendInOrder sends the groups one by one and stops at the first that throws.
// It returns how many went out.
export async function sendInOrder(
  groups: unknown[][],
  send: (events: unknown[], g: number) => Promise<unknown>,
): Promise<number> {
  for (let g = 0; g < groups.length; g++) {
    try {
      await send(groups[g], g);
    } catch {
      return g;
    }
  }
  return groups.length;
}
