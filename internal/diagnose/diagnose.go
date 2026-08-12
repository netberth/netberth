// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

// Package diagnose implements `netberth doctor` — a pre-flight self check that
// validates configuration, database, TLS material, writable state and ports
// without starting the full server.
package diagnose

import (
	"crypto/tls"
	"database/sql"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/mattn/go-sqlite3"
	"github.com/netberth/netberth/internal/config"
	_ "github.com/netberth/netberth/internal/db" // registers the pgx-rebind driver
	"github.com/netberth/netberth/internal/tlsutil"
)

// Check is a single named diagnostic result.
type Check struct {
	Name   string
	OK     bool
	Detail string
}

// Result contains all checks plus a convenience AllOK predicate.
type Result struct {
	Checks []Check
}

func (r Result) AllOK() bool {
	for _, c := range r.Checks {
		if !c.OK {
			return false
		}
	}
	return true
}

// Run executes the diagnostic suite against the given configuration.
func Run(cfg *config.Config) Result {
	res := Result{}
	res.Checks = append(res.Checks, checkConfig(cfg))
	res.Checks = append(res.Checks, checkDatabase(cfg))
	res.Checks = append(res.Checks, checkStateDir(cfg))
	res.Checks = append(res.Checks, checkTLS(cfg))
	res.Checks = append(res.Checks, checkPort("server", cfg.Server.Host, cfg.Server.Port))
	proxyPort := 8080
	if v := os.Getenv("NB_PROXY_PORT"); v != "" {
		fmt.Sscanf(v, "%d", &proxyPort)
	}
	res.Checks = append(res.Checks, checkPort("proxy", "0.0.0.0", proxyPort))
	return res
}

func checkConfig(cfg *config.Config) Check {
	if cfg == nil {
		return Check{Name: "config", OK: false, Detail: "nil configuration"}
	}
	if cfg.Server.Port < 1 || cfg.Server.Port > 65535 {
		return Check{Name: "config", OK: false, Detail: fmt.Sprintf("invalid server port %d", cfg.Server.Port)}
	}
	return Check{Name: "config", OK: true, Detail: fmt.Sprintf("host=%s port=%d driver=%s", cfg.Server.Host, cfg.Server.Port, dbDriverLabel(cfg))}
}

func dbDriverLabel(cfg *config.Config) string {
	if cfg.Database.Driver == "" || cfg.Database.Driver == "sqlite" {
		return "sqlite"
	}
	return cfg.Database.Driver
}

func checkDatabase(cfg *config.Config) Check {
	if dbDriverLabel(cfg) != "sqlite" {
		if cfg.Database.DSN == "" {
			return Check{Name: "database", OK: false, Detail: "postgres driver requires NB_DB_DSN"}
		}
		db, err := sql.Open("pgx-rebind", cfg.Database.DSN)
		if err != nil {
			return Check{Name: "database", OK: false, Detail: err.Error()}
		}
		defer db.Close()
		if err := db.Ping(); err != nil {
			return Check{Name: "database", OK: false, Detail: "postgres ping failed: " + err.Error()}
		}
		return Check{Name: "database", OK: true, Detail: "postgres reachable"}
	}

	db, err := sql.Open("sqlite3", cfg.Database.Path)
	if err != nil {
		return Check{Name: "database", OK: false, Detail: err.Error()}
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		return Check{Name: "database", OK: false, Detail: "sqlite open failed: " + err.Error()}
	}
	var integrity string
	if err := db.QueryRow("PRAGMA quick_check").Scan(&integrity); err != nil {
		return Check{Name: "database", OK: false, Detail: "integrity check failed: " + err.Error()}
	}
	if integrity != "ok" {
		return Check{Name: "database", OK: false, Detail: "integrity: " + integrity}
	}
	return Check{Name: "database", OK: true, Detail: cfg.Database.Path}
}

func checkStateDir(cfg *config.Config) Check {
	dataDir := filepath.Dir(cfg.Database.Path)
	if dbDriverLabel(cfg) != "sqlite" {
		dataDir = "./data"
	}
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return Check{Name: "state-dir", OK: false, Detail: err.Error()}
	}
	probe := filepath.Join(dataDir, ".doctor-write-probe")
	f, err := os.OpenFile(probe, os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return Check{Name: "state-dir", OK: false, Detail: err.Error()}
	}
	f.Close()
	os.Remove(probe)
	return Check{Name: "state-dir", OK: true, Detail: dataDir}
}

func checkTLS(cfg *config.Config) Check {
	if !cfg.Server.TLSEnabled {
		return Check{Name: "tls", OK: true, Detail: "disabled"}
	}
	certPath, keyPath := cfg.Server.TLSCert, cfg.Server.TLSKey
	if certPath == "" && keyPath == "" {
		dataDir := filepath.Dir(cfg.Database.Path)
		if dbDriverLabel(cfg) != "sqlite" {
			dataDir = "./data"
		}
		certPath = filepath.Join(dataDir, "tls", "server.crt")
		keyPath = filepath.Join(dataDir, "tls", "server.key")
		if _, err := tlsutil.EnsureSelfSigned(certPath, keyPath, []string{"localhost", "127.0.0.1", "::1"}); err != nil {
			return Check{Name: "tls", OK: false, Detail: "auto cert generation failed: " + err.Error()}
		}
		return Check{Name: "tls", OK: true, Detail: "auto self-signed: " + certPath}
	}
	if certPath == "" || keyPath == "" {
		return Check{Name: "tls", OK: false, Detail: "NB_TLS_CERT and NB_TLS_KEY must be set together"}
	}
	if _, err := tls.LoadX509KeyPair(certPath, keyPath); err != nil {
		return Check{Name: "tls", OK: false, Detail: err.Error()}
	}
	return Check{Name: "tls", OK: true, Detail: certPath}
}

func checkPort(name, host string, port int) Check {
	if port < 1 || port > 65535 {
		return Check{Name: "port:" + name, OK: false, Detail: fmt.Sprintf("invalid port %d", port)}
	}
	ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		return Check{Name: "port:" + name, OK: false, Detail: err.Error()}
	}
	ln.Close()
	return Check{Name: "port:" + name, OK: true, Detail: fmt.Sprintf("%s:%d free", host, port)}
}

// Print writes a human-readable report to w.
func Print(w interface{ Write([]byte) (int, error) }, res Result) {
	fmt.Fprintln(w, "NetBerth doctor")
	for _, c := range res.Checks {
		status := "OK  "
		if !c.OK {
			status = "FAIL"
		}
		fmt.Fprintf(w, "  [%s] %-18s %s\n", status, c.Name, c.Detail)
	}
	if res.AllOK() {
		fmt.Fprintln(w, "All checks passed.")
	} else {
		fmt.Fprintln(w, "Some checks failed.")
	}
}

// AllOKString is a helper for callers that want a compact summary.
func AllOKString(res Result) string {
	if res.AllOK() {
		return "all checks passed"
	}
	var failed []string
	for _, c := range res.Checks {
		if !c.OK {
			failed = append(failed, c.Name)
		}
	}
	return "failed: " + strings.Join(failed, ", ")
}
