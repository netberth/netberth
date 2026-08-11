// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package service

import (
	"database/sql"
	"fmt"
	"sync/atomic"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/netberth/netberth/internal/model"
)

var wireTestDBCounter int64

func setupWireDB(t *testing.T) *sql.DB {
	t.Helper()
	n := atomic.AddInt64(&wireTestDBCounter, 1)
	db, err := sql.Open("sqlite3", fmt.Sprintf("file:nbwire_%d?mode=memory&cache=shared", n))
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
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("create table: %v\n%s", err, s)
		}
	}
	return db
}

func seedWireRows(t *testing.T, db *sql.DB) {
	t.Helper()
	rows := []string{
		`INSERT INTO forward_rules (id, tenant_id, owner_id, name, protocol, listen_addr, listen_port, target_addr, target_port, enable_ipv6, max_conns, enabled) VALUES ('f1','t1','u1','fwd','tcp','',22010,'192.0.2.1',80,1,0,1)`,
		`INSERT INTO forward_whitelist (id, rule_id, value) VALUES ('w1','f1','192.0.2.0/24')`,
		`INSERT INTO forward_blacklist (id, rule_id, value) VALUES ('b1','f1','203.0.113.0/24')`,
		`INSERT INTO proxy_rules (id, tenant_id, owner_id, name, target_url, tls_enabled, cert_id, force_https, http2, websocket, url_rewrite, basic_auth_user, basic_auth_hash, max_conns, enabled) VALUES ('p1','t1','u1','prx','http://192.0.2.2:8080',0,'',0,1,0,'','','',0,1)`,
		`INSERT INTO proxy_domains (id, rule_id, domain) VALUES ('pd1','p1','example.com')`,
		`INSERT INTO proxy_ip_whitelist (id, rule_id, value) VALUES ('pw1','p1','192.0.2.0/24')`,
		`INSERT INTO proxy_ua_whitelist (id, rule_id, value) VALUES ('pu1','p1','Mozilla/5.0')`,
		`INSERT INTO ddns_configs (id, tenant_id, name, provider, domain, sub_domain, record_type, ttl, credentials, get_ip_url, get_ip_type, net_interface, interval_seconds, enabled) VALUES ('d1','t1','dd','cloudflare','example.com','@','A',600,'{"token":"x"}','','url','',300,1)`,
		`INSERT INTO stun_tunnels (id, tenant_id, name, protocol, local_port, remote_port, stun_server, target_addr, target_port, enabled) VALUES ('s1','t1','st','tcp',30010,30011,'stun.example.com:19302','192.0.2.3',80,1)`,
		`INSERT INTO wol_devices (id, tenant_id, name, mac, broadcast, port) VALUES ('wol1','t1','pc','AA:BB:CC:DD:EE:FF','255.255.255.255',9)`,
		`INSERT INTO cron_jobs (id, tenant_id, name, schedule, type, command, module_id, module_type, enabled) VALUES ('c1','t1','job','*/5 * * * *','shell','echo hi','','',1)`,
		`INSERT INTO acme_certificates (id, tenant_id, name, domains, provider, dns_provider, dns_config, email, auto_renew, renew_days, status) VALUES ('a1','t1','cert','["example.com"]','letsencrypt','cloudflare','{"token":"x"}','admin@example.com',1,30,'pending')`,
		`INSERT INTO storage_mounts (id, tenant_id, name, type, source, services, ftp_port, enabled) VALUES ('m1','t1','mnt','local','/mnt/user/media','["webdav"]',2121,1)`,
	}
	for _, s := range rows {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("seed: %v\n%s", err, s)
		}
	}
}

