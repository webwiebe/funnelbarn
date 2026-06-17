/**
 * FunnelBarn replay API client.
 *
 * Talks to the FunnelBarn HTTP API with an API key to (1) resolve a W3C
 * trace_id to the session recording that captured it and (2) fetch that
 * recording's rrweb chunks for local replay.
 */

export interface TraceLookup {
  trace_id: string;
  project_id: string;
  session_id: string;
  recording_id: string;
  occurred_at: string;
  offset_ms: number;
  url?: string;
  recording_started_at: string;
  first_chunk_index: number;
  last_chunk_index: number;
  chunk_count: number;
  duration_ms: number;
  environment?: string;
  page_url?: string;
}

/** A minimal fetch signature so the client is testable without a browser/global. */
export type FetchLike = (
  url: string,
  init?: { method?: string; headers?: Record<string, string> }
) => Promise<{ ok: boolean; status: number; json: () => Promise<unknown>; text: () => Promise<string> }>;

export interface ClientOptions {
  endpoint: string;
  apiKey: string;
  fetchImpl?: FetchLike;
}

export class ReplayClient {
  private readonly endpoint: string;
  private readonly apiKey: string;
  private readonly fetchImpl: FetchLike;

  constructor(opts: ClientOptions) {
    this.endpoint = opts.endpoint.replace(/\/$/, "");
    this.apiKey = opts.apiKey;
    const f = opts.fetchImpl ?? (globalThis.fetch as unknown as FetchLike | undefined);
    if (!f) {
      throw new Error("no fetch implementation available (Node 18+ or pass fetchImpl)");
    }
    this.fetchImpl = f;
  }

  private headers(): Record<string, string> {
    return { "x-funnelbarn-api-key": this.apiKey };
  }

  /** Resolve a trace_id to its recording. Throws with a clear message on 404/!ok. */
  async lookupTrace(traceId: string): Promise<TraceLookup> {
    const url = `${this.endpoint}/api/v1/traces/${encodeURIComponent(traceId)}`;
    const resp = await this.fetchImpl(url, { headers: this.headers() });
    if (resp.status === 404) {
      throw new Error(`trace ${traceId} not found (no recording captured it, or wrong project key)`);
    }
    if (!resp.ok) {
      throw new Error(`trace lookup failed: HTTP ${resp.status} ${await safeText(resp)}`);
    }
    return (await resp.json()) as TraceLookup;
  }

  /** Fetch a single recording chunk's rrweb event array. */
  async fetchChunk(recordingId: string, index: number): Promise<unknown[]> {
    const url = `${this.endpoint}/api/v1/recordings/${encodeURIComponent(recordingId)}/chunks/${index}`;
    const resp = await this.fetchImpl(url, { headers: this.headers() });
    if (!resp.ok) {
      throw new Error(`chunk ${index} fetch failed: HTTP ${resp.status} ${await safeText(resp)}`);
    }
    const data = (await resp.json()) as unknown;
    if (!Array.isArray(data)) {
      throw new Error(`chunk ${index} is not an event array`);
    }
    return data;
  }

  /**
   * Fetch every chunk in [first, last] and concatenate into one ordered rrweb
   * event stream. Missing chunks are skipped with a warning rather than aborting
   * — a single lost chunk still leaves a partially replayable recording.
   */
  async fetchAllEvents(lookup: TraceLookup, onWarn?: (msg: string) => void): Promise<unknown[]> {
    const events: unknown[] = [];
    for (let i = lookup.first_chunk_index; i <= lookup.last_chunk_index; i++) {
      try {
        const chunk = await this.fetchChunk(lookup.recording_id, i);
        events.push(...chunk);
      } catch (err) {
        onWarn?.(`skipping chunk ${i}: ${(err as Error).message}`);
      }
    }
    return events;
  }
}

async function safeText(resp: { text: () => Promise<string> }): Promise<string> {
  try {
    return (await resp.text()).slice(0, 200);
  } catch {
    return "";
  }
}
