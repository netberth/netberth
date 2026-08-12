#!/bin/bash
# NetBerth QA runner — isolated simulated environment.
#
# Every suite gets a fresh instance + data dir on its own port, so results
# are reproducible, brute-force lockouts cannot leak between suites, and the
# user's demo instance is never touched.
set -uo pipefail
DIR="$(cd "$(dirname "$0")" && pwd)"
REPO="$(cd "$DIR/.." && pwd)"

BIN="${NB_QA_BIN:-}"
USER="${NB_QA_USER:-admin}"
if [ -z "${NB_QA_PASS:-}" ]; then
  PASS=$(openssl rand -hex 12 2>/dev/null) || PASS=$(LC_ALL=C tr -dc 'a-zA-Z0-9' </dev/urandom 2>/dev/null | head -c 24) || true
  export NB_QA_PASS="$PASS"
else
  PASS="$NB_QA_PASS"
fi
KEEP="${NB_QA_KEEP:-0}"

SEC_PORT="${NB_QA_SEC_PORT:-18444}"
LOAD_PORT="${NB_QA_LOAD_PORT:-18445}"
E2E_PORT="${NB_QA_E2E_PORT:-18446}"
BND_PORT="${NB_QA_BND_PORT:-18544}"
SOAK_PORT="${NB_QA_SOAK_PORT:-18545}"
CHAOS_PORT="${NB_QA_CHAOS_PORT:-19443}"
SOAK="${NB_QA_SOAK:-0}"

FAILED=0

if [ -z "$BIN" ]; then
  BIN=/tmp/netberth-qa
  echo "== building QA binary -> $BIN =="
  (cd "$REPO" && go build -o "$BIN" ./cmd/netberth) || { echo "build failed"; exit 1; }
fi

start_instance() { # port data_dir
  local port=$1 dir=$2
  mkdir -p "$dir"
  NB_DB_PATH="$dir/nb.db" NB_SERVER_HOST=127.0.0.1 NB_SERVER_PORT="$port" \
    NB_PROXY_PORT=18081 NB_LOG_LEVEL=warn "$BIN" >"$dir/server.log" 2>&1 &
  echo $!
}

wait_http() { # base_url
  for _ in $(seq 1 80); do
    curl -sf "$1/api/v1/system/status" >/dev/null 2>&1 && return 0
    sleep 0.25
  done
  return 1
}

seed_pass() { # data_dir — first-run admin password printed in server.log
  grep -o '"password":"[^"]*"' "$1/server.log" 2>/dev/null | head -1 | cut -d'"' -f4
}

prepare_instance() { # base_url data_dir new_pass
  local base=$1 dir=$2 newpass=$3 seed tok rc
  seed=$(seed_pass "$dir")
  [ -n "$seed" ] || return 1
  tok=$(curl -s -X POST "$base/api/v1/auth/login" -H 'Content-Type: application/json' \
    -d "{\"username\":\"$USER\",\"password\":\"$seed\"}" | \
    python3 -c "import json,sys;print(json.load(sys.stdin)['data']['tokens']['access_token'])") || return 1
  rc=$(curl -s -o /dev/null -w '%{http_code}' -X POST "$base/api/v1/auth/change-password" \
    -H 'Content-Type: application/json' -H "Authorization: Bearer $tok" \
    -d "{\"old_password\":\"$seed\",\"new_password\":\"$newpass\"}")
  [ "$rc" = "200" ] || return 1
  rc=$(curl -s -o /dev/null -w '%{http_code}' -X POST "$base/api/v1/auth/login" \
    -H 'Content-Type: application/json' -d "{\"username\":\"$USER\",\"password\":\"$newpass\"}")
  [ "$rc" = "200" ]
}

stop_instance() { # pid
  kill -TERM "$1" 2>/dev/null
  wait "$1" 2>/dev/null
}

cleanup_dir() { # dir
  if [ "$KEEP" = "1" ]; then
    echo "  (keeping data at $1)"
  else
    rm -rf "$1"
  fi
}

echo "=== NetBerth QA (isolated simulated environment) ==="
echo "targets: security=:$SEC_PORT load=:$LOAD_PORT e2e=:$E2E_PORT boundary=:$BND_PORT"
echo "        smoke/stress/sim=self-contained chaos=:$CHAOS_PORT soak=:$SOAK_PORT(optional)"
echo "user=$USER bin=$BIN"

echo
echo "--- [1/8] security ---"
SEC_DIR=$(mktemp -d /private/tmp/nb-qa-sec.XXXXXX)
SEC_PID=$(start_instance "$SEC_PORT" "$SEC_DIR")
if wait_http "http://127.0.0.1:$SEC_PORT"; then
  SEC_PASS=$(seed_pass "$SEC_DIR")
  SEC_PASS="${SEC_PASS:-$PASS}"
  "$DIR/security/security.sh" "http://127.0.0.1:$SEC_PORT" "$USER" "$SEC_PASS" || FAILED=1
else
  echo "  [FATAL] security instance did not start (see $SEC_DIR/server.log)"
  FAILED=1
fi
stop_instance "$SEC_PID"
cleanup_dir "$SEC_DIR"

