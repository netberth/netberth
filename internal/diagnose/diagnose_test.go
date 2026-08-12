// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package diagnose

import (
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/netberth/netberth/internal/config"
	"github.com/netberth/netberth/internal/tlsutil"
)

func osPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func freeConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg := config.Default()
	cfg.Server.Host = "127.0.0.1"
	cfg.Server.Port = osPort(t)
	cfg.Database.Path = filepath.Join(t.TempDir(), "doctor.db")
	t.Setenv("NB_PROXY_PORT", "")
	return cfg
}

func TestCheckPort(t *testing.T) {
	free := osPort(t)
	if c := checkPort("server", "127.0.0.1", free); !c.OK {
		t.Fatalf("expected free port OK, got %+v", c)
	}

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer l.Close()
	busy := l.Addr().(*net.TCPAddr).Port
	if c := checkPort("server", "127.0.0.1", busy); c.OK {
		t.Fatal("expected occupied port to fail")
	}

	if c := checkPort("server", "127.0.0.1", 0); c.OK {
		t.Fatal("expected invalid port to fail")
	}
}

func TestRunSQLiteHappyPath(t *testing.T) {
	cfg := freeConfig(t)
	res := Run(cfg)
	if !res.AllOK() {
		t.Fatalf("expected all checks to pass: %s", AllOKString(res))
	}
}

func TestRunPostgresWithoutDSN(t *testing.T) {
	cfg := freeConfig(t)
	cfg.Database.Driver = "postgres"
	cfg.Database.DSN = ""
	res := Run(cfg)
	if res.AllOK() {
		t.Fatal("expected failure for postgres without DSN")
	}
	found := false
	for _, c := range res.Checks {
		if c.Name == "database" && !c.OK {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected failing database check, got %+v", res.Checks)
	}
}

func TestRunTLSProvidedPair(t *testing.T) {
	cfg := freeConfig(t)
	dir := t.TempDir()
	certPath := filepath.Join(dir, "server.crt")
	keyPath := filepath.Join(dir, "server.key")
	if _, err := tlsutil.EnsureSelfSigned(certPath, keyPath, []string{"localhost"}); err != nil {
		t.Fatalf("generate cert: %v", err)
	}
	cfg.Server.TLSEnabled = true
	cfg.Server.TLSCert = certPath
	cfg.Server.TLSKey = keyPath
	if res := Run(cfg); !res.AllOK() {
		t.Fatalf("expected all checks to pass with valid TLS pair: %s", AllOKString(res))
	}

	// Broken pair must fail.
	cfg.Server.TLSCert = filepath.Join(dir, "missing.crt")
	if res := Run(cfg); res.AllOK() {
		t.Fatal("expected failure for missing TLS cert")
	}
}

func TestRunTLSAutoGenerates(t *testing.T) {
	cfg := freeConfig(t)
	cfg.Server.TLSEnabled = true
	res := Run(cfg)
	if !res.AllOK() {
		t.Fatalf("expected auto TLS to pass: %s", AllOKString(res))
	}
	certPath := filepath.Join(filepath.Dir(cfg.Database.Path), "tls", "server.crt")
	if _, err := os.Stat(certPath); err != nil {
		t.Fatalf("auto cert not created: %v", err)
	}
}

func TestPrint(t *testing.T) {
	cfg := freeConfig(t)
	res := Run(cfg)
	w := httptest.NewRecorder()
	Print(w, res)
	body := w.Body.String()
	if !strings.Contains(body, "NetBerth doctor") || !strings.Contains(body, "All checks passed.") {
		t.Fatalf("unexpected print output: %s", body)
	}
}
