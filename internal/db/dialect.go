// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package db

import (
	"strconv"
	"strings"
)

// Dialect identifies the SQL flavor a database connection speaks.
type Dialect string

const (
	DialectSQLite   Dialect = "sqlite"
	DialectPostgres Dialect = "postgres"
)

// Rebind converts '?' placeholders to Postgres $N style. It is the identity
// function for SQLite. '?' characters inside single-quoted literals are left
// untouched.
func Rebind(query string, d Dialect) string {
	if d != DialectPostgres {
		return query
	}
	var b strings.Builder
	b.Grow(len(query) + 8)
	n := 0
	const (
		stNormal = iota
		stSingle
		stDouble
		stLineComment
		stBlockComment
	)
	state := stNormal
	for i := 0; i < len(query); i++ {
		c := query[i]
		switch state {
		case stSingle:
			b.WriteByte(c)
			if c == '\'' {
				if i+1 < len(query) && query[i+1] == '\'' {
					b.WriteByte(query[i+1])
					i++
				} else {
					state = stNormal
				}
			}
		case stDouble:
			b.WriteByte(c)
			if c == '"' {
				if i+1 < len(query) && query[i+1] == '"' {
					b.WriteByte(query[i+1])
					i++
				} else {
					state = stNormal
				}
			}
		case stLineComment:
			b.WriteByte(c)
			if c == '\n' {
				state = stNormal
			}
		case stBlockComment:
			b.WriteByte(c)
			if c == '*' && i+1 < len(query) && query[i+1] == '/' {
				b.WriteByte('/')
				i++
				state = stNormal
			}
		default: // stNormal
			switch {
			case c == '\'':
				b.WriteByte(c)
				state = stSingle
			case c == '"':
				b.WriteByte(c)
				state = stDouble
			case c == '-' && i+1 < len(query) && query[i+1] == '-':
				b.WriteString("--")
				i++
				state = stLineComment
			case c == '/' && i+1 < len(query) && query[i+1] == '*':
				b.WriteString("/*")
				i++
				state = stBlockComment
			case c == '?':
				n++
				b.WriteByte('$')
				b.WriteString(strconv.Itoa(n))
			default:
				b.WriteByte(c)
			}
		}
	}
	return b.String()
}
