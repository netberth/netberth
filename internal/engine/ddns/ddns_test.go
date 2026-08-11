// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package ddns

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netberth/netberth/internal/model"
)

type mockDDNSDB struct {
	configs []model.DDNSConfig
	err     error
	updates atomic.Int64
}

func (m *mockDDNSDB) GetConfigs() ([]model.DDNSConfig, error) { return m.configs, m.err }
func (m *mockDDNSDB) UpdateIP(id, ip string) error             { m.updates.Add(1); return nil }

func TestStartDBError(t *testing.T) {
	e := New(&mockDDNSDB{err: errors.New("boom")})
	if err := e.Start(); err == nil {
		t.Fatal("expected start error")
	}
}

func TestStartWithDisabledConfigs(t *testing.T) {
	e := New(&mockDDNSDB{configs: []model.DDNSConfig{{ID: "d1", Enabled: false}}})
	if err := e.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	e.Stop()
}

func TestReloadAndRemove(t *testing.T) {
	e := New(&mockDDNSDB{})
	cfg := model.DDNSConfig{ID: "d1", Name: "dd", Enabled: false}
	e.Reload(cfg)
	e.Remove("d1")
	e.Remove("missing")

	// Enabled config starts a goroutine; cancel it promptly.
	e.Reload(model.DDNSConfig{ID: "d2", Name: "dd2", Enabled: true,
		GetIPURL: "http://127.0.0.1:1", Provider: "unknown", Interval: 60})
	time.Sleep(50 * time.Millisecond)
	e.Remove("d2")
}

func TestGetPublicIP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("203.0.113.9"))
	}))
	defer srv.Close()

	e := New(&mockDDNSDB{})
	ip, err := e.getPublicIP(model.DDNSConfig{GetIPURL: srv.URL})
	if err != nil {
		t.Fatalf("getPublicIP: %v", err)
	}
	if ip != "203.0.113.9" {
		t.Fatalf("expected 203.0.113.9, got %q", ip)
	}

	if _, err := e.getPublicIP(model.DDNSConfig{GetIPURL: "http://127.0.0.1:1"}); err == nil {
		t.Fatal("expected error for unreachable URL")
	}

	if _, err := e.getPublicIP(model.DDNSConfig{GetIPType: "interface", NetInterface: "nonexistent-iface-0"}); err == nil {
		t.Fatal("expected error for missing interface")
	}
}

func TestUpdateDNSUnsupportedAndMissingCreds(t *testing.T) {
	e := New(&mockDDNSDB{})
	if err := e.updateDNS(model.DDNSConfig{Provider: "nope"}, "1.2.3.4"); err == nil {
		t.Fatal("expected error for unsupported provider")
	}
	if err := e.updateDNS(model.DDNSConfig{Provider: "cloudflare"}, "1.2.3.4"); err == nil {
		t.Fatal("expected error for missing cloudflare credentials")
	}
}

func TestUpdateFailureDoesNotTouchDB(t *testing.T) {
	db := &mockDDNSDB{}
	e := New(db)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("203.0.113.9"))
	}))
	defer srv.Close()

	e.update(model.DDNSConfig{Name: "dd", GetIPURL: srv.URL, Provider: "unknown"})
	if db.updates.Load() != 0 {
		t.Fatalf("expected no DB update on provider failure, got %d", db.updates.Load())
	}
}
