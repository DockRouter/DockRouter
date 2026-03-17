// Package tls handles TLS certificate management
package tls

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"sync"
	"time"
)

// Manager handles certificate lifecycle
type Manager struct {
	mu        sync.RWMutex
	certs     map[string]*tls.Certificate
	store     *Store
	acme      *ACMEClient
	challenge *ChallengeSolver
	logger    Logger
}

// NewManager creates a new TLS manager
func NewManager(store *Store, acme *ACMEClient, challenge *ChallengeSolver, logger Logger) *Manager {
	return &Manager{
		certs:     make(map[string]*tls.Certificate),
		store:     store,
		acme:      acme,
		challenge: challenge,
		logger:    logger,
	}
}

// LoadFromDisk loads all existing certificates from storage
func (m *Manager) LoadFromDisk() error {
	domains, err := m.store.List()
	if err != nil {
		return err
	}

	for _, domain := range domains {
		cert, err := m.store.Load(domain)
		if err != nil {
			m.logger.Warn("Failed to load certificate",
				"domain", domain,
				"error", err,
			)
			continue
		}

		m.mu.Lock()
		m.certs[domain] = cert
		m.mu.Unlock()

		m.logger.Info("Loaded certificate from disk",
			"domain", domain,
		)
	}

	return nil
}

// GetCertificate returns a certificate for SNI (for tls.Config.GetCertificate)
func (m *Manager) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	if hello == nil || hello.ServerName == "" {
		return nil, fmt.Errorf("no SNI server name")
	}

	domain := hello.ServerName

	m.mu.RLock()
	cert, ok := m.certs[domain]
	m.mu.RUnlock()

	if ok {
		return cert, nil
	}

	// Try to load from disk
	if m.store.Exists(domain) {
		cert, err := m.store.Load(domain)
		if err == nil {
			m.mu.Lock()
			m.certs[domain] = cert
			m.mu.Unlock()
			return cert, nil
		}
	}

	// Try to provision on-demand
	go func() {
		if err := m.EnsureCertificate(domain); err != nil {
			m.logger.Error("Failed to provision certificate on-demand",
				"domain", domain,
				"error", err,
			)
		}
	}()

	return nil, fmt.Errorf("certificate not found for %s", domain)
}

// EnsureCertificate provisions a certificate for a domain if needed
func (m *Manager) EnsureCertificate(domain string) error {
	// Check if we already have a valid cert
	m.mu.RLock()
	cert, ok := m.certs[domain]
	m.mu.RUnlock()

	if ok {
		// Check if valid
		if !m.needsRenewal(cert) {
			return nil
		}
	}

	// Check store
	if m.store.Exists(domain) {
		certPEM, _, err := m.store.LoadPEM(domain)
		if err == nil && !ShouldRenew(certPEM) {
			cert, err := m.store.Load(domain)
			if err == nil {
				m.mu.Lock()
				m.certs[domain] = cert
				m.mu.Unlock()
				return nil
			}
		}
	}

	// Provision new certificate via ACME
	return m.provisionCertificate(domain)
}

// provisionCertificate provisions a new certificate via ACME
func (m *Manager) provisionCertificate(domain string) error {
	if m.acme == nil {
		return fmt.Errorf("ACME client not initialized")
	}

	m.logger.Info("Provisioning certificate",
		"domain", domain,
	)

	// Create order
	order, err := m.acme.RequestOrder([]string{domain})
	if err != nil {
		return fmt.Errorf("failed to create order: %w", err)
	}

	// Process authorizations
	for _, authURL := range order.Authorizations {
		if err := m.processAuthorization(authURL); err != nil {
			return fmt.Errorf("authorization failed: %w", err)
		}
	}

	// Generate private key
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("failed to generate key: %w", err)
	}

	// Generate CSR
	csr, err := m.generateCSR(privKey, domain)
	if err != nil {
		return fmt.Errorf("failed to generate CSR: %w", err)
	}

	// Finalize order
	if err := m.acme.FinalizeOrder(order, csr); err != nil {
		return fmt.Errorf("failed to finalize order: %w", err)
	}

	// Download certificate
	certPEM, err := m.acme.DownloadCertificate(order.CertificateURL)
	if err != nil {
		return fmt.Errorf("failed to download certificate: %w", err)
	}

	// Encode private key
	keyPEM, err := m.encodePrivateKey(privKey)
	if err != nil {
		return fmt.Errorf("failed to encode key: %w", err)
	}

	// Save to store
	if err := m.store.Save(domain, certPEM, keyPEM); err != nil {
		return fmt.Errorf("failed to save certificate: %w", err)
	}

	// Save metadata
	expiry, _ := GetExpiry(certPEM)
	meta := &CertMeta{
		Domain:    domain,
		Expiry:    expiry.Unix(),
		Issuer:    "Let's Encrypt",
		CreatedAt: time.Now().Unix(),
	}
	m.store.SaveMeta(domain, meta)

	// Load into memory
	loadedCert, err := m.store.Load(domain)
	if err != nil {
		return err
	}

	m.mu.Lock()
	m.certs[domain] = loadedCert
	m.mu.Unlock()

	m.logger.Info("Certificate provisioned",
		"domain", domain,
		"expiry", expiry,
	)

	return nil
}

