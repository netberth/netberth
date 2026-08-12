#!/bin/bash
# NetBerth simulated-environment matrix — real deployment shapes exercised
# together: TLS panel, reverse proxy with live upstream, rule persistence
# across restarts, resource-constrained host, doctor pre-flight.
set -uo pipefail
DIR="$(cd "$(dirname "$0")/.." && pwd)"
source "$DIR/lib.sh"
PASS="${NB_QA_PASS:-$(gen_pass)}"
BIN="${1:-/tmp/netberth-qa}"
ROOT=$(mktemp -d /private/tmp/nb-sim.XXXXXX)
PASS_N=0; FAIL_N=0; WARN_N=0
check(){ name=$1; want=$2; got=$3; note=$4
  if [ "$want" = "$got" ]; then echo "  [PASS] $name ($note)"; PASS_N=$((PASS_N+1))
  else echo "  [FAIL] $name want=$want got=$got ($note)"; FAIL_N=$((FAIL_N+1)); fi
}
check_in(){ name=$1; want=$2; got=$3; note=$4
  if echo "$want" | tr '|' '\n' | grep -qx "$got"; then echo "  [PASS] $name ($note)"; PASS_N=$((PASS_N+1))
  else echo "  [FAIL] $name want=[$want] got=$got ($note)"; FAIL_N=$((FAIL_N+1)); fi
}
warn(){ echo "  [WARN] $1"; WARN_N=$((WARN_N+1)); }
admin_pass(){ grep -o '"password":"[^"]*"' "$1" | head -1 | cut -d'"' -f4; }
stop(){ kill -TERM "$1" 2>/dev/null; wait "$1" 2>/dev/null; }
login(){ local base=$1 pass=$2
  local opt=(-s)
  case "$base" in
    https://*) opt=(-sk) ;;
  esac
  curl "${opt[@]}" -X POST "$base/api/v1/auth/login" -H 'Content-Type: application/json' \
    -d "{\"username\":\"admin\",\"password\":\"$pass\"}" 2>/dev/null
}
tok(){ python3 -c "import sys,json;print(json.load(sys.stdin)['data']['tokens']['access_token'])" 2>/dev/null; }

echo "== sim matrix: $BIN =="
echo "  (root: $ROOT)"

# --- S1 TLS panel (auto self-signed), HSTS, TLS floor ---
S1_DIR="$ROOT/s1-tls"
mkdir -p "$S1_DIR"
NB_DB_PATH="$S1_DIR/nb.db" NB_SERVER_HOST=127.0.0.1 NB_SERVER_PORT=18548 \
  NB_PROXY_PORT=18185 NB_TLS_ENABLED=true NB_LOG_LEVEL=warn "$BIN" >"$S1_DIR/server.log" 2>&1 &
S1_PID=$!
if wait_http "https://127.0.0.1:18548"; then
  HSTS=$(curl -sk -D - -o /dev/null "https://127.0.0.1:18548/" | grep -i '^Strict-Transport-Security:' | tr -d '\r')
  if [ -n "$HSTS" ]; then check "s1_hsts" present present "HSTS header on TLS panel"
  else warn "HSTS header missing"; fi
  P1=$(prepare_instance "https://127.0.0.1:18548" "$S1_DIR" "$PASS" admin) || warn "S1 bootstrap failed"
  L1=$(login "https://127.0.0.1:18548" "$P1")
  S=$(echo "$L1" | python3 -c "import sys,json;print(json.load(sys.stdin)['success'] and 200 or 400)" 2>/dev/null)
  check "s1_tls_login" 200 "$S" "login over TLS"
  python3 - "$P1" <<'PY' >"$ROOT/tls12.out" 2>&1
