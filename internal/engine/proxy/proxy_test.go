// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package proxy

import (
	"net"
	"net/http"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/netberth/netberth/internal/model"
)

type mockProxyDB struct {
	rules []model.ProxyRule
}

func (m *mockProxyDB) GetRules() ([]model.ProxyRule, error) { return m.rules, nil }

func TestProxyStartStop(t *testing.T) {
	eng := New(&mockProxyDB{})
	if err := eng.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	eng.Stop()
}

func TestProxyReload(t *testing.T) {
	eng := New(&mockProxyDB{})
	_ = eng.Start("127.0.0.1:0")
	defer eng.Stop()
	time.Sleep(100 * time.Millisecond)

	rule := model.ProxyRule{
		ID: "prx-1", Name: "test", Domains: []string{"test.local"},
		TargetURL: "http://127.0.0.1:9999", Enabled: true,
	}
	eng.Reload(rule)
	time.Sleep(100 * time.Millisecond)

	eng.mu.RLock()
	_, exists := eng.routes["test.local"]
	eng.mu.RUnlock()
	if !exists { t.Fatal("route should exist after Reload") }

	// Disable
	rule.Enabled = false
	eng.Reload(rule)
	time.Sleep(100 * time.Millisecond)

	eng.mu.RLock()
	_, exists = eng.routes["test.local"]
	eng.mu.RUnlock()
	if exists { t.Fatal("route should be removed after disable") }
}

func TestProxyWildcardRouting(t *testing.T) {
	eng := New(&mockProxyDB{})

	// Add a wildcard route
	rule := model.ProxyRule{
		ID: "wild", Name: "wild", Domains: []string{"*.example.com"},
		TargetURL: "http://127.0.0.1:8888", Enabled: true,
	}
	eng.addRoute(rule)

	// Test domain matching via serveHTTP
	req, _ := http.NewRequest("GET", "http://sub.example.com/path", nil)
	w := &testResponseWriter{}

	start := runtime.NumGoroutine()
	eng.serveHTTP(w, req)
	time.Sleep(50 * time.Millisecond)
	after := runtime.NumGoroutine()

	// Should match wildcard, try to proxy → will fail (no backend) with some status
	if w.status == 0 { t.Error("handler should have responded") }
	t.Logf("wildcard route response: status=%d", w.status)

	if after > start+5 {
		t.Errorf("goroutine leak on wildcard route: %d → %d", start, after)
	}
}

func TestProxyACLRejection(t *testing.T) {
	eng := New(&mockProxyDB{})
	_ = eng.Start("127.0.0.1:0")
	defer eng.Stop()
	time.Sleep(100 * time.Millisecond)

	// Start a real backend
	backendLn, _ := net.Listen("tcp", "127.0.0.1:0")
	backendPort := backendLn.Addr().(*net.TCPAddr).Port
	go func() {
		for {
			conn, err := backendLn.Accept()
			if err != nil { return }
			buf := make([]byte, 1024)
			n, _ := conn.Read(buf)
			conn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nOK"))
			_ = n
			conn.Close()
		}
	}()

	rule := model.ProxyRule{
		ID: "acl-test", Name: "acl", Domains: []string{"acl.local"},
		TargetURL: "http://127.0.0.1:" + backdoor(backendPort),
		IPBlacklist: []model.ACLEntry{{Value: "127.0.0.1"}},
		Enabled: true,
	}
	eng.addRoute(rule)

	req, _ := http.NewRequest("GET", "http://acl.local/test", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := &testResponseWriter{}
	eng.serveHTTP(w, req)

	if w.status != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for blacklisted IP, got %d", w.status)
	}
}

func TestGoroutineLeakProxyReload(t *testing.T) {
	eng := New(&mockProxyDB{})
	_ = eng.Start("127.0.0.1:0")
	defer eng.Stop()
	time.Sleep(100 * time.Millisecond)

	start := runtime.NumGoroutine()

	for i := 0; i < 30; i++ {
		rule := model.ProxyRule{
			ID: "reload-leak", Name: "reload", Domains: []string{"leak.local"},
			TargetURL: "http://127.0.0.1:9999", Enabled: true,
		}
		eng.Reload(rule)
	}

	time.Sleep(200 * time.Millisecond)
	runtime.GC()
	after := runtime.NumGoroutine()

	t.Logf("proxy reload goroutines: before=%d after=%d", start, after)
	if after > start+10 {
		t.Errorf("possible goroutine leak on proxy reload: %d → %d", start, after)
	}
}

type testResponseWriter struct {
	status int
	header http.Header
	body   []byte
}

func (w *testResponseWriter) Header() http.Header {
	if w.header == nil { w.header = make(http.Header) }
	return w.header
}
func (w *testResponseWriter) Write(b []byte) (int, error) {
	w.body = append(w.body, b...)
	return len(b), nil
}
func (w *testResponseWriter) WriteHeader(status int) { w.status = status }

func backdoor(port int) string { return strconv.Itoa(port) }
