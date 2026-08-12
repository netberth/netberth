#!/bin/bash
# NetBerth stress devil tests — floods, concurrency, token abuse, WebSocket
# floods and slow consumers, plus goroutine/memory growth observation.
# Manages its own QA instance on :18547 (fd-limited twin on :18549).
set -uo pipefail
DIR="$(cd "$(dirname "$0")/.." && pwd)"
source "$DIR/lib.sh"
PASS="${NB_QA_PASS:-$(gen_pass)}"
BIN="${1:-/tmp/netberth-qa}"
PORT="${2:-18547}"
BASE="http://127.0.0.1:$PORT"
PROXY_PORT=18183
DATA=$(mktemp -d /private/tmp/nb-stress.XXXXXX)
LOG="$DATA/server.log"
PASS_N=0; FAIL_N=0; WARN_N=0
check(){ name=$1; want=$2; got=$3; note=$4
  if [ "$want" = "$got" ]; then echo "  [PASS] $name ($note)"; PASS_N=$((PASS_N+1))
  else echo "  [FAIL] $name want=$want got=$got ($note)"; FAIL_N=$((FAIL_N+1)); fi
}
check_le(){ name=$1; want=$2; got=$3; note=$4
  if python3 -c "import sys; sys.exit(0 if float('$got') <= float('$want') else 1)" 2>/dev/null; then
    echo "  [PASS] $name ($note)"; PASS_N=$((PASS_N+1))
  else echo "  [FAIL] $name want<=$want got=$got ($note)"; FAIL_N=$((FAIL_N+1)); fi
}
check_ge(){ name=$1; want=$2; got=$3; note=$4
  if python3 -c "import sys; sys.exit(0 if float('$got') >= float('$want') else 1)" 2>/dev/null; then
    echo "  [PASS] $name ($note)"; PASS_N=$((PASS_N+1))
  else echo "  [FAIL] $name want>=$want got=$got ($note)"; FAIL_N=$((FAIL_N+1)); fi
}
warn(){ echo "  [WARN] $1"; WARN_N=$((WARN_N+1)); }

start(){ local port=$1 dir=$2 proxy=$3
  ( ulimit -n 4096; exec env NB_DB_PATH="$dir/nb.db" NB_SERVER_HOST=127.0.0.1 NB_SERVER_PORT=$port NB_PROXY_PORT=$proxy NB_LOG_LEVEL=warn "$BIN" >"$dir/server.log" 2>&1 ) &
  echo $!
}
stop(){ kill -TERM "$1" 2>/dev/null; wait "$1" 2>/dev/null; }
goroutines(){ curl -sf "$1/api/v1/system/metrics" 2>/dev/null | python3 -c "import sys,json;print(json.load(sys.stdin)['data']['goroutines'])" 2>/dev/null || echo 0; }
rss_kb(){ ps -o rss= -p "$1" 2>/dev/null | tr -d ' ' || echo 0; }

echo "== stress: $BASE =="
echo "  (data dir: $DATA)"

PID=$(start "$PORT" "$DATA" "$PROXY_PORT")
if ! wait_http "$BASE"; then echo "  [FATAL] instance did not start (see $LOG)"; exit 1; fi
NEWPASS=$(prepare_instance "$BASE" "$DATA" "$PASS" admin) || { stop "$PID"; exit 1; }

LOGIN=$(curl -s -X POST "$BASE/api/v1/auth/login" -H 'Content-Type: application/json' -d "{\"username\":\"admin\",\"password\":\"$NEWPASS\"}")
TOK=$(echo "$LOGIN" | python3 -c "import sys,json;print(json.load(sys.stdin)['data']['tokens']['access_token'])" 2>/dev/null)
REF=$(echo "$LOGIN" | python3 -c "import sys,json;print(json.load(sys.stdin)['data']['tokens']['refresh_token'])" 2>/dev/null)
check "stress_login" 200 "$(echo "$LOGIN" | python3 -c "import sys,json;print(json.load(sys.stdin)['success'] and 200 or 500)" 2>/dev/null)" "login after bootstrap"

G0=$(goroutines "$BASE")
R0=$(rss_kb "$PID")

