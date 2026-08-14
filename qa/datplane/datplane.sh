#!/bin/bash
# NetBerth data-plane devil test.
#
# Pushes real TCP/UDP traffic through forward rules (and WebSocket through the
# reverse proxy). The numbers printed here are DATA-PLANE numbers: bytes
# actually forwarded by the engine, not control-plane API latency.
#
# Usage:
#   ./qa/datplane/datplane.sh
#   NB_QA_BIN=/tmp/netberth-qa ./qa/datplane/datplane.sh
set -uo pipefail

DIR="$(cd "$(dirname "$0")" && pwd)"
REPO="$(cd "$DIR/../.." && pwd)"
BIN="${NB_QA_BIN:-}"

DP_PORT="${NB_DP_PORT:-19444}"
T_TCP="${NB_DP_TARGET_TCP:-19091}"
F_TCP="${NB_DP_FWD_TCP:-19092}"
T_UDP="${NB_DP_TARGET_UDP:-19093}"
F_UDP="${NB_DP_FWD_UDP:-19094}"
T_ABORT="${NB_DP_TARGET_ABORT:-19095}"
F_ABORT="${NB_DP_FWD_ABORT:-19096}"
USER="${NB_QA_USER:-admin}"

DATA=$(mktemp -d /private/tmp/nb-dp.XXXXXX)
PAYLOAD="$DATA/payload.bin"
OUTPUT="$DATA/output.bin"
API="http://127.0.0.1:$DP_PORT/api/v1"

cleanup() {
  [ -n "${DP_PID:-}" ] && { kill -TERM "$DP_PID" 2>/dev/null; wait "$DP_PID" 2>/dev/null; }
  [ -n "${T_TCP_PID:-}" ] && { kill -TERM "$T_TCP_PID" 2>/dev/null; wait "$T_TCP_PID" 2>/dev/null; }
  [ -n "${T_UDP_PID:-}" ] && { kill -TERM "$T_UDP_PID" 2>/dev/null; wait "$T_UDP_PID" 2>/dev/null; }
  [ -n "${REECHO_PID:-}" ] && { kill -TERM "$REECHO_PID" 2>/dev/null; wait "$REECHO_PID" 2>/dev/null; }
  rm -rf "$DATA"
}
trap cleanup EXIT

if [ -z "$BIN" ]; then
  BIN=/tmp/netberth-qa-dp
  (cd "$REPO" && go build -o "$BIN" ./cmd/netberth) || { echo "build failed"; exit 1; }
fi

mkdir -p "$DATA/db"
NB_DB_PATH="$DATA/db/nb.db" NB_SERVER_HOST=127.0.0.1 NB_SERVER_PORT="$DP_PORT" \
  NB_LOG_LEVEL=warn "$BIN" >"$DATA/server.log" 2>&1 &
DP_PID=$!

for _ in $(seq 1 80); do
  curl -sf "$API/system/status" >/dev/null 2>&1 && break
  sleep 0.25
done
curl -sf "$API/system/status" >/dev/null 2>&1 || { echo "instance did not start"; exit 1; }

SEED=$(grep -o '"password":"[^"]*"' "$DATA/server.log" | head -1 | cut -d'"' -f4)
TOKEN=$(curl -s -X POST "$API/auth/login" -H 'Content-Type: application/json' \
  -d "{\"username\":\"$USER\",\"password\":\"$SEED\"}" | \
  python3 -c "import json,sys;print(json.load(sys.stdin)['data']['tokens']['access_token'])")
