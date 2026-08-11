// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package db

import (
	"os"
	"testing"
)

// TestPostgresIntegration exercises the full postgres path against a real
// server. It is skipped unless NB_TEST_POSTGRES_DSN is set, so CI does not
// require a Postgres instance.
func TestPostgresIntegration(t *testing.T) {
	dsn := os.Getenv("NB_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("NB_TEST_POSTGRES_DSN not set; skipping live Postgres integration")
	}

	database, err := OpenDatabase("postgres", dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	defer database.Close()

	// The rebind driver must convert '?' to $N transparently.
	var count int
	if err := database.QueryRow("SELECT COUNT(*) FROM users WHERE enabled = ?", 1).Scan(&count); err != nil {
		t.Fatalf("rebind query: %v", err)
	}

	seeded, err := SeedAdminUser(database, "integration-hash")
	if err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	if !seeded {
		t.Log("admin already seeded; skipping user assertions")
		return
	}
	var username, hash string
	if err := database.QueryRow("SELECT username, password_hash FROM users WHERE username = ?", "admin").Scan(&username, &hash); err != nil {
		t.Fatalf("query seeded admin: %v", err)
	}
	if username != "admin" || hash != "integration-hash" {
		t.Fatalf("unexpected seeded admin: %q/%q", username, hash)
	}
}