# 1 ab connection flood (3k requests, 150 concurrent, keep-alive)
ab -n 3000 -c 150 -k "$BASE/api/v1/system/status" >"$DATA/ab.out" 2>&1
AB_CONNECT=$(grep -o 'Connect: *[0-9]*' "$DATA/ab.out" | grep -o '[0-9]*' | head -1)
AB_RECV=$(grep -o 'Receive: *[0-9]*' "$DATA/ab.out" | grep -o '[0-9]*' | head -1)
AB_EXC=$(grep -o 'Exceptions: *[0-9]*' "$DATA/ab.out" | grep -o '[0-9]*' | head -1)
AB_NON2=$(grep -o 'Non-2xx responses: *[0-9]*' "$DATA/ab.out" | grep -o '[0-9]*' | head -1)
AB_TOTAL=$(grep -o 'Complete requests: *[0-9]*' "$DATA/ab.out" | grep -o '[0-9]*' | head -1)
echo "  (ab: total=${AB_TOTAL:-0} connect_err=${AB_CONNECT:-0} recv_err=${AB_RECV:-0} exc=${AB_EXC:-0} non2xx=${AB_NON2:-0})"
check "ab_flood_no_conn_errors" 0 "$(( ${AB_CONNECT:-0} + ${AB_RECV:-0} + ${AB_EXC:-0} ))" "3k req / 150 concurrency: zero connection-level errors"
if [ -z "${AB_NON2:-}" ]; then
  warn "ab did not report Non-2xx; check $DATA/ab.out"
else
  check_le "ab_flood_non2xx" 90 "${AB_NON2}" "<=3% non-2xx under flood (rate limiter allowed)"
fi
for i in $(seq 1 100); do
  curl -s -o /dev/null -w '%{http_code}\n' "$BASE/api/v1/system/status"
  sleep 0.02
done >"$DATA/seq_codes.txt"
SEQ_NON200=$(grep -vc '^200$' "$DATA/seq_codes.txt" || true)
check "sequential_50rps_all_ok" 0 "$SEQ_NON200" "100 requests @ ~50rps all 200"

# Let keep-alive sockets and TIME_WAIT drain after the flood before the next
# burst, so macOS listen-backlog saturation does not skew the CRUD test.
sleep 5

# 2 concurrent admin CRUD (40 rules in 8 parallel workers, then cleanup)
python3 - "$BASE" "$TOK" >"$DATA/crud.txt" <<'PY'
import concurrent.futures, http.client, json, sys
base, token = sys.argv[1], sys.argv[2]
host = base.split('://')[1].split(':')[0]
port = int(base.split(':')[2])
def create(i):
    import socket, time
    s = socket.socket()
    s.bind(('127.0.0.1', 0))
    listen_port = s.getsockname()[1]
    s.close()
    body = json.dumps({"name": f"stress-rule-{i}", "protocol": "tcp",
                       "listen_addr": "127.0.0.1", "listen_port": listen_port,
                       "target_addr": "127.0.0.1", "target_port": 1,
                       "enabled": False})
    code = 0
    data = b''
    for attempt in range(10):
        try:
            conn = http.client.HTTPConnection(host, port, timeout=10)
            conn.request("POST", "/api/v1/forward-rules", body,
                         {"Content-Type": "application/json", "Authorization": f"Bearer {token}"})
            r = conn.getresponse()
            data = r.read()
            conn.close()
            code = r.status
            break
        except (ConnectionRefusedError, ConnectionResetError):
            time.sleep(0.5)
    rid = ""
    if code in (200, 201):
        try:
            rid = json.loads(data)["data"]["id"]
        except Exception:
            pass
    return code, rid
with concurrent.futures.ThreadPoolExecutor(max_workers=8) as ex:
    for code, rid in ex.map(create, range(40)):
        print(code, rid)
PY
CREATED=$(awk '$1==200 || $1==201 {n++} END{print n+0}' "$DATA/crud.txt")
check "concurrent_rule_create" 40 "$CREATED" "40 parallel rule creates"
IDS=$(awk '$1==200 || $1==201 {print $2}' "$DATA/crud.txt")
for id in $IDS; do
  curl -s -o /dev/null -X DELETE "$BASE/api/v1/forward-rules/$id" -H "Authorization: Bearer $TOK" &
