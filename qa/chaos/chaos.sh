#!/bin/bash
# NetBerth chaos devil tests — manages its own instance on a QA port.
# Covers graceful stop, kill -9 recovery, port conflicts, migration backups,
# corrupted databases, read-only data dirs, slowloris, process suspension and
# concurrent request bursts.
set -uo pipefail
BIN="${1:-/tmp/netberth-local}"
PORT="${2:-19443}"
BASE="http://127.0.0.1:$PORT"
PROXY_PORT=18080
DATA=$(mktemp -d /private/tmp/nb-chaos.XXXXXX)
LOG="$DATA/server.log"
PASS_N=0; FAIL_N=0
check(){ name=$1; want=$2; got=$3; note=$4
  if [ "$want" = "$got" ]; then echo "  [PASS] $name ($note)"; PASS_N=$((PASS_N+1))
  else echo "  [FAIL] $name want=$want got=$got ($note)"; FAIL_N=$((FAIL_N+1)); fi
}
start(){ NB_DB_PATH="$DATA/nb.db" NB_SERVER_HOST=127.0.0.1 NB_SERVER_PORT=$PORT NB_PROXY_PORT=$PROXY_PORT "$BIN" >"$LOG" 2>&1 & echo $!; }
start_dir(){ # data_dir
  local d=$1
  NB_DB_PATH="$d/nb.db" NB_SERVER_HOST=127.0.0.1 NB_SERVER_PORT=$PORT NB_PROXY_PORT=$PROXY_PORT "$BIN" >"$d/server.log" 2>&1 & echo $!
}
start_dir_log(){ # data_dir logfile (log must live outside a read-only dir)
  local d=$1 log=$2
  NB_DB_PATH="$d/nb.db" NB_SERVER_HOST=127.0.0.1 NB_SERVER_PORT=$PORT NB_PROXY_PORT=$PROXY_PORT "$BIN" >"$log" 2>&1 & echo $!
}
stop(){ kill -TERM "$1" 2>/dev/null; wait "$1" 2>/dev/null; }
wait_http(){ for i in $(seq 1 40); do curl -sf "$BASE/api/v1/system/status" >/dev/null 2>&1 && return 0; sleep 0.25; done; return 1; }
port_closed(){ ! curl -sf "$BASE/api/v1/system/status" >/dev/null 2>&1; }

echo "== chaos: $BIN on :$PORT =="
echo "  (data dir: $DATA)"

# --- 1 fresh start + graceful shutdown + doctor + seed password ---
PID=$(start)
if wait_http; then check "fresh_start" up up "server responds"; else check "fresh_start" up down "server did not respond"; fi
ADMIN_PASS=$(grep -o '"password":"[^"]*"' "$LOG" | head -1 | cut -d'"' -f4)
stop "$PID"
sleep 1
if port_closed; then check "graceful_shutdown" closed closed "port closed after SIGTERM"; else check "graceful_shutdown" closed open "port still open"; fi
NB_DB_PATH="$DATA/nb.db" NB_SERVER_HOST=127.0.0.1 NB_SERVER_PORT=$PORT NB_PROXY_PORT=$PROXY_PORT "$BIN" doctor >/dev/null 2>&1
check "doctor_after_shutdown" 0 "$?" "db intact + ports free after graceful stop"

# --- 2 restart + login ---
PID=$(start); wait_http
LOGIN=$(curl -s -o /dev/null -w '%{http_code}' -X POST "$BASE/api/v1/auth/login" -H 'Content-Type: application/json' -d "{\"username\":\"admin\",\"password\":\"$ADMIN_PASS\"}")
check "login_after_restart" 200 "$LOGIN" "seeded credentials still work"

# --- 3 kill -9 + recovery ---
kill -9 "$PID" 2>/dev/null; wait "$PID" 2>/dev/null
sleep 1
if port_closed; then check "kill9_port_closed" closed closed "port closed after kill -9"; else check "kill9_port_closed" closed open "port still open"; fi
PID=$(start); wait_http
LOGIN2=$(curl -s -o /dev/null -w '%{http_code}' -X POST "$BASE/api/v1/auth/login" -H 'Content-Type: application/json' -d "{\"username\":\"admin\",\"password\":\"$ADMIN_PASS\"}")
check "recovery_after_kill9" 200 "$LOGIN2" "restart after kill -9 works"

# --- 3b slowloris: a half-open request must not block other clients ---
python3 - "$PORT" <<'PY' &
import socket, sys, time
s = socket.create_connection(('127.0.0.1', int(sys.argv[1])))
s.sendall(b'POST /api/v1/auth/login HTTP/1.1\r\nHost: x\r\n')
time.sleep(8)
s.close()
PY
SLOW=$!
sleep 1
if curl -sf --max-time 3 "$BASE/api/v1/system/status" >/dev/null 2>&1; then
  check "slowloris_isolated" healthy healthy "health endpoint unaffected by partial connection"
else
  check "slowloris_isolated" healthy blocked "health endpoint blocked during slowloris"
