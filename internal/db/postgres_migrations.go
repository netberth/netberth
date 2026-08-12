// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package db

import "database/sql"

// postgresMigrations mirrors the SQLite schema (internal/db/migrations.go).
// Column sets are kept in sync by postgres_migrations_test.go.
func postgresMigrations() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS tenants (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			plan TEXT NOT NULL DEFAULT 'free',
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL DEFAULT '',
			username TEXT UNIQUE NOT NULL,
			email TEXT DEFAULT '',
			password_hash TEXT NOT NULL,
			role TEXT NOT NULL DEFAULT 'admin',
			enabled INTEGER NOT NULL DEFAULT 1,
			otp_enabled INTEGER NOT NULL DEFAULT 0,
			otp_secret TEXT DEFAULT '',
			password_changed INTEGER NOT NULL DEFAULT 0,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (tenant_id) REFERENCES tenants(id)
		)`,
		`CREATE TABLE IF NOT EXISTS forward_rules (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL DEFAULT '',
			owner_id  TEXT NOT NULL DEFAULT '',
			name TEXT NOT NULL,
			protocol TEXT NOT NULL DEFAULT 'tcp',
			listen_addr TEXT NOT NULL DEFAULT '',
			listen_port INTEGER NOT NULL,
			target_addr TEXT NOT NULL,
			target_port INTEGER NOT NULL,
			enable_ipv6 INTEGER NOT NULL DEFAULT 1,
			max_conns INTEGER NOT NULL DEFAULT 0,
			enabled INTEGER NOT NULL DEFAULT 0,
			schedule_on TEXT DEFAULT '',
			schedule_off TEXT DEFAULT '',
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS forward_whitelist (
			id TEXT PRIMARY KEY,
			rule_id TEXT NOT NULL,
			value TEXT NOT NULL,
			FOREIGN KEY (rule_id) REFERENCES forward_rules(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS forward_blacklist (
			id TEXT PRIMARY KEY,
			rule_id TEXT NOT NULL,
			value TEXT NOT NULL,
			FOREIGN KEY (rule_id) REFERENCES forward_rules(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_fwd_tenant ON forward_rules(tenant_id)`,
		`CREATE INDEX IF NOT EXISTS idx_fwd_wl_rule ON forward_whitelist(rule_id)`,
		`CREATE INDEX IF NOT EXISTS idx_fwd_bl_rule ON forward_blacklist(rule_id)`,
		`CREATE TABLE IF NOT EXISTS proxy_rules (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL DEFAULT '',
			owner_id  TEXT NOT NULL DEFAULT '',
			name TEXT NOT NULL,
			target_url TEXT NOT NULL,
			tls_enabled INTEGER NOT NULL DEFAULT 0,
			cert_id TEXT DEFAULT '',
			force_https INTEGER NOT NULL DEFAULT 0,
			http2 INTEGER NOT NULL DEFAULT 1,
			websocket INTEGER NOT NULL DEFAULT 0,
			url_rewrite TEXT DEFAULT '',
			basic_auth_user TEXT DEFAULT '',
			basic_auth_hash TEXT DEFAULT '',
			max_conns INTEGER NOT NULL DEFAULT 0,
			enabled INTEGER NOT NULL DEFAULT 0,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS proxy_domains (
			id TEXT PRIMARY KEY,
			rule_id TEXT NOT NULL,
			domain TEXT NOT NULL,
			FOREIGN KEY (rule_id) REFERENCES proxy_rules(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS proxy_ip_whitelist (
			id TEXT PRIMARY KEY,
			rule_id TEXT NOT NULL,
			value TEXT NOT NULL,
			FOREIGN KEY (rule_id) REFERENCES proxy_rules(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS proxy_ip_blacklist (
			id TEXT PRIMARY KEY,
			rule_id TEXT NOT NULL,
			value TEXT NOT NULL,
			FOREIGN KEY (rule_id) REFERENCES proxy_rules(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS proxy_ua_whitelist (
			id TEXT PRIMARY KEY,
			rule_id TEXT NOT NULL,
			value TEXT NOT NULL,
			FOREIGN KEY (rule_id) REFERENCES proxy_rules(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS proxy_ua_blacklist (
			id TEXT PRIMARY KEY,
			rule_id TEXT NOT NULL,
			value TEXT NOT NULL,
			FOREIGN KEY (rule_id) REFERENCES proxy_rules(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_prx_tenant ON proxy_rules(tenant_id)`,
		`CREATE INDEX IF NOT EXISTS idx_prx_dom ON proxy_domains(rule_id)`,
		`CREATE INDEX IF NOT EXISTS idx_prx_iwl ON proxy_ip_whitelist(rule_id)`,
		`CREATE INDEX IF NOT EXISTS idx_prx_ibl ON proxy_ip_blacklist(rule_id)`,
		`CREATE TABLE IF NOT EXISTS ddns_configs (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL DEFAULT '',
			name TEXT NOT NULL,
			provider TEXT NOT NULL,
			domain TEXT NOT NULL,
			sub_domain TEXT NOT NULL DEFAULT '@',
			record_type TEXT NOT NULL DEFAULT 'A',
			ttl INTEGER NOT NULL DEFAULT 600,
			credentials TEXT NOT NULL DEFAULT '{}',
			get_ip_url TEXT DEFAULT '',
			get_ip_type TEXT NOT NULL DEFAULT 'url',
			net_interface TEXT DEFAULT '',
			interval_seconds INTEGER NOT NULL DEFAULT 300,
			enabled INTEGER NOT NULL DEFAULT 0,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS stun_tunnels (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL DEFAULT '',
			name TEXT NOT NULL,
			protocol TEXT NOT NULL DEFAULT 'tcp',
			local_port INTEGER NOT NULL,
			remote_port INTEGER NOT NULL,
			stun_server TEXT NOT NULL DEFAULT 'stun.l.google.com:19302',
			target_addr TEXT NOT NULL,
			target_port INTEGER NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 0,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS wol_devices (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL DEFAULT '',
			name TEXT NOT NULL,
			mac TEXT NOT NULL,
			broadcast TEXT NOT NULL DEFAULT '255.255.255.255',
			port INTEGER NOT NULL DEFAULT 9,
			platform TEXT DEFAULT '',
			platform_key TEXT DEFAULT '',
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS cron_jobs (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL DEFAULT '',
			name TEXT NOT NULL,
			schedule TEXT NOT NULL,
			type TEXT NOT NULL DEFAULT 'command',
			command TEXT DEFAULT '',
			module_id TEXT DEFAULT '',
			module_type TEXT DEFAULT '',
			enabled INTEGER NOT NULL DEFAULT 1,
			last_run TIMESTAMP,
			next_run TIMESTAMP,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS acme_certificates (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL DEFAULT '',
			name TEXT NOT NULL,
			domains TEXT NOT NULL DEFAULT '[]',
			provider TEXT NOT NULL DEFAULT 'letsencrypt',
			dns_provider TEXT NOT NULL DEFAULT '',
			dns_config TEXT NOT NULL DEFAULT '{}',
			email TEXT NOT NULL DEFAULT '',
			auto_renew INTEGER NOT NULL DEFAULT 1,
			renew_days INTEGER NOT NULL DEFAULT 30,
			cert_path TEXT DEFAULT '',
			key_path TEXT DEFAULT '',
			expires_at TIMESTAMP,
			status TEXT NOT NULL DEFAULT 'pending',
			error TEXT DEFAULT '',
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS storage_mounts (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL DEFAULT '',
			name TEXT NOT NULL,
			type TEXT NOT NULL DEFAULT 'local',
			source TEXT NOT NULL,
			username TEXT DEFAULT '',
			password TEXT DEFAULT '',
			services TEXT NOT NULL DEFAULT '["filebrowser"]',
			ftp_port INTEGER DEFAULT 2121,
			enabled INTEGER NOT NULL DEFAULT 0,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS refresh_tokens (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			token_hash TEXT NOT NULL UNIQUE,
			expires_at TIMESTAMP NOT NULL,
			revoked_at TIMESTAMP,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS audit_events (
			id BIGSERIAL PRIMARY KEY,
			tenant_id TEXT NOT NULL DEFAULT '',
			user_id TEXT NOT NULL DEFAULT '',
			username TEXT NOT NULL DEFAULT '',
			action TEXT NOT NULL DEFAULT '',
			resource_type TEXT NOT NULL DEFAULT '',
			resource_id TEXT NOT NULL DEFAULT '',
			changes TEXT NOT NULL DEFAULT '',
			remote_addr TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_resource ON audit_events(resource_type, resource_id)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_tenant ON audit_events(tenant_id)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_created ON audit_events(created_at)`,

		// === Webhooks (v1.3) ===
		`CREATE TABLE IF NOT EXISTS webhook_endpoints (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL DEFAULT '',
			name TEXT NOT NULL,
			url TEXT NOT NULL,
			secret TEXT NOT NULL DEFAULT '',
			events TEXT NOT NULL DEFAULT '[]',
			enabled INTEGER NOT NULL DEFAULT 1,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
	}
}

func runPostgresMigrations(db *sql.DB) error {
	for _, m := range postgresMigrations() {
		if _, err := db.Exec(m); err != nil {
			return err
		}
	}
	// Parity with SQLite's ensureColumn: new databases already have the
	// column, older ones created before v1.1 get it here.
	if _, err := db.Exec(`ALTER TABLE users ADD COLUMN IF NOT EXISTS enabled INTEGER NOT NULL DEFAULT 1`); err != nil {
		return err
	}
	return nil
}