echo
echo "--- [2/8] load (k6) ---"
LOAD_DIR=$(mktemp -d /private/tmp/nb-qa-load.XXXXXX)
# Throughput test: disable the per-IP rate limiter so k6 measures raw HTTP/WS
# capacity. Rate limiting itself is covered by security + chaos suites.
export NB_RATE_LIMIT_RATE=100000 NB_RATE_LIMIT_BURST=200000
LOAD_PID=$(start_instance "$LOAD_PORT" "$LOAD_DIR")
unset NB_RATE_LIMIT_RATE NB_RATE_LIMIT_BURST
if wait_http "http://127.0.0.1:$LOAD_PORT"; then
  if command -v k6 >/dev/null 2>&1; then
    if prepare_instance "http://127.0.0.1:$LOAD_PORT" "$LOAD_DIR" "$PASS"; then
      k6 run "$DIR/load/load.js" --env BASE="http://127.0.0.1:$LOAD_PORT" --env USER="$USER" --env PASS="$PASS" || FAILED=1
    else
      echo "  [FATAL] could not prepare load instance (forced password change)"
      FAILED=1
    fi
  else
    echo "  k6 not installed; skip load test"
  fi
else
  echo "  [FATAL] load instance did not start (see $LOAD_DIR/server.log)"
  FAILED=1
fi
stop_instance "$LOAD_PID"
cleanup_dir "$LOAD_DIR"

echo
echo "--- [3/8] e2e (Playwright) ---"
E2E_DIR=$(mktemp -d /private/tmp/nb-qa-e2e.XXXXXX)
E2E_PID=$(start_instance "$E2E_PORT" "$E2E_DIR")
if wait_http "http://127.0.0.1:$E2E_PORT"; then
  if command -v playwright >/dev/null 2>&1; then
    if prepare_instance "http://127.0.0.1:$E2E_PORT" "$E2E_DIR" "$PASS"; then
      NB_QA_BASE="http://127.0.0.1:$E2E_PORT" NB_QA_USER="$USER" NB_QA_PASS="$PASS" \
        NODE_PATH="$(npm root -g)" playwright test --config "$DIR/e2e/playwright.config.js" || FAILED=1
    else
      echo "  [FATAL] could not prepare e2e instance (forced password change)"
      FAILED=1
    fi
  else
    echo "  playwright not installed; skip e2e test"
  fi
else
  echo "  [FATAL] e2e instance did not start (see $E2E_DIR/server.log)"
  FAILED=1
fi
stop_instance "$E2E_PID"
cleanup_dir "$E2E_DIR"

echo
echo "--- [4/8] chaos (own instance on :$CHAOS_PORT) ---"
"$DIR/chaos/chaos.sh" "$BIN" "$CHAOS_PORT" || FAILED=1

echo
echo "--- [5/8] boundary/fuzz (own instance on :$BND_PORT) ---"
BND_DIR=$(mktemp -d /private/tmp/nb-qa-bnd.XXXXXX)
BND_PID=$(start_instance "$BND_PORT" "$BND_DIR")
if wait_http "http://127.0.0.1:$BND_PORT"; then
  BND_PASS=$(seed_pass "$BND_DIR")
  BND_PASS="${BND_PASS:-$PASS}"
  "$DIR/boundary/boundary.sh" "http://127.0.0.1:$BND_PORT" "$USER" "$BND_PASS" || FAILED=1
else
  echo "  [FATAL] boundary instance did not start (see $BND_DIR/server.log)"
  FAILED=1
fi
stop_instance "$BND_PID"
cleanup_dir "$BND_DIR"

echo
echo "--- [6/8] smoke (self-contained :18553) ---"
"$DIR/smoke/smoke.sh" "$BIN" || FAILED=1

echo
echo "--- [7/8] stress (self-contained :18547/:18549) ---"
"$DIR/stress/stress.sh" "$BIN" || FAILED=1

echo
echo "--- [8/8] sim matrix (self-contained TLS/proxy/constrained) ---"
"$DIR/sim/sim.sh" "$BIN" || FAILED=1

if [ "$SOAK" = "1" ]; then
  echo
  echo "--- [extra] soak (k6, own instance on :$SOAK_PORT) ---"
  SOAK_DIR=$(mktemp -d /private/tmp/nb-qa-soak.XXXXXX)
  export NB_RATE_LIMIT_RATE=100000 NB_RATE_LIMIT_BURST=200000
  SOAK_PID=$(start_instance "$SOAK_PORT" "$SOAK_DIR")
  unset NB_RATE_LIMIT_RATE NB_RATE_LIMIT_BURST
  if wait_http "http://127.0.0.1:$SOAK_PORT"; then
    if command -v k6 >/dev/null 2>&1; then
      if prepare_instance "http://127.0.0.1:$SOAK_PORT" "$SOAK_DIR" "$PASS"; then
        k6 run "$DIR/soak/soak.js" --env BASE="http://127.0.0.1:$SOAK_PORT" \
          --env USER="$USER" --env PASS="$PASS" --env SOAK_SECONDS="${NB_QA_SOAK_SECONDS:-600}" || FAILED=1
      else
        echo "  [FATAL] could not prepare soak instance"
        FAILED=1
      fi
    else
      echo "  k6 not installed; skip soak"
    fi
  else
    echo "  [FATAL] soak instance did not start (see $SOAK_DIR/server.log)"
    FAILED=1
  fi
  stop_instance "$SOAK_PID"
  cleanup_dir "$SOAK_DIR"
fi

echo
if [ "$FAILED" -eq 0 ]; then
  echo "=== QA ALL GREEN ==="
else
  echo "=== QA HAS FAILURES (see above) ==="
  exit 1
fi
