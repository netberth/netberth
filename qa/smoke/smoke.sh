#!/bin/bash
# NetBerth normal-use smoke — full authenticated user journey with latency
# capture: login, refresh, dashboard, users CRUD, forward/proxy rule CRUD,
# audit, logout, WebSocket status. Fresh instance on :18553.
set -uo pipefail
DIR="$(cd "$(dirname "$0")/.." && pwd)"
BIN="${1:-/tmp/netberth-qa}"
BASE="http://127.0.0.1:18553"
DATA=$(mktemp -d /private/tmp/nb-smoke.XXXXXX)
LOG="$DATA/server.log"
source "$DIR/lib.sh"
PASS="${NB_QA_PASS:-$(gen_pass)}"
PASS_N=0; FAIL_N=0
check(){ name=$1; want=$2; got=$3; note=$4
  if [ "$want" = "$got" ]; then echo "  [PASS] $name ($note)"; PASS_N=$((PASS_N+1))
  else echo "  [FAIL] $name want=$want got=$got ($note)"; FAIL_N=$((FAIL_N+1)); fi
}
ms(){ T0=$(date +%s%N); "$@" >/dev/null 2>&1; T1=$(date +%s%N); echo $(( (T1 - T0) / 1000000 )); }

NB_DB_PATH="$DATA/nb.db" NB_SERVER_HOST=127.0.0.1 NB_SERVER_PORT=18553 NB_PROXY_PORT=18189 NB_LOG_LEVEL=warn "$BIN" >"$LOG" 2>&1 &
PID=$!
for _ in $(seq 1 60); do curl -sf "$BASE/api/v1/system/status" >/dev/null 2>&1 && break; sleep 0.25; done
PASSV=$(grep -o '"password":"[^"]*"' "$LOG" | head -1 | cut -d'"' -f4)
if [ -z "$PASSV" ]; then echo "  [FATAL] no seeded admin password"; kill -TERM "$PID" 2>/dev/null; exit 1; fi
NEWPASS=$(prepare_instance "$BASE" "$DATA" "$PASS" admin) || { kill -TERM "$PID" 2>/dev/null; exit 1; }

echo "== smoke: $BASE =="
T0=$(date +%s%N); curl -s -o "$DATA/login.json" -X POST "$BASE/api/v1/auth/login" -H 'Content-Type: application/json' -d "{\"username\":\"admin\",\"password\":\"$NEWPASS\"}"; T1=$(date +%s%N); L_MS=$(( (T1 - T0) / 1000000 ))
ACC=$(python3 -c "import json;print(json.load(open('$DATA/login.json'))['data']['tokens']['access_token'])" 2>/dev/null)
REF=$(python3 -c "import json;print(json.load(open('$DATA/login.json'))['data']['tokens']['refresh_token'])" 2>/dev/null)
check "smoke_login" 200 "$(python3 -c "import json;d=json.load(open('$DATA/login.json'));print(200 if d['success'] else 500)" 2>/dev/null)" "login (${L_MS}ms)"
AH="Authorization: Bearer $ACC"
CT='Content-Type: application/json'

T0=$(date +%s%N); curl -s -o "$DATA/refresh.json" -X POST "$BASE/api/v1/auth/refresh" -H "$CT" -d "{\"refresh_token\":\"$REF\"}"; T1=$(date +%s%N); R_MS=$(( (T1 - T0) / 1000000 ))
check "smoke_refresh" 200 "$(python3 -c "import json;d=json.load(open('$DATA/refresh.json'));print(200 if d['success'] else 500)" 2>/dev/null)" "refresh rotation (${R_MS}ms)"
check "smoke_refresh_old_revoked" 401 "$(curl -s -o /dev/null -w '%{http_code}' -X POST "$BASE/api/v1/auth/refresh" -H "$CT" -d "{\"refresh_token\":\"$REF\"}")" "old refresh revoked"

