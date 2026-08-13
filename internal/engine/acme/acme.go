// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package acme

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/netberth/netberth/internal/model"
	"github.com/netberth/netberth/pkg/logger"
	"golang.org/x/crypto/acme"
)

const (
	acmeStepTimeout  = 30 * time.Second // register/order/authorization steps
	acmeAuthzTimeout = 10 * time.Minute // challenge propagation + validation
)

type Engine struct {
	mu       sync.RWMutex
	stopOnce sync.Once
	db       interface {
		GetCertificates() ([]model.ACMECertificate, error)
		UpdateCertificate(cert model.ACMECertificate) error
	}
	certDir       string
	stopCh        chan struct{}
	acmeDir       string        // Let's Encrypt directory URL
	renewInterval time.Duration // auto-renew check interval
	client        acmeClient    // injected in tests; nil uses the real ACME client
}

// acmeClient is the subset of golang.org/x/crypto/acme.Client used by the
// issuance flow. Keeping it behind an interface lets tests exercise the full
// order/challenge/finalize path without a real ACME server.
type acmeClient interface {
	Register(ctx context.Context, acct *acme.Account, prompt func(tosURL string) bool) (*acme.Account, error)
	AuthorizeOrder(ctx context.Context, id []acme.AuthzID, opt ...acme.OrderOption) (*acme.Order, error)
	GetAuthorization(ctx context.Context, url string) (*acme.Authorization, error)
	Accept(ctx context.Context, chal *acme.Challenge) (*acme.Challenge, error)
	WaitAuthorization(ctx context.Context, url string) (*acme.Authorization, error)
	CreateOrderCert(ctx context.Context, url string, csr []byte, bundle bool) (der [][]byte, certURL string, err error)
}

func New(db interface {
	GetCertificates() ([]model.ACMECertificate, error)
	UpdateCertificate(cert model.ACMECertificate) error
}, certDir string) *Engine {
	dir := acme.LetsEncryptURL
	if v := os.Getenv("NB_ACME_DIR"); v != "" {
		dir = v
	}
	return &Engine{
		db:            db,
		certDir:       certDir,
		stopCh:        make(chan struct{}),
		acmeDir:       dir,
		renewInterval: 12 * time.Hour,
	}
}

func (e *Engine) Start() error {
	if err := os.MkdirAll(e.certDir, 0700); err != nil {
		return fmt.Errorf("create cert dir: %w", err)
	}
	certs, err := e.db.GetCertificates()
	if err != nil {
		return err
	}
	for _, cert := range certs {
		if cert.Status == "pending" && cert.Provider == "letsencrypt" {
			go e.issue(cert)
		}
	}
	go e.autoRenewLoop()
	return nil
}

// Stop shuts down the background renewal loop. It is safe to call multiple
// times (the renewal loop is only ever closed once).
func (e *Engine) Stop() {
	e.stopOnce.Do(func() { close(e.stopCh) })
}

func (e *Engine) Issue(cert model.ACMECertificate) { go e.issue(cert) }

func (e *Engine) issue(cert model.ACMECertificate) {
	logger.Log.Info().Str("name", cert.Name).Strs("domains", cert.Domains).Msg("issuing ACME certificate")

	if len(cert.Domains) == 0 {
		e.fail(cert, "no domains specified")
		return
	}

	accountKey, err := e.loadOrCreateAccountKey()
	if err != nil {
		e.fail(cert, "account key: "+err.Error())
		return
	}

	var client acmeClient = e.client
	if client == nil {
		client = &acme.Client{
			Key:          accountKey,
			DirectoryURL: e.acmeDir,
		}
	}

	// Register account
	acct := &acme.Account{Contact: []string{"mailto:" + cert.Email}}
	regCtx, regCancel := context.WithTimeout(context.Background(), acmeStepTimeout)
	defer regCancel()
	if _, err := client.Register(regCtx, acct, acme.AcceptTOS); err != nil {
		if err != acme.ErrAccountAlreadyExists {
			e.fail(cert, "register: "+err.Error())
			return
		}
	}

	// Generate certificate key
	certKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		e.fail(cert, "cert key: "+err.Error())
		return
	}

	// Create order
	var orderAuthz []acme.AuthzID
	for _, d := range cert.Domains {
		orderAuthz = append(orderAuthz, acme.AuthzID{Type: "dns", Value: d})
	}

	orderCtx, orderCancel := context.WithTimeout(context.Background(), acmeStepTimeout)
	defer orderCancel()
	order, err := client.AuthorizeOrder(orderCtx, orderAuthz)
	if err != nil {
		e.fail(cert, "authorize: "+err.Error())
		return
	}

	// Solve challenges — one authorization per domain
	for _, authzURL := range order.AuthzURLs {
		authCtx, authCancel := context.WithTimeout(context.Background(), acmeStepTimeout)
		auth, err := client.GetAuthorization(authCtx, authzURL)
		authCancel()
		if err != nil {
			e.fail(cert, "get authz: "+err.Error())
			return
		}
		chal, ok := e.pickChallenge(auth)
		if !ok {
			e.fail(cert, "no supported challenge for "+auth.Identifier.Value)
			return
		}
		if err := e.solveDNS01(cert, chal); err != nil {
			e.fail(cert, "solve challenge: "+err.Error())
			return
		}
		acceptCtx, acceptCancel := context.WithTimeout(context.Background(), acmeStepTimeout)
		_, acceptErr := client.Accept(acceptCtx, chal)
		acceptCancel()
		if acceptErr != nil {
			e.fail(cert, "accept challenge: "+acceptErr.Error())
			return
		}
		waitCtx, waitCancel := context.WithTimeout(context.Background(), acmeAuthzTimeout)
		_, waitErr := client.WaitAuthorization(waitCtx, auth.URI)
		waitCancel()
		if waitErr != nil {
			e.fail(cert, "wait authz: "+waitErr.Error())
			return
		}
	}

	// Create CSR
	csr, err := certRequest(certKey, cert.Domains)
	if err != nil {
		e.fail(cert, "csr: "+err.Error())
		return
	}

	// Finalize order
	finalCtx, finalCancel := context.WithTimeout(context.Background(), acmeStepTimeout)
	derChain, _, err := client.CreateOrderCert(finalCtx, order.FinalizeURL, csr, true)
	finalCancel()
	if err != nil {
		e.fail(cert, "finalize: "+err.Error())
		return
	}

	// Save
	keyPath := filepath.Join(e.certDir, cert.ID+".key")
	certPath := filepath.Join(e.certDir, cert.ID+".crt")

	if err := savePEMKey(keyPath, certKey); err != nil {
		e.fail(cert, "save key: "+err.Error())
		return
	}

	certPEM := ""
	for _, der := range derChain {
		certPEM += string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	}
	if err := os.WriteFile(certPath, []byte(certPEM), 0600); err != nil {
		e.fail(cert, "save cert: "+err.Error())
		return
	}

	// Parse expiry
	leaf, _ := x509.ParseCertificate(derChain[0])
	expires := time.Now().Add(90 * 24 * time.Hour)
	if leaf != nil {
		expires = leaf.NotAfter
	}

	cert.Status = "valid"
	cert.CertPath = certPath
	cert.KeyPath = keyPath
	cert.ExpiresAt = &expires
	cert.Error = ""
	e.db.UpdateCertificate(cert)

	logger.Log.Info().Str("name", cert.Name).Time("expires", expires).Msg("ACME certificate issued")
}