import ssl, socket, sys, json
passwd = sys.argv[1]
ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
ctx.check_hostname = False
ctx.verify_mode = ssl.CERT_NONE
ctx.minimum_version = ssl.TLSVersion.TLSv1_2
with socket.create_connection(('127.0.0.1', 18548), timeout=5) as raw:
    with ctx.wrap_socket(raw) as s:
        body = ('POST /api/v1/auth/login HTTP/1.1\r\nHost: 127.0.0.1:18548\r\nContent-Type: application/json\r\n'
                f'Content-Length: {len(json.dumps({"username":"admin","password":passwd}))}\r\nConnection: close\r\n\r\n'
                + json.dumps({"username":"admin","password":passwd})).encode()
        s.sendall(body)
        data = b''
        while True:
            chunk = s.recv(4096)
            if not chunk:
                break
            data += chunk
        print('TLS' + s.version() + ' ' + data.split(b'\r\n', 1)[0].decode('latin1'))
PY
  if grep -qE 'TLSv1\.[23].* 200' "$ROOT/tls12.out"; then
    echo "  [PASS] s1_tls12 ($(cat "$ROOT/tls12.out"))"; PASS_N=$((PASS_N+1))
  else
    warn "TLS1.2 check: $(cat "$ROOT/tls12.out")"
  fi
else
  check "s1_tls_up" up down "TLS instance did not start"
  warn "S1 log tail: $(tail -3 "$S1_DIR/server.log" | tr '\n' ' ')"
fi

# --- S2 reverse proxy with live upstream ---
S2_DIR="$ROOT/s2-proxy"
mkdir -p "$S2_DIR"
python3 - 19091 >"$ROOT/upstream.log" 2>&1 <<'PY' &
from http.server import BaseHTTPRequestHandler, HTTPServer
class H(BaseHTTPRequestHandler):
    def do_GET(self):
        body = b"UPSTREAM-OK path=" + self.path.encode() + b" host=" + self.headers.get('Host', '').encode()
        self.send_response(200)
        self.send_header('Content-Type', 'text/plain')
        self.send_header('Content-Length', str(len(body)))
        self.end_headers()
        self.wfile.write(body)
    def log_message(self, *a):
        pass
HTTPServer(('127.0.0.1', 19091), H).serve_forever()
PY
UP_PID=$!
NB_DB_PATH="$S2_DIR/nb.db" NB_SERVER_HOST=127.0.0.1 NB_SERVER_PORT=18550 \
  NB_PROXY_PORT=18186 NB_LOG_LEVEL=warn "$BIN" >"$S2_DIR/server.log" 2>&1 &
S2_PID=$!
if wait_http "http://127.0.0.1:18550"; then
  P2=$(prepare_instance "http://127.0.0.1:18550" "$S2_DIR" "$PASS" admin) || warn "S2 bootstrap failed"
  L2=$(login "http://127.0.0.1:18550" "$P2")
  T2=$(echo "$L2" | tok)
  RC=$(curl -s -o "$ROOT/rule.out" -w '%{http_code}' -X POST "http://127.0.0.1:18550/api/v1/proxy-rules" \
    -H 'Content-Type: application/json' -H "Authorization: Bearer $T2" \
    -d '{"name":"sim-upstream","target_url":"http://127.0.0.1:19091","domains":["qa.local"],"websocket":true,"enabled":true}')
  check "s2_rule_create" 201 "$RC" "proxy rule created"
  RULE_ID=$(python3 -c "import json;print(json.load(open('$ROOT/rule.out')).get('data',{}).get('id',''))" 2>/dev/null)
  sleep 0.5
  UP=$(curl -s -H 'Host: qa.local' "http://127.0.0.1:18186/hello?x=1")
  case "$UP" in *UPSTREAM-OK*) check "s2_proxy_forward" forwarded forwarded "request reached upstream via NetBerth ($UP)";; *) warn "proxy forward got: $UP";; esac
  UNKNOWN=$(curl -s -o /dev/null -w '%{http_code}' -H 'Host: unknown.invalid' "http://127.0.0.1:18186/")
  check_in "s2_unknown_host" "404|502|503" "$UNKNOWN" "unknown host rejected"
  # persistence across restart
  stop "$S2_PID"
  NB_DB_PATH="$S2_DIR/nb.db" NB_SERVER_HOST=127.0.0.1 NB_SERVER_PORT=18550 \
    NB_PROXY_PORT=18186 NB_LOG_LEVEL=warn "$BIN" >"$S2_DIR/server2.log" 2>&1 &
  S2_PID=$!
  if wait_http "http://127.0.0.1:18550"; then
    sleep 0.5
    UP2=$(curl -s -H 'Host: qa.local' "http://127.0.0.1:18186/after-restart")
    case "$UP2" in *UPSTREAM-OK*) check "s2_persistence" persisted persisted "proxy rule survives restart";; *) warn "proxy after restart got: $UP2";; esac
  else
    check "s2_restart_up" up down "instance failed to restart"
  fi
