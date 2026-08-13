// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package acme

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/netberth/netberth/internal/model"
	"golang.org/x/crypto/acme"
)

type mockACMEDB struct {
	mu        sync.Mutex
	certs     []model.ACMECertificate
	updates   []model.ACMECertificate
	err       error
	updateErr error
}

func (m *mockACMEDB) GetCertificates() ([]model.ACMECertificate, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.certs, m.err
}

func (m *mockACMEDB) UpdateCertificate(c model.ACMECertificate) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.updateErr != nil {
		return m.updateErr
	}
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

// fakeACME is a scripted ACME client used to exercise the full issuance path.
type fakeACME struct {
	mu             sync.Mutex
	registerErr    error
	registerExists bool
	authorizeErr   error
	authzErr       error
	acceptErr      error
	waitErr        error
	finalizeErr    error
	order          *acme.Order
	auth           *acme.Authorization
	der            [][]byte
	acceptCalled   bool
}

func (f *fakeACME) Register(_ context.Context, _ *acme.Account, _ func(string) bool) (*acme.Account, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.registerExists {
		return nil, acme.ErrAccountAlreadyExists
	}
	return &acme.Account{}, f.registerErr
}

func (f *fakeACME) AuthorizeOrder(_ context.Context, _ []acme.AuthzID, _ ...acme.OrderOption) (*acme.Order, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.order, f.authorizeErr
}

func (f *fakeACME) GetAuthorization(_ context.Context, _ string) (*acme.Authorization, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.auth, f.authzErr
}

func (f *fakeACME) Accept(_ context.Context, _ *acme.Challenge) (*acme.Challenge, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.acceptCalled = true
	return nil, f.acceptErr
}

func (f *fakeACME) WaitAuthorization(_ context.Context, _ string) (*acme.Authorization, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return nil, f.waitErr
}

func (f *fakeACME) CreateOrderCert(_ context.Context, _ string, _ []byte, _ bool) ([][]byte, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.der, "", f.finalizeErr
}

func testLeafDER(t *testing.T, notAfter time.Time) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     notAfter,
		DNSNames:     []string{"example.com"},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	return der
}

func newSuccessFake(t *testing.T) *fakeACME {
	t.Helper()
	return &fakeACME{
		order: &acme.Order{
			AuthzURLs:   []string{"https://acme.test/authz/1"},
			FinalizeURL: "https://acme.test/finalize",
		},
		auth: &acme.Authorization{
			URI:        "https://acme.test/authz/1",
			Identifier: acme.AuthzID{Type: "dns", Value: "example.com"},
			Challenges: []*acme.Challenge{{Type: "dns-01", Token: "tok"}},
		},
		der: [][]byte{testLeafDER(t, time.Now().Add(30*24*time.Hour))},
	}
}

func runIssue(t *testing.T, db *mockACMEDB, fake *fakeACME) model.ACMECertificate {
	t.Helper()
	e := New(db, t.TempDir())
	e.client = fake
	e.issue(model.ACMECertificate{ID: "a1", Name: "x", Domains: []string{"example.com"}, Email: "a@b.com"})
	return db.lastUpdate()
}

func runIssueFailure(t *testing.T, db *mockACMEDB, fake *fakeACME, wantSubstr string) model.ACMECertificate {
	t.Helper()
	e := New(db, t.TempDir())
	e.client = fake
	e.issue(model.ACMECertificate{ID: "a1", Name: "x", Domains: []string{"example.com"}, Email: "a@b.com"})
	last := waitForACMEFailure(t, db)
	if !strings.Contains(last.Error, wantSubstr) {
		t.Fatalf("expected error containing %q, got %q", wantSubstr, last.Error)
	}
	return last
}

