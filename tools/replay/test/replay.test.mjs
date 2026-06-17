import { describe, it } from 'node:test';
import assert from 'node:assert/strict';
import {
  ReplayClient,
  assembleEvents,
  buildReplayHTML,
  parseCliArgs,
} from '../dist/index.js';

function jsonResp(status, body) {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: async () => body,
    text: async () => JSON.stringify(body),
  };
}

const LOOKUP = {
  trace_id: 't1',
  project_id: 'p1',
  session_id: 's1',
  recording_id: 'r1',
  occurred_at: '2026-01-01T00:00:04Z',
  offset_ms: 4000,
  recording_started_at: '2026-01-01T00:00:00Z',
  first_chunk_index: 0,
  last_chunk_index: 2,
  chunk_count: 3,
  duration_ms: 9000,
  page_url: 'https://shop.example/checkout',
};

describe('ReplayClient', () => {
  it('lookupTrace resolves a trace to its recording', async () => {
    const client = new ReplayClient({
      endpoint: 'https://fb.example/',
      apiKey: 'k',
      fetchImpl: async (url, init) => {
        assert.equal(url, 'https://fb.example/api/v1/traces/t1');
        assert.equal(init.headers['x-funnelbarn-api-key'], 'k');
        return jsonResp(200, LOOKUP);
      },
    });
    const got = await client.lookupTrace('t1');
    assert.equal(got.recording_id, 'r1');
    assert.equal(got.offset_ms, 4000);
  });

  it('lookupTrace throws a clear error on 404', async () => {
    const client = new ReplayClient({
      endpoint: 'https://fb.example',
      apiKey: 'k',
      fetchImpl: async () => jsonResp(404, { error: 'not found' }),
    });
    await assert.rejects(() => client.lookupTrace('missing'), /not found/);
  });

  it('fetchChunk rejects a non-array body', async () => {
    const client = new ReplayClient({
      endpoint: 'https://fb.example',
      apiKey: 'k',
      fetchImpl: async () => jsonResp(200, { not: 'an array' }),
    });
    await assert.rejects(() => client.fetchChunk('r1', 0), /not an event array/);
  });

  it('fetchAllEvents concatenates chunks and skips missing ones', async () => {
    const warnings = [];
    const client = new ReplayClient({
      endpoint: 'https://fb.example',
      apiKey: 'k',
      fetchImpl: async (url) => {
        if (url.endsWith('/chunks/0')) return jsonResp(200, [{ type: 2, timestamp: 1 }]);
        if (url.endsWith('/chunks/1')) return jsonResp(500, {}); // missing/broken
        if (url.endsWith('/chunks/2')) return jsonResp(200, [{ type: 3, timestamp: 2 }]);
        return jsonResp(404, {});
      },
    });
    const events = await client.fetchAllEvents(LOOKUP, (w) => warnings.push(w));
    assert.equal(events.length, 2);
    assert.equal(warnings.length, 1);
    assert.match(warnings[0], /chunk 1/);
  });
});

describe('assembleEvents', () => {
  it('orders by timestamp, detects snapshot, computes duration', () => {
    const res = assembleEvents([
      { type: 3, timestamp: 300 },
      { type: 2, timestamp: 100 },
      { type: 3, timestamp: 200 },
      { bogus: true }, // filtered out
    ]);
    assert.equal(res.events.length, 3);
    assert.equal(res.events[0].timestamp, 100);
    assert.equal(res.events[2].timestamp, 300);
    assert.equal(res.hasSnapshot, true);
    assert.equal(res.durationMs, 200);
  });

  it('reports no snapshot when type 2 is absent', () => {
    const res = assembleEvents([{ type: 3, timestamp: 1 }]);
    assert.equal(res.hasSnapshot, false);
  });
});

describe('buildReplayHTML', () => {
  it('inlines events, seek, and an escaped banner', () => {
    const html = buildReplayHTML({
      events: [{ type: 2, timestamp: 100 }],
      seekMs: 4000,
      playerJs: 'window.rrwebPlayer = function(){};',
      playerCss: '.x{}',
      banner: 'trace <b>t1</b>',
    });
    assert.match(html, /var seekMs = 4000;/);
    assert.match(html, /"type":2/);
    assert.match(html, /trace &lt;b&gt;t1&lt;\/b&gt;/); // banner HTML-escaped
    assert.match(html, /window\.rrwebPlayer/);
  });
});

describe('parseCliArgs', () => {
  it('throws when required args are missing', () => {
    assert.throws(() => parseCliArgs([], {}), /missing required/);
  });

  it('falls back to environment variables', () => {
    const args = parseCliArgs(['--trace', 'abc'], {
      FUNNELBARN_ENDPOINT: 'https://fb.example',
      FUNNELBARN_API_KEY: 'envkey',
    });
    assert.equal(args.trace, 'abc');
    assert.equal(args.endpoint, 'https://fb.example');
    assert.equal(args.apiKey, 'envkey');
    assert.equal(args.headed, true);
  });

  it('--headless disables the visible window', () => {
    const args = parseCliArgs(
      ['--trace', 'abc', '--endpoint', 'https://x', '--api-key', 'k', '--headless'],
      {}
    );
    assert.equal(args.headed, false);
  });

  it('--dry-run is parsed', () => {
    const args = parseCliArgs(
      ['--trace', 'abc', '--endpoint', 'https://x', '--api-key', 'k', '--dry-run'],
      {}
    );
    assert.equal(args.dryRun, true);
  });
});
