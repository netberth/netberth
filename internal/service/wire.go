// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package service

import (
	"database/sql"
	"encoding/json"
	"os"

	"github.com/netberth/netberth/internal/engine/acme"
	"github.com/netberth/netberth/internal/engine/cron"
	"github.com/netberth/netberth/internal/engine/ddns"
	"github.com/netberth/netberth/internal/engine/forward"
	"github.com/netberth/netberth/internal/engine/proxy"
	"github.com/netberth/netberth/internal/engine/storage"
	"github.com/netberth/netberth/internal/engine/stun"
	"github.com/netberth/netberth/internal/engine/wol"
	"github.com/netberth/netberth/internal/model"
	"github.com/netberth/netberth/pkg/logger"
)

type Wire struct {
	db      *sql.DB
	bus     *Bus
	Forward *forward.Engine
	Proxy   *proxy.Engine
	DDNS    *ddns.Engine
	STUN    *stun.Engine
	WOL     *wol.Engine
	Cron    *cron.Engine
	ACME    *acme.Engine
	Storage *storage.Engine
}

func NewWire(db *sql.DB, certDir string) *Wire {
	w := &Wire{db: db, bus: NewBus()}

	w.Forward = forward.New(&forwardDB{db})
	w.Proxy = proxy.New(&proxyDB{db})
	w.DDNS = ddns.New(&ddnsDB{db})
	w.STUN = stun.New(&stunDB{db})
	w.WOL = wol.New(&wolDB{db})
	w.Cron = cron.New(&cronDB{db})
	w.ACME = acme.New(&acmeDB{db}, certDir)
	w.Storage = storage.New(&storageDB{db})

	w.setupEvents()
	return w
}

func (w *Wire) StartAll() error {
	for _, fn := range []func() error{w.Forward.Start} {
		if err := fn(); err != nil {
			return err
		}
	}
	for _, fn := range []func(string) error{
		func(s string) error {
			proxyPort := os.Getenv("NB_PROXY_PORT")
			if proxyPort == "" {
				proxyPort = "8080"
			}
			return w.Proxy.Start(":" + proxyPort)
		},
	} {
		if err := fn(""); err != nil {
			logger.Log.Warn().Err(err).Msg("engine start warning")
		}
	}
	for _, fn := range []func() error{w.DDNS.Start, w.STUN.Start, w.Cron.Start, w.ACME.Start, w.Storage.Start} {
		if err := fn(); err != nil {
			logger.Log.Warn().Err(err).Msg("engine start warning")
		}
	}
	return nil
}

func (w *Wire) StopAll() {
	w.Forward.Stop()
	w.Proxy.Stop()
	w.DDNS.Stop()
	w.STUN.Stop()
}

func (w *Wire) Subscribe(resource string, onCreate, onUpdate, onDelete func(string)) {
	w.bus.Subscribe(EventType(resource+":created"), func(e Event) { onCreate(e.ID) })
	w.bus.Subscribe(EventType(resource+":updated"), func(e Event) { onUpdate(e.ID) })
	w.bus.Subscribe(EventType(resource+":deleted"), func(e Event) { onDelete(e.ID) })
}

func (w *Wire) setupEvents() {
	w.Subscribe("forward",
		func(id string) {
			r := loadForwardRule(w.db, id)
			if r != nil {
				w.Forward.Reload(*r)
			}
		},
		func(id string) {
			r := loadForwardRule(w.db, id)
			if r != nil {
				w.Forward.Reload(*r)
			}
		},
		w.Forward.Remove,
	)
	w.Subscribe("proxy",
		func(id string) {
			r := loadProxyRule(w.db, id)
			if r != nil {
				w.Proxy.Reload(*r)
			}
		},
		func(id string) {
			r := loadProxyRule(w.db, id)
			if r != nil {
				w.Proxy.Reload(*r)
			}
		},
		w.Proxy.Remove,
	)
	w.Subscribe("ddns",
		func(id string) {
			c := loadDDNSConfig(w.db, id)
			if c != nil {
				w.DDNS.Reload(*c)
			}
		},
		func(id string) {
			c := loadDDNSConfig(w.db, id)
			if c != nil {
				w.DDNS.Reload(*c)
			}
		},
		w.DDNS.Remove,
	)
	w.Subscribe("stun",
		func(id string) {
			t := loadSTUNTunnel(w.db, id)
			if t != nil {
				w.STUN.Reload(*t)
			}
		},
		func(id string) {
			t := loadSTUNTunnel(w.db, id)
			if t != nil {
				w.STUN.Reload(*t)
			}
		},
		w.STUN.Remove,
	)
	w.Subscribe("cron",
		func(id string) {
			j := loadCronJob(w.db, id)
			if j != nil {
				w.Cron.Reload(*j)
			}
		},
		func(id string) {
			j := loadCronJob(w.db, id)
			if j != nil {
				w.Cron.Reload(*j)
			}
		},
		w.Cron.Remove,
	)
	w.Subscribe("storage",
		func(id string) {
			m := loadStorageMount(w.db, id)
			if m != nil {
				w.Storage.Reload(*m)
			}
		},
		func(id string) {
			m := loadStorageMount(w.db, id)
			if m != nil {
				w.Storage.Reload(*m)
			}
		},
		w.Storage.Remove,
	)
}

