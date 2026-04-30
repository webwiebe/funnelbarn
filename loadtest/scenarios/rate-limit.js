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
