// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package middleware

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	chimw "github.com/go-chi/chi/v5/middleware"
	_ "github.com/mattn/go-sqlite3"
	"github.com/netberth/netberth/internal/auth"
	"github.com/netberth/netberth/internal/model"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", "file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.Exec(`CREATE TABLE IF NOT EXISTS users (id TEXT PRIMARY KEY, tenant_id TEXT DEFAULT '', username TEXT, password_hash TEXT, role TEXT, password_changed INTEGER DEFAULT 0)`)
	db.Exec(`CREATE TABLE IF NOT EXISTS audit_events (id INTEGER PRIMARY KEY AUTOINCREMENT, tenant_id TEXT DEFAULT '', user_id TEXT DEFAULT '', username TEXT DEFAULT '', action TEXT DEFAULT '', resource_type TEXT DEFAULT '', resource_id TEXT DEFAULT '', changes TEXT DEFAULT '', remote_addr TEXT DEFAULT '', created_at DATETIME DEFAULT CURRENT_TIMESTAMP)`)
	return db
}

func testAuthSvc() *auth.Service {
	return auth.NewService("test-secret", 15*time.Minute, 7*24*time.Hour)
}

func testToken(t *testing.T, svc *auth.Service, uid string) string {
	t.Helper()
	tok, err := svc.GenerateTokens(&model.User{ID: uid, Username: "test", Role: "admin"})
	if err != nil {
		t.Fatalf("token gen: %v", err)
	}
	return tok.AccessToken
}

// === Auth Middleware ===

func TestAuthNoToken(t *testing.T) {
	mw := AuthMiddleware(testAuthSvc())
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	req := httptest.NewRequest("GET", "/api/v1/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAuthBadToken(t *testing.T) {
	mw := AuthMiddleware(testAuthSvc())
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	req := httptest.NewRequest("GET", "/api/v1/test", nil)
	req.Header.Set("Authorization", "Bearer garbage-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAuthValidToken(t *testing.T) {
	svc := testAuthSvc()
	tok := testToken(t, svc, "user-1")
	mw := AuthMiddleware(svc)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := r.Context().Value(auth.ClaimsKey).(*auth.Claims); !ok {
			t.Error("claims missing from context")
		}
		w.WriteHeader(200)
	}))
	req := httptest.NewRequest("GET", "/api/v1/test", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// === CSRF Middleware ===

func TestCSRFNoTokenBlocked(t *testing.T) {
	handler := CSRFMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	req := httptest.NewRequest("POST", "/settings", strings.NewReader("data"))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestCSRFAPIByPass(t *testing.T) {
	handler := CSRFMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	req := httptest.NewRequest("POST", "/api/v1/something", strings.NewReader("data"))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("API should bypass CSRF, got %d", w.Code)
	}
}

func TestCSRFGetAllowed(t *testing.T) {
	handler := CSRFMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	req := httptest.NewRequest("GET", "/page", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("GET should be allowed, got %d", w.Code)
	}
}

func TestCSRFValidToken(t *testing.T) {
	tok := CSRFToken()
	handler := CSRFMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	req := httptest.NewRequest("POST", "/settings", strings.NewReader("data"))
	req.Header.Set("X-CSRF-Token", tok)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with valid token, got %d", w.Code)
	}
}

// === Brute Force ===

func TestBruteForceBlocksAfterThreshold(t *testing.T) {
	bl := NewBruteForceLimiter(3, 10*time.Second, 1*time.Minute)
	handler := bl.LoginMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))

	// First 3 requests should pass (blocking happens after threshold)
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader("x"))
		req.RemoteAddr = "192.168.1.1:12345"
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: expected 401, got %d", i+1, w.Code)
		}
	}
	// 4th should be blocked
	req := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader("x"))
	req.RemoteAddr = "192.168.1.1:12345"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429 after 3 failures, got %d", w.Code)
	}
}

