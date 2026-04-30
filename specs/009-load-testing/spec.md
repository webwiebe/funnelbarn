# Spec 009: Load Testing

## Goal
Create k6 load test scripts that validate the system can handle expected traffic, confirm rate limiting fires correctly, and give a baseline for performance regression detection.

## Directory structure to create
```
loadtest/
  README.md
  config.js          — shared config (base URL, default API key)
  scenarios/
    ingest.js        — event ingest throughput test
    rate-limit.js    — confirm 429 fires on login + ingest
    analysis.js      — funnel analysis query under concurrent load
```

## loadtest/config.js
```js
export const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';
export const API_KEY  = __ENV.API_KEY  || 'test-api-key';
export const PROJECT_ID = __ENV.PROJECT_ID || '';
```

## loadtest/scenarios/ingest.js

Test that event ingest can sustain 200 RPS for 30s without errors.

```js
import http from 'k6/http';
import { check, sleep } from 'k6';
import { BASE_URL, API_KEY, PROJECT_ID } from '../config.js';

export const options = {
  scenarios: {
    ingest_load: {
      executor: 'constant-arrival-rate',
      rate: 200,           // 200 RPS
      timeUnit: '1s',
      duration: '30s',
      preAllocatedVUs: 50,
      maxVUs: 100,
    },
  },
  thresholds: {
    http_req_failed:   ['rate<0.01'],   // <1% error rate
    http_req_duration: ['p(95)<200'],   // 95th pct under 200ms
  },
};

const payload = JSON.stringify({
  name: 'page_view',
  url: 'https://example.com/pricing',
  referrer: 'https://google.com',
  properties: { plan: 'pro' },
});

export default function () {
  const res = http.post(`${BASE_URL}/api/v1/events`, payload, {
    headers: {
      'Content-Type': 'application/json',
      'X-API-Key': API_KEY,
    },
  });
  check(res, { '202 accepted': (r) => r.status === 202 });
}
```

## loadtest/scenarios/rate-limit.js

Confirm the rate limiter fires 429 on the login endpoint after burst.
This is a correctness test, not a throughput test — it should produce 429s intentionally.

```js
import http from 'k6/http';
import { check } from 'k6';
import { BASE_URL } from '../config.js';

export const options = {
  scenarios: {
    hammer_login: {
      executor: 'shared-iterations',
      vus: 10,
      iterations: 30,   // 30 rapid login attempts — should trigger 429
    },
  },
  // We EXPECT some 429s here — do not fail on them
  thresholds: {
    'checks{expect_rate_limit}': ['rate>0.3'],  // at least 30% should be rate-limited
  },
};

const payload = JSON.stringify({ username: 'notauser', password: 'notapassword' });

export default function () {
  const res = http.post(`${BASE_URL}/api/v1/login`, payload, {
    headers: { 'Content-Type': 'application/json' },
  });
  check(res, {
    'expect_rate_limit': (r) => r.status === 401 || r.status === 429,
    'rate limited at least sometimes': (r) => r.status === 429,
  }, { expect_rate_limit: true });
}
```

## loadtest/scenarios/analysis.js

Test funnel analysis endpoint under 20 concurrent users for 30s.
Requires PROJECT_ID and FUNNEL_ID env vars to be set.

```js
import http from 'k6/http';
import { check, sleep } from 'k6';
import { BASE_URL } from '../config.js';

export const options = {
  vus: 20,
  duration: '30s',
  thresholds: {
    http_req_failed:   ['rate<0.01'],
    http_req_duration: ['p(95)<2000'],  // analysis queries can be slower
  },
};

const SESSION_COOKIE = __ENV.SESSION_COOKIE || '';
const PROJECT_ID     = __ENV.PROJECT_ID || '';
const FUNNEL_ID      = __ENV.FUNNEL_ID  || '';

export default function () {
  const res = http.get(
    `${BASE_URL}/api/v1/projects/${PROJECT_ID}/funnels/${FUNNEL_ID}/analysis`,
    { headers: { Cookie: `funnelbarn_session=${SESSION_COOKIE}` } }
  );
  check(res, { '200 ok': (r) => r.status === 200 });
  sleep(1);
}
```

## loadtest/README.md

Write a README covering:
1. **Prerequisites** — how to install k6 (`brew install k6` / binary download)
2. **Running ingest test** — `BASE_URL=... API_KEY=... k6 run loadtest/scenarios/ingest.js`
3. **Running rate-limit validation** — `BASE_URL=... k6 run loadtest/scenarios/rate-limit.js`
4. **Running analysis test** — env vars needed, how to get SESSION_COOKIE and FUNNEL_ID
5. **Interpreting results** — what the thresholds mean, how to read k6 summary output
6. **Baseline** — placeholder table for recording results per release (columns: date, version, p50, p95, p99, RPS, error_rate)

## Acceptance criteria
- `loadtest/` directory exists with all 4 files
- k6 scripts are valid JavaScript (no syntax errors)
- README is complete with all 6 sections
- No changes to Go code or Go tests — this is scripts only
