// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package acme

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/netberth/netberth/internal/model"
	"golang.org/x/crypto/acme"
)

type mockACMEDB struct {
	mu      sync.Mutex
	certs   []model.ACMECertificate
	updates []model.ACMECertificate
	err     error
}

func (m *mockACMEDB) GetCertificates() ([]model.ACMECertificate, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.certs, m.err
}

func (m *mockACMEDB) UpdateCertificate(c model.ACMECertificate) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.updates = append(m.updates, c)
	return nil
}

func (m *mockACMEDB) lastUpdate() model.ACMECertificate {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.updates) == 0 {
		return model.ACMECertificate{}
	}
	return m.updates[len(m.updates)-1]
}

func waitForACMEFailure(t *testing.T, db *mockACMEDB) model.ACMECertificate {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if c := db.lastUpdate(); c.Status == "error" {
			return c
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for ACME failure update")
	return model.ACMECertificate{}
}

func TestNewUsesEnvDirectory(t *testing.T) {
	t.Setenv("NB_ACME_DIR", "http://127.0.0.1:1/dir")
	e := New(&mockACMEDB{}, t.TempDir())
	if e.acmeDir != "http://127.0.0.1:1/dir" {
		t.Fatalf("expected env directory, got %q", e.acmeDir)
	}
}

func TestStartDBError(t *testing.T) {
	t.Setenv("NB_ACME_DIR", "http://127.0.0.1:1/dir")
	e := New(&mockACMEDB{err: errors.New("boom")}, t.TempDir())
	if err := e.Start(); err == nil {
		t.Fatal("expected start error")
	}
}

func TestStartAndStopNoPending(t *testing.T) {
	t.Setenv("NB_ACME_DIR", "http://127.0.0.1:1/dir")
	e := New(&mockACMEDB{certs: []model.ACMECertificate{{ID: "a1", Status: "valid"}}}, t.TempDir())
	if err := e.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	e.Stop()
}

func TestIssueNoDomainsFails(t *testing.T) {
	t.Setenv("NB_ACME_DIR", "http://127.0.0.1:1/dir")
	db := &mockACMEDB{}
	e := New(db, t.TempDir())
	e.Issue(model.ACMECertificate{ID: "a1", Name: "x"})
	last := waitForACMEFailure(t, db)
	if last.Error != "no domains specified" {
		t.Fatalf("expected 'no domains specified', got %q", last.Error)
	}
}

func TestIssueNetworkFailure(t *testing.T) {
	t.Setenv("NB_ACME_DIR", "http://127.0.0.1:1/dir")
	db := &mockACMEDB{}
	e := New(db, t.TempDir())
	e.Issue(model.ACMECertificate{ID: "a2", Name: "x", Domains: []string{"example.com"}, Email: "a@b.com"})
	last := waitForACMEFailure(t, db)
	if last.ID != "a2" || last.Error == "" {
		t.Fatalf("expected failure update for a2, got %+v", last)
	}
}

func TestPickChallenge(t *testing.T) {
	e := New(&mockACMEDB{}, t.TempDir())

	auth := &acme.Authorization{Challenges: []*acme.Challenge{{Type: "http-01", Token: "t"}, {Type: "dns-01", Token: "d"}}}
	chal, ok := e.pickChallenge(auth)
	if !ok || chal.Type != "dns-01" {
		t.Fatalf("expected dns-01 preferred, got %+v ok=%v", chal, ok)
	}

	auth2 := &acme.Authorization{Challenges: []*acme.Challenge{{Type: "http-01", Token: "t"}}}
	chal2, ok2 := e.pickChallenge(auth2)
	if !ok2 || chal2.Type != "http-01" {
		t.Fatalf("expected http-01 fallback, got %+v ok=%v", chal2, ok2)
	}

	if _, ok3 := e.pickChallenge(&acme.Authorization{}); ok3 {
		t.Fatal("expected no challenge for empty authorization")
	}
}

func TestAccountKeyAndCSR(t *testing.T) {
	dir := t.TempDir()
	e := New(&mockACMEDB{}, dir)

	key, err := e.loadOrCreateAccountKey()
	if err != nil {
		t.Fatalf("loadOrCreateAccountKey: %v", err)
	}
	key2, err := e.loadOrCreateAccountKey()
	if err != nil {
		t.Fatalf("second loadOrCreateAccountKey: %v", err)
	}
	if key2 == nil {
		t.Fatal("expected loaded key")
	}

	csr, err := certRequest(key, []string{"example.com"})
	if err != nil {
		t.Fatalf("certRequest: %v", err)
	}
	if len(csr) == 0 {
		t.Fatal("expected non-empty CSR")
	}
}

func TestSolveDNS01(t *testing.T) {
	e := New(&mockACMEDB{}, t.TempDir())
	if err := e.solveDNS01(model.ACMECertificate{Domains: []string{"example.com"}}, &acme.Challenge{Type: "dns-01", Token: "tok"}); err != nil {
		t.Fatalf("solveDNS01: %v", err)
	}
}

func TestRenewFailsFast(t *testing.T) {
	t.Setenv("NB_ACME_DIR", "http://127.0.0.1:1/dir")
	db := &mockACMEDB{}
	e := New(db, t.TempDir())
	e.renew(model.ACMECertificate{ID: "a3", Name: "x", Domains: []string{"example.com"}, Email: "a@b.com"})
	last := waitForACMEFailure(t, db)
	if last.ID != "a3" {
		t.Fatalf("expected failure for a3, got %+v", last)
	}
}

func TestStartWithPendingCert(t *testing.T) {
	t.Setenv("NB_ACME_DIR", "http://127.0.0.1:1/dir")
	db := &mockACMEDB{certs: []model.ACMECertificate{{
		ID: "a4", Name: "x", Status: "pending", Provider: "letsencrypt",
		Domains: []string{"example.com"}, Email: "a@b.com",
	}}}
	e := New(db, t.TempDir())
	if err := e.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	waitForACMEFailure(t, db)
	e.Stop()
}

func TestRenewDue(t *testing.T) {
	e := New(&mockACMEDB{}, t.TempDir())
	now := time.Now()
	future := now.Add(90 * 24 * time.Hour)
	soon := now.Add(24 * time.Hour)

	certs := []model.ACMECertificate{
		{ID: "due", AutoRenew: true, Status: "valid", ExpiresAt: &soon, RenewDays: 30},
		{ID: "not-due", AutoRenew: true, Status: "valid", ExpiresAt: &future, RenewDays: 30},
		{ID: "no-renew", AutoRenew: false, Status: "valid", ExpiresAt: &soon, RenewDays: 30},
		{ID: "not-valid", AutoRenew: true, Status: "pending", ExpiresAt: &soon, RenewDays: 30},
		{ID: "no-expiry", AutoRenew: true, Status: "valid", RenewDays: 30},
	}
	due := e.renewDue(certs)
	if len(due) != 1 || due[0].ID != "due" {
		t.Fatalf("expected only 'due' certificate, got %+v", due)
	}
}
