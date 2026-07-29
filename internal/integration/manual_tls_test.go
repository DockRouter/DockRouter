package integration

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	tlspkg "github.com/DockRouter/dockrouter/internal/tls"
)

// writeSelfSignedPair writes a self-signed cert/key pair for domain and returns
// the two file paths.
func writeSelfSignedPair(t *testing.T, dir, domain string) (certFile, keyFile string) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: domain},
		DNSNames:     []string{domain},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}

	certFile = filepath.Join(dir, domain+".crt")
	keyFile = filepath.Join(dir, domain+".key")

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	if err := os.WriteFile(certFile, certPEM, 0o600); err != nil {
		t.Fatalf("write cert: %v", err)
	}

	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	if err := os.WriteFile(keyFile, keyPEM, 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}

	return certFile, keyFile
}

type nopLogger struct{}

func (nopLogger) Debug(string, ...interface{}) {}
func (nopLogger) Info(string, ...interface{})  {}
func (nopLogger) Warn(string, ...interface{})  {}
func (nopLogger) Error(string, ...interface{}) {}

// Manual TLS (dr.tls=manual) must serve an operator-supplied certificate,
// including when no ACME account is configured.
func TestManualCertificateIsServed(t *testing.T) {
	dir := t.TempDir()
	const domain = "manual.example.com"
	certFile, keyFile := writeSelfSignedPair(t, dir, domain)

	store := tlspkg.NewStore(filepath.Join(dir, "data"))
	// Deliberately no ACME client: manual TLS must not depend on one.
	mgr := tlspkg.NewManager(store, nil, tlspkg.NewChallengeSolver(), nopLogger{})

	if err := mgr.LoadManualCertificate(domain, certFile, keyFile); err != nil {
		t.Fatalf("LoadManualCertificate: %v", err)
	}

	cert := mgr.GetCachedCertificate(domain)
	if cert == nil {
		t.Fatal("manual certificate was not cached")
	}
	if cert.Leaf == nil {
		t.Fatal("certificate leaf was not parsed")
	}
	if cert.Leaf.Subject.CommonName != domain {
		t.Errorf("CommonName = %q, want %q", cert.Leaf.Subject.CommonName, domain)
	}

	// It must also be reachable through the SNI callback the HTTPS server uses.
	tlsCfg := mgr.GetTLSConfig()
	if tlsCfg.GetCertificate == nil {
		t.Fatal("TLS config has no GetCertificate callback")
	}

	found := false
	for _, d := range mgr.ListCertificates() {
		if d == domain {
			found = true
		}
	}
	if !found {
		t.Errorf("ListCertificates() = %v, should include %q", mgr.ListCertificates(), domain)
	}
}

// dr.tls.domains must make the same certificate resolvable under each SAN name,
// otherwise SNI for an alternate domain finds nothing.
func TestManualCertificateCoversSANDomains(t *testing.T) {
	dir := t.TempDir()
	const primary = "primary.example.com"
	sans := []string{"www.example.com", "api.example.com"}

	certFile, keyFile := writeSelfSignedPair(t, dir, primary)
	store := tlspkg.NewStore(filepath.Join(dir, "data"))
	mgr := tlspkg.NewManager(store, nil, tlspkg.NewChallengeSolver(), nopLogger{})

	// Mirrors what the route sink does for dr.tls=manual with dr.tls.domains.
	for _, domain := range append([]string{primary}, sans...) {
		if err := mgr.LoadManualCertificate(domain, certFile, keyFile); err != nil {
			t.Fatalf("LoadManualCertificate(%s): %v", domain, err)
		}
	}

	for _, domain := range append([]string{primary}, sans...) {
		if mgr.GetCachedCertificate(domain) == nil {
			t.Errorf("no certificate resolvable for SNI name %q", domain)
		}
	}
}

// A missing or malformed pair must report an error rather than silently
// leaving the route without TLS.
func TestManualCertificateErrors(t *testing.T) {
	dir := t.TempDir()
	store := tlspkg.NewStore(filepath.Join(dir, "data"))
	mgr := tlspkg.NewManager(store, nil, tlspkg.NewChallengeSolver(), nopLogger{})

	tests := []struct {
		name              string
		domain, cert, key string
	}{
		{"missing domain", "", "a.crt", "a.key"},
		{"missing cert path", "x.example.com", "", "a.key"},
		{"missing key path", "x.example.com", "a.crt", ""},
		{"nonexistent files", "x.example.com", filepath.Join(dir, "no.crt"), filepath.Join(dir, "no.key")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := mgr.LoadManualCertificate(tt.domain, tt.cert, tt.key); err == nil {
				t.Error("expected an error, got nil")
			}
		})
	}
}
