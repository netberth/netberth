// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientIPIgnoresSpoofedHeadersByDefault(t *testing.T) {
	r, err := NewClientIPResolver(nil)
	if err != nil {
		t.Fatalf("NewClientIPResolver: %v", err)
	}
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "203.0.113.7:1234"
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	req.Header.Set("X-Real-IP", "5.6.7.8")
	req.Header.Set("True-Client-IP", "9.9.9.9")
	if got := r.ClientIP(req); got != "203.0.113.7" {
		t.Fatalf("expected peer IP, got %q", got)
	}
}

func TestClientIPTrustedProxyXFF(t *testing.T) {
	r, err := NewClientIPResolver([]string{"10.0.0.1"})
	if err != nil {
		t.Fatalf("NewClientIPResolver: %v", err)
	}
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.1:9999"
	req.Header.Set("X-Forwarded-For", "203.0.113.9, 10.0.0.1")
	if got := r.ClientIP(req); got != "203.0.113.9" {
		t.Fatalf("expected client from XFF, got %q", got)
	}
}

func TestClientIPTrustedProxyXFFAllTrusted(t *testing.T) {
	r, err := NewClientIPResolver([]string{"10.0.0.0/8"})
	if err != nil {
		t.Fatalf("NewClientIPResolver: %v", err)
	}
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.1:9999"
	req.Header.Set("X-Forwarded-For", "10.0.0.2, 10.0.0.1")
	if got := r.ClientIP(req); got != "10.0.0.2" {
		t.Fatalf("expected leftmost XFF when all trusted, got %q", got)
	}
}

func TestClientIPTrustedProxyRealAndTrueIP(t *testing.T) {
	r, err := NewClientIPResolver([]string{"10.0.0.1"})
	if err != nil {
		t.Fatalf("NewClientIPResolver: %v", err)
	}
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.1:9999"
	req.Header.Set("X-Real-IP", "198.51.100.4")
	if got := r.ClientIP(req); got != "198.51.100.4" {
		t.Fatalf("expected X-Real-IP, got %q", got)
	}

	req2 := httptest.NewRequest("GET", "/", nil)
	req2.RemoteAddr = "10.0.0.1:9999"
	req2.Header.Set("True-Client-IP", "2001:db8::5")
	if got := r.ClientIP(req2); got != "2001:db8::5" {
		t.Fatalf("expected True-Client-IP, got %q", got)
	}
}

func TestClientIPIPv6Peer(t *testing.T) {
	r, err := NewClientIPResolver(nil)
	if err != nil {
		t.Fatalf("NewClientIPResolver: %v", err)
	}
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "[::1]:8080"
	if got := r.ClientIP(req); got != "::1" {
		t.Fatalf("expected ::1, got %q", got)
	}
}

func TestClientIPResolverInvalidTrusted(t *testing.T) {
	if _, err := NewClientIPResolver([]string{"not-an-ip"}); err == nil {
		t.Fatal("expected error for invalid trusted proxy")
	}
	if _, err := NewClientIPResolver([]string{"10.0.0.0/8"}); err != nil {
		t.Fatalf("valid CIDR should not error: %v", err)
	}
}

func TestClientIPMiddlewareSetsRemoteAddr(t *testing.T) {
	r, err := NewClientIPResolver(nil)
	if err != nil {
		t.Fatalf("NewClientIPResolver: %v", err)
	}
	handler := r.Middleware(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.RemoteAddr != "198.51.100.9" {
			t.Fatalf("expected normalized RemoteAddr, got %q", req.RemoteAddr)
		}
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "198.51.100.9:443"
	handler.ServeHTTP(httptest.NewRecorder(), req)
}
