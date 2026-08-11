// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package db

import "testing"

func TestRebindSQLiteIdentity(t *testing.T) {
	q := "SELECT * FROM users WHERE id = ? AND role = ?"
	if got := Rebind(q, DialectSQLite); got != q {
		t.Fatalf("sqlite rebind changed query: %s", got)
	}
}

func TestRebindPostgres(t *testing.T) {
	cases := []struct{ in, want string }{
		{"SELECT * FROM users WHERE id = ?", "SELECT * FROM users WHERE id = $1"},
		{"SELECT * FROM users WHERE id = ? AND role = ?", "SELECT * FROM users WHERE id = $1 AND role = $2"},
		{"INSERT INTO t (a, b) VALUES (?, ?)", "INSERT INTO t (a, b) VALUES ($1, $2)"},
		{"UPDATE t SET a = ?, b = ? WHERE id = ?", "UPDATE t SET a = $1, b = $2 WHERE id = $3"},
		{"DELETE FROM t WHERE id = ?", "DELETE FROM t WHERE id = $1"},
		{"SELECT '?' AS q, ? AS v", "SELECT '?' AS q, $1 AS v"},
		{"SELECT ? /* ? */ , 'it''s ?'", "SELECT $1 /* ? */ , 'it''s ?'"},
	}
	for _, c := range cases {
		if got := Rebind(c.in, DialectPostgres); got != c.want {
			t.Errorf("Rebind(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestRebindNoPlaceholders(t *testing.T) {
	q := "SELECT 1"
	if got := Rebind(q, DialectPostgres); got != q {
		t.Fatalf("expected unchanged query, got %q", got)
	}
}

func TestRebindSkipsCommentsAndQuotedIdentifiers(t *testing.T) {
	cases := []struct{ in, want string }{
		{"SELECT ? /* ? */ , 'it''s ?'", "SELECT $1 /* ? */ , 'it''s ?'"},
		{"SELECT ? -- trailing ?\n, ?", "SELECT $1 -- trailing ?\n, $2"},
		{`SELECT "?" FROM t WHERE id = ?`, `SELECT "?" FROM t WHERE id = $1`},
		{"SELECT ? /* a /* nested? */ still */, ?", "SELECT $1 /* a /* nested? */ still */, $2"},
	}
	for _, c := range cases {
		if got := Rebind(c.in, DialectPostgres); got != c.want {
			t.Errorf("Rebind(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
