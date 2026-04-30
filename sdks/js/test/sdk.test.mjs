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
});