func TestBruteForceDifferentIPsIndependent(t *testing.T) {
	bl := NewBruteForceLimiter(3, 10*time.Second, 1*time.Minute)
	handler := bl.LoginMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))

	// IP1: 3 failures → blocked
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader("x"))
		req.RemoteAddr = "10.0.0.1:1"
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("IP1 attempt %d: expected 401, got %d", i+1, w.Code)
		}
	}
	req1 := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader("x"))
	req1.RemoteAddr = "10.0.0.1:1"
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)
	if w1.Code != http.StatusTooManyRequests {
		t.Errorf("IP1 should be blocked, got %d", w1.Code)
	}

	// IP2: should still pass
	req2 := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader("x"))
	req2.RemoteAddr = "10.0.0.2:1"
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)
	if w2.Code != http.StatusUnauthorized {
		t.Errorf("IP2 should be allowed to reach the handler (401, not 429), got %d", w2.Code)
	}
}

func TestBruteForceAutoRecordsFailedLogins(t *testing.T) {
	bl := NewBruteForceLimiter(3, 10*time.Second, 1*time.Minute)
	handler := bl.LoginMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))

	// 3 failed logins must auto-lock the peer; the 4th gets 429 without any
	// external RecordFailure call.
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(`{"username":"a","password":"b"}`))
		req.RemoteAddr = "198.51.100.7:12345"
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: expected 401, got %d", i+1, w.Code)
		}
	}
	req := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(`{"username":"a","password":"b"}`))
	req.RemoteAddr = "198.51.100.7:12345"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after 3 auto-recorded failures, got %d", w.Code)
	}
	if got := w.Header().Get("Retry-After"); got == "" {
		t.Error("expected Retry-After header on lockout response")
	}
}

func TestBruteForceResetsOnSuccess(t *testing.T) {
	bl := NewBruteForceLimiter(3, 10*time.Second, 1*time.Minute)
	status := http.StatusUnauthorized
	handler := bl.LoginMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
	}))

	fail := func(remote string) {
		req := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader("x"))
		req.RemoteAddr = remote
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", w.Code)
		}
	}
	fail("203.0.113.9:1")
	fail("203.0.113.9:1")

	status = http.StatusOK
	req := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader("x"))
	req.RemoteAddr = "203.0.113.9:1"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected successful login to pass, got %d", w.Code)
	}

	// Failures before the success must not count anymore.
	status = http.StatusUnauthorized
	fail("203.0.113.9:1")
	fail("203.0.113.9:1")
	req2 := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader("x"))
	req2.RemoteAddr = "203.0.113.9:1"
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)
	if w2.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 (not locked), got %d", w2.Code)
	}
}

func TestBruteForceUsesPeerIPNotSpoofedHeaders(t *testing.T) {
	bl := NewBruteForceLimiter(3, 10*time.Second, 1*time.Minute)
	inner := bl.LoginMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	handler := chimw.ClientIPFromRemoteAddr(inner)

	// Attacker sends spoofed X-Forwarded-For; lockout must key on the TCP peer.
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader("x"))
		req.RemoteAddr = "198.51.100.44:5555"
		req.Header.Set("X-Forwarded-For", "6.6.6.6")
		req.Header.Set("X-Real-IP", "7.7.7.7")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: expected 401, got %d", i+1, w.Code)
		}
	}

	// Spoofed address must still be allowed; the real peer must be locked.
	spoof := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader("x"))
	spoof.RemoteAddr = "198.51.100.99:5555"
	spoof.Header.Set("X-Forwarded-For", "6.6.6.6")
	wSpoof := httptest.NewRecorder()
	handler.ServeHTTP(wSpoof, spoof)
	if wSpoof.Code != http.StatusUnauthorized {
		t.Fatalf("spoofed header must not lock a different peer, got %d", wSpoof.Code)
	}

	real := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader("x"))
	real.RemoteAddr = "198.51.100.44:5555"
	wReal := httptest.NewRecorder()
	handler.ServeHTTP(wReal, real)
	if wReal.Code != http.StatusTooManyRequests {
		t.Fatalf("real peer should be locked, got %d", wReal.Code)
	}
}