func TestWireDBAdapters(t *testing.T) {
	db := setupWireDB(t)
	seedWireRows(t, db)

	rules, err := (&forwardDB{db}).GetRules()
	if err != nil {
		t.Fatalf("forward GetRules: %v", err)
	}
	if len(rules) != 1 || rules[0].ID != "f1" {
		t.Fatalf("forward rules mismatch: %+v", rules)
	}
	if len(rules[0].Whitelist) != 1 || rules[0].Whitelist[0].Value != "192.0.2.0/24" {
		t.Fatalf("forward whitelist mismatch: %+v", rules[0].Whitelist)
	}
	if len(rules[0].Blacklist) != 1 || rules[0].Blacklist[0].Value != "203.0.113.0/24" {
		t.Fatalf("forward blacklist mismatch: %+v", rules[0].Blacklist)
	}

	proxies, err := (&proxyDB{db}).GetRules()
	if err != nil {
		t.Fatalf("proxy GetRules: %v", err)
	}
	if len(proxies) != 1 || len(proxies[0].Domains) != 1 || proxies[0].Domains[0] != "example.com" {
		t.Fatalf("proxy rules mismatch: %+v", proxies)
	}
	if len(proxies[0].IPWhitelist) != 1 || len(proxies[0].UAWhitelist) != 1 {
		t.Fatalf("proxy lists mismatch: %+v", proxies[0])
	}

	cfgs, err := (&ddnsDB{db}).GetConfigs()
	if err != nil {
		t.Fatalf("ddns GetConfigs: %v", err)
	}
	if len(cfgs) != 1 || cfgs[0].Credentials["token"] != "x" {
		t.Fatalf("ddns configs mismatch: %+v", cfgs)
	}
	if err := (&ddnsDB{db}).UpdateIP("d1", "1.2.3.4"); err != nil {
		t.Fatalf("ddns UpdateIP: %v", err)
	}

	tunnels, err := (&stunDB{db}).GetTunnels()
	if err != nil {
		t.Fatalf("stun GetTunnels: %v", err)
	}
	if len(tunnels) != 1 || tunnels[0].STUNServer != "stun.example.com:19302" {
		t.Fatalf("stun tunnels mismatch: %+v", tunnels)
	}

	devices, err := (&wolDB{db}).GetDevices()
	if err != nil {
		t.Fatalf("wol GetDevices: %v", err)
	}
	if len(devices) != 1 || devices[0].MAC != "AA:BB:CC:DD:EE:FF" {
		t.Fatalf("wol devices mismatch: %+v", devices)
	}

	jobs, err := (&cronDB{db}).GetJobs()
	if err != nil {
		t.Fatalf("cron GetJobs: %v", err)
	}
	if len(jobs) != 1 || jobs[0].Schedule != "*/5 * * * *" {
		t.Fatalf("cron jobs mismatch: %+v", jobs)
	}

	certs, err := (&acmeDB{db}).GetCertificates()
	if err != nil {
		t.Fatalf("acme GetCertificates: %v", err)
	}
	if len(certs) != 1 || len(certs[0].Domains) != 1 || certs[0].Status != "pending" {
		t.Fatalf("acme certs mismatch: %+v", certs)
	}
	if err := (&acmeDB{db}).UpdateCertificate(model.ACMECertificate{
		ID: "a1", CertPath: "/certs/a.pem", KeyPath: "/certs/a.key", Status: "valid",
	}); err != nil {
		t.Fatalf("acme UpdateCertificate: %v", err)
	}
	var status string
	db.QueryRow("SELECT status FROM acme_certificates WHERE id='a1'").Scan(&status)
	if status != "valid" {
		t.Fatalf("acme status not updated: %s", status)
	}

	mounts, err := (&storageDB{db}).GetMounts()
	if err != nil {
		t.Fatalf("storage GetMounts: %v", err)
	}
	if len(mounts) != 1 || len(mounts[0].Services) != 1 || mounts[0].Services[0] != "webdav" {
		t.Fatalf("storage mounts mismatch: %+v", mounts)
	}
}

func TestWireLoaders(t *testing.T) {
	db := setupWireDB(t)
	seedWireRows(t, db)

	if r := loadForwardRule(db, "f1"); r == nil || len(r.Whitelist) != 1 {
		t.Fatalf("loadForwardRule mismatch: %+v", r)
	}
	if r := loadForwardRule(db, "missing"); r != nil {
		t.Fatalf("expected nil forward rule, got %+v", r)
	}
	if r := loadProxyRule(db, "p1"); r == nil || len(r.Domains) != 1 || r.Domains[0] != "example.com" {
		t.Fatalf("loadProxyRule mismatch: %+v", r)
	}
	if r := loadProxyRule(db, "missing"); r != nil {
		t.Fatalf("expected nil proxy rule, got %+v", r)
	}
	if c := loadDDNSConfig(db, "d1"); c == nil || c.Credentials["token"] != "x" {
		t.Fatalf("loadDDNSConfig mismatch: %+v", c)
	}
	if c := loadDDNSConfig(db, "missing"); c != nil {
		t.Fatalf("expected nil ddns config, got %+v", c)
	}
	if s := loadSTUNTunnel(db, "s1"); s == nil || s.TargetAddr != "192.0.2.3" {
		t.Fatalf("loadSTUNTunnel mismatch: %+v", s)
	}
	if s := loadSTUNTunnel(db, "missing"); s != nil {
		t.Fatalf("expected nil stun tunnel, got %+v", s)
	}
	if j := loadCronJob(db, "c1"); j == nil || j.Schedule != "*/5 * * * *" {
		t.Fatalf("loadCronJob mismatch: %+v", j)
	}
	if j := loadCronJob(db, "missing"); j != nil {
		t.Fatalf("expected nil cron job, got %+v", j)
	}
	if m := loadStorageMount(db, "m1"); m == nil || len(m.Services) != 1 {
		t.Fatalf("loadStorageMount mismatch: %+v", m)
	}
	if m := loadStorageMount(db, "missing"); m != nil {
		t.Fatalf("expected nil storage mount, got %+v", m)
	}
	if a := loadWireACL(db, "forward_whitelist", "f1"); len(a) != 1 {
		t.Fatalf("loadWireACL mismatch: %+v", a)
	}
	if a := loadWireACL(db, "forward_whitelist", "missing"); len(a) != 0 {
		t.Fatalf("expected empty ACL, got %+v", a)
	}
	if s := loadWireStrings(db, "proxy_ua_whitelist", "p1"); len(s) != 1 {
		t.Fatalf("loadWireStrings mismatch: %+v", s)
	}
	if d := loadWireDomains(db, "p1"); len(d) != 1 || d[0] != "example.com" {
		t.Fatalf("loadWireDomains mismatch: %+v", d)
	}
}

