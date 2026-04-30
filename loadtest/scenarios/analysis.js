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
