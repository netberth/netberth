// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package db

import (
	"context"
	"database/sql"
	"database/sql/driver"

	"github.com/jackc/pgx/v5/stdlib"
)

// rebindDriver wraps the pgx stdlib driver and rewrites '?' placeholders to
// Postgres $N style before the query reaches the database. This keeps the rest
// of the codebase on standard database/sql '?' placeholders.
type rebindDriver struct {
	inner driver.Driver
}

func init() {
	sql.Register("pgx-rebind", &rebindDriver{inner: stdlib.GetDefaultDriver()})
}

func (d *rebindDriver) Open(name string) (driver.Conn, error) {
	conn, err := d.inner.Open(name)
	if err != nil {
		return nil, err
	}
	return &rebindConn{Conn: conn}, nil
}

type rebindConn struct {
	driver.Conn
}

func (c *rebindConn) Prepare(query string) (driver.Stmt, error) {
	stmt, err := c.Conn.Prepare(Rebind(query, DialectPostgres))
	if err != nil {
		return nil, err
	}
	return &rebindStmt{Stmt: stmt}, nil
}

func (c *rebindConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	if pc, ok := c.Conn.(driver.ConnPrepareContext); ok {
		stmt, err := pc.PrepareContext(ctx, Rebind(query, DialectPostgres))
		if err != nil {
			return nil, err
		}
		return &rebindStmt{Stmt: stmt}, nil
	}
	return c.Prepare(query)
}

func (c *rebindConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if qc, ok := c.Conn.(driver.QueryerContext); ok {
		return qc.QueryContext(ctx, Rebind(query, DialectPostgres), args)
	}
	return nil, driver.ErrSkip
}

func (c *rebindConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if ec, ok := c.Conn.(driver.ExecerContext); ok {
		return ec.ExecContext(ctx, Rebind(query, DialectPostgres), args)
	}
	return nil, driver.ErrSkip
}

func (c *rebindConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if bt, ok := c.Conn.(driver.ConnBeginTx); ok {
		return bt.BeginTx(ctx, opts)
	}
	return c.Begin()
}

func (c *rebindConn) Ping(ctx context.Context) error {
	if p, ok := c.Conn.(driver.Pinger); ok {
		return p.Ping(ctx)
	}
	return nil
}

type rebindStmt struct {
	driver.Stmt
}

func (s *rebindStmt) Exec(args []driver.Value) (driver.Result, error) {
	return s.Stmt.Exec(args)
}

func (s *rebindStmt) Query(args []driver.Value) (driver.Rows, error) {
	return s.Stmt.Query(args)
}

func (s *rebindStmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	if ec, ok := s.Stmt.(driver.StmtExecContext); ok {
		return ec.ExecContext(ctx, args)
	}
	return nil, driver.ErrSkip
}

func (s *rebindStmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	if qc, ok := s.Stmt.(driver.StmtQueryContext); ok {
		return qc.QueryContext(ctx, args)
	}
	return nil, driver.ErrSkip
}
