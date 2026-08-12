// NetBerth long-duration soak — mixed public/authenticated/WebSocket load with
// a ramp-up, sustained plateau, spike and drain. Designed to expose leaks,
// lockups and slow degradation that short load tests miss.
//
// Usage:
//   k6 run qa/soak/soak.js --env BASE=http://127.0.0.1:18545 \
//     --env USER=admin --env PASS="$NB_QA_PASS" --env SOAK_SECONDS=1800
import http from 'k6/http';
import { check, sleep } from 'k6';
import ws from 'k6/ws';
import { Rate } from 'k6/metrics';

const BASE = __ENV.BASE || 'http://127.0.0.1:18545';
const USER = __ENV.USER || 'admin';
const PASS = __ENV.PASS || '';
if (!PASS) throw new Error('PASS is required: run with --env PASS=...');
const TOTAL = Number(__ENV.SOAK_SECONDS || 600);
const WS_URL = BASE.replace(/^http/, 'ws');
const wsOkRate = new Rate('ws_ok_rate');

const headers = { 'Content-Type': 'application/json' };

export const options = {
  scenarios: {
    mixed: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: '60s', target: 80 },
        { duration: `${Math.max(60, TOTAL - 150)}s`, target: 80 },
        { duration: '60s', target: 200 },
        { duration: '30s', target: 0 },
      ],
      exec: 'mixed',
      gracefulStop: '30s',
    },
    websocket: {
      executor: 'constant-vus',
      vus: 8,
      duration: `${TOTAL}s`,
      startTime: '30s',
      exec: 'wsTest',
    },
    refresh_rotation: {
      executor: 'constant-vus',
      vus: 3,
      duration: `${TOTAL}s`,
      startTime: '30s',
      exec: 'refreshTest',
    },
  },
  thresholds: {
    http_req_failed: ['rate<0.001'],
    'http_req_duration{scenario:mixed}': ['p(95)<800'],
    ws_ok_rate: ['rate>0.95'],
  },
};

export function setup() {
  const r = http.post(
    `${BASE}/api/v1/auth/login`,
    JSON.stringify({ username: USER, password: PASS }),
    { headers },
  );
  const body = r.json();
  if (!body?.data?.tokens?.access_token) {
    throw new Error(`login failed: ${r.status} ${r.body}`);
  }
  return {
    access: body.data.tokens.access_token,
    refresh: body.data.tokens.refresh_token,
  };
}

function authHeaders(token) {
  return { ...headers, Authorization: `Bearer ${token}` };
}

export function mixed(data) {
  // Mostly reads, occasional auth rotation, rare CRUD mutation.
  const r1 = http.get(`${BASE}/api/v1/system/status`);
  check(r1, { 'status 200': (res) => res.status === 200 });

  const r2 = http.get(`${BASE}/api/v1/system/metrics`);
  check(r2, { 'metrics 200': (res) => res.status === 200 });

  const reads = ['/api/v1/system/dashboard', '/api/v1/forward-rules', '/api/v1/users', '/api/v1/audit'];
  const r3 = http.get(`${BASE}${reads[__ITER % reads.length]}`, { headers: authHeaders(data.access) });
  check(r3, { [`authed ${reads[__ITER % reads.length]} 200`]: (res) => res.status === 200 });

  if (__ITER % 200 === 0) {
    const name = `soak-rule-${__VU}-${__ITER}`;
    const listenPort = 23000 + ((__VU * 40 + __ITER) % 1000);
    const created = http.post(
      `${BASE}/api/v1/forward-rules`,
      JSON.stringify({
        name,
        protocol: 'tcp',
        listen_addr: '127.0.0.1',
        listen_port: listenPort,
        target_addr: '127.0.0.1',
        target_port: 1,
        enabled: false,
      }),
      { headers: authHeaders(data.access) },
    );
    check(created, { 'rule create 201': (res) => res.status === 201 || res.status === 200 });
    if (created.status === 201 || created.status === 200) {
      const id = created.json()?.data?.id;
      if (id) {
        const del = http.del(`${BASE}/api/v1/forward-rules/${id}`, null, {
          headers: authHeaders(data.access),
        });
        check(del, { 'rule delete 200': (res) => res.status === 200 });
      }
    }
  }

  sleep(0.05 + Math.random() * 0.25);
}

// Refresh rotation is exercised in its own small scenario with per-VU fresh
// logins: a single shared refresh token must never be raced by many VUs
// (that would test the race, not steady-state behavior).
export function refreshTest() {
  const login = http.post(
    `${BASE}/api/v1/auth/login`,
    JSON.stringify({ username: USER, password: PASS }),
    { headers },
  );
  check(login, { 'rotation login 200': (res) => res.status === 200 });
  const refresh = login.json()?.data?.tokens?.refresh_token;
  if (refresh) {
    const rr = http.post(`${BASE}/api/v1/auth/refresh`, JSON.stringify({ refresh_token: refresh }), { headers });
    check(rr, { 'refresh rotation 200': (res) => res.status === 200 });
  }
  sleep(1);
}

export function wsTest() {
  let got = false;
  let open = false;
  ws.connect(`${WS_URL}/api/v1/ws`, null, (socket) => {
    socket.on('open', () => {
      open = true;
    });
    socket.on('message', (data) => {
      if (data.length > 0) {
        got = true;
        socket.close();
      }
    });
    socket.on('error', () => {});
    socket.setTimeout(() => socket.close(), 5000);
  });
  check(open && got, {
    'ws opened and received status': (v) => v === true,
  });
  wsOkRate.add(open && got);
  sleep(0.2);
}