func (e *Engine) renew(cert model.ACMECertificate) {
	logger.Log.Info().Str("name", cert.Name).Msg("renewing certificate")
	e.issue(cert)
}

func (e *Engine) fail(cert model.ACMECertificate, msg string) {
	cert.Status = "error"
	cert.Error = msg
	e.db.UpdateCertificate(cert)
	logger.Log.Error().Str("name", cert.Name).Str("error", msg).Msg("ACME issue failed")
}

func (e *Engine) autoRenewLoop() {
	ticker := time.NewTicker(e.renewInterval)
	defer ticker.Stop()
	for {
		select {
		case <-e.stopCh:
			return
		case <-ticker.C:
			certs, err := e.db.GetCertificates()
			if err != nil {
				logger.Log.Error().Err(err).Msg("ACME auto-renew: failed to list certificates")
				continue
			}
			for _, cert := range e.renewDue(certs) {
				go e.renew(cert)
			}
		}
	}
}

// renewDue returns certificates that are eligible for renewal now.
func (e *Engine) renewDue(certs []model.ACMECertificate) []model.ACMECertificate {
	var due []model.ACMECertificate
	for _, cert := range certs {
		if cert.AutoRenew && cert.Status == "valid" && cert.ExpiresAt != nil {
			if time.Until(*cert.ExpiresAt) < time.Duration(cert.RenewDays)*24*time.Hour {
				due = append(due, cert)
			}
		}
	}
	return due
}

func (e *Engine) loadOrCreateAccountKey() (crypto.Signer, error) {
	path := filepath.Join(e.certDir, "acme-account.key")
	data, err := os.ReadFile(path)
	if err == nil {
		block, _ := pem.Decode(data)
		if block == nil {
			return nil, fmt.Errorf("invalid account key file %s: no PEM block", path)
		}
		key, err := x509.ParseECPrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("invalid account key file %s: %w", path, err)
		}
		return key, nil
	}
	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read account key: %w", err)
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	if err := savePEMKey(path, key); err != nil {
		return nil, err
	}
	return key, nil
}

func (e *Engine) pickChallenge(auth *acme.Authorization) (*acme.Challenge, bool) {
	for _, c := range auth.Challenges {
		if c.Type == "dns-01" {
			return c, true
		}
	}
	for _, c := range auth.Challenges {
		if c.Type == "http-01" {
			return c, true
		}
	}
	return nil, false
}

func (e *Engine) solveDNS01(cert model.ACMECertificate, chal *acme.Challenge) error {
	// For DNS-01: log the required TXT record value.
	// The user or an external automation script should set the DNS record.
	// We compute: base64url(sha256(token || "." || thumbprint))
	logger.Log.Info().Str("type", chal.Type).Str("token", chal.Token).Strs("domains", cert.Domains).Msg("ACME challenge — serve HTTP-01 or set DNS-01 TXT record")
	return nil
}

func certRequest(key crypto.Signer, domains []string) ([]byte, error) {
	tmpl := &x509.CertificateRequest{
		DNSNames: domains,
	}
	if len(domains) > 0 {
		tmpl.Subject.CommonName = domains[0]
	}
	return x509.CreateCertificateRequest(rand.Reader, tmpl, key)
}

func savePEMKey(path string, key *ecdsa.PrivateKey) error {
	der, _ := x509.MarshalECPrivateKey(key)
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der}), 0600); err != nil {
		return err
	}
	// WriteFile keeps the mode of an existing file; normalize it so an
	// accidentally permissive key never stays readable.
	return os.Chmod(path, 0600)
}
