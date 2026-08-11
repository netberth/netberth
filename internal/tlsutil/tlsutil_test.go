// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package tlsutil

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEnsureSelfSignedCreatesValidCert(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "server.crt")
	keyPath := filepath.Join(dir, "server.key")

	cert, err := EnsureSelfSigned(certPath, keyPath, []string{"localhost", "127.0.0.1", "::1"})
	if err != nil {
		t.Fatalf("EnsureSelfSigned: %v", err)
	}
	if len(cert.Certificate) == 0 {
		t.Fatal("expected certificate chain")
	}

	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatalf("parse certificate: %v", err)
	}
	if leaf.NotAfter.Before(time.Now().Add(9 * 365 * 24 * time.Hour)) {
		t.Errorf("expected ~10 year validity, expires %v", leaf.NotAfter)
	}
	if leaf.IsCA {
		t.Error("leaf certificate must not be a CA")
	}
	if len(leaf.DNSNames) != 1 || leaf.DNSNames[0] != "localhost" {
		t.Errorf("unexpected DNS SANs: %v", leaf.DNSNames)
	}
	if len(leaf.IPAddresses) != 2 {
		t.Errorf("expected 2 IP SANs, got %v", leaf.IPAddresses)
	}

	for _, path := range []string{certPath, keyPath} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		if info.Mode().Perm() != 0600 {
			t.Errorf("%s permissions = %o, want 600", path, info.Mode().Perm())
		}
	}
}

func TestEnsureSelfSignedIdempotent(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "server.crt")
	keyPath := filepath.Join(dir, "server.key")

	c1, err := EnsureSelfSigned(certPath, keyPath, []string{"localhost"})
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	c2, err := EnsureSelfSigned(certPath, keyPath, []string{"localhost"})
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if !bytes.Equal(c1.Certificate[0], c2.Certificate[0]) {
		t.Fatal("certificate was regenerated on second call")
	}
}

func TestLoadMissingFiles(t *testing.T) {
	dir := t.TempDir()
	if _, err := Load(filepath.Join(dir, "nope.crt"), filepath.Join(dir, "nope.key")); err == nil {
		t.Fatal("expected error loading missing files")
	}
}

func TestServerTLSConfig(t *testing.T) {
	dir := t.TempDir()
	cert, err := EnsureSelfSigned(filepath.Join(dir, "c.pem"), filepath.Join(dir, "k.pem"), []string{"localhost"})
	if err != nil {
		t.Fatalf("EnsureSelfSigned: %v", err)
	}
	cfg := ServerTLSConfig(cert)
	if cfg.MinVersion != tls.VersionTLS12 {
		t.Fatalf("expected MinVersion TLS1.2, got %v", cfg.MinVersion)
	}
	if len(cfg.Certificates) != 1 {
		t.Fatalf("expected 1 certificate, got %d", len(cfg.Certificates))
	}
}

func TestHTTPSHandshakeWithGeneratedCert(t *testing.T) {
	dir := t.TempDir()
	cert, err := EnsureSelfSigned(filepath.Join(dir, "c.pem"), filepath.Join(dir, "k.pem"), []string{"127.0.0.1"})
	if err != nil {
		t.Fatalf("EnsureSelfSigned: %v", err)
	}

	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("tls-ok"))
	}))
	srv.TLS = ServerTLSConfig(cert)
	srv.StartTLS()
	defer srv.Close()

	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatalf("parse certificate: %v", err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(leaf)

	client := srv.Client()
	tr, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("unexpected transport %T", client.Transport)
	}
	tr.TLSClientConfig = &tls.Config{RootCAs: pool, ServerName: "127.0.0.1", MinVersion: tls.VersionTLS12}

	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("https get: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "tls-ok" {
		t.Fatalf("unexpected body: %q", body)
	}
	if resp.TLS == nil || resp.TLS.Version < tls.VersionTLS12 {
		t.Fatal("TLS version below 1.2")
	}
}