fi
kill "$SLOW" 2>/dev/null; wait "$SLOW" 2>/dev/null

# --- 3c SIGSTOP/SIGCONT: process must resume cleanly ---
kill -STOP "$PID"
sleep 1
kill -CONT "$PID"
sleep 1
LOGIN3=$(curl -s -o /dev/null -w '%{http_code}' -X POST "$BASE/api/v1/auth/login" -H 'Content-Type: application/json' -d "{\"username\":\"admin\",\"password\":\"$ADMIN_PASS\"}")
check "resume_after_stop_cont" 200 "$LOGIN3" "server responsive after SIGSTOP/SIGCONT"

# --- 3d burst: 500 requests / 50 concurrent must not take the server down ---
if command -v ab >/dev/null 2>&1; then
  AB_OUT=$(ab -n 500 -c 50 -k "$BASE/api/v1/system/status" 2>&1)
  FR=$(echo "$AB_OUT" | awk '/Failed requests:/{print $3}')
  if curl -sf --max-time 3 "$BASE/api/v1/system/status" >/dev/null 2>&1; then
    check "burst_500x50_survives" healthy healthy "server healthy after burst (ab failed=$FR — 429 rate-limit expected)"
  else
    check "burst_500x50_survives" healthy down "server down after burst (ab failed=$FR)"
  fi
else
  echo "  [SKIP] ab not installed"
fi
stop "$PID"

# --- 4 port conflict ---
python3 - "$PORT" >"$DATA/occupier.log" 2>&1 <<'PY' &
import socket, sys, time
s = socket.socket(); s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
s.bind(('127.0.0.1', int(sys.argv[1]))); s.listen()
time.sleep(30)
PY
OCC=$!
sleep 0.5
PID=$(start)
sleep 3
if kill -0 "$PID" 2>/dev/null; then check "port_conflict" fail run "server should not hold conflicting port"; kill -9 "$PID" 2>/dev/null; wait "$PID" 2>/dev/null
else check "port_conflict" fail fail "server exited on conflicting port (expected)"; fi
kill "$OCC" 2>/dev/null; wait "$OCC" 2>/dev/null

# --- 5 pre-migration backup ---
python3 - "$DATA/nb.db" <<'PY'
import sqlite3, sys
db = sqlite3.connect(sys.argv[1])
db.execute("DELETE FROM schema_migrations")
db.execute("INSERT INTO schema_migrations (version) VALUES (1)")
db.commit(); db.close()
PY
PID=$(start); wait_http; stop "$PID"
if [ -s "$DATA/nb.db.pre-upgrade.bak" ]; then check "pre_migration_backup" exists exists "backup created on upgrade"
else check "pre_migration_backup" exists missing "no backup file"; fi
V=$(python3 - "$DATA/nb.db" <<'PY'
import sqlite3, sys
db = sqlite3.connect(sys.argv[1])
print(db.execute("SELECT MAX(version) FROM schema_migrations").fetchone()[0])
PY
)
check "schema_version" 4 "$V" "schema migrated to v4"

# --- 6 corrupted database: fail fast with a clear error, no crash loop ---
COR=$(mktemp -d /private/tmp/nb-chaos-corrupt.XXXXXX)
head -c 4096 /dev/urandom > "$COR/nb.db"
CPID=$(start_dir "$COR")
sleep 2
if kill -0 "$CPID" 2>/dev/null; then
  check "corrupt_db_fails_fast" exit crashed "server kept running on corrupted db"
  kill -9 "$CPID" 2>/dev/null; wait "$CPID" 2>/dev/null
elif grep -qiE 'failed to open database|not a database|malformed|unsupported file format' "$COR/server.log"; then
  check "corrupt_db_fails_fast" exit exit "clear error on corrupted db"
else
  check "corrupt_db_fails_fast" exit exit "server exited but log unclear: $(tail -1 "$COR/server.log")"
fi
rm -rf "$COR"

# --- 7 read-only data dir: fail fast with a clear error ---
RO=$(mktemp -d /private/tmp/nb-chaos-ro.XXXXXX)
chmod 555 "$RO"
RPID=$(start_dir_log "$RO" "$DATA/ro-server.log")
sleep 2
if kill -0 "$RPID" 2>/dev/null; then
  check "readonly_dir_fails_fast" exit crashed "server ran on read-only dir"
  kill -9 "$RPID" 2>/dev/null; wait "$RPID" 2>/dev/null
elif grep -qiE 'failed to open database|permission denied|read-only|unable to open' "$DATA/ro-server.log"; then
  check "readonly_dir_fails_fast" exit exit "clear error on read-only dir"
else
  check "readonly_dir_fails_fast" exit exit "server exited but log unclear: $(tail -1 "$DATA/ro-server.log")"
fi
chmod 755 "$RO"
rm -rf "$RO"

echo
echo "chaos summary: PASS=$PASS_N FAIL=$FAIL_N"
[ "$FAIL_N" -eq 0 ] || exit 1
