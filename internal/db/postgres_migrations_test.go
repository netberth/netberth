// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package db

import (
	"fmt"
	"sort"
	"strings"
	"testing"
)

// tableColumns extracts table -> column name set from CREATE TABLE DDL.
func tableColumns(ddls []string) map[string]map[string]bool {
	res := make(map[string]map[string]bool)
	for _, ddl := range ddls {
		trimmed := strings.TrimSpace(ddl)
		if !strings.HasPrefix(trimmed, "CREATE TABLE") {
			continue
		}
		open := strings.Index(trimmed, "(")
		close := strings.LastIndex(trimmed, ")")
		if open < 0 || close <= open {
			continue
		}
		afterTable := trimmed[len("CREATE TABLE"):]
		nameFields := strings.Fields(afterTable[:strings.Index(afterTable, "(")])
		if len(nameFields) == 0 {
			continue
		}
		table := strings.Trim(nameFields[len(nameFields)-1], `"`)

		cols := make(map[string]bool)
		body := stripSQLComments(trimmed[open+1 : close])
		depth := 0
		var cur strings.Builder
		flush := func() {
			col := strings.TrimSpace(cur.String())
			cur.Reset()
			if col == "" {
				return
			}
			upper := strings.ToUpper(col)
			for _, prefix := range []string{"CONSTRAINT", "FOREIGN", "PRIMARY", "UNIQUE", "CHECK"} {
				if strings.HasPrefix(upper, prefix) {
					return
				}
			}
			fields := strings.Fields(col)
			if len(fields) > 0 {
				cols[fields[0]] = true
			}
		}
		for _, r := range body {
			switch r {
			case '(':
				depth++
			case ')':
				depth--
			case ',':
				if depth == 0 {
					flush()
					continue
				}
			}
			cur.WriteRune(r)
		}
		flush()
		res[table] = cols
	}
	return res
}

func stripSQLComments(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '-' && i+1 < len(s) && s[i+1] == '-' {
			for i < len(s) && s[i] != '\n' {
				i++
			}
			if i < len(s) {
				b.WriteByte('\n')
			}
			continue
		}
		if s[i] == '/' && i+1 < len(s) && s[i+1] == '*' {
			i += 2
			for i+1 < len(s) && !(s[i] == '*' && s[i+1] == '/') {
				i++
			}
			i++
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func sortedKeys(m map[string]map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func TestSchemaParitySQLitePostgres(t *testing.T) {
	sqlite := tableColumns(sqliteMigrations())
	pg := tableColumns(postgresMigrations())

	// users.enabled is added by ensureColumn on SQLite (v1.1).
	sqlite["users"]["enabled"] = true

	for _, table := range sortedKeys(sqlite) {
		pgCols, ok := pg[table]
		if !ok {
			t.Errorf("postgres is missing table %s", table)
			continue
		}
		for col := range sqlite[table] {
			if !pgCols[col] {
				t.Errorf("postgres table %s is missing column %s", table, col)
			}
		}
	}
	for _, table := range sortedKeys(pg) {
		if _, ok := sqlite[table]; !ok {
			t.Errorf("sqlite is missing table %s (postgres has it)", table)
		}
	}

	if len(sqlite) != len(pg) {
		t.Errorf("table count mismatch: sqlite=%d postgres=%d", len(sqlite), len(pg))
	}
}

func TestTableColumnsParser(t *testing.T) {
	cols := tableColumns([]string{
		"CREATE TABLE IF NOT EXISTS t (id TEXT PRIMARY KEY, name TEXT NOT NULL, CONSTRAINT c CHECK (name <> ''))",
	})
	got := fmt.Sprint(sortedKeys2(cols["t"]))
	if got != "[id name]" {
		t.Fatalf("unexpected parsed columns: %s", got)
	}
}

func sortedKeys2(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