for p in /api/v1/system/status /api/v1/system/metrics /api/v1/system/dashboard /api/v1/forward-rules /api/v1/proxy-rules /api/v1/audit /api/v1/auth/me; do
  C=$(curl -s -o /dev/null -w '%{http_code}' -H "$AH" "$BASE$p")
  check "smoke_get_${p//\//_}" 200 "$C" "GET $p"
done

UC=$(curl -s -o "$DATA/user.json" -w '%{http_code}' -X POST "$BASE/api/v1/users" -H "$CT" -H "$AH" -d '{"username":"smoke1","email":"smoke1@example.com","role":"viewer","password":"SmokePass123!"}')
USER_ID=$(python3 -c "import json;print(json.load(open('$DATA/user.json'))['data']['id'])" 2>/dev/null)
check "smoke_user_create" 201 "$UC" "create viewer"
check "smoke_user_update" 200 "$(curl -s -o /dev/null -w '%{http_code}' -X PUT "$BASE/api/v1/users/$USER_ID" -H "$CT" -H "$AH" -d '{"email":"smoke2@example.com","role":"viewer","enabled":true}')" "update viewer"
check "smoke_user_delete" 200 "$(curl -s -o /dev/null -w '%{http_code}' -X DELETE "$BASE/api/v1/users/$USER_ID" -H "$AH")" "delete viewer"

FPORT=$(free_port)
FC=$(curl -s -o "$DATA/fwd.json" -w '%{http_code}' -X POST "$BASE/api/v1/forward-rules" -H "$CT" -H "$AH" -d "{\"name\":\"smoke-fwd\",\"protocol\":\"tcp\",\"listen_addr\":\"127.0.0.1\",\"listen_port\":$FPORT,\"target_addr\":\"127.0.0.1\",\"target_port\":1,\"enabled\":false}")
FID=$(python3 -c "import json;print(json.load(open('$DATA/fwd.json'))['data']['id'])" 2>/dev/null)
check "smoke_forward_create" 201 "$FC" "create forward rule"
check "smoke_forward_delete" 200 "$(curl -s -o /dev/null -w '%{http_code}' -X DELETE "$BASE/api/v1/forward-rules/$FID" -H "$AH")" "delete forward rule"

PC=$(curl -s -o "$DATA/proxy.json" -w '%{http_code}' -X POST "$BASE/api/v1/proxy-rules" -H "$CT" -H "$AH" -d '{"name":"smoke-proxy","target_url":"http://127.0.0.1:19091","domains":["smoke.local"],"enabled":false}')
PRID=$(python3 -c "import json;print(json.load(open('$DATA/proxy.json'))['data']['id'])" 2>/dev/null)
check "smoke_proxy_create" 201 "$PC" "create proxy rule"
check "smoke_proxy_delete" 200 "$(curl -s -o /dev/null -w '%{http_code}' -X DELETE "$BASE/api/v1/proxy-rules/$PRID" -H "$AH")" "delete proxy rule"

# Webhook delivery end-to-end: create endpoint -> trigger event -> receiver gets signed POST.
WP=$(free_port)
WOUT="$DATA/webhook.json"
python3 - "$WOUT" "$WP" <<'PY' &
import http.server, json, sys
out, port = sys.argv[1], int(sys.argv[2])
class H(http.server.BaseHTTPRequestHandler):
    def do_POST(self):
        n = int(self.headers.get('Content-Length', 0))
        body = self.rfile.read(n).decode('utf-8', 'replace')
        with open(out, 'w') as f:
            json.dump({'path': self.path, 'body': body,
                       'sig': self.headers.get('X-NetBerth-Signature', '')}, f)
        self.send_response(204)
        self.end_headers()
    def log_message(self, *a):
        pass
http.server.HTTPServer(('127.0.0.1', port), H).serve_forever()
PY
WHPID=$!
sleep 0.5
WCRE=$(curl -s -o "$DATA/webhook.json.tmp" -w '%{http_code}' -X POST "$BASE/api/v1/webhooks" -H "$CT" -H "$AH" \
  -d "{\"name\":\"smoke-hook\",\"url\":\"http://127.0.0.1:$WP/hook\",\"secret\":\"qa-secret\",\"events\":[\"forward:created\"],\"enabled\":true}")
