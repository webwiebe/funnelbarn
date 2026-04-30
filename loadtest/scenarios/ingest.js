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
      'x-funnelbarn-api-key': API_KEY,
    },
  });
  check(res, { '202 accepted': (r) => r.status === 202 });
}
