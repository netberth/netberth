#!/bin/bash
# NetBerth HTTP boundary/fuzz devil tests — raw TCP + curl against a QA instance.
# Covers malformed requests, oversized headers, chunked smuggling, slowloris,
# path traversal, invalid UTF-8, method abuse and connection hygiene.
set -uo pipefail
BASE="${1:-http://127.0.0.1:18544}"
TMP=$(mktemp -d /private/tmp/nb-bnd.XXXXXX)
trap 'rm -rf "$TMP"' EXIT
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

HOST=$(python3 -c "from urllib.parse import urlparse; print(urlparse('$BASE').hostname or '127.0.0.1')")
PORT=$(python3 -c "from urllib.parse import urlparse; print(urlparse('$BASE').port or 80)")

raw(){ # raw <label> <bytes> — returns HTTP status code or ERR:<type>
  python3 - "$HOST" "$PORT" "$2" <<'PY'
import socket, sys
host, port, data = sys.argv[1], int(sys.argv[2]), sys.argv[3].encode('latin1')
s = socket.create_connection((host, port), timeout=5)
s.sendall(data)
s.settimeout(5)
try:
    out = b''
    while True:
        chunk = s.recv(4096)
        if not chunk:
            break
        out += chunk
        if b'\r\n\r\n' in out:
            break
    first = out.split(b'\r\n', 1)[0].decode('latin1', 'replace')
    parts = first.split(' ', 2)
    print(parts[1] if len(parts) > 1 else 'NO_STATUS')
except Exception as e:
    print('ERR:' + type(e).__name__)
finally:
    s.close()
PY
}

echo "== boundary/fuzz: $BASE =="

# 1 request line abuse
c=$(raw bad_version $'GET / HTTP/9.9\r\nHost: qa.local\r\n\r\n')
check_in "request_bad_version" "400|505|ERR:" "$c" "HTTP/9.9 rejected"

c=$(raw missing_host $'GET / HTTP/1.1\r\n\r\n')
check_in "request_missing_host" "400|ERR:" "$c" "HTTP/1.1 without Host rejected"

c=$(raw nul_url $'GET /%00 HTTP/1.1\r\nHost: qa.local\r\n\r\n')
check_in "request_nul_url" "200|400|404" "$c" "NUL byte URL handled without crash"

# 2 header injection: bare CR inside a header value must be rejected
c=$(raw header_crlf $'GET / HTTP/1.1\r\nHost: qa.local\r\nX-Foo: a\rSet-Cookie: evil=1\rX-Bar: b\r\n\r\n')
check_in "header_crlf_injection" "400|ERR:" "$c" "control chars in header value rejected"

# 3 oversized header (70 KB > 64 KB MaxHeaderBytes)
BIG=$(python3 -c "print('A'*70000)")
c=$(raw big_header "GET / HTTP/1.1\r\nHost: qa.local\r\nX-Big: $BIG\r\n\r\n")
check_in "oversized_header" "431|400|ERR:" "$c" "70KB header rejected"

# 4 chunked / smuggling
c=$(raw bad_chunk $'POST /api/v1/auth/login HTTP/1.1\r\nHost: qa.local\r\nContent-Type: application/json\r\nTransfer-Encoding: chunked\r\n\r\nZZZ\r\n\r\n')
check_in "bad_chunked" "400|ERR:" "$c" "invalid chunk size rejected"

c=$(raw clte_conflict $'POST /api/v1/auth/login HTTP/1.1\r\nHost: qa.local\r\nContent-Type: application/json\r\nContent-Length: 5\r\nTransfer-Encoding: chunked\r\n\r\n0\r\n\r\n')
check_in "clte_conflict" "400|501|ERR:" "$c" "CL+TE conflict rejected"

c=$(raw short_body $'POST /api/v1/auth/login HTTP/1.1\r\nHost: qa.local\r\nContent-Type: application/json\r\nContent-Length: 100\r\n\r\nabc')
check_in "content_length_mismatch" "400|408|ERR:TimeoutError|ERR:ConnectionResetError" "$c" "short body handled without crash (server may wait for remaining bytes)"

# 5 protocol quirks
c=$(raw http10 $'GET /api/v1/system/status HTTP/1.0\r\n\r\n')
check_in "http10_status" "200" "$c" "HTTP/1.0 request served"

# 6 method abuse
for m in TRACE CONNECT PATCH; do
  c=$(curl -s -o /dev/null -w '%{http_code}' -X "$m" "$BASE/api/v1/system/status" 2>/dev/null)
  check_in "method_${m}_status" "400|404|405|501" "$c" "$m rejected"
done

