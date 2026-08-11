// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package db

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

var uuidRegex = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestOpenCreatesSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "netberth.db")
	database, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer database.Close()

	rows, err := database.Query("SELECT name FROM sqlite_master WHERE type='table' ORDER BY name")
	if err != nil {
		t.Fatalf("query tables: %v", err)
	}
	defer rows.Close()
	got := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan table: %v", err)
		}
		got[name] = true
	}

	want := []string{
		"tenants", "users", "forward_rules", "forward_whitelist", "forward_blacklist",
		"proxy_rules", "proxy_domains", "proxy_ip_whitelist", "proxy_ip_blacklist",
		"proxy_ua_whitelist", "proxy_ua_blacklist", "ddns_configs", "stun_tunnels",
		"wol_devices", "cron_jobs", "acme_certificates", "storage_mounts",
		"settings", "audit_events",
	}
	for _, table := range want {
		if !got[table] {
			t.Errorf("missing table %s", table)
		}
	}

	var mode string
	if err := database.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatalf("journal mode: %v", err)
	}
	if mode != "wal" {
		t.Errorf("expected WAL journal mode, got %q", mode)
	}

	var fk int
	if err := database.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil {
		t.Fatalf("foreign_keys pragma: %v", err)
	}
	if fk != 1 {
		t.Errorf("expected foreign_keys=1, got %d", fk)
	}

	// Data directory is created automatically.
	if _, err := os.Stat(filepath.Dir(path)); err != nil {
		t.Errorf("data dir not created: %v", err)
	}
}

func TestOpenIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "netberth.db")
	database, err := Open(path)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	database.Close()

	database2, err := Open(path)
	if err != nil {
		t.Fatalf("second open: %v", err)
	}
	defer database2.Close()

	var count int
	if err := database2.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table'").Scan(&count); err != nil {
		t.Fatalf("count tables: %v", err)
	}
	if count < 19 {
		t.Errorf("expected at least 19 tables after re-open, got %d", count)
	}
}

func TestSeedAdminUser(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "netberth.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer database.Close()

	seeded, err := SeedAdminUser(database, "hash-value")
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if !seeded {
		t.Fatal("expected first seed to report success")
	}

	var users, tenants int
	database.QueryRow("SELECT COUNT(*) FROM users").Scan(&users)
	database.QueryRow("SELECT COUNT(*) FROM tenants").Scan(&tenants)
	if users != 1 || tenants != 1 {
		t.Fatalf("expected 1 user and 1 tenant, got %d/%d", users, tenants)
	}

	var username, hash string
	if err := database.QueryRow("SELECT username, password_hash FROM users WHERE username='admin'").Scan(&username, &hash); err != nil {
		t.Fatalf("query admin: %v", err)
	}
	if username != "admin" || hash != "hash-value" {
		t.Fatalf("unexpected admin row: %q/%q", username, hash)
	}

	// Second seed must be a no-op.
	seeded2, err := SeedAdminUser(database, "other-hash")
	if err != nil {
		t.Fatalf("second seed: %v", err)
	}
	if seeded2 {
		t.Fatal("second seed should report false")
	}
	database.QueryRow("SELECT COUNT(*) FROM users").Scan(&users)
	if users != 1 {
		t.Fatalf("second seed created extra users: %d", users)
	}
}

func TestSeedAdminUserClosedDB(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "netberth.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	database.Close()
	if _, err := SeedAdminUser(database, "x"); err == nil {
		t.Fatal("expected error when seeding a closed database")
	}
}

func TestNewUUIDFormat(t *testing.T) {
	for i := 0; i < 20; i++ {
		id := newUUID()
		if !uuidRegex.MatchString(id) {
			t.Fatalf("invalid UUID format: %q", id)
		}
	}
	if newUUID() == newUUID() {
		t.Fatal("expected distinct UUIDs")
	}
}
