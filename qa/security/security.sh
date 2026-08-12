#!/bin/bash
# NetBerth API security devil tests (curl-based)
set -uo pipefail
BASE="${1:-http://127.0.0.1:18444}"
USER="${2:-admin}"
PASS="${3:-${NB_QA_PASS:-}}"
TMP=$(mktemp -d /private/tmp/nb-sec.XXXXXX)
trap 'rm -rf "$TMP"' EXIT
if [ -z "$PASS" ]; then
  echo "  [FATAL] no password: pass it as argument 3 or set NB_QA_PASS" >&2
  exit 1
fi
PASS_N=0; FAIL_N=0; WARN_N=0

check(){ name=$1; want=$2; got=$3; note=$4
  ok=0
  if [ "$want" = "$got" ]; then
    ok=1
  elif [[ "$want" == *or* ]]; then
    for w in $(echo "$want" | sed 's/or/ /g'); do
      [ "$w" = "$got" ] && ok=1
    done
  fi
  if [ "$ok" = "1" ]; then echo "  [PASS] $name ($note)"; PASS_N=$((PASS_N+1))
  else echo "  [FAIL] $name want=$want got=$got ($note)"; FAIL_N=$((FAIL_N+1)); fi
}
warn(){ echo "  [WARN] $1"; WARN_N=$((WARN_N+1)); }

post(){ curl -s -o "$TMP/body" -w '%{http_code}' -X POST "$BASE$1" -H 'Content-Type: application/json' --data-binary "$2" 2>/dev/null; }
apost(){ curl -s -o "$TMP/body" -w '%{http_code}' -X POST "$BASE$1" -H 'Content-Type: application/json' -H "Authorization: Bearer $TOK" -d "$2" 2>/dev/null; }
put(){ curl -s -o "$TMP/body" -w '%{http_code}' -X PUT "$BASE$1" -H 'Content-Type: application/json' -H "Authorization: Bearer $TOK" -d "$2" 2>/dev/null; }
get(){ if [ -n "${2:-}" ]; then curl -s -o "$TMP/body" -w '%{http_code}' -H "Authorization: Bearer $2" "$BASE$1"; else curl -s -o "$TMP/body" -w '%{http_code}' "$BASE$1"; fi; }
del(){ curl -s -o "$TMP/body" -w '%{http_code}' -X DELETE "$BASE$1" -H "Authorization: Bearer $TOK"; }
tok(){ python3 -c "import json;d=json.load(open('$TMP/body'));print(d['data']['tokens']['$1'])" 2>/dev/null; }

echo "== security: $BASE =="

# 1-4 basic auth
c=$(post /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}"); check "login_ok" 200 "$c" "valid credentials"
ACC=$(tok access_token); REF=$(tok refresh_token); TOK=$ACC

# 1b first-run forced password change (seeded admin must change before API use)
c=$(curl -s -o "$TMP/body" -w '%{http_code}' -X POST "$BASE/api/v1/auth/change-password" -H 'Content-Type: application/json' -H "Authorization: Bearer $ACC" -d "{\"old_password\":\"$PASS\",\"new_password\":\"QaAdminPass123!\"}")
check "first_run_forced_password_change" 200 "$c" "seeded admin must change password"
PASS="QaAdminPass123!"
c=$(post /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}"); check "relogin_after_password_change" 200 "$c" "login with new password"
ACC=$(tok access_token); REF=$(tok refresh_token); TOK=$ACC

c=$(post /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"wrongpass123\"}"); check "login_wrong_password" 401 "$c" "bad password"
c=$(post /api/v1/auth/login "{}"); check "login_empty" 400 "$c" "missing fields"
c=$(post /api/v1/auth/login "{"); check "login_malformed_json" 400 "$c" "broken JSON"

# 5 oversized body
python3 -c "import json;json.dump({'username':'$USER','password':'x'*5_000_000},open('$TMP/big.json','w'))"
c=$(curl -s -o "$TMP/body" -w '%{http_code}' -X POST "$BASE/api/v1/auth/login" -H 'Content-Type: application/json' --data-binary @"$TMP/big.json")
if [ "$c" = "400" ] || [ "$c" = "413" ]; then check "login_oversized" "400or413" "$c" "5MB body rejected before argon2"
else warn "oversized body: 5MB password got HTTP $c (argon2 CPU cost accepted)"; fi

# 5b long password / username rejected without hashing
LP=$(python3 -c "print('p'*129)")
c=$(post /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$LP\"}"); check "login_password_too_long" 400 "$c" "129-byte password rejected"
c=$(post /api/v1/auth/login "{\"username\":\"$(python3 -c "print('u'*65)")\",\"password\":\"x\"}"); check "login_username_too_long" 400 "$c" "65-byte username rejected"