func TestWireNewSubscribeAndEvents(t *testing.T) {
	db := setupWireDB(t)
	w := NewWire(db, t.TempDir())
	if w.Forward == nil || w.Proxy == nil || w.DDNS == nil || w.STUN == nil ||
		w.WOL == nil || w.Cron == nil || w.ACME == nil || w.Storage == nil {
		t.Fatal("NewWire did not initialize all engines")
	}
	if w.Bus() == nil {
		t.Fatal("expected non-nil bus")
	}

	var created, updated, deleted int32
	w.Subscribe("probe",
		func(id string) { atomic.AddInt32(&created, 1) },
		func(id string) { atomic.AddInt32(&updated, 1) },
		func(id string) { atomic.AddInt32(&deleted, 1) },
	)
	w.Bus().Publish(Event{Type: "probe:created", ID: "x"})
	w.Bus().Publish(Event{Type: "probe:updated", ID: "x"})
	w.Bus().Publish(Event{Type: "probe:deleted", ID: "x"})
	if atomic.LoadInt32(&created) != 1 || atomic.LoadInt32(&updated) != 1 || atomic.LoadInt32(&deleted) != 1 {
		t.Fatalf("probe counters: %d/%d/%d", created, updated, deleted)
	}

	// setupEvents: missing rows exercise the nil branches without engine work.
	w.Bus().Publish(Event{Type: EventForwardCreated, ID: "missing"})
	w.Bus().Publish(Event{Type: EventProxyUpdated, ID: "missing"})
	w.Bus().Publish(Event{Type: EventDDNSDeleted, ID: "missing"})
	w.Bus().Publish(Event{Type: EventSTUNCreated, ID: "missing"})
	w.Bus().Publish(Event{Type: EventCronUpdated, ID: "missing"})
	w.Bus().Publish(Event{Type: EventStorageDeleted, ID: "missing"})

	// Reload branch with a disabled forward rule (safe: no listeners started).
	db.Exec("INSERT INTO forward_rules (id, name, protocol, listen_port, target_addr, target_port, enabled) VALUES ('f2','x','tcp',1,'192.0.2.9',80,0)")
	w.Bus().Publish(Event{Type: EventForwardCreated, ID: "f2"})

	w.StopAll()
}

func TestWireAdapterScanError(t *testing.T) {
	db := setupWireDB(t)
	db.Exec("DROP TABLE forward_whitelist")
	db.Exec("DROP TABLE forward_blacklist")
	db.Exec("DROP TABLE forward_rules")
	db.Exec(`CREATE TABLE forward_rules (id TEXT PRIMARY KEY, tenant_id TEXT, owner_id TEXT, name TEXT, protocol TEXT, listen_addr TEXT, listen_port TEXT, target_addr TEXT, target_port INTEGER, enable_ipv6 INTEGER, max_conns INTEGER, enabled INTEGER, schedule_on TEXT, schedule_off TEXT, created_at DATETIME, updated_at DATETIME)`)
	db.Exec(`INSERT INTO forward_rules (id, name, listen_port) VALUES ('f1','x','not-a-port')`)

	if _, err := (&forwardDB{db}).GetRules(); err == nil {
		t.Fatal("expected scan error from broken schema")
	}
}