// processAuthorization handles ACME authorization
func (m *Manager) processAuthorization(authURL string) error {
	auth, err := m.acme.GetAuthorization(authURL)
	if err != nil {
		return err
	}

	// Find HTTP-01 challenge
	var httpChallenge *Challenge
	for _, ch := range auth.Challenges {
		if ch.Type == "http-01" {
			httpChallenge = &ch
			break
		}
	}

	if httpChallenge == nil {
		return fmt.Errorf("no HTTP-01 challenge available")
	}

	// Compute key authorization
	keyAuth := httpChallenge.Token + "." + m.getAccountThumbprint()

	// Store token for challenge handler
	m.challenge.SetToken(httpChallenge.Token, keyAuth)
	defer m.challenge.RemoveToken(httpChallenge.Token)

	// Trigger challenge
	if _, err := m.acme.TriggerChallenge(httpChallenge.URL); err != nil {
		return err
	}

	// Wait for challenge to complete
	for i := 0; i < 30; i++ {
		time.Sleep(time.Second)

		ch, err := m.acme.GetChallenge(httpChallenge.URL)
		if err != nil {
			return err
		}

		if ch.Status == "valid" {
			return nil
		}
		if ch.Status == "invalid" {
			return fmt.Errorf("challenge failed: %v", ch.Error)
		}
	}

	return fmt.Errorf("challenge timed out")
}

// Renew renews a certificate
func (m *Manager) Renew(domain string) error {
	// Remove old cert from memory
	m.mu.Lock()
	delete(m.certs, domain)
	m.mu.Unlock()

	// Provision new cert
	return m.provisionCertificate(domain)
}

// generateCSR generates a Certificate Signing Request
func (m *Manager) generateCSR(privKey *ecdsa.PrivateKey, domain string) ([]byte, error) {
	template := &x509.CertificateRequest{
		Subject:  pkix.Name{CommonName: domain},
		DNSNames: []string{domain},
	}

	csr, err := x509.CreateCertificateRequest(rand.Reader, template, privKey)
	if err != nil {
		return nil, err
	}

	return csr, nil
}

// encodePrivateKey encodes a private key to PEM
func (m *Manager) encodePrivateKey(key *ecdsa.PrivateKey) ([]byte, error) {
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, err
	}

	pemBlock := &pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: der,
	}

	return pem.EncodeToMemory(pemBlock), nil
}

// needsRenewal checks if a certificate needs renewal
func (m *Manager) needsRenewal(cert *tls.Certificate) bool {
	if cert == nil || cert.Leaf == nil {
		return true
	}
	return time.Until(cert.Leaf.NotAfter) < 30*24*time.Hour
}

// getAccountThumbprint returns the account key thumbprint (RFC 7638)
func (m *Manager) getAccountThumbprint() string {
	if m.acme == nil || m.acme.privateKey == nil {
		return ""
	}
	return computeJWKThumbprint(m.acme.privateKey.PublicKey)
}

// computeJWKThumbprint computes the JWK thumbprint according to RFC 7638
func computeJWKThumbprint(pubKey ecdsa.PublicKey) string {
	// RFC 7638: JWK Thumbprint is computed from a JWK with only required members
	// in lexicographical order: crv, kty, x, y for EC keys
	xBytes := pubKey.X.Bytes()
	yBytes := pubKey.Y.Bytes()

	// Pad to 32 bytes for P-256
	xPadded := padToLength(xBytes, 32)
	yPadded := padToLength(yBytes, 32)

	// Create canonical JWK JSON with members in lexicographic order
	jwk := fmt.Sprintf(`{"crv":"P-256","kty":"EC","x":"%s","y":"%s"}`,
		base64URLEncode(xPadded),
		base64URLEncode(yPadded),
	)

	// SHA-256 hash
	hash := sha256.Sum256([]byte(jwk))

	// Base64url encode without padding
	return base64URLEncode(hash[:])
}

// padToLength pads bytes to specified length
func padToLength(b []byte, length int) []byte {
	if len(b) >= length {
		return b[len(b)-length:]
	}
	padded := make([]byte, length)
	copy(padded[length-len(b):], b)
	return padded
}

// GetCertificate returns a cached certificate
func (m *Manager) GetCachedCertificate(domain string) *tls.Certificate {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.certs[domain]
}

// ListCertificates returns all managed domains
func (m *Manager) ListCertificates() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	domains := make([]string, 0, len(m.certs))
	for d := range m.certs {
		domains = append(domains, d)
	}
	return domains
}

// GetTLSConfig returns a tls.Config for the manager
func (m *Manager) GetTLSConfig() *tls.Config {
	return &tls.Config{
		GetCertificate: m.GetCertificate,
		MinVersion:     tls.VersionTLS12,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		},
	}
}

// GenerateSelfSigned generates a self-signed certificate for fallback
func GenerateSelfSigned(domain string) (*tls.Certificate, error) {
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	serialNumber, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))

	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject:      pkix.Name{CommonName: domain},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{domain},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &privKey.PublicKey, privKey)
	if err != nil {
		return nil, err
	}

	cert := &tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  privKey,
		Leaf:        template,
	}

	return cert, nil
}

// SaveAccountKey saves the ACME account key
func (m *Manager) SaveAccountKey() error {
	if m.acme == nil || m.acme.privateKey == nil {
		return nil
	}

	keyDER, err := x509.MarshalECPrivateKey(m.acme.privateKey)
	if err != nil {
		return err
	}

	path := m.store.dataDir + "/accounts/account.key"
	os.MkdirAll(m.store.dataDir+"/accounts", 0700)

	return os.WriteFile(path, keyDER, 0600)
}

// LoadAccountKey loads the ACME account key
func (m *Manager) LoadAccountKey() error {
	path := m.store.dataDir + "/accounts/account.key"
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	key, err := x509.ParseECPrivateKey(data)
	if err != nil {
		return err
	}

	m.acme.privateKey = key
	return nil
}