# 5c oversized refresh/change-password bodies rejected
c=$(post /api/v1/auth/refresh "{\"refresh_token\":\"$(python3 -c "print('t'*1_000_000)")\"}")
if [ "$c" = "400" ] || [ "$c" = "413" ]; then check "refresh_oversized" "400or413" "$c" "1MB refresh body rejected"
else warn "refresh oversized: got $c"; fi
c=$(curl -s -o "$TMP/body" -w '%{http_code}' -X POST "$BASE/api/v1/auth/change-password" -H 'Content-Type: application/json' -H "Authorization: Bearer $TOK" --data-binary @"$TMP/big.json")
if [ "$c" = "400" ] || [ "$c" = "413" ]; then check "change_password_oversized" "400or413" "$c" "5MB change-password body rejected"
else warn "change-password oversized: got $c"; fi

# 6 refresh rotation
c=$(post /api/v1/auth/refresh "{\"refresh_token\":\"$REF\"}"); check "refresh_ok" 200 "$c" "refresh works"
REF2=$(python3 -c "import json;d=json.load(open('$TMP/body'));print(d['data']['refresh_token'])" 2>/dev/null)
c=$(post /api/v1/auth/refresh "{\"refresh_token\":\"$REF\"}"); check "refresh_old_revoked" 401 "$c" "old refresh rejected after rotation"

# 7 logout revokes
c=$(apost /api/v1/auth/logout "{\"refresh_token\":\"$REF2\"}"); check "logout_ok" 200 "$c" "logout endpoint"
c=$(post /api/v1/auth/refresh "{\"refresh_token\":\"$REF2\"}"); check "refresh_after_logout" 401 "$c" "revoked by logout"

# 8 password change revokes all refresh tokens
TOK=$ACC
login2(){ curl -s -o "$TMP/body" -w '%{http_code}' -X POST "$BASE/api/v1/auth/login" -H 'Content-Type: application/json' -d "$1"; }
c=$(apost /api/v1/users "{\"username\":\"qatmp1\",\"email\":\"qa1@example.com\",\"role\":\"operator\",\"password\":\"QaPass123!\"}")
check "admin_create_user" 201 "$c" "create temp user"
ID1=$(python3 -c "import json;d=json.load(open('$TMP/body'));print(d['data']['id'])")
c=$(login2 "{\"username\":\"qatmp1\",\"password\":\"QaPass123!\"}"); check "temp_login" 200 "$c" "temp user login"
TACC=$(python3 -c "import json;d=json.load(open('$TMP/body'));print(d['data']['tokens']['access_token'])")
TREF=$(python3 -c "import json;d=json.load(open('$TMP/body'));print(d['data']['tokens']['refresh_token'])")
c=$(curl -s -o "$TMP/body" -w '%{http_code}' -X POST "$BASE/api/v1/auth/change-password" -H 'Content-Type: application/json' -H "Authorization: Bearer $TACC" -d '{"old_password":"QaPass123!","new_password":"NewPass456!"}')
check "temp_change_password" 200 "$c" "change password"
c=$(post /api/v1/auth/refresh "{\"refresh_token\":\"$TREF\"}"); check "refresh_after_password_change" 401 "$c" "all refresh tokens revoked"
c=$(del /api/v1/users/$ID1); check "cleanup_user" 200 "$c" "delete temp user"

# 9 disabled user cannot login
c=$(apost /api/v1/users "{\"username\":\"qatmp2\",\"email\":\"qa2@example.com\",\"role\":\"viewer\",\"password\":\"QaPass123!\"}")
ID2=$(python3 -c "import json;d=json.load(open('$TMP/body'));print(d['data']['id'])")
c=$(put /api/v1/users/$ID2 '{"enabled":false}'); check "disable_user" 200 "$c" "disable via admin"
c=$(login2 "{\"username\":\"qatmp2\",\"password\":\"QaPass123!\"}"); check "disabled_login_blocked" 401 "$c" "disabled user rejected"
c=$(del /api/v1/users/$ID2); check "cleanup_user2" 200 "$c" "delete disabled user"

# 10 last-admin / self-delete protection
c=$(get /api/v1/users "$ACC"); check "list_users" 200 "$c" "admin can list"
ADMIN_ID=$(python3 -c "import json;d=json.load(open('$TMP/body'));print([u['id'] for u in d['data'] if u['username']=='$USER'][0])")
c=$(del /api/v1/users/$ADMIN_ID); check "self_delete_blocked" 400 "$c" "cannot delete own account"

# 10b RBAC: viewer/operator cannot manage users or read audit
c=$(apost /api/v1/users "{\"username\":\"qatmp3\",\"email\":\"qa3@example.com\",\"role\":\"viewer\",\"password\":\"QaPass123!\"}")
ID3=$(python3 -c "import json;d=json.load(open('$TMP/body'));print(d['data']['id'])")
c=$(login2 "{\"username\":\"qatmp3\",\"password\":\"QaPass123!\"}"); VACC=$(tok access_token)
c=$(get /api/v1/users "$VACC"); check "viewer_users_forbidden" 403 "$c" "viewer cannot list users"
c=$(get /api/v1/audit "$VACC"); check "viewer_audit_forbidden" 403 "$c" "viewer cannot read audit"
vpost(){ curl -s -o "$TMP/body" -w '%{http_code}' -X POST "$BASE$1" -H 'Content-Type: application/json' -H "Authorization: Bearer $VACC" -d "$2" 2>/dev/null; }
c=$(vpost /api/v1/users "{\"username\":\"hacker\",\"email\":\"h@example.com\",\"role\":\"admin\",\"password\":\"HackPass123!\"}" ); check "viewer_create_user_forbidden" 403 "$c" "viewer cannot create admin"
c=$(del /api/v1/users/$ID3); check "cleanup_user3" 200 "$c" "delete viewer"