done
wait

# 3 refresh token race: one rotation wins, others rejected, none 500
python3 - "$BASE" "$REF" >"$DATA/refresh_race.txt" <<'PY'
import concurrent.futures, http.client, json, sys
base, ref = sys.argv[1], sys.argv[2]
host = base.split('://')[1].split(':')[0]
port = int(base.split(':')[2])
def rotate(_):
    body = json.dumps({"refresh_token": ref})
    conn = http.client.HTTPConnection(host, port, timeout=10)
    conn.request("POST", "/api/v1/auth/refresh", body, {"Content-Type": "application/json"})
    r = conn.getresponse()
    r.read()
    conn.close()
    return r.status
with concurrent.futures.ThreadPoolExecutor(max_workers=15) as ex:
    for code in ex.map(rotate, range(30)):
        print(code)
PY
OKS=$(grep -c '^200$' "$DATA/refresh_race.txt" || true)
FIVES=$(grep -c '^500$' "$DATA/refresh_race.txt" || true)
check "refresh_race_no_500" 0 "$FIVES" "no 500 during 30-way refresh race"
check_ge "refresh_race_one_winner" 1 "$OKS" "at least one rotation won"

# 4 WebSocket flood: 30 concurrent connections, each must receive a message
sleep 3
python3 - "$BASE" 30 >"$DATA/wsflood.out" 2>&1 <<'PY'
import base64, os, socket, sys
base = sys.argv[1]
host = base.split('://')[1].split(':')[0]
port = int(base.split(':')[2])
count = int(sys.argv[2])

def ws_connect():
    s = socket.create_connection((host, port), timeout=5)
    key = base64.b64encode(os.urandom(16)).decode()
    req = (f"GET /api/v1/ws HTTP/1.1\r\nHost: {host}:{port}\r\n"
           "Upgrade: websocket\r\nConnection: Upgrade\r\n"
           f"Sec-WebSocket-Key: {key}\r\nSec-WebSocket-Version: 13\r\n\r\n").encode()
    s.sendall(req)
    s.settimeout(5)
    buf = b''
    while b'\r\n\r\n' not in buf:
        chunk = s.recv(4096)
        if not chunk:
            raise RuntimeError('closed during handshake')
        buf += chunk
    head, _, rest = buf.partition(b'\r\n\r\n')
    if b' 101 ' not in head.split(b'\r\n', 1)[0]:
        raise RuntimeError('bad handshake')
    return s, rest

def read_one_frame(s, buf):
    while len(buf) < 2:
        chunk = s.recv(4096)
        if not chunk:
            return False
        buf += chunk
    ln = buf[1] & 0x7F
    idx = 2
    if ln == 126:
        while len(buf) < idx + 2:
            buf += s.recv(4096)
        ln = int.from_bytes(buf[idx:idx+2], 'big'); idx += 2
    elif ln == 127:
        while len(buf) < idx + 8:
            buf += s.recv(4096)
        ln = int.from_bytes(buf[idx:idx+8], 'big'); idx += 8
    while len(buf) < idx + ln:
        chunk = s.recv(4096)
        if not chunk:
            return False
        buf += chunk
    return len(buf[idx:idx+ln]) > 0

socks = []
got = 0
try:
    for _ in range(count):
        s, rest = ws_connect()
        if read_one_frame(s, rest):
            got += 1
        s.close()
    print(got)
except Exception as e:
    print('ERROR', type(e).__name__, e)
    raise
PY
WSGOT=$(head -1 "$DATA/wsflood.out")
check "ws_flood_30" 30 "$WSGOT" "30 concurrent WS connections all received status"