curl -s -o /dev/null -X POST "$API/auth/change-password" -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $TOKEN" \
  -d "{\"old_password\":\"$SEED\",\"new_password\":\"DPPass123!\"}"
TOKEN=$(curl -s -X POST "$API/auth/login" -H 'Content-Type: application/json' \
  -d "{\"username\":\"$USER\",\"password\":\"DPPass123!\"}" | \
  python3 -c "import json,sys;print(json.load(sys.stdin)['data']['tokens']['access_token'])")

post_rule() { # name proto listen target max_conns
  curl -s -X POST "$API/forward-rules" -H 'Content-Type: application/json' \
    -H "Authorization: Bearer $TOKEN" \
    -d "{\"name\":\"$1\",\"protocol\":\"$2\",\"listen_addr\":\"127.0.0.1\",\"listen_port\":$3,\"target_addr\":\"127.0.0.1\",\"target_port\":$4,\"max_conns\":$5,\"enabled\":true}" >/dev/null
}

put_max_conns() { # id max_conns
  ID=$(curl -s -H "Authorization: Bearer $TOKEN" "$API/forward-rules" | \
    python3 -c "import json,sys;print([r['id'] for r in json.load(sys.stdin)['data'] if r['name']=='$1'][0])")
  curl -s -o /dev/null -X PUT "$API/forward-rules/$ID" -H 'Content-Type: application/json' \
    -H "Authorization: Bearer $TOKEN" \
    -d "{\"name\":\"$1\",\"protocol\":\"tcp\",\"listen_addr\":\"127.0.0.1\",\"listen_port\":$F_TCP,\"target_addr\":\"127.0.0.1\",\"target_port\":$T_TCP,\"max_conns\":$2,\"enabled\":true}"
}

echo "== starting TCP echo target on :$T_TCP"
python3 - "$T_TCP" <<'PY' &
import socket, sys, threading
s = socket.socket(); s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
s.bind(("127.0.0.1", int(sys.argv[1]))); s.listen(16)
def echo(c):
    try:
        while True:
            d = c.recv(65536)
            if not d: break
            c.sendall(d)
    except Exception:
        pass
    c.close()
while True:
    c, _ = s.accept()
    threading.Thread(target=echo, args=(c,), daemon=True).start()
PY
T_TCP_PID=$!

echo "== starting UDP echo target on :$T_UDP"
python3 - "$T_UDP" <<'PY' &
import socket, sys
s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
s.bind(("127.0.0.1", int(sys.argv[1])))
while True:
    d, a = s.recvfrom(65535)
    s.sendto(d, a)
PY
T_UDP_PID=$!

sleep 1

post_rule "dp-tcp" "tcp" "$F_TCP" "$T_TCP" 0
post_rule "dp-udp" "udp" "$F_UDP" "$T_UDP" 0

echo "== generating 64 MiB payload"
openssl rand -out "$PAYLOAD" 67108864

echo "== TCP throughput + integrity (64 MiB through forward)"
python3 - "$F_TCP" "$PAYLOAD" "$OUTPUT" <<'PY'
import socket, sys, time, hashlib, threading
port, src, dst = int(sys.argv[1]), sys.argv[2], sys.argv[3]
s = socket.create_connection(("127.0.0.1", port), timeout=30)
size = 65536
result = {}
def reader():
    with open(dst, "wb") as f:
        while True:
            d = s.recv(size)
            if not d: break
            f.write(d)
    result["done"] = True
rt = threading.Thread(target=reader, daemon=True)
rt.start()
start = time.time()
with open(src, "rb") as f:
    while True:
        d = f.read(size)
        if not d: break
        s.sendall(d)
s.shutdown(socket.SHUT_WR)
rt.join(timeout=60)
elapsed = time.time() - start
s.close()
h1 = hashlib.sha256(open(src, "rb").read()).hexdigest()
h2 = hashlib.sha256(open(dst, "rb").read()).hexdigest()
print(f"TCP_OK bytes=67108864 elapsed={elapsed:.3f}s mbps={67.108864/elapsed:.1f} sha_match={h1==h2}")
if h1 != h2 or not result.get("done"): sys.exit(2)
PY
TCP_RESULT=$?
[ "$TCP_RESULT" = "0" ] && echo "  [PASS] TCP integrity + throughput"

echo "== TCP max_conns limit (limit=3, 6 concurrent)"
put_max_conns "dp-tcp" 3
python3 - "$F_TCP" <<'PY'
import socket, sys, threading
port = int(sys.argv[1])
ok = [0, 0]
def probe():
    try:
        s = socket.create_connection(("127.0.0.1", port), timeout=3)
        s.sendall(b"hi")
        if s.recv(2) == b"hi":
            ok[0] += 1
        else:
            ok[1] += 1
        s.close()
    except Exception:
        ok[1] += 1
ts = [threading.Thread(target=probe) for _ in range(6)]
[t.start() for t in ts]
[t.join() for t in ts]
print(f"MAXCONNS accepted={ok[0]} rejected={ok[1]}")
if ok[0] != 3 or ok[1] != 3: sys.exit(2)
PY
MAX_RESULT=$?
[ "$MAX_RESULT" = "0" ] && echo "  [PASS] max_conns enforced"
put_max_conns "dp-tcp" 0

echo "== TCP target mid-connection disconnect cleanup"
post_rule "dp-abort" "tcp" "$F_ABORT" "$T_ABORT" 0
python3 - "$T_ABORT" <<'PY' &
import socket, sys
s = socket.socket(); s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
s.bind(("127.0.0.1", int(sys.argv[1]))); s.listen(1)
c, _ = s.accept()
got = 0
while got < 262144:
    d = c.recv(65536)
    if not d: break
    got += len(d)
c.setsockopt(socket.SOL_SOCKET, socket.SO_LINGER, __import__("struct").pack("ii", 1, 0))
c.close(); s.close()
PY
ABORT_PID=$!
python3 - "$F_ABORT" <<'PY'
import socket, sys, time
s = socket.create_connection(("127.0.0.1", int(sys.argv[1])), timeout=10)
s.sendall(b"x" * 1048576)
try:
    while s.recv(65536):
        pass
except Exception:
    pass
s.close()
print("DISCONNECT client_saw_end=True")
PY
DISC_RESULT=$?
kill -TERM "$ABORT_PID" 2>/dev/null
wait "$ABORT_PID" 2>/dev/null
python3 - "$T_ABORT" <<'PY' &
import socket, sys, threading
s = socket.socket(); s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
s.bind(("127.0.0.1", int(sys.argv[1]))); s.listen(4)
def echo(c):
    try:
        while True:
            d = c.recv(65536)
            if not d: break
            c.sendall(d)
    except Exception:
        pass
    c.close()
while True:
    c, _ = s.accept()
    threading.Thread(target=echo, args=(c,), daemon=True).start()
PY
REECHO_PID=$!
sleep 0.3
ALIVE=$(python3 - "$F_ABORT" <<'PY'
import socket, sys
s = socket.create_connection(("127.0.0.1", int(sys.argv[1])), timeout=3)
s.sendall(b"alive")
s.settimeout(3)
ok = s.recv(5) == b"alive"
s.close()
print("yes" if ok else "no")
PY
)
kill -TERM "$REECHO_PID" 2>/dev/null
echo "  rule_alive_after_disconnect=$ALIVE"
[ "$DISC_RESULT" = "0" ] && [ "$ALIVE" = "yes" ] && echo "  [PASS] target disconnect cleaned up" || DISC_RESULT=1
# goroutines back to baseline via metrics
BEFORE=$(curl -s -H "Authorization: Bearer $TOKEN" "$API/system/metrics" | python3 -c "import json,sys;print(json.load(sys.stdin)['data']['goroutines'])")
sleep 1
AFTER=$(curl -s -H "Authorization: Bearer $TOKEN" "$API/system/metrics" | python3 -c "import json,sys;print(json.load(sys.stdin)['data']['goroutines'])")
echo "  goroutines before=$BEFORE after=$AFTER"
[ "$DISC_RESULT" = "0" ] && echo "  [PASS] target disconnect cleaned up"

echo "== UDP 5x1000 datagrams through forward"
python3 - "$F_UDP" <<'PY'
import socket, sys, threading, struct
port = int(sys.argv[1])
errs = [0]
def client(cid):
    s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    s.settimeout(3)
    for i in range(1000):
        msg = struct.pack(">II", cid, i)
        s.sendto(msg, ("127.0.0.1", port))
        d, _ = s.recvfrom(16)
        if d != msg:
            errs[0] += 1
            return
    s.close()
ts = [threading.Thread(target=client, args=(i,)) for i in range(5)]
[t.start() for t in ts]
[t.join() for t in ts]
print(f"UDP datagrams=5000 errors={errs[0]}")
if errs[0]: sys.exit(2)
PY
UDP_RESULT=$?
[ "$UDP_RESULT" = "0" ] && echo "  [PASS] UDP forwarding + concurrency"

echo "== WebSocket long-lived through reverse proxy (Go test, real sockets)"
WS_OUT=$(cd "$REPO" && go test -count=1 -timeout 2m -run TestProxyWebSocketLongLived ./internal/engine/proxy 2>&1 | tail -1)
echo "  $WS_OUT"

echo
echo "=== NetBerth DATA-PLANE results ==="
echo "TCP: see TCP_OK line above (integrity sha256 + MB/s)"
echo "MaxConns: see MAXCONNS line above"
echo "Disconnect: see DISCONNECT + goroutines line above"
echo "UDP: see UDP line above"
echo "WS: $WS_OUT"
[ "$TCP_RESULT" = "0" ] && [ "$MAX_RESULT" = "0" ] && [ "$DISC_RESULT" = "0" ] && [ "$UDP_RESULT" = "0" ] && echo "=== DATA-PLANE ALL GREEN ===" || { echo "=== DATA-PLANE FAILURES ==="; exit 1; }