# 10c JWT tampering / malformed auth
c=$(get /api/v1/auth/me); check "no_token_me" 401 "$c" "missing bearer rejected"
c=$(get /api/v1/auth/me "Bearer garbage"); check "garbage_token_me" 401 "$c" "garbage token rejected"
TAMPERED="${ACC%?}X"
c=$(get /api/v1/auth/me "Bearer $TAMPERED"); check "tampered_token_me" 401 "$c" "tampered signature rejected"

# 10d SQL injection attempts must never 500
for payload in "admin' OR '1'='1" 'admin"; DROP TABLE users;--' "admin\\\" OR true --"; do
  c=$(post /api/v1/auth/login "{\"username\":\"$payload\",\"password\":\"x\"}")
  if [ "$c" = "500" ]; then check "sqli_$payload" "!500" "$c" "SQL injection crashed login"
  elif [ "$c" = "401" ] || [ "$c" = "400" ] || [ "$c" = "429" ]; then check "sqli_ok" "400or401or429" "$c" "SQL injection rejected"
  else check "sqli_$payload" "400or401or429" "$c" "unexpected status"; fi
done

# 10e wrong methods on API routes must not be served
c=$(curl -s -o /dev/null -w '%{http_code}' -X PUT "$BASE/api/v1/auth/login"); 
if [ "$c" = "404" ] || [ "$c" = "405" ]; then check "method_put_login" "404or405" "$c" "PUT login not served"
else warn "PUT /auth/login returned $c"; fi
c=$(curl -s -o /dev/null -w '%{http_code}' -X DELETE "$BASE/api/v1/system/status")
if [ "$c" = "404" ] || [ "$c" = "405" ]; then check "method_delete_status" "404or405" "$c" "DELETE status not served"
else warn "DELETE /system/status returned $c"; fi

# 11 demo endpoint absent
c=$(get /api/v1/auth/demo); check "demo_endpoint_absent" 404 "$c" "no demo mode"

# 12 security headers
curl -s -D "$TMP/h" -o /dev/null "$BASE/"
H1=$(grep -ci '^X-Content-Type-Options: nosniff' "$TMP/h" || true)
check "header_nosniff" 1 "$H1" "X-Content-Type-Options"
H2=$(grep -ci '^X-Frame-Options: DENY' "$TMP/h" || true)
check "header_framing" 1 "$H2" "X-Frame-Options"

# 13 metrics public
c=$(get /api/v1/system/metrics); check "metrics_public" 200 "$c" "machine-readable metrics"

# 14 brute force lockout (5 failures -> 429 on 6th)
STATUSES=""
LAST=""
for i in $(seq 1 6); do
  s=$(post /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"badpass$i\"}")
  STATUSES="$STATUSES $s"
  LAST=$s
done
if [ "$LAST" = "429" ]; then check "bruteforce_locked" 429 "$LAST" "per-IP lockout after repeated failures"
else warn "bruteforce: 6 rapid failures, last got $LAST (no per-IP lockout)"; fi
if [ "$LAST" = "429" ]; then
  c=$(post /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}")
  check "bruteforce_blocks_correct_password" 429 "$c" "correct password rejected while locked"
fi

# 15 global per-IP rate limit (separate from login lockout)
PORT_NUM=$(echo "$BASE" | sed 's#.*:##')
R429=$(python3 - "$PORT_NUM" <<'PY'
import http.client, sys, threading
port = int(sys.argv[1])
results = []
def hit():
    try:
        c = http.client.HTTPConnection("127.0.0.1", port, timeout=10)
        c.request("GET", "/api/v1/system/status")
        r = c.getresponse()
        results.append(r.status)
        r.read()
        c.close()
    except Exception:
        results.append(0)
threads = [threading.Thread(target=hit) for _ in range(300)]
for t in threads: t.start()
for t in threads: t.join()
print(results.count(429))
PY
)
if [ "$R429" -gt 0 ]; then
  check "global_rate_limit" observed observed "300 concurrent status requests rate-limited per IP ($R429 got 429)"
else
  check "global_rate_limit" observed missing "no 429 in 300 concurrent requests"
fi

echo
echo "security summary: PASS=$PASS_N FAIL=$FAIL_N WARN=$WARN_N"
[ "$FAIL_N" -eq 0 ] || exit 1