func (w *Wire) Bus() *Bus { return w.bus }

// Database adapters implementing engine interfaces

type forwardDB struct{ *sql.DB }

func (d *forwardDB) GetRules() ([]model.ForwardRule, error) {
	rows, err := d.DB.Query(`SELECT id, tenant_id, owner_id, name, protocol, listen_addr, listen_port, target_addr, target_port, enable_ipv6, max_conns, enabled, schedule_on, schedule_off FROM forward_rules`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var rules []model.ForwardRule
	for rows.Next() {
		var r model.ForwardRule
		rows.Scan(&r.ID, &r.TenantID, &r.OwnerID, &r.Name, &r.Protocol, &r.ListenAddr, &r.ListenPort, &r.TargetAddr, &r.TargetPort, &r.EnableIPv6, &r.MaxConns, &r.Enabled, &r.ScheduleOn, &r.ScheduleOff)
		r.Whitelist = loadWireACL(d.DB, "forward_whitelist", r.ID)
		r.Blacklist = loadWireACL(d.DB, "forward_blacklist", r.ID)
		rules = append(rules, r)
	}
	return rules, nil
}

type proxyDB struct{ *sql.DB }

func (d *proxyDB) GetRules() ([]model.ProxyRule, error) {
	rows, _ := d.DB.Query("SELECT id, tenant_id, owner_id, name, target_url, tls_enabled, cert_id, force_https, http2, websocket, url_rewrite, basic_auth_user, basic_auth_hash, max_conns, enabled FROM proxy_rules")
	defer rows.Close()
	var rules []model.ProxyRule
	for rows.Next() {
		var r model.ProxyRule
		rows.Scan(&r.ID, &r.TenantID, &r.OwnerID, &r.Name, &r.TargetURL, &r.TLSEnabled, &r.CertID, &r.ForceHTTPS, &r.HTTP2, &r.Websocket, &r.URLRewrite, &r.BasicAuthUser, &r.BasicAuthHash, &r.MaxConns, &r.Enabled)
		r.Domains = loadWireStrings(d.DB, "proxy_domains", r.ID)
		r.IPWhitelist = loadWireACL(d.DB, "proxy_ip_whitelist", r.ID)
		r.IPBlacklist = loadWireACL(d.DB, "proxy_ip_blacklist", r.ID)
		r.UAWhitelist = loadWireStrings(d.DB, "proxy_ua_whitelist", r.ID)
		r.UABlacklist = loadWireStrings(d.DB, "proxy_ua_blacklist", r.ID)
		rules = append(rules, r)
	}
	return rules, nil
}

type ddnsDB struct{ *sql.DB }

func (d *ddnsDB) GetConfigs() ([]model.DDNSConfig, error) {
	rows, _ := d.DB.Query("SELECT id, name, provider, domain, sub_domain, record_type, ttl, credentials, get_ip_url, get_ip_type, net_interface, interval_seconds, enabled FROM ddns_configs")
	defer rows.Close()
	var cfgs []model.DDNSConfig
	for rows.Next() {
		var c model.DDNSConfig
		var creds string
		rows.Scan(&c.ID, &c.Name, &c.Provider, &c.Domain, &c.SubDomain, &c.RecordType, &c.TTL, &creds, &c.GetIPURL, &c.GetIPType, &c.NetInterface, &c.Interval, &c.Enabled)
		json.Unmarshal([]byte(creds), &c.Credentials)
		cfgs = append(cfgs, c)
	}
	return cfgs, nil
}

func (d *ddnsDB) UpdateIP(id, ip string) error {
	_, err := d.DB.Exec("UPDATE ddns_configs SET updated_at = CURRENT_TIMESTAMP WHERE id = ?", id)
	return err
}

type stunDB struct{ *sql.DB }

func (d *stunDB) GetTunnels() ([]model.STUNTunnel, error) {
	rows, _ := d.DB.Query("SELECT id, name, protocol, local_port, remote_port, stun_server, target_addr, target_port, enabled FROM stun_tunnels")
	defer rows.Close()
	var tunnels []model.STUNTunnel
	for rows.Next() {
		var t model.STUNTunnel
		rows.Scan(&t.ID, &t.Name, &t.Protocol, &t.LocalPort, &t.RemotePort, &t.STUNServer, &t.TargetAddr, &t.TargetPort, &t.Enabled)
		tunnels = append(tunnels, t)
	}
	return tunnels, nil
}

type wolDB struct{ *sql.DB }

func (d *wolDB) GetDevices() ([]model.WOLDevice, error) {
	rows, _ := d.DB.Query("SELECT id, name, mac, broadcast, port FROM wol_devices")
	defer rows.Close()
	var devices []model.WOLDevice
	for rows.Next() {
		var dev model.WOLDevice
		rows.Scan(&dev.ID, &dev.Name, &dev.MAC, &dev.Broadcast, &dev.Port)
		devices = append(devices, dev)
	}
	return devices, nil
}

type cronDB struct{ *sql.DB }

func (d *cronDB) GetJobs() ([]model.CronJob, error) {
	rows, _ := d.DB.Query("SELECT id, name, schedule, type, command, module_id, module_type, enabled FROM cron_jobs")
	defer rows.Close()
	var jobs []model.CronJob
	for rows.Next() {
		var j model.CronJob
		rows.Scan(&j.ID, &j.Name, &j.Schedule, &j.Type, &j.Command, &j.ModuleID, &j.ModuleType, &j.Enabled)
		jobs = append(jobs, j)
	}
	return jobs, nil
}

type acmeDB struct{ *sql.DB }

func (d *acmeDB) GetCertificates() ([]model.ACMECertificate, error) {
	rows, _ := d.DB.Query("SELECT id, name, domains, provider, dns_provider, dns_config, email, auto_renew, renew_days, expires_at, status FROM acme_certificates")
	defer rows.Close()
	var certs []model.ACMECertificate
	for rows.Next() {
		var c model.ACMECertificate
		var domains, dnsConfig string
		rows.Scan(&c.ID, &c.Name, &domains, &c.Provider, &c.DNSProvider, &dnsConfig, &c.Email, &c.AutoRenew, &c.RenewDays, &c.ExpiresAt, &c.Status)
		json.Unmarshal([]byte(domains), &c.Domains)
		json.Unmarshal([]byte(dnsConfig), &c.DNSConfig)
		certs = append(certs, c)
	}
	return certs, nil
}

func (d *acmeDB) UpdateCertificate(cert model.ACMECertificate) error {
	domains, _ := json.Marshal(cert.Domains)
	_, err := d.DB.Exec(
		`UPDATE acme_certificates SET cert_path=?, key_path=?, expires_at=?, status=?, error=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`,
		cert.CertPath, cert.KeyPath, cert.ExpiresAt, cert.Status, cert.Error, cert.ID,
	)
	_ = domains
	return err
}

type storageDB struct{ *sql.DB }

func (d *storageDB) GetMounts() ([]model.StorageMount, error) {
	rows, _ := d.DB.Query("SELECT id, name, type, source, username, password, services, ftp_port, enabled FROM storage_mounts")
	defer rows.Close()
	var mounts []model.StorageMount
	for rows.Next() {
		var m model.StorageMount
		var services string
		rows.Scan(&m.ID, &m.Name, &m.Type, &m.Source, &m.Username, &m.Password, &services, &m.FTPPort, &m.Enabled)
		json.Unmarshal([]byte(services), &m.Services)
		mounts = append(mounts, m)
	}
	return mounts, nil
}

// Loaders that read a single record by ID for reload on change

func loadForwardRule(db *sql.DB, id string) *model.ForwardRule {
	var r model.ForwardRule
	err := db.QueryRow(
		"SELECT id, tenant_id, owner_id, name, protocol, listen_addr, listen_port, target_addr, target_port, enable_ipv6, max_conns, enabled, schedule_on, schedule_off FROM forward_rules WHERE id=?", id,
	).Scan(&r.ID, &r.TenantID, &r.OwnerID, &r.Name, &r.Protocol, &r.ListenAddr, &r.ListenPort, &r.TargetAddr, &r.TargetPort, &r.EnableIPv6, &r.MaxConns, &r.Enabled, &r.ScheduleOn, &r.ScheduleOff)
	if err != nil {
		return nil
	}
	r.Whitelist = loadWireACL(db, "forward_whitelist", id)
	r.Blacklist = loadWireACL(db, "forward_blacklist", id)
	return &r
}

func loadProxyRule(db *sql.DB, id string) *model.ProxyRule {
	var r model.ProxyRule
	err := db.QueryRow("SELECT id, tenant_id, owner_id, name, target_url, tls_enabled, force_https, http2, websocket, url_rewrite, basic_auth_user, basic_auth_hash, max_conns, enabled FROM proxy_rules WHERE id=?", id).
		Scan(&r.ID, &r.TenantID, &r.OwnerID, &r.Name, &r.TargetURL, &r.TLSEnabled, &r.ForceHTTPS, &r.HTTP2, &r.Websocket, &r.URLRewrite, &r.BasicAuthUser, &r.BasicAuthHash, &r.MaxConns, &r.Enabled)
	if err != nil {
		return nil
	}
	r.Domains = loadWireStrings(db, "proxy_domains", id)
	r.IPWhitelist = loadWireACL(db, "proxy_ip_whitelist", id)
	r.IPBlacklist = loadWireACL(db, "proxy_ip_blacklist", id)
	r.UAWhitelist = loadWireStrings(db, "proxy_ua_whitelist", id)
	r.UABlacklist = loadWireStrings(db, "proxy_ua_blacklist", id)
	return &r
}

func loadWireACL(db *sql.DB, table, ruleID string) []model.ACLEntry {
	rows, _ := db.Query("SELECT id, value FROM "+table+" WHERE rule_id = ?", ruleID)
	if rows == nil {
		return nil
	}
	defer rows.Close()
	var e []model.ACLEntry
	for rows.Next() {
		var a model.ACLEntry
		rows.Scan(&a.ID, &a.Value)
		e = append(e, a)
	}
	return e
}

func loadWireStrings(db *sql.DB, table, ruleID string) []string {
	rows, _ := db.Query("SELECT value FROM "+table+" WHERE rule_id = ?", ruleID)
	if rows == nil {
		return nil
	}
	defer rows.Close()
	var s []string
	for rows.Next() {
		var v string
		rows.Scan(&v)
		s = append(s, v)
	}
	return s
}

func loadDDNSConfig(db *sql.DB, id string) *model.DDNSConfig {
	var c model.DDNSConfig
	var creds string
	err := db.QueryRow("SELECT id, name, provider, domain, sub_domain, record_type, ttl, credentials, get_ip_url, get_ip_type, net_interface, interval_seconds, enabled FROM ddns_configs WHERE id=?", id).
		Scan(&c.ID, &c.Name, &c.Provider, &c.Domain, &c.SubDomain, &c.RecordType, &c.TTL, &creds, &c.GetIPURL, &c.GetIPType, &c.NetInterface, &c.Interval, &c.Enabled)
	if err != nil {
		return nil
	}
	json.Unmarshal([]byte(creds), &c.Credentials)
	return &c
}

func loadSTUNTunnel(db *sql.DB, id string) *model.STUNTunnel {
	var t model.STUNTunnel
	err := db.QueryRow("SELECT id, name, protocol, local_port, remote_port, stun_server, target_addr, target_port, enabled FROM stun_tunnels WHERE id=?", id).
		Scan(&t.ID, &t.Name, &t.Protocol, &t.LocalPort, &t.RemotePort, &t.STUNServer, &t.TargetAddr, &t.TargetPort, &t.Enabled)
	if err != nil {
		return nil
	}
	return &t
}

func loadCronJob(db *sql.DB, id string) *model.CronJob {
	var j model.CronJob
	err := db.QueryRow("SELECT id, name, schedule, type, command, module_id, module_type, enabled FROM cron_jobs WHERE id=?", id).
		Scan(&j.ID, &j.Name, &j.Schedule, &j.Type, &j.Command, &j.ModuleID, &j.ModuleType, &j.Enabled)
	if err != nil {
		return nil
	}
	return &j
}

func loadStorageMount(db *sql.DB, id string) *model.StorageMount {
	var m model.StorageMount
	var services string
	err := db.QueryRow("SELECT id, name, type, source, username, password, services, ftp_port, enabled FROM storage_mounts WHERE id=?", id).
		Scan(&m.ID, &m.Name, &m.Type, &m.Source, &m.Username, &m.Password, &services, &m.FTPPort, &m.Enabled)
	if err != nil {
		return nil
	}
	json.Unmarshal([]byte(services), &m.Services)
	return &m
}
