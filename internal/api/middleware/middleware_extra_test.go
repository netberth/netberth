// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/netberth/netberth/internal/auth"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
}

func TestRequireRole(t *testing.T) {
	handler := RequireRole("admin")(okHandler())

	// No claims in context.
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without claims, got %d", w.Code)
	}

	// Wrong role.
	req := httptest.NewRequest("GET", "/", nil)
	req = req.WithContext(context.WithValue(req.Context(), auth.ClaimsKey, &auth.Claims{Role: "user"}))
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req)
	if w2.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for wrong role, got %d", w2.Code)
	}

	// Correct role.
	req2 := httptest.NewRequest("GET", "/", nil)
	req2 = req2.WithContext(context.WithValue(req2.Context(), auth.ClaimsKey, &auth.Claims{Role: "admin"}))
	w3 := httptest.NewRecorder()
	handler.ServeHTTP(w3, req2)
	if w3.Code != http.StatusOK {
		t.Fatalf("expected 200 for admin, got %d", w3.Code)
	}
}

func TestCORSWithOrigin(t *testing.T) {
	called := false
	handler := CORSMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(200)
	}))
	req := httptest.NewRequest("GET", "/api/v1/x", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Header().Get("Access-Control-Allow-Origin") != "http://localhost:5173" {
		t.Fatalf("missing allow-origin: %v", w.Header())
	}
	if w.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatal("missing allow-credentials")
	}
	if w.Header().Get("Access-Control-Allow-Methods") == "" || w.Header().Get("Access-Control-Allow-Headers") == "" {
		t.Fatal("missing CORS allow headers")
	}
	if !called || w.Code != 200 {
		t.Fatalf("next handler not called: called=%v code=%d", called, w.Code)
	}
}

func TestCORSPreflight(t *testing.T) {
	called := false
	handler := CORSMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(200)
	}))
	req := httptest.NewRequest(http.MethodOptions, "/api/v1/x", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204 preflight, got %d", w.Code)
	}
	if called {
		t.Fatal("preflight must not reach next handler")
	}
}

func TestCORSWithoutOrigin(t *testing.T) {
	handler := CORSMiddleware(okHandler())
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("allow-origin should only be set when Origin is present")
	}
}

func TestSecurityHeaders(t *testing.T) {
	handler := SecurityHeaders(okHandler())
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	for _, h := range []string{
		"X-Content-Type-Options", "X-Frame-Options", "X-XSS-Protection",
		"Referrer-Policy", "Permissions-Policy",
	} {
		if w.Header().Get(h) == "" {
			t.Fatalf("missing security header %s", h)
		}
	}
}

func TestLoggingMiddleware(t *testing.T) {
	handler := LoggingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		w.Write([]byte("hello"))
	}))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/x", nil))
	if w.Code != http.StatusTeapot {
		t.Fatalf("expected 418, got %d", w.Code)
	}
	if w.Body.String() != "hello" {
		t.Fatalf("expected body hello, got %q", w.Body.String())
	}
}

func TestAuthInvalidFormats(t *testing.T) {
	handler := AuthMiddleware(testAuthSvc())(okHandler())
	for _, h := range []string{"Basic abc", "Bearer", "Bearer "} {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Authorization", h)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("header %q: expected 401, got %d", h, w.Code)
		}
	}
}

func TestExtractAuditInfo(t *testing.T) {
	cases := []struct {
		path, method, resType, resID, action string
	}{
		{"/api/v1/forward-rules", "POST", "forward_rule", "", "created"},
		{"/api/v1/forward-rules/abc", "PUT", "forward_rule", "abc", "updated"},
		{"/api/v1/forward-rules/abc", "DELETE", "forward_rule", "abc", "deleted"},
		{"/api/v1/proxy-rules", "POST", "proxy_rule", "", "created"},
		{"/api/v1/ddns/x", "PUT", "ddns_config", "x", "updated"},
		{"/api/v1/stun", "POST", "stun_tunnel", "", "created"},
		{"/api/v1/wol", "POST", "wol_device", "", "created"},
		{"/api/v1/cron", "POST", "cron_job", "", "created"},
		{"/api/v1/acme", "POST", "acme_certificate", "", "created"},
		{"/api/v1/storage", "POST", "storage_mount", "", "created"},
		{"/api/v1/users", "POST", "user", "", "created"},
		{"/api/v1/users/abc", "DELETE", "user", "abc", "deleted"},
		{"/api/v1/forward-rules", "GET", "", "", ""},
		{"/api/v1/unknown", "POST", "", "", ""},
		{"/not-api", "POST", "", "", ""},
	}
	for _, c := range cases {
		rt, rid, act := extractAuditInfo(c.path, c.method)
		if rt != c.resType || rid != c.resID || act != c.action {
			t.Errorf("%s %s: got (%s,%s,%s), want (%s,%s,%s)",
				c.method, c.path, rt, rid, act, c.resType, c.resID, c.action)
		}
	}
}

func TestForcePasswordAllowedPaths(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	db.Exec("INSERT INTO users (id, username, password_hash, role, password_changed) VALUES ('up1','up','hash','admin',0)")
	handler := ForcePasswordChange(db)(okHandler())
	for _, path := range []string{"/api/v1/auth/me", "/api/v1/auth/2fa/setup", "/api/v1/system/status"} {
		req := httptest.NewRequest("GET", path, nil)
		req = req.WithContext(context.WithValue(req.Context(), auth.ClaimsKey, &auth.Claims{UserID: "up1", Role: "admin"}))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("%s: expected 200 for allowed path, got %d", path, w.Code)
		}
	}

	// No claims → pass through.
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/forward-rules", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("expected pass-through without claims, got %d", w.Code)
	}
}

func TestCSRFInvalidToken(t *testing.T) {
	handler := CSRFMiddleware(okHandler())
	req := httptest.NewRequest("POST", "/settings", strings.NewReader("{}"))
	req.Header.Set("X-CSRF-Token", "wrong-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for invalid CSRF token, got %d", w.Code)
	}
}