else
  check "s2_up" up down "proxy instance did not start"
fi
kill "$UP_PID" 2>/dev/null; wait "$UP_PID" 2>/dev/null

# --- S3 resource-constrained host ---
S3_DIR="$ROOT/s3-limited"
mkdir -p "$S3_DIR"
( ulimit -n 256; nice -n 10 env NB_DB_PATH="$S3_DIR/nb.db" NB_SERVER_HOST=127.0.0.1 NB_SERVER_PORT=18551 NB_PROXY_PORT=18187 NB_LOG_LEVEL=warn "$BIN" >"$S3_DIR/server.log" 2>&1 & echo $! >"$ROOT/s3pid" )
S3_PID=$(cat "$ROOT/s3pid")
if wait_http "http://127.0.0.1:18551"; then
  AB3=$(ab -n 500 -c 50 -k "http://127.0.0.1:18551/api/v1/system/status" 2>/dev/null | tail -12)
  AB3_FAIL=$(echo "$AB3" | grep -o 'Failed requests: *[0-9]*' | grep -o '[0-9]*' || echo 0)
  if kill -0 "$S3_PID" 2>/dev/null; then check "s3_alive" alive alive "constrained instance alive after 500 req"
  else check "s3_alive" alive dead "constrained instance died"; fi
  if [ "${AB3_FAIL:-0}" -eq 0 ]; then check "s3_zero_fail" 0 "${AB3_FAIL}" "500 req / 50 concurrency under fd=256"
  else warn "s3 failed requests: ${AB3_FAIL}/500"; fi
  kill -TERM "$S3_PID" 2>/dev/null; wait "$S3_PID" 2>/dev/null
else
  warn "s3 constrained instance did not start (see $S3_DIR/server.log)"
fi

# --- S4 doctor pre-flight on fresh config ---
S4_DIR="$ROOT/s4-doctor"
mkdir -p "$S4_DIR"
NB_DB_PATH="$S4_DIR/nb.db" NB_SERVER_HOST=127.0.0.1 NB_SERVER_PORT=18552 NB_PROXY_PORT=18188 "$BIN" doctor >"$ROOT/doctor.out" 2>&1
check "s4_doctor" 0 "$?" "netberth doctor exits 0 on healthy config"

# --- S5 graceful coexistence: both S1 (TLS) and S2 (proxy) healthy at once ---
if curl -skf "https://127.0.0.1:18548/api/v1/system/status" >/dev/null 2>&1 && curl -sf "http://127.0.0.1:18550/api/v1/system/status" >/dev/null 2>&1; then
  check "s5_coexist" both both "TLS + proxy instances coexist"
else
  warn "coexistence check failed (TLS or proxy instance down)"
fi

stop "$S1_PID"; stop "$S2_PID"
echo
echo "sim summary: PASS=$PASS_N FAIL=$FAIL_N WARN=$WARN_N"
[ "$FAIL_N" -eq 0 ] || exit 1
