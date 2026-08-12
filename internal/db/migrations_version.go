// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package db

import (
	"database/sql"
	"fmt"
	"io"
	"os"
	"strings"
)

// SchemaVersion is the current schema version. Bump it when business tables
// change so existing databases get a pre-migration backup.
const SchemaVersion = 3

// ensureSchemaMigrations creates the version bookkeeping table. The DDL is
// portable across SQLite and PostgreSQL.
func ensureSchemaMigrations(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`)
	return err
}

func currentSchemaVersion(db *sql.DB) (int, error) {
	var v int
	if err := db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&v); err != nil {
		return 0, err
	}
	return v, nil
}

func recordSchemaVersion(db *sql.DB, v int) error {
	_, err := db.Exec(`INSERT INTO schema_migrations (version) VALUES (?) ON CONFLICT(version) DO NOTHING`, v)
	return err
}

// backupSQLiteBeforeUpgrade snapshots a file-based SQLite database before a
// migration runs. In-memory databases are skipped.
func backupSQLiteBeforeUpgrade(path string) error {
	if path == ":memory:" || strings.Contains(path, "mode=memory") {
		return nil
	}
	src, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open source db: %w", err)
	}
	defer src.Close()

	backupPath := path + ".pre-upgrade.bak"
	dst, err := os.OpenFile(backupPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("create backup: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("copy backup: %w", err)
	}
	return nil
}

func isFileBasedSQLite(path string) bool {
	return path != ":memory:" && !strings.Contains(path, "mode=memory")
}