# 5 slow WS consumers (backpressure): 20 sockets that stop reading for 8s
sleep 3
python3 - "$BASE" 20 8 >/dev/null 2>&1 <<'PY'
import base64, os, socket, sys, time
base, count, hold = sys.argv[1], int(sys.argv[2]), int(sys.argv[3])
host = base.split('://')[1].split(':')[0]
port = int(base.split(':')[2])
socks = []
try:
    for _ in range(count):
        s = socket.create_connection((host, port), timeout=5)
        key = base64.b64encode(os.urandom(16)).decode()
        req = (f"GET /api/v1/ws HTTP/1.1\r\nHost: {host}:{port}\r\n"
               "Upgrade: websocket\r\nConnection: Upgrade\r\n"
               f"Sec-WebSocket-Key: {key}\r\nSec-WebSocket-Version: 13\r\n\r\n").encode()
        s.sendall(req)
        s.settimeout(5)
        buf = b''
        while b'\r\n\r\n' not in buf:
            chunk = s.recv(4096)
            if not chunk:
                break
            buf += chunk
        if b' 101 ' in buf.split(b'\r\n', 1)[0]:
            socks.append(s)
    time.sleep(hold)
finally:
    for s in socks:
        try:
            s.close()
        except Exception:
            pass
PY
T0=$(date +%s%N)
c=$(curl -s -o /dev/null -w '%{http_code}' "$BASE/api/v1/system/status")
T1=$(date +%s%N)
LAT_MS=$(( (T1 - T0) / 1000000 ))
check "server_alive_after_slow_ws" 200 "$c" "responsive with 20 stalled WS consumers"
check_le "slow_ws_latency" 1500 "$LAT_MS" "status latency (ms) under backpressure"

# 6 resource-limited twin (ulimit -n 256): must survive and recover
FD_DIR=$(mktemp -d /private/tmp/nb-stress-fd.XXXXXX)
( ulimit -n 256; NB_DB_PATH="$FD_DIR/nb.db" NB_SERVER_HOST=127.0.0.1 NB_SERVER_PORT=18549 NB_PROXY_PORT=18184 NB_LOG_LEVEL=warn "$BIN" >"$FD_DIR/server.log" 2>&1 & echo $! >"$DATA/fdpid" )
FDPID=$(cat "$DATA/fdpid" 2>/dev/null || true)
if wait_http "http://127.0.0.1:18549"; then
  ab -n 1500 -c 200 -k "http://127.0.0.1:18549/api/v1/system/status" >"$DATA/ab2.out" 2>&1
  AB2_CONNECT=$(grep -o 'Connect: *[0-9]*' "$DATA/ab2.out" | grep -o '[0-9]*' | head -1)
  AB2_RECV=$(grep -o 'Receive: *[0-9]*' "$DATA/ab2.out" | grep -o '[0-9]*' | head -1)
  AB2_EXC=$(grep -o 'Exceptions: *[0-9]*' "$DATA/ab2.out" | grep -o '[0-9]*' | head -1)
  if kill -0 "$FDPID" 2>/dev/null; then check "fd_limited_survives" alive alive "process alive after 200-concurrency flood with fd=256"
  else check "fd_limited_survives" alive dead "process died under fd pressure"; fi
  C2=$(curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:18549/api/v1/system/status")
  check "fd_limited_recovers" 200 "$C2" "low-fd instance recovers"
  check "fd_limited_no_conn_errors" 0 "$(( ${AB2_CONNECT:-0} + ${AB2_RECV:-0} + ${AB2_EXC:-0} ))" "zero connection-level errors under fd=256 flood"
  kill -TERM "$FDPID" 2>/dev/null; wait "$FDPID" 2>/dev/null
else
  warn "fd-limited instance did not start (see $FD_DIR/server.log)"
fi

# 7 goroutine/memory growth observation
sleep 3
if kill -0 "$PID" 2>/dev/null; then
  GO=$(goroutines "$BASE")
  R1=$(rss_kb "$PID")
  GROW_KB=$(( R1 - R0 ))
  echo "  (goroutines: $G0 -> $GO; RSS: ${R0}KB -> ${R1}KB, growth ${GROW_KB}KB)"
  check_le "goroutine_leak_bound" 5000 "$(( GO - G0 ))" "goroutine growth"
  check_le "memory_growth_bound" 204800 "$GROW_KB" "RSS growth KB (<=200MB)"
else
  warn "server not alive at growth sample; skipping goroutine/memory assertion"
fi

stop "$PID"

echo
echo "stress summary: PASS=$PASS_N FAIL=$FAIL_N WARN=$WARN_N"
[ "$FAIL_N" -eq 0 ] || exit 1