WHOOK_ID=$(python3 -c "import json;print(json.load(open('$DATA/webhook.json.tmp'))['data']['id'])" 2>/dev/null)
check "smoke_webhook_create" 201 "$WCRE" "create webhook endpoint"
FPORT2=$(free_port)
WFC=$(curl -s -o "$DATA/webhook-fwd.json" -w '%{http_code}' -X POST "$BASE/api/v1/forward-rules" -H "$CT" -H "$AH" \
  -d "{\"name\":\"smoke-webhook-trigger\",\"protocol\":\"tcp\",\"listen_addr\":\"127.0.0.1\",\"listen_port\":$FPORT2,\"target_addr\":\"127.0.0.1\",\"target_port\":1,\"enabled\":false}")
check "smoke_webhook_trigger_rule" 201 "$WFC" "forward rule create fires event"
for _ in $(seq 1 24); do [ -s "$WOUT" ] && break; sleep 0.25; done
if [ -s "$WOUT" ]; then
  SIGOK=$(python3 -c "import json;d=json.load(open('$WOUT'));print(1 if d['sig'].startswith('sha256=') and 'forward:created' in d['body'] else 0)")
  check "smoke_webhook_delivery" 1 "$SIGOK" "signed event delivered to receiver"
else
  check "smoke_webhook_delivery" 1 0 "no webhook delivery within 6s"
fi
WFID=$(python3 -c "import json;print(json.load(open('$DATA/webhook-fwd.json'))['data']['id'])" 2>/dev/null)
[ -n "$WFID" ] && curl -s -o /dev/null -X DELETE "$BASE/api/v1/forward-rules/$WFID" -H "$AH" 2>/dev/null || true
[ -n "$WHOOK_ID" ] && curl -s -o /dev/null -X DELETE "$BASE/api/v1/webhooks/$WHOOK_ID" -H "$AH"
kill "$WHPID" 2>/dev/null; wait "$WHPID" 2>/dev/null

T0=$(date +%s%N)
python3 - "$BASE" >"$DATA/ws.out" 2>&1 <<'PY'
import base64, os, socket, sys
base = sys.argv[1]
host = base.split('://')[1].split(':')[0]
port = int(base.split(':')[2])
s = socket.create_connection((host, port), timeout=5)
key = base64.b64encode(os.urandom(16)).decode()
req = (f"GET /api/v1/ws HTTP/1.1\r\nHost: {host}:{port}\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n"
       f"Sec-WebSocket-Key: {key}\r\nSec-WebSocket-Version: 13\r\n\r\n").encode()
s.sendall(req)
s.settimeout(5)
resp = s.recv(4096)
ok = b' 101 ' in resp.split(b'\r\n', 1)[0]
if ok:
    h = s.recv(2)
    ln = h[1] & 0x7F
    if ln == 126:
        ln = int.from_bytes(s.recv(2), 'big')
    payload = b''
    while len(payload) < ln:
        payload += s.recv(ln - len(payload))
    ok = len(payload) > 0
s.close()
print('OK' if ok else 'FAIL')
PY
T1=$(date +%s%N); WS_MS=$(( (T1 - T0) / 1000000 ))
check "smoke_ws" OK "$(cat "$DATA/ws.out")" "websocket status message (${WS_MS}ms)"

REF2=$(python3 -c "import json;print(json.load(open('$DATA/refresh.json'))['data']['refresh_token'])" 2>/dev/null)
check "smoke_logout" 200 "$(curl -s -o /dev/null -w '%{http_code}' -X POST "$BASE/api/v1/auth/logout" -H "$CT" -H "$AH" -d "{\"refresh_token\":\"$REF2\"}")" "logout"

kill -TERM "$PID" 2>/dev/null; wait "$PID" 2>/dev/null
echo
echo "smoke summary: PASS=$PASS_N FAIL=$FAIL_N"
[ "$FAIL_N" -eq 0 ] || exit 1
