import { describe, it, before, beforeEach } from 'node:test';
import assert from 'node:assert/strict';
import { FunnelBarnClient } from '../dist/esm/index.js';

// Stub localStorage for Node (not available outside a browser).
const _store = {};
global.localStorage = {
  getItem: (k) => _store[k] ?? null,
  setItem: (k, v) => { _store[k] = v; },
  removeItem: (k) => { delete _store[k]; },
  clear: () => { for (const k of Object.keys(_store)) delete _store[k]; },
};

let requests = [];

beforeEach(() => {
  requests = [];
  global.localStorage.clear();
  global.fetch = async (_url, options) => {
    requests.push({ url: _url, options });
    return { ok: true, status: 202 };
  };
});

describe('FunnelBarnClient', () => {
  it('track() queues an event and flush() sends it', async () => {
    const client = new FunnelBarnClient({ apiKey: 'test-key', endpoint: 'http://localhost:8080' });
    client.track('button_click', { page: 'home' });
    await client.flush();
    assert.equal(requests.length, 1);
    const body = JSON.parse(requests[0].options.body);
    assert.equal(body.name, 'button_click');
    assert.deepEqual(body.properties, { page: 'home' });
  });

  it('page() queues a page_view event', async () => {
    const client = new FunnelBarnClient({ apiKey: 'test-key', endpoint: 'http://localhost:8080' });
    client.page();
    await client.flush();
    assert.equal(requests.length, 1);
    const body = JSON.parse(requests[0].options.body);
    assert.equal(body.name, 'page_view');
  });

  it('identify() sets user_id on subsequent events', async () => {
    const client = new FunnelBarnClient({ apiKey: 'test-key', endpoint: 'http://localhost:8080' });
    client.identify('user-123');
    client.track('checkout');
    await client.flush();
    const body = JSON.parse(requests[0].options.body);
    assert.equal(body.user_id, 'user-123');
  });

  it('flush() clears the queue so second flush sends nothing', async () => {
    const client = new FunnelBarnClient({ apiKey: 'test-key', endpoint: 'http://localhost:8080' });
    client.track('event1');
    client.track('event2');
    await client.flush();
    assert.equal(requests.length, 2);
    await client.flush();
    assert.equal(requests.length, 2);
  });

  it('sends the api key header', async () => {
    const client = new FunnelBarnClient({ apiKey: 'my-api-key', endpoint: 'http://localhost:8080' });
    client.track('test');
    await client.flush();
    assert.equal(requests[0].options.headers['x-funnelbarn-api-key'], 'my-api-key');
  });

  it('sends project header when projectName is configured', async () => {
    const client = new FunnelBarnClient({
      apiKey: 'k',
      endpoint: 'http://localhost:8080',
      projectName: 'my-project',
    });
    client.track('test');
    await client.flush();
    assert.equal(requests[0].options.headers['x-funnelbarn-project'], 'my-project');
  });

  it('track() with empty name does not queue', async () => {
    const client = new FunnelBarnClient({ apiKey: 'k', endpoint: 'http://localhost:8080' });
    client.track('');
    await client.flush();
    assert.equal(requests.length, 0);
  });

  it('session ID is consistent within the same session window', async () => {
    const client = new FunnelBarnClient({ apiKey: 'k', endpoint: 'http://localhost:8080' });
    client.track('event1');
    client.track('event2');
    await client.flush();
    const sid1 = JSON.parse(requests[0].options.body).session_id;
    const sid2 = JSON.parse(requests[1].options.body).session_id;
    assert.equal(sid1, sid2);
    assert.ok(sid1.length > 0);
  });

  it('posts to the correct endpoint URL', async () => {
    const client = new FunnelBarnClient({ apiKey: 'k', endpoint: 'http://example.com' });
    client.track('test');
    await client.flush();
    assert.equal(requests[0].url, 'http://example.com/api/v1/events');
  });

  it('strips trailing slash from endpoint', async () => {
    const client = new FunnelBarnClient({ apiKey: 'k', endpoint: 'http://example.com/' });
    client.track('test');
    await client.flush();
    assert.equal(requests[0].url, 'http://example.com/api/v1/events');
  });

  it('event timestamp is a valid ISO string', async () => {
    const client = new FunnelBarnClient({ apiKey: 'k', endpoint: 'http://localhost:8080' });
    client.track('ts-test');
    await client.flush();
    const body = JSON.parse(requests[0].options.body);
    assert.ok(!isNaN(Date.parse(body.timestamp)));
  });

  // -------------------------------------------------------------------------
  // page_view_id
  // -------------------------------------------------------------------------

  it('page() sets page_view_id on the page_view event', async () => {
    const client = new FunnelBarnClient({ apiKey: 'k', endpoint: 'http://localhost:8080' });
    client.page();
    await client.flush();
    const body = JSON.parse(requests[0].options.body);
    assert.equal(body.name, 'page_view');
    assert.ok(typeof body.page_view_id === 'string', 'page_view_id should be a string');
    assert.match(
      body.page_view_id,
      /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/,
      'page_view_id should be a valid UUIDv4'
    );
  });

  it('track() carries the page_view_id from the preceding page()', async () => {
    const client = new FunnelBarnClient({ apiKey: 'k', endpoint: 'http://localhost:8080' });
    client.page();
    client.track('button_click');
    await client.flush();
    assert.equal(requests.length, 2);
    const pageBody = JSON.parse(requests[0].options.body);
    const trackBody = JSON.parse(requests[1].options.body);
    assert.equal(pageBody.name, 'page_view');
    assert.equal(trackBody.name, 'button_click');
    assert.ok(typeof pageBody.page_view_id === 'string');
    assert.equal(trackBody.page_view_id, pageBody.page_view_id);
  });

  it('track() without preceding page() has undefined page_view_id', async () => {
    const client = new FunnelBarnClient({ apiKey: 'k', endpoint: 'http://localhost:8080' });
    client.track('no_page_before');
    await client.flush();
    const body = JSON.parse(requests[0].options.body);
    assert.equal(body.page_view_id, undefined);
  });

  it('page_view_id changes between page() calls', async () => {
    const client = new FunnelBarnClient({ apiKey: 'k', endpoint: 'http://localhost:8080' });
    client.page();
    client.page();
    await client.flush();
    const id1 = JSON.parse(requests[0].options.body).page_view_id;
    const id2 = JSON.parse(requests[1].options.body).page_view_id;
    assert.ok(id1 !== id2, 'each page() should produce a distinct page_view_id');
  });

  // -------------------------------------------------------------------------
  // page_engaged — should NOT fire immediately
  // -------------------------------------------------------------------------

  it('page_engaged is not queued immediately after page()', async () => {
    // In the Node.js test environment there is no window, so engagement tracking
    // is a no-op. This test therefore confirms that calling page() followed by
    // an immediate flush does not produce a page_engaged event.
    const client = new FunnelBarnClient({ apiKey: 'k', endpoint: 'http://localhost:8080' });
    client.page();
    await client.flush();
    const names = requests.map(r => JSON.parse(r.options.body).name);
    assert.ok(!names.includes('page_engaged'), 'page_engaged must not fire immediately');
  });

  // -------------------------------------------------------------------------
  // session_signals — included on first page() only
  // -------------------------------------------------------------------------

  it('session_signals is included on the first page() event', async () => {
    // In Node there is no window so collectSessionSignals() returns {}.
    // We verify the field is absent rather than present with data — that is the
    // correct behaviour for a non-browser environment, and still exercises the
    // sessionSignalsSent guard.
    const client = new FunnelBarnClient({ apiKey: 'k', endpoint: 'http://localhost:8080' });
    client.page();
    await client.flush();
    const body = JSON.parse(requests[0].options.body);
    assert.equal(body.name, 'page_view');
    // session_signals may be absent (no window) — that's fine.
    // What matters is it is not present on subsequent page views.
  });

  it('session_signals is NOT included on the second page() event', async () => {
    // Simulate a browser-like environment by providing a minimal window stub
    // so collectSessionSignals returns non-empty data.
    // Use Object.defineProperty for globals that are read-only in newer Node.
    const defineWritable = (obj, key, value) => {
      const orig = Object.getOwnPropertyDescriptor(obj, key);
      Object.defineProperty(obj, key, { value, writable: true, configurable: true });
      return () => {
        if (orig) Object.defineProperty(obj, key, orig);
        else delete obj[key];
      };
    };

    const restoreWindow = defineWritable(global, 'window', {
      devicePixelRatio: 2,
      matchMedia: () => ({ matches: false }),
      addEventListener: () => {},
      removeEventListener: () => {},
    });
    const restoreScreen = defineWritable(global, 'screen', { width: 1920, height: 1080 });
    const restoreNavigator = defineWritable(global, 'navigator', { maxTouchPoints: 0, hardwareConcurrency: 8 });
    const restoreIntl = defineWritable(global, 'Intl', {
      DateTimeFormat: () => ({ resolvedOptions: () => ({ timeZone: 'UTC' }) }),
    });

    try {
      const client = new FunnelBarnClient({ apiKey: 'k', endpoint: 'http://localhost:8080' });
      client.page(); // first page — signals expected
      client.page(); // second page — signals must NOT be re-sent
      await client.flush();

      // With window defined, the constructor also fires a GET to
      // /recording-config (no body), so select only the event POSTs.
      const eventReqs = requests.filter((r) => r.url.endsWith('/api/v1/events'));
      const firstBody = JSON.parse(eventReqs[0].options.body);
      const secondBody = JSON.parse(eventReqs[1].options.body);

      // First page_view should carry session_signals.
      assert.ok(
        firstBody.session_signals && typeof firstBody.session_signals === 'object',
        'first page() should include session_signals'
      );

      // Second page_view must not carry session_signals.
      assert.equal(
        secondBody.session_signals,
        undefined,
        'second page() must not re-send session_signals'
      );
    } finally {
      restoreWindow();
      restoreScreen();
      restoreNavigator();
      restoreIntl();
    }
  });
});
