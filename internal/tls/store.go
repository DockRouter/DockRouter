// Package tls handles TLS certificate management
package tls

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Errors
var ErrNoPEMData = errors.New("no PEM data found")

// Store handles certificate filesystem storage
type Store struct {
	mu      sync.RWMutex
	dataDir string
}

// NewStore creates a new certificate store
func NewStore(dataDir string) *Store {
	return &Store{dataDir: dataDir}
}

// CertMeta holds certificate metadata
type CertMeta struct {
	Domain    string `json:"domain"`
	Expiry    int64  `json:"expiry"`
	Issuer    string `json:"issuer,omitempty"`
	CreatedAt int64  `json:"created_at"`
}

// Save writes a certificate to disk
func (s *Store) Save(domain string, certPEM, keyPEM []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dir := filepath.Join(s.dataDir, "certificates", domain)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	// Write cert.pem
	if err := os.WriteFile(filepath.Join(dir, "cert.pem"), certPEM, 0600); err != nil {
		return err
	}

	// Write key.pem
	if err := os.WriteFile(filepath.Join(dir, "key.pem"), keyPEM, 0600); err != nil {
		return err
	}

	return nil
}

// SaveMeta saves certificate metadata
func (s *Store) SaveMeta(domain string, meta *CertMeta) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dir := filepath.Join(s.dataDir, "certificates", domain)
	metaBytes, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "meta.json"), metaBytes, 0600)
}

// Load reads a certificate from disk
func (s *Store) Load(domain string) (*tls.Certificate, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	dir := filepath.Join(s.dataDir, "certificates", domain)
	cert, err := tls.LoadX509KeyPair(
		filepath.Join(dir, "cert.pem"),
		filepath.Join(dir, "key.pem"),
	)
	if err != nil {
		return nil, err
	}
	return &cert, nil
}

// LoadPEM loads certificate PEM data
func (s *Store) LoadPEM(domain string) (certPEM, keyPEM []byte, err error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	dir := filepath.Join(s.dataDir, "certificates", domain)
	certPEM, err = os.ReadFile(filepath.Join(dir, "cert.pem"))
	if err != nil {
		return nil, nil, err
	}
	keyPEM, err = os.ReadFile(filepath.Join(dir, "key.pem"))
	if err != nil {
		return nil, nil, err
	}
	return certPEM, keyPEM, nil
}

// LoadMeta loads certificate metadata
func (s *Store) LoadMeta(domain string) (*CertMeta, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	dir := filepath.Join(s.dataDir, "certificates", domain)
	data, err := os.ReadFile(filepath.Join(dir, "meta.json"))
	if err != nil {
		return nil, err
	}
	var meta CertMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

// Exists checks if a certificate exists
func (s *Store) Exists(domain string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	dir := filepath.Join(s.dataDir, "certificates", domain)
	_, err := os.Stat(filepath.Join(dir, "cert.pem"))
	return err == nil
}

// List returns all domains with certificates
func (s *Store) List() ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	dir := filepath.Join(s.dataDir, "certificates")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	domains := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			domains = append(domains, entry.Name())
		}
	}
	return domains, nil
}

// Delete removes a certificate
func (s *Store) Delete(domain string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dir := filepath.Join(s.dataDir, "certificates", domain)
	return os.RemoveAll(dir)
}

// GetExpiry extracts expiry time from certificate
func GetExpiry(certPEM []byte) (time.Time, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return time.Time{}, ErrNoPEMData
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return time.Time{}, err
	}

	return cert.NotAfter, nil
}

// IsValid checks if certificate is valid and not expiring soon
func IsValid(certPEM []byte, renewBefore time.Duration) (bool, error) {
	expiry, err := GetExpiry(certPEM)
	if err != nil {
		return false, err
	}

	return time.Until(expiry) > renewBefore, nil
}

// ShouldRenew checks if certificate needs renewal (30 days before expiry)
func ShouldRenew(certPEM []byte) bool {
	expiry, err := GetExpiry(certPEM)
	if err != nil {
		return true
	}
	return time.Until(expiry) < 30*24*time.Hour
}
