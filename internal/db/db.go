// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package db

import (
	"crypto/rand"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func Open(path string) (*sql.DB, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}

	// WAL mode: concurrent readers + single writer. _txlock=immediate prevents
	// "database is locked" by acquiring write locks eagerly. _busy_timeout=5000
	// means SQLite will wait up to 5 seconds before returning SQLITE_BUSY.
	dsn := path + "?_journal_mode=WAL&_foreign_keys=on&_busy_timeout=5000&_txlock=immediate&_cache_size=-8000"
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// SQLite WAL allows concurrent reads. Set connections to number of CPUs
	// minus one reserved for the writer, minimum 2.
	conns := max(2, runtime.NumCPU()-1)
	db.SetMaxOpenConns(conns)
	db.SetMaxIdleConns(conns)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(1 * time.Minute)

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}

	// PRAGMA for WAL safety and performance
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA busy_timeout=5000",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			return nil, fmt.Errorf("pragma %s: %w", p, err)
		}
	}

	if err := runMigrations(db); err != nil {
		return nil, fmt.Errorf("run migrations: %w", err)
	}
	return db, nil
}

func SeedAdminUser(db *sql.DB, passwordHash string) (bool, error) {
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count); err != nil {
		return false, err
	}
	if count > 0 {
		return false, nil
	}
	// Create default tenant first (required by FK)
	tenantID := newUUID()
	db.Exec("INSERT OR IGNORE INTO tenants (id, name, plan) VALUES (?, ?, ?)", tenantID, "Default", "free")
	id := newUUID()
	_, err := db.Exec(
		"INSERT INTO users (id, tenant_id, username, password_hash, role) VALUES (?, ?, ?, ?, ?)",
		id, tenantID, "admin", passwordHash, "admin",
	)
	return true, err
}

func newUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
