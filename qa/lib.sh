#!/bin/bash
# Shared helpers for NetBerth QA suites.
set -uo pipefail

seed_pass() { # data_dir — first-run admin password is printed once in server.log
  grep -o '"password":"[^"]*"' "$1/server.log" 2>/dev/null | head -1 | cut -d'"' -f4
}

gen_pass() { # prints a random alphanumeric password (24 chars)
  local p=""
  p=$(openssl rand -hex 12 2>/dev/null) || true
  if [ -z "$p" ]; then
    p=$(LC_ALL=C tr -dc 'a-zA-Z0-9' </dev/urandom 2>/dev/null | head -c 24) || true
  fi
  printf '%s' "$p"
}

free_port() { # prints a currently-free TCP port
  python3 - <<'PY'
import socket
s = socket.socket()
s.bind(('127.0.0.1', 0))
print(s.getsockname()[1])
s.close()
PY
}

wait_http() { # base_url
  local base=$1
  local opts=(-sf)
  case "$base" in
    https://*) opts=(-skf) ;;
  esac
  for _ in $(seq 1 80); do
    curl "${opts[@]}" "$base/api/v1/system/status" >/dev/null 2>&1 && return 0
    sleep 0.25
  done
  return 1
}

# prepare_instance BASE DATA_DIR [TARGET_PASS] [USERNAME]
# Fresh instances force a password change before protected APIs work. This
# performs the first-run flow: login with the seeded password, then change it
# to TARGET_PASS (default: $NB_QA_PASS or a random generated password) so all
# suites share one credential. No admin password is hardcoded anywhere.
prepare_instance() {
  local base=$1 dir=$2
  local target=${3:-${NB_QA_PASS:-$(gen_pass)}}
  local user=${4:-admin}
  local curl_opt=(-s)
  case "$base" in
    https://*) curl_opt=(-sk) ;;
  esac
  local seeded
  seeded=$(seed_pass "$dir")
  if [ -z "$seeded" ]; then
    echo "  [FATAL] no seeded admin password in $dir/server.log" >&2
    return 1
  fi
  local login body
  login=$(curl "${curl_opt[@]}" -X POST "$base/api/v1/auth/login" -H 'Content-Type: application/json' \
    -d "{\"username\":\"$user\",\"password\":\"$seeded\"}") || return 1
  local acc
  acc=$(printf '%s' "$login" | python3 -c \
    "import sys,json;print(json.load(sys.stdin)['data']['tokens']['access_token'])" 2>/dev/null) || return 1
  body=$(python3 -c "import json,sys;print(json.dumps({'old_password':sys.argv[1],'new_password':sys.argv[2]}))" "$seeded" "$target")
  local code
  code=$(curl "${curl_opt[@]}" -o /dev/null -w '%{http_code}' -X POST "$base/api/v1/auth/change-password" \
    -H 'Content-Type: application/json' -H "Authorization: Bearer $acc" -d "$body")
  if [ "$code" != "200" ]; then
    echo "  [FATAL] password change failed: HTTP $code" >&2
    return 1
  fi
  echo "$target"
}
