// NetBerth load devil test — public, authenticated and WebSocket scenarios.
import http from 'k6/http';
import { check, sleep } from 'k6';
import ws from 'k6/ws';

const BASE = __ENV.BASE || 'http://127.0.0.1:18445';
const USER = __ENV.USER || 'admin';
const PASS = __ENV.PASS || '';
if (!PASS) throw new Error('PASS is required: run with --env PASS=...');
const WS_URL = BASE.replace(/^http/, 'ws');
const SOAK = __ENV.SOAK === '1' || __ENV.SOAK === 'true';
const MAIN_DUR = SOAK ? '120s' : '30s';
const SHORT_DUR = SOAK ? '60s' : '12s';

export const options = {
  scenarios: {
    public_api: { executor: 'constant-vus', vus: 50, duration: MAIN_DUR, exec: 'publicAPI' },
    authed_api: { executor: 'constant-vus', vus: 20, duration: MAIN_DUR, startTime: '5s', exec: 'authedAPI' },
    login_stress: { executor: 'constant-vus', vus: 10, duration: SHORT_DUR, startTime: '5s', exec: 'loginStress' },
    user_churn: { executor: 'constant-vus', vus: 3, duration: SHORT_DUR, startTime: '8s', exec: 'userChurn' },
    websocket: { executor: 'constant-vus', vus: 5, duration: SOAK ? '60s' : '15s', startTime: '10s', exec: 'wsTest' },
  },
  thresholds: {
    http_req_failed: ['rate<0.01'],
    'http_req_duration{scenario:public_api}': ['p(95)<500'],
    'http_req_duration{scenario:authed_api}': ['p(95)<1000'],
    'http_req_duration{scenario:login_stress}': ['p(95)<2000'],
  },
};

export function setup() {
  const r = http.post(`${BASE}/api/v1/auth/login`, JSON.stringify({ username: USER, password: PASS }), {
    headers: { 'Content-Type': 'application/json' },
  });
  const body = r.json();
  if (!body?.data?.tokens?.access_token) throw new Error(`login failed: ${r.status} ${r.body}`);
  return { token: body.data.tokens.access_token };
}

export function publicAPI() {
  const r1 = http.get(`${BASE}/api/v1/system/status`);
  check(r1, { 'public status 200': (res) => res.status === 200 });
  const r2 = http.get(`${BASE}/api/v1/system/metrics`);
  check(r2, { 'public metrics 200': (res) => res.status === 200 });
  sleep(0.05);
}

export function authedAPI(data) {
  const headers = { Authorization: `Bearer ${data.token}` };
  for (const p of ['/api/v1/system/dashboard', '/api/v1/forward-rules', '/api/v1/users', '/api/v1/audit']) {
    const r = http.get(`${BASE}${p}`, { headers });
    check(r, { [`GET ${p} 200`]: (res) => res.status === 200 });
  }
  sleep(0.1);
}

export function loginStress() {
  const r = http.post(
    `${BASE}/api/v1/auth/login`,
    JSON.stringify({ username: USER, password: PASS }),
    { headers: { 'Content-Type': 'application/json' } }
  );
  check(r, { 'login 200': (res) => res.status === 200 });
}

export function userChurn(data) {
  const headers = { Authorization: `Bearer ${data.token}` };
  const username = `k6u${__VU}-${__ITER}`;
  const body = JSON.stringify({
    username,
    email: `${username}@example.com`,
    role: 'viewer',
    password: 'K6Pass123!',
  });
  const c = http.post(`${BASE}/api/v1/users`, body, { headers });
  check(c, { 'churn create 201': (res) => res.status === 201 });
  if (c.status === 201) {
    const id = c.json().data.id;
    const d = http.del(`${BASE}/api/v1/users/${id}`, null, { headers });
    check(d, { 'churn delete 200': (res) => res.status === 200 });
  }
}

export function wsTest() {
  let got = false;
  ws.connect(`${WS_URL}/api/v1/ws`, null, function (socket) {
    socket.on('open', () => {});
    socket.on('message', (data) => {
      got = data.length > 0;
      socket.close();
    });
    socket.on('error', () => {});
    socket.setTimeout(() => socket.close(), 4000);
  });
  check(got, { 'ws status message received': (v) => v === true });
}
