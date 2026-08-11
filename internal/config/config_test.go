// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefault(t *testing.T) {
	c := Default()
	if c.Server.Host != "0.0.0.0" || c.Server.Port != 8443 {
		t.Fatalf("unexpected server defaults: %+v", c.Server)
	}
	if c.Server.ReadTimeout != 30*time.Second || c.Server.WriteTimeout != 30*time.Second {
		t.Fatalf("unexpected timeouts: %+v", c.Server)
	}
	if c.Server.TLSEnabled || c.Server.TLSCert != "" || c.Server.TLSKey != "" {
		t.Fatalf("TLS should default to disabled: %+v", c.Server)
	}
	if c.Database.Path != "./data/netberth.db" {
		t.Fatalf("unexpected db path: %s", c.Database.Path)
	}
}

func TestLoadYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	yaml := `server:
  host: 127.0.0.1
  port: 9443
  tls_enabled: true
  tls_cert: /tmp/cert.pem
  tls_key: /tmp/key.pem
`
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if c.Server.Host != "127.0.0.1" || c.Server.Port != 9443 {
		t.Fatalf("unexpected server: %+v", c.Server)
	}
	if !c.Server.TLSEnabled || c.Server.TLSCert != "/tmp/cert.pem" || c.Server.TLSKey != "/tmp/key.pem" {
		t.Fatalf("unexpected TLS config: %+v", c.Server)
	}
}

func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	c, err := Load(filepath.Join(t.TempDir(), "missing.yaml"))
	if err != nil {
		t.Fatalf("load missing: %v", err)
	}
	if c.Server.Port != 8443 || c.Server.TLSEnabled {
		t.Fatalf("expected defaults for missing file: %+v", c.Server)
	}
}

func TestApplyEnv(t *testing.T) {
	t.Setenv("NB_SERVER_HOST", "127.0.0.1")
	t.Setenv("NB_SERVER_PORT", "9443")
	t.Setenv("NB_TLS_ENABLED", "true")
	t.Setenv("NB_TLS_CERT", "/x/cert.pem")
	t.Setenv("NB_TLS_KEY", "/x/key.pem")
	t.Setenv("NB_DB_PATH", "/data/nb.db")

	c := Default()
	c.applyEnv()
	if c.Server.Host != "127.0.0.1" || c.Server.Port != 9443 {
		t.Fatalf("unexpected server: %+v", c.Server)
	}
	if !c.Server.TLSEnabled || c.Server.TLSCert != "/x/cert.pem" || c.Server.TLSKey != "/x/key.pem" {
		t.Fatalf("unexpected TLS: %+v", c.Server)
	}
	if c.Database.Path != "/data/nb.db" {
		t.Fatalf("unexpected db path: %s", c.Database.Path)
	}
}

func TestParseBool(t *testing.T) {
	cases := map[string]bool{
		"1": true, "true": true, "TRUE": true, "yes": true, "on": true,
		"0": false, "false": false, "no": false, "off": false, "": false, "garbage": false,
	}
	for in, want := range cases {
		if got := parseBool(in); got != want {
			t.Errorf("parseBool(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestParsePort(t *testing.T) {
	cases := map[string]int{"8443": 8443, "1234": 1234, "abc": 8443, "": 8443, "80extra": 80}
	for in, want := range cases {
		if got := parsePort(in); got != want {
			t.Errorf("parsePort(%q) = %d, want %d", in, got, want)
		}
	}
}