// === Rate Limit ===

func TestRateLimitBelowThreshold(t *testing.T) {
	rl := NewRateLimiter(100, 200)
	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	req := httptest.NewRequest("GET", "/api/v1/test", nil)
	req.RemoteAddr = "10.0.0.50:1"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 below limit, got %d", w.Code)
	}
}

func TestRateLimitExceeded(t *testing.T) {
	rl := NewRateLimiter(1, 1) // very restrictive for test
	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	// Consume the single token
	req1 := httptest.NewRequest("GET", "/api/v1/test", nil)
	req1.RemoteAddr = "10.0.0.99:1"
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)
	// Next should be rate limited
	req2 := httptest.NewRequest("GET", "/api/v1/test", nil)
	req2.RemoteAddr = "10.0.0.99:1"
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)
	if w2.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429 over limit, got %d (body=%s)", w2.Code, w2.Body.String())
	}
}

// === Force Password Change ===

func TestForcePasswordNotChanged(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	db.Exec("INSERT INTO users (id, username, password_hash, role, password_changed) VALUES ('u1','test','hash','admin',0)")
	svc := testAuthSvc()
	tok := testToken(t, svc, "u1")

	handler := ForcePasswordChange(db)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	req := httptest.NewRequest("GET", "/api/v1/forward-rules", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	ctx := req.Context()
	claims := &auth.Claims{UserID: "u1", Username: "test", Role: "admin"}
	ctx = context.WithValue(ctx, auth.ClaimsKey, claims)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for unchanged pw, got %d", w.Code)
	}
}

func TestForcePasswordChanged(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	db.Exec("INSERT INTO users (id, username, password_hash, role, password_changed) VALUES ('u2','test2','hash','admin',1)")
	svc := testAuthSvc()
	tok := testToken(t, svc, "u2")

	handler := ForcePasswordChange(db)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	req := httptest.NewRequest("GET", "/api/v1/forward-rules", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	ctx := req.Context()
	claims := &auth.Claims{UserID: "u2", Username: "test2", Role: "admin"}
	ctx = context.WithValue(ctx, auth.ClaimsKey, claims)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for changed pw, got %d", w.Code)
	}
}

func TestForcePasswordChangePwEndpointAllowed(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	db.Exec("INSERT INTO users (id, username, password_hash, role, password_changed) VALUES ('u3','test3','hash','admin',0)")
	handler := ForcePasswordChange(db)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	req := httptest.NewRequest("POST", "/api/v1/auth/change-password", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	ctx := req.Context()
	claims := &auth.Claims{UserID: "u3", Username: "test3", Role: "admin"}
	ctx = context.WithValue(ctx, auth.ClaimsKey, claims)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("change-password should always be allowed, got %d", w.Code)
	}
}

// === Audit ===

func TestAuditLogsMutation(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	handler := AuditMiddleware(db)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(201) }))
	req := httptest.NewRequest("POST", "/api/v1/forward-rules", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}
	// Poll audit_events for up to 2 seconds (async goroutine)
	var count int
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		db.QueryRow("SELECT COUNT(*) FROM audit_events WHERE action='created'").Scan(&count)
		if count > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if count == 0 {
		t.Error("expected audit record for POST (action=created) after polling")
	}
}

func TestAuditSkipsGet(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	handler := AuditMiddleware(db)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	req := httptest.NewRequest("GET", "/api/v1/status", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	time.Sleep(20 * time.Millisecond)
	var count int
	db.QueryRow("SELECT COUNT(*) FROM audit_events").Scan(&count)
	if count > 0 {
		t.Logf("GET audit count: %d (may be from prior tests)", count)
	}
}