# 7 path traversal must never leak files outside the app root
for p in "/../../etc/passwd" "/..%2f..%2fetc%2fpasswd" "/%2e%2e/%2e%2e/etc/passwd" "/static/../../etc/passwd"; do
  c=$(curl -s -o "$TMP/body" -w '%{http_code}' --path-as-is "$BASE$p" 2>/dev/null)
  if [ "$c" = "200" ] && grep -q "root:" "$TMP/body" 2>/dev/null; then
    check "traversal_no_leak" "noleak" "LEAK" "$p leaked file content"
  else
    check_in "traversal_no_leak" "200|400|404" "$c" "$p safe (SPA fallback or blocked)"
  fi
done
c=$(curl -s -o /dev/null -w '%{http_code}' --path-as-is "$BASE/api/v1/../../etc/passwd")
check_in "traversal_api_blocked" "400|404" "$c" "API path traversal blocked"

# 8 malformed JSON / invalid UTF-8 / wrong types must be 400 (429 = rate limited is acceptable)
sleep 1
c=$(curl -s -o /dev/null -w '%{http_code}' -X POST "$BASE/api/v1/auth/login" -H 'Content-Type: application/json' --data-binary '[]' 2>/dev/null)
check_in "json_array_login" "400|429" "$c" "JSON array rejected"
c=$(curl -s -o /dev/null -w '%{http_code}' -X POST "$BASE/api/v1/auth/login" -H 'Content-Type: application/json' --data-binary '"string"' 2>/dev/null)
check_in "json_string_login" "400|429" "$c" "JSON string rejected"
c=$(curl -s -o /dev/null -w '%{http_code}' -X POST "$BASE/api/v1/auth/login" -H 'Content-Type: application/json' --data-binary 'null' 2>/dev/null)
check_in "json_null_login" "400|429" "$c" "JSON null rejected"
python3 -c "open('$TMP/invalid_utf8.json','wb').write(b'{\"username\":\"\xff\xfe\",\"password\":\"x\"}')"
c=$(curl -s -o /dev/null -w '%{http_code}' -X POST "$BASE/api/v1/auth/login" -H 'Content-Type: application/json' --data-binary @"$TMP/invalid_utf8.json" 2>/dev/null)
check_in "json_invalid_utf8" "400|401|429" "$c" "invalid UTF-8 handled (rejected or clean 401)"

# 9 long query string must stay within limits
c=$(curl -s -o /dev/null -w '%{http_code}' "$BASE/api/v1/system/status?$(python3 -c "print('k'*5000+'=1')")" 2>/dev/null)
check "long_query" 200 "$c" "5KB query string accepted"

# 10 websocket upgrade without handshake
c=$(curl -s -o /dev/null -w '%{http_code}' -H 'Upgrade: websocket' -H 'Connection: Upgrade' "$BASE/api/v1/ws" 2>/dev/null)
check_in "ws_bad_handshake" "400|426" "$c" "incomplete WS upgrade rejected"

# 11 slowloris: server must cut off a partial-header connection near
# ReadHeaderTimeout (5s default) while remaining responsive.
python3 - "$HOST" "$PORT" <<'PY' >"$TMP/slow.out" 2>&1
import socket, sys, time
host, port = sys.argv[1], int(sys.argv[2])
s = socket.create_connection((host, port), timeout=10)
s.sendall(b'GET / HTTP/1.1\r\nHost: qa.local\r\nX-Slow: ')
s.settimeout(9)
start = time.time()
try:
    data = s.recv(128)
    elapsed = time.time() - start
    if not data:
        print('CLOSED %.1f' % elapsed)
    else:
        print('DATA %.1f %r' % (elapsed, data[:20]))
except Exception as e:
    print('CLOSED %.1f (%s)' % (time.time() - start, type(e).__name__))
finally:
    s.close()
PY
SLOW_RESULT=$(cat "$TMP/slow.out")
case "$SLOW_RESULT" in
  CLOSED*)
    SLOW_T=$(echo "$SLOW_RESULT" | grep -o '[0-9.]*' | head -1)
    if python3 -c "exit(0 if float('$SLOW_T') <= 8 else 1)"; then
      echo "  [PASS] slowloris_cutoff (closed in ${SLOW_T}s, target <=8s)"; PASS_N=$((PASS_N+1))
    else
      warn "slowloris closed late at ${SLOW_T}s"
    fi
    ;;
  DATA*)
    warn "slowloris got data instead of close: $SLOW_RESULT"
    ;;
  *)
    warn "slowloris unexpected: $SLOW_RESULT"
    ;;
esac
c=$(curl -s -o /dev/null -w '%{http_code}' "$BASE/api/v1/system/status" 2>/dev/null)
check "server_alive_after_slowloris" 200 "$c" "server responsive after slowloris"

# 12 zero-length and empty JSON body
c=$(curl -s -o /dev/null -w '%{http_code}' -X POST "$BASE/api/v1/auth/login" -H 'Content-Type: application/json' --data-binary '' 2>/dev/null)
check_in "empty_body_login" "400|429" "$c" "empty body rejected"

echo
echo "boundary summary: PASS=$PASS_N FAIL=$FAIL_N WARN=$WARN_N"
[ "$FAIL_N" -eq 0 ] || exit 1