func TestIssueSuccess(t *testing.T) {
	db := &mockACMEDB{}
	fake := newSuccessFake(t)
	last := runIssue(t, db, fake)

	if last.Status != "valid" {
		t.Fatalf("expected status valid, got %q (error=%q)", last.Status, last.Error)
	}
	if last.Error != "" {
		t.Fatalf("expected no error, got %q", last.Error)
	}
	if last.CertPath == "" || last.KeyPath == "" {
		t.Fatalf("expected cert/key paths, got %+v", last)
	}
	if _, err := os.Stat(last.CertPath); err != nil {
		t.Fatalf("cert file missing: %v", err)
	}
	if _, err := os.Stat(last.KeyPath); err != nil {
		t.Fatalf("key file missing: %v", err)
	}
	leaf, err := x509.ParseCertificate(fake.der[0])
	if err != nil {
		t.Fatalf("parse test cert: %v", err)
	}
	if last.ExpiresAt == nil || !last.ExpiresAt.Equal(leaf.NotAfter) {
		t.Fatalf("expected expiry %v, got %v", leaf.NotAfter, last.ExpiresAt)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if !fake.acceptCalled {
		t.Fatal("expected challenge Accept to be called")
	}
}

func TestIssueSuccessWhenAccountAlreadyExists(t *testing.T) {
	db := &mockACMEDB{}
	fake := newSuccessFake(t)
	fake.registerExists = true
	last := runIssue(t, db, fake)
	if last.Status != "valid" {
		t.Fatalf("expected valid despite existing account, got %q (error=%q)", last.Status, last.Error)
	}
}

func TestIssueRegisterError(t *testing.T) {
	fake := newSuccessFake(t)
	fake.registerErr = errors.New("reg fail")
	runIssueFailure(t, &mockACMEDB{}, fake, "register:")
}

func TestIssueAuthorizeError(t *testing.T) {
	fake := newSuccessFake(t)
	fake.authorizeErr = errors.New("order fail")
	runIssueFailure(t, &mockACMEDB{}, fake, "authorize:")
}

func TestIssueGetAuthzError(t *testing.T) {
	fake := newSuccessFake(t)
	fake.authzErr = errors.New("authz fail")
	runIssueFailure(t, &mockACMEDB{}, fake, "get authz:")
}

func TestIssueNoSupportedChallenge(t *testing.T) {
	fake := newSuccessFake(t)
	fake.auth = &acme.Authorization{
		URI:        "https://acme.test/authz/1",
		Identifier: acme.AuthzID{Type: "dns", Value: "example.com"},
	}
	runIssueFailure(t, &mockACMEDB{}, fake, "no supported challenge")
}

func TestIssueAcceptError(t *testing.T) {
	fake := newSuccessFake(t)
	fake.acceptErr = errors.New("accept fail")
	runIssueFailure(t, &mockACMEDB{}, fake, "accept challenge:")
}

func TestIssueWaitAuthzError(t *testing.T) {
	fake := newSuccessFake(t)
	fake.waitErr = errors.New("wait fail")
	runIssueFailure(t, &mockACMEDB{}, fake, "wait authz:")
}

func TestIssueFinalizeError(t *testing.T) {
	fake := newSuccessFake(t)
	fake.finalizeErr = errors.New("finalize fail")
	runIssueFailure(t, &mockACMEDB{}, fake, "finalize:")
}

func TestIssueSaveKeyError(t *testing.T) {
	db := &mockACMEDB{}
	fake := newSuccessFake(t)
	dir := t.TempDir()
	// Create a valid account key, then make the certificate key path a
	// directory so only the final key write fails.
	e := New(db, dir)
	e.client = fake
	if _, err := e.loadOrCreateAccountKey(); err != nil {
		t.Fatalf("setup account key: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "a1.key"), 0700); err != nil {
		t.Fatalf("setup key dir: %v", err)
	}
	e.issue(model.ACMECertificate{ID: "a1", Name: "x", Domains: []string{"example.com"}, Email: "a@b.com"})
	last := waitForACMEFailure(t, db)
	if !strings.Contains(last.Error, "save key:") {
		t.Fatalf("expected save key error, got %q", last.Error)
	}
}

func TestIssueSaveCertError(t *testing.T) {
	db := &mockACMEDB{}
	fake := newSuccessFake(t)
	dir := t.TempDir()
	// Key path is a pre-existing regular file (save normalizes it to 0600
	// and succeeds), while the cert path is a directory so the cert write
	// fails.
	if err := os.WriteFile(filepath.Join(dir, "a1.key"), []byte("old"), 0644); err != nil {
		t.Fatalf("setup key file: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "a1.crt"), 0700); err != nil {
		t.Fatalf("setup cert dir: %v", err)
	}
	e := New(db, dir)
	e.client = fake
	e.issue(model.ACMECertificate{ID: "a1", Name: "x", Domains: []string{"example.com"}, Email: "a@b.com"})
	last := waitForACMEFailure(t, db)
	if !strings.Contains(last.Error, "save cert:") {
		t.Fatalf("expected save cert error, got %q", last.Error)
	}
}

func TestIssueUnparseableLeafUsesDefaultExpiry(t *testing.T) {
	db := &mockACMEDB{}
	fake := newSuccessFake(t)
	fake.der = [][]byte{[]byte("not a certificate")}
	last := runIssue(t, db, fake)
	if last.Status != "valid" {
		t.Fatalf("expected valid with default expiry, got %q (error=%q)", last.Status, last.Error)
	}
	if last.ExpiresAt == nil {
		t.Fatal("expected non-nil expiry")
	}
	want := time.Now().Add(90 * 24 * time.Hour)
	if diff := last.ExpiresAt.Sub(want); diff < -time.Minute || diff > time.Minute {
		t.Fatalf("expected default 90d expiry, got %v", last.ExpiresAt)
	}
}

func TestIssueDBUpdateErrorIgnored(t *testing.T) {
	db := &mockACMEDB{updateErr: errors.New("db fail")}
	fake := newSuccessFake(t)
	e := New(db, t.TempDir())
	e.client = fake
	e.issue(model.ACMECertificate{ID: "a1", Name: "x", Domains: []string{"example.com"}, Email: "a@b.com"})
	if got := db.lastUpdate(); got.ID != "" {
		t.Fatalf("expected no DB update on error, got %+v", got)
	}
}

func TestAutoRenewLoopTicker(t *testing.T) {
	t.Setenv("NB_ACME_DIR", "http://127.0.0.1:1/dir")
	soon := time.Now().Add(24 * time.Hour)
	db := &mockACMEDB{certs: []model.ACMECertificate{{
		ID: "due", AutoRenew: true, Status: "valid", ExpiresAt: &soon, RenewDays: 30,
		Domains: []string{"example.com"}, Email: "a@b.com",
	}}}
	fake := &fakeACME{registerErr: errors.New("renew fail")}
	e := New(db, t.TempDir())
	e.client = fake
	e.renewInterval = 30 * time.Millisecond
	go e.autoRenewLoop()
	defer e.Stop()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if c := db.lastUpdate(); c.Status == "error" {
			if c.ID != "due" {
				t.Fatalf("expected renewal attempt for 'due', got %+v", c)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for auto-renew attempt")
}

func TestStopIsIdempotent(t *testing.T) {
	e := New(&mockACMEDB{}, t.TempDir())
	e.Stop()
	e.Stop() // must not panic
}

func TestStartMkdirAllError(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(dir, []byte("file"), 0600); err != nil {
		t.Fatalf("setup: %v", err)
	}
	e := New(&mockACMEDB{}, dir)
	if err := e.Start(); err == nil {
		t.Fatal("expected error when cert dir cannot be created")
	}
}

func TestLoadOrCreateAccountKeyCorrupt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "acme-account.key")

	t.Run("no pem block", func(t *testing.T) {
		if err := os.WriteFile(path, []byte("garbage"), 0600); err != nil {
			t.Fatalf("write: %v", err)
		}
		e := New(&mockACMEDB{}, dir)
		if _, err := e.loadOrCreateAccountKey(); err == nil {
			t.Fatal("expected error for key file without PEM block")
		}
	})

	t.Run("invalid ec key", func(t *testing.T) {
		block := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: []byte("not a valid der")})
		if err := os.WriteFile(path, block, 0600); err != nil {
			t.Fatalf("write: %v", err)
		}
		e := New(&mockACMEDB{}, dir)
		if _, err := e.loadOrCreateAccountKey(); err == nil {
			t.Fatal("expected error for invalid EC key")
		}
	})
}

func TestAutoRenewLoopDBErrorContinues(t *testing.T) {
	db := &mockACMEDB{err: errors.New("db down")}
	e := New(db, t.TempDir())
	e.renewInterval = 20 * time.Millisecond
	go e.autoRenewLoop()
	time.Sleep(80 * time.Millisecond) // several ticks must not panic
	e.Stop()
}
