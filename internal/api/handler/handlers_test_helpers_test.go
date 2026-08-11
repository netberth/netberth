// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package handler

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

var fullTestDBCounter int64

func setupFullTestDB(t *testing.T) *sql.DB {
	t.Helper()
	n := atomic.AddInt64(&fullTestDBCounter, 1)
	// Shared-cache memory DB: nested queries on the same *sql.DB must see
	// the same database across connections (see HANDOVER.md §6.2).
	db, err := sqlOpen(fmt.Sprintf("file:nbtest_%d?mode=memory&cache=shared", n))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	stmts := []string{
		`CREATE TABLE IF NOT EXISTS forward_rules (id TEXT PRIMARY KEY, tenant_id TEXT, owner_id TEXT, name TEXT, protocol TEXT, listen_addr TEXT, listen_port INTEGER, target_addr TEXT, target_port INTEGER, enable_ipv6 INTEGER, max_conns INTEGER, enabled INTEGER, schedule_on TEXT DEFAULT '', schedule_off TEXT DEFAULT '', created_at DATETIME, updated_at DATETIME)`,
		`CREATE TABLE IF NOT EXISTS forward_whitelist (id TEXT PRIMARY KEY, rule_id TEXT, value TEXT)`,
		`CREATE TABLE IF NOT EXISTS forward_blacklist (id TEXT PRIMARY KEY, rule_id TEXT, value TEXT)`,
		`CREATE TABLE IF NOT EXISTS proxy_rules (id TEXT PRIMARY KEY, tenant_id TEXT, owner_id TEXT, name TEXT, target_url TEXT, tls_enabled INTEGER, cert_id TEXT, force_https INTEGER, http2 INTEGER, websocket INTEGER, url_rewrite TEXT, basic_auth_user TEXT, basic_auth_hash TEXT, max_conns INTEGER, enabled INTEGER, created_at DATETIME, updated_at DATETIME)`,
		`CREATE TABLE IF NOT EXISTS proxy_domains (id TEXT PRIMARY KEY, rule_id TEXT, domain TEXT)`,
		`CREATE TABLE IF NOT EXISTS proxy_ip_whitelist (id TEXT PRIMARY KEY, rule_id TEXT, value TEXT)`,
		`CREATE TABLE IF NOT EXISTS proxy_ip_blacklist (id TEXT PRIMARY KEY, rule_id TEXT, value TEXT)`,
		`CREATE TABLE IF NOT EXISTS proxy_ua_whitelist (id TEXT PRIMARY KEY, rule_id TEXT, value TEXT)`,
		`CREATE TABLE IF NOT EXISTS proxy_ua_blacklist (id TEXT PRIMARY KEY, rule_id TEXT, value TEXT)`,
		`CREATE TABLE IF NOT EXISTS ddns_configs (id TEXT PRIMARY KEY, tenant_id TEXT, name TEXT, provider TEXT, domain TEXT, sub_domain TEXT, record_type TEXT, ttl INTEGER, credentials TEXT, get_ip_url TEXT, get_ip_type TEXT, net_interface TEXT, interval_seconds INTEGER, enabled INTEGER, created_at DATETIME, updated_at DATETIME)`,
		`CREATE TABLE IF NOT EXISTS stun_tunnels (id TEXT PRIMARY KEY, tenant_id TEXT, name TEXT, protocol TEXT, local_port INTEGER, remote_port INTEGER, stun_server TEXT, target_addr TEXT, target_port INTEGER, enabled INTEGER, created_at DATETIME, updated_at DATETIME)`,
		`CREATE TABLE IF NOT EXISTS wol_devices (id TEXT PRIMARY KEY, tenant_id TEXT, name TEXT, mac TEXT, broadcast TEXT, port INTEGER, platform TEXT, platform_key TEXT, created_at DATETIME, updated_at DATETIME)`,
		`CREATE TABLE IF NOT EXISTS cron_jobs (id TEXT PRIMARY KEY, tenant_id TEXT, name TEXT, schedule TEXT, type TEXT, command TEXT, module_id TEXT, module_type TEXT, enabled INTEGER, last_run DATETIME, next_run DATETIME, created_at DATETIME, updated_at DATETIME)`,
		`CREATE TABLE IF NOT EXISTS acme_certificates (id TEXT PRIMARY KEY, tenant_id TEXT, name TEXT, domains TEXT, provider TEXT, dns_provider TEXT, dns_config TEXT, email TEXT, auto_renew INTEGER, renew_days INTEGER, cert_path TEXT DEFAULT '', key_path TEXT DEFAULT '', expires_at DATETIME, status TEXT DEFAULT 'pending', error TEXT DEFAULT '', created_at DATETIME, updated_at DATETIME)`,
		`CREATE TABLE IF NOT EXISTS storage_mounts (id TEXT PRIMARY KEY, tenant_id TEXT, name TEXT, type TEXT, source TEXT, username TEXT DEFAULT '', password TEXT DEFAULT '', services TEXT DEFAULT '[]', ftp_port INTEGER DEFAULT 2121, enabled INTEGER DEFAULT 0, created_at DATETIME, updated_at DATETIME)`,
		`CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY, value TEXT)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("create table failed: %v\n%s", err, s)
		}
	}
	return db
}

func doJSON(t *testing.T, fn func(http.ResponseWriter, *http.Request), method, target string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	fn(w, req)
	return w
}

func doPathJSON(t *testing.T, fn func(http.ResponseWriter, *http.Request), method, target, id string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", id)
	w := httptest.NewRecorder()
	fn(w, req)
	return w
}

func decodeResponse(t *testing.T, w *httptest.ResponseRecorder, v interface{}) {
	t.Helper()
	if err := json.NewDecoder(w.Body).Decode(v); err != nil {
		t.Fatalf("decode response %d: %v (body=%s)", w.Code, err, w.Body.String())
	}
}

func expectStatus(t *testing.T, w *httptest.ResponseRecorder, want int) {
	t.Helper()
	if w.Code != want {
		t.Fatalf("expected status %d, got %d (body=%s)", want, w.Code, w.Body.String())
	}
}
