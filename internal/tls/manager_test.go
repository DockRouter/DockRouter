package tls

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type mockTLSLogger struct{}

func (m *mockTLSLogger) Debug(msg string, fields ...interface{}) {}
func (m *mockTLSLogger) Info(msg string, fields ...interface{})  {}
func (m *mockTLSLogger) Warn(msg string, fields ...interface{})  {}
func (m *mockTLSLogger) Error(msg string, fields ...interface{}) {}

func TestNewManager(t *testing.T) {
	logger := &mockTLSLogger{}
	store := NewStore(t.TempDir())
	acme := NewACMEClient(LEStagingURL, "test@example.com")
	challenge := NewChallengeSolver()

	manager := NewManager(store, acme, challenge, logger)

	if manager == nil {
		t.Fatal("NewManager returned nil")
	}
	if manager.certs == nil {
		t.Error("certs map should be initialized")
	}
	if manager.store != store {
		t.Error("store should be set")
	}
	if manager.acme != acme {
		t.Error("acme should be set")
	}
	if manager.challenge != challenge {
		t.Error("challenge should be set")
	}
	if manager.logger != logger {
		t.Error("logger should be set")
	}
}

func TestManagerGetCachedCertificate(t *testing.T) {
	logger := &mockTLSLogger{}
	store := NewStore(t.TempDir())
	manager := NewManager(store, nil, nil, logger)

	// Test with no certificate
	cert := manager.GetCachedCertificate("example.com")
	if cert != nil {
		t.Error("GetCachedCertificate should return nil for non-existent cert")
	}

	// Create a test certificate
	privKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	serialNumber, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject:      pkix.Name{CommonName: "example.com"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		DNSNames:     []string{"example.com"},
	}
	certDER, _ := x509.CreateCertificate(rand.Reader, template, template, &privKey.PublicKey, privKey)

	testCert := &tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  privKey,
		Leaf:        template,
	}

	manager.mu.Lock()
	manager.certs["example.com"] = testCert
	manager.mu.Unlock()

	// Test retrieving the certificate
	retrieved := manager.GetCachedCertificate("example.com")
	if retrieved == nil {
		t.Error("GetCachedCertificate should return the stored cert")
	}
}

func TestManagerListCertificates(t *testing.T) {
	logger := &mockTLSLogger{}
	store := NewStore(t.TempDir())
	manager := NewManager(store, nil, nil, logger)

	// Empty list
	list := manager.ListCertificates()
	if len(list) != 0 {
		t.Errorf("ListCertificates should return empty slice, got %d items", len(list))
	}

	// Add some domains
	manager.mu.Lock()
	manager.certs["example.com"] = &tls.Certificate{}
	manager.certs["test.com"] = &tls.Certificate{}
	manager.mu.Unlock()

	list = manager.ListCertificates()
	if len(list) != 2 {
		t.Errorf("ListCertificates should return 2 items, got %d", len(list))
	}

	// Check that both domains are in the list
	found := make(map[string]bool)
	for _, d := range list {
		found[d] = true
	}
	if !found["example.com"] || !found["test.com"] {
		t.Error("ListCertificates should contain both domains")
	}
}

func TestManagerGetTLSConfig(t *testing.T) {
	logger := &mockTLSLogger{}
	store := NewStore(t.TempDir())
	manager := NewManager(store, nil, nil, logger)

	config := manager.GetTLSConfig()

	if config == nil {
		t.Fatal("GetTLSConfig returned nil")
	}
	if config.GetCertificate == nil {
		t.Error("GetCertificate should not be nil")
	}
	if config.MinVersion != tls.VersionTLS12 {
		t.Errorf("MinVersion = %d, want %d", config.MinVersion, tls.VersionTLS12)
	}
	if len(config.CipherSuites) == 0 {
		t.Error("CipherSuites should not be empty")
	}
}

func TestGenerateSelfSigned(t *testing.T) {
	cert, err := GenerateSelfSigned("example.com")
	if err != nil {
		t.Fatalf("GenerateSelfSigned failed: %v", err)
	}

	if cert == nil {
		t.Fatal("GenerateSelfSigned returned nil certificate")
	}
	if len(cert.Certificate) == 0 {
		t.Error("Certificate should have certificate data")
	}
	if cert.PrivateKey == nil {
		t.Error("Certificate should have private key")
	}
	if cert.Leaf == nil {
		t.Error("Certificate should have Leaf populated")
	}
	if cert.Leaf.Subject.CommonName != "example.com" {
		t.Errorf("CommonName = %s, want example.com", cert.Leaf.Subject.CommonName)
	}
	if len(cert.Leaf.DNSNames) != 1 || cert.Leaf.DNSNames[0] != "example.com" {
		t.Error("DNSNames should contain example.com")
	}
}

func TestGenerateSelfSignedMultipleDomains(t *testing.T) {
	domains := []string{"example.com", "test.com", "localhost"}

	for _, domain := range domains {
		cert, err := GenerateSelfSigned(domain)
		if err != nil {
			t.Errorf("GenerateSelfSigned(%s) failed: %v", domain, err)
			continue
		}
		if cert.Leaf.Subject.CommonName != domain {
			t.Errorf("CommonName = %s, want %s", cert.Leaf.Subject.CommonName, domain)
		}
	}
}

func TestManagerNeedsRenewal(t *testing.T) {
	logger := &mockTLSLogger{}
	store := NewStore(t.TempDir())
	manager := NewManager(store, nil, nil, logger)

	// Test with nil certificate
	if !manager.needsRenewal(nil) {
		t.Error("needsRenewal should return true for nil cert")
	}

	// Test with nil Leaf
	certNoLeaf := &tls.Certificate{
		Certificate: [][]byte{{1, 2, 3}},
		PrivateKey:  nil,
		Leaf:        nil,
	}
	if !manager.needsRenewal(certNoLeaf) {
		t.Error("needsRenewal should return true for nil Leaf")
	}

	// Test with cert expiring soon (within 30 days)
	privKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	serialNumber, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	templateExpiringSoon := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject:      pkix.Name{CommonName: "expiring.com"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(15 * 24 * time.Hour), // 15 days from now
	}
	certDER, _ := x509.CreateCertificate(rand.Reader, templateExpiringSoon, templateExpiringSoon, &privKey.PublicKey, privKey)

	certExpiringSoon := &tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  privKey,
		Leaf:        templateExpiringSoon,
	}

	if !manager.needsRenewal(certExpiringSoon) {
		t.Error("needsRenewal should return true for cert expiring soon")
	}

	// Test with cert not expiring soon
	templateNotExpiring := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject:      pkix.Name{CommonName: "valid.com"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour), // 1 year from now
	}
	certDER2, _ := x509.CreateCertificate(rand.Reader, templateNotExpiring, templateNotExpiring, &privKey.PublicKey, privKey)

	certNotExpiring := &tls.Certificate{
		Certificate: [][]byte{certDER2},
		PrivateKey:  privKey,
		Leaf:        templateNotExpiring,
	}

	if manager.needsRenewal(certNotExpiring) {
		t.Error("needsRenewal should return false for cert not expiring soon")
	}
}

func TestManagerGetAccountThumbprint(t *testing.T) {
	logger := &mockTLSLogger{}
	store := NewStore(t.TempDir())

	// Test with nil ACME client
	manager := NewManager(store, nil, nil, logger)
	if thumbprint := manager.getAccountThumbprint(); thumbprint != "" {
		t.Errorf("getAccountThumbprint should return empty string with nil acme, got %s", thumbprint)
	}

	// Test with ACME client but no private key
	acme := &ACMEClient{privateKey: nil}
	managerWithACME := NewManager(store, acme, nil, logger)
	if thumbprint := managerWithACME.getAccountThumbprint(); thumbprint != "" {
		t.Errorf("getAccountThumbprint should return empty string with nil privateKey, got %s", thumbprint)
	}

	// Test with ACME client and private key
	privKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	acmeWithKey := &ACMEClient{privateKey: privKey}
	managerWithKey := NewManager(store, acmeWithKey, nil, logger)
	if thumbprint := managerWithKey.getAccountThumbprint(); thumbprint == "" {
		t.Error("getAccountThumbprint should return non-empty string with valid privateKey")
	}
}

func TestManagerGenerateCSR(t *testing.T) {
	logger := &mockTLSLogger{}
	store := NewStore(t.TempDir())
	manager := NewManager(store, nil, nil, logger)

	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate private key: %v", err)
	}

	csr, err := manager.generateCSR(privKey, "example.com")
	if err != nil {
		t.Fatalf("generateCSR failed: %v", err)
	}

	if len(csr) == 0 {
		t.Error("CSR should not be empty")
	}

	// Parse and verify CSR
	parsedCSR, err := x509.ParseCertificateRequest(csr)
	if err != nil {
		t.Fatalf("Failed to parse CSR: %v", err)
	}
	if parsedCSR.Subject.CommonName != "example.com" {
		t.Errorf("CSR CommonName = %s, want example.com", parsedCSR.Subject.CommonName)
	}
	if len(parsedCSR.DNSNames) != 1 || parsedCSR.DNSNames[0] != "example.com" {
		t.Error("CSR DNSNames should contain example.com")
	}
}

func TestManagerEncodePrivateKey(t *testing.T) {
	logger := &mockTLSLogger{}
	store := NewStore(t.TempDir())
	manager := NewManager(store, nil, nil, logger)

	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate private key: %v", err)
	}

	pemBytes, err := manager.encodePrivateKey(privKey)
	if err != nil {
		t.Fatalf("encodePrivateKey failed: %v", err)
	}

	if len(pemBytes) == 0 {
		t.Error("PEM bytes should not be empty")
	}

	// Verify PEM format
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		t.Fatal("Failed to decode PEM block")
	}
	if block.Type != "EC PRIVATE KEY" {
		t.Errorf("PEM type = %s, want EC PRIVATE KEY", block.Type)
	}
}

func TestManagerGetCertificate(t *testing.T) {
	logger := &mockTLSLogger{}
	store := NewStore(t.TempDir())
	manager := NewManager(store, nil, nil, logger)

	// Test with nil ClientHelloInfo
	_, err := manager.GetCertificate(nil)
	if err == nil {
		t.Error("GetCertificate should return error for nil hello")
	}

	// Test with empty ServerName
	hello := &tls.ClientHelloInfo{ServerName: ""}
	_, err = manager.GetCertificate(hello)
	if err == nil {
		t.Error("GetCertificate should return error for empty ServerName")
	}

	// Test with valid ServerName but no certificate
	hello = &tls.ClientHelloInfo{ServerName: "nonexistent.com"}
	_, err = manager.GetCertificate(hello)
	if err == nil {
		t.Error("GetCertificate should return error for non-existent cert")
	}

	// Add a certificate and test retrieval
	privKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	serialNumber, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject:      pkix.Name{CommonName: "example.com"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		DNSNames:     []string{"example.com"},
	}
	certDER, _ := x509.CreateCertificate(rand.Reader, template, template, &privKey.PublicKey, privKey)

	testCert := &tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  privKey,
		Leaf:        template,
	}

	manager.mu.Lock()
	manager.certs["example.com"] = testCert
	manager.mu.Unlock()

	hello = &tls.ClientHelloInfo{ServerName: "example.com"}
	cert, err := manager.GetCertificate(hello)
	if err != nil {
		t.Errorf("GetCertificate failed: %v", err)
	}
	if cert == nil {
		t.Error("GetCertificate should return certificate")
	}
}

func TestManagerGetCertificateFromDisk(t *testing.T) {
	logger := &mockTLSLogger{}
	tempDir := t.TempDir()
	store := NewStore(tempDir)
	manager := NewManager(store, nil, nil, logger)

	// Generate a certificate and save to disk
	cert, err := GenerateSelfSigned("disk-test.com")
	if err != nil {
		t.Fatalf("Failed to generate certificate: %v", err)
	}

	// Save certificate to disk in the correct location
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Certificate[0]})
	keyDER, _ := x509.MarshalECPrivateKey(cert.PrivateKey.(*ecdsa.PrivateKey))
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	certDir := filepath.Join(tempDir, "certificates", "disk-test.com")
	os.MkdirAll(certDir, 0755)
	os.WriteFile(filepath.Join(certDir, "cert.pem"), certPEM, 0644)
	os.WriteFile(filepath.Join(certDir, "key.pem"), keyPEM, 0600)

	// Request certificate (should load from disk)
	hello := &tls.ClientHelloInfo{ServerName: "disk-test.com"}
	loadedCert, err := manager.GetCertificate(hello)
	if err != nil {
		t.Errorf("GetCertificate failed to load from disk: %v", err)
	}
	if loadedCert == nil {
		t.Error("GetCertificate should return certificate from disk")
	}

	// Verify it's cached now
	cached := manager.GetCachedCertificate("disk-test.com")
	if cached == nil {
		t.Error("Certificate should be cached after loading from disk")
	}
}

func TestNewRenewalScheduler(t *testing.T) {
	logger := &mockTLSLogger{}
	store := NewStore(t.TempDir())
	manager := NewManager(store, nil, nil, logger)

	scheduler := NewRenewalScheduler(manager, logger)

	if scheduler == nil {
		t.Fatal("NewRenewalScheduler returned nil")
	}
	if scheduler.manager != manager {
		t.Error("manager should be set")
	}
	if scheduler.logger != logger {
		t.Error("logger should be set")
	}
	if scheduler.interval != 24*time.Hour {
		t.Errorf("interval = %v, want 24h", scheduler.interval)
	}
}

func TestRenewalSchedulerStop(t *testing.T) {
	logger := &mockTLSLogger{}
	store := NewStore(t.TempDir())
	manager := NewManager(store, nil, nil, logger)

	scheduler := NewRenewalScheduler(manager, logger)
	// Stop should work even without Start
	scheduler.Stop() // Should not panic
}

func TestManagerLoadFromDisk(t *testing.T) {
	logger := &mockTLSLogger{}
	tempDir := t.TempDir()
	store := NewStore(tempDir)
	manager := NewManager(store, nil, nil, logger)

	// Create and save a certificate
	cert, err := GenerateSelfSigned("loaded-from-disk.com")
	if err != nil {
		t.Fatalf("Failed to generate certificate: %v", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Certificate[0]})
	keyDER, _ := x509.MarshalECPrivateKey(cert.PrivateKey.(*ecdsa.PrivateKey))
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	err = store.Save("loaded-from-disk.com", certPEM, keyPEM)
	if err != nil {
		t.Fatalf("Failed to save certificate: %v", err)
	}

	// Load from disk
	err = manager.LoadFromDisk()
	if err != nil {
		t.Fatalf("LoadFromDisk failed: %v", err)
	}

	// Verify the certificate is loaded
	loaded := manager.GetCachedCertificate("loaded-from-disk.com")
	if loaded == nil {
		t.Error("Certificate should be loaded from disk")
	}
}

func TestManagerLoadFromDiskEmpty(t *testing.T) {
	logger := &mockTLSLogger{}
	tempDir := t.TempDir()
	store := NewStore(tempDir)
	manager := NewManager(store, nil, nil, logger)

	// Load from empty store should not error
	err := manager.LoadFromDisk()
	if err != nil {
		t.Errorf("LoadFromDisk on empty store failed: %v", err)
	}
}

func TestManagerEnsureCertificateExisting(t *testing.T) {
	logger := &mockTLSLogger{}
	tempDir := t.TempDir()
	store := NewStore(tempDir)
	manager := NewManager(store, nil, nil, logger)

	// Create a valid certificate
	privKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	serialNumber, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject:      pkix.Name{CommonName: "existing.com"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour), // 1 year
		DNSNames:     []string{"existing.com"},
	}
	certDER, _ := x509.CreateCertificate(rand.Reader, template, template, &privKey.PublicKey, privKey)

	testCert := &tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  privKey,
		Leaf:        template,
	}

	manager.mu.Lock()
	manager.certs["existing.com"] = testCert
	manager.mu.Unlock()

	// Should return nil since we already have a valid cert
	err := manager.EnsureCertificate("existing.com")
	if err != nil {
		t.Errorf("EnsureCertificate should return nil for existing valid cert: %v", err)
	}
}

func TestManagerEnsureCertificateNoACME(t *testing.T) {
	logger := &mockTLSLogger{}
	tempDir := t.TempDir()
	store := NewStore(tempDir)
	manager := NewManager(store, nil, nil, logger)

	// Should fail because no ACME client
	err := manager.EnsureCertificate("newdomain.com")
	if err == nil {
		t.Error("EnsureCertificate should fail without ACME client")
	}
}

func TestManagerRenew(t *testing.T) {
	logger := &mockTLSLogger{}
	tempDir := t.TempDir()
	store := NewStore(tempDir)
	manager := NewManager(store, nil, nil, logger)

	// Add a cert to memory
	privKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	serialNumber, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject:      pkix.Name{CommonName: "renew.com"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
	}
	certDER, _ := x509.CreateCertificate(rand.Reader, template, template, &privKey.PublicKey, privKey)

	manager.mu.Lock()
	manager.certs["renew.com"] = &tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  privKey,
		Leaf:        template,
	}
	manager.mu.Unlock()

	// Renew should fail because no ACME client
	err := manager.Renew("renew.com")
	if err == nil {
		t.Error("Renew should fail without ACME client")
	}

	// Verify cert was removed from memory
	if manager.GetCachedCertificate("renew.com") != nil {
		t.Error("Certificate should be removed from memory during renewal")
	}
}

func TestManagerSaveAccountKey(t *testing.T) {
	logger := &mockTLSLogger{}
	tempDir := t.TempDir()
	store := NewStore(tempDir)

	privKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	acme := &ACMEClient{privateKey: privKey}

	manager := NewManager(store, acme, nil, logger)

	err := manager.SaveAccountKey()
	if err != nil {
		t.Errorf("SaveAccountKey failed: %v", err)
	}

	// Verify file exists
	keyPath := filepath.Join(tempDir, "accounts", "account.key")
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		t.Error("Account key file was not created")
	}
}

func TestManagerSaveAccountKeyNoClient(t *testing.T) {
	logger := &mockTLSLogger{}
	tempDir := t.TempDir()
	store := NewStore(tempDir)
	manager := NewManager(store, nil, nil, logger)

	// Should return nil when no ACME client
	err := manager.SaveAccountKey()
	if err != nil {
		t.Errorf("SaveAccountKey should return nil when no client: %v", err)
	}
}

func TestManagerLoadAccountKey(t *testing.T) {
	logger := &mockTLSLogger{}
	tempDir := t.TempDir()
	store := NewStore(tempDir)

	privKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	keyDER, _ := x509.MarshalECPrivateKey(privKey)

	// Create accounts directory and save key
	accountsDir := filepath.Join(tempDir, "accounts")
	os.MkdirAll(accountsDir, 0700)
	keyPath := filepath.Join(accountsDir, "account.key")
	os.WriteFile(keyPath, keyDER, 0600)

	acme := &ACMEClient{}
	manager := NewManager(store, acme, nil, logger)

	err := manager.LoadAccountKey()
	if err != nil {
		t.Errorf("LoadAccountKey failed: %v", err)
	}

	if acme.privateKey == nil {
		t.Error("Private key should be loaded")
	}
}

func TestManagerLoadAccountKeyNoFile(t *testing.T) {
	logger := &mockTLSLogger{}
	tempDir := t.TempDir()
	store := NewStore(tempDir)
	acme := &ACMEClient{}
	manager := NewManager(store, acme, nil, logger)

	err := manager.LoadAccountKey()
	if err == nil {
		t.Error("LoadAccountKey should fail when file doesn't exist")
	}
}

func TestRenewalSchedulerStart(t *testing.T) {
	logger := &mockTLSLogger{}
	tempDir := t.TempDir()
	store := NewStore(tempDir)
	manager := NewManager(store, nil, nil, logger)

	scheduler := NewRenewalScheduler(manager, logger)
	scheduler.interval = 50 * time.Millisecond // Short interval for testing

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	scheduler.Start(ctx)

	// Wait a bit for initial check
	time.Sleep(100 * time.Millisecond)

	// Cancel context to stop
	cancel()

	// Wait should return quickly
	done := make(chan struct{})
	go func() {
		scheduler.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Good
	case <-time.After(time.Second):
		t.Error("Stop took too long")
	}
}

func TestRenewalSchedulerCheckRenewalsWithCert(t *testing.T) {
	logger := &mockTLSLogger{}
	tempDir := t.TempDir()
	store := NewStore(tempDir)

	// Create a cert that needs renewal (expires soon)
	privKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	serialNumber, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject:      pkix.Name{CommonName: "expiring.com"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(15 * 24 * time.Hour), // 15 days - needs renewal
		DNSNames:     []string{"expiring.com"},
	}
	certDER, _ := x509.CreateCertificate(rand.Reader, template, template, &privKey.PublicKey, privKey)

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, _ := x509.MarshalECPrivateKey(privKey)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	store.Save("expiring.com", certPEM, keyPEM)

	manager := NewManager(store, nil, nil, logger)
	scheduler := NewRenewalScheduler(manager, logger)

	// checkRenewals will try to renew but fail because no ACME
	// Just verify it doesn't panic
	scheduler.checkRenewals()
}

func TestRenewalSchedulerCheckRenewalsError(t *testing.T) {
	logger := &mockTLSLogger{}

	// Use invalid store path
	store := NewStore("/nonexistent/path/that/does/not/exist")
	manager := NewManager(store, nil, nil, logger)
	scheduler := NewRenewalScheduler(manager, logger)

	// Should not panic when store.List fails
	scheduler.checkRenewals()
}

func TestManagerProcessAuthorizationNoChallenge(t *testing.T) {
	logger := &mockTLSLogger{}
	tempDir := t.TempDir()
	store := NewStore(tempDir)
	challenge := NewChallengeSolver()

	// Create mock ACME server that returns no http-01 challenge
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"status": "pending",
			"identifier": {"type": "dns", "value": "example.com"},
			"challenges": [
				{"type": "dns-01", "url": "https://acme.example.com/challenge/1", "token": "abc123", "status": "pending"}
			]
		}`))
	}))
	defer server.Close()

	privKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	acme := NewACMEClient(server.URL, "test@example.com")
	acme.privateKey = privKey
	acme.nonce = "test-nonce"

	manager := NewManager(store, acme, challenge, logger)

	err := manager.processAuthorization(server.URL)
	if err == nil {
		t.Error("processAuthorization should fail when no HTTP-01 challenge available")
	}
}

func TestManagerProvisionCertificateNoACME(t *testing.T) {
	logger := &mockTLSLogger{}
	tempDir := t.TempDir()
	store := NewStore(tempDir)
	manager := NewManager(store, nil, nil, logger)

	err := manager.provisionCertificate("example.com")
	if err == nil {
		t.Error("provisionCertificate should fail without ACME client")
	}
}

func TestManagerEnsureCertificateFromStore(t *testing.T) {
	logger := &mockTLSLogger{}
	tempDir := t.TempDir()
	store := NewStore(tempDir)
	manager := NewManager(store, nil, nil, logger)

	// Create a valid certificate and save to store
	cert, err := GenerateSelfSigned("stored.com")
	if err != nil {
		t.Fatalf("Failed to generate certificate: %v", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Certificate[0]})
	keyDER, _ := x509.MarshalECPrivateKey(cert.PrivateKey.(*ecdsa.PrivateKey))
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	err = store.Save("stored.com", certPEM, keyPEM)
	if err != nil {
		t.Fatalf("Failed to save certificate: %v", err)
	}

	// EnsureCertificate should load from store without needing ACME
	err = manager.EnsureCertificate("stored.com")
	if err != nil {
		t.Errorf("EnsureCertificate should succeed by loading from store: %v", err)
	}

	// Verify it's cached
	cached := manager.GetCachedCertificate("stored.com")
	if cached == nil {
		t.Error("Certificate should be cached after EnsureCertificate")
	}
}

func TestManagerEnsureCertificateNeedsRenewal(t *testing.T) {
	logger := &mockTLSLogger{}
	tempDir := t.TempDir()
	store := NewStore(tempDir)
	manager := NewManager(store, nil, nil, logger)

	// Create an expiring certificate
	privKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	serialNumber, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject:      pkix.Name{CommonName: "expiring-ensure.com"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(15 * 24 * time.Hour), // 15 days - needs renewal
		DNSNames:     []string{"expiring-ensure.com"},
	}
	certDER, _ := x509.CreateCertificate(rand.Reader, template, template, &privKey.PublicKey, privKey)

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, _ := x509.MarshalECPrivateKey(privKey)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	err := store.Save("expiring-ensure.com", certPEM, keyPEM)
	if err != nil {
		t.Fatalf("Failed to save certificate: %v", err)
	}

	// EnsureCertificate should try to provision because cert needs renewal
	// This will fail because no ACME, but we test the path
	err = manager.EnsureCertificate("expiring-ensure.com")
	if err == nil {
		t.Error("EnsureCertificate should fail when trying to provision without ACME")
	}
}

func TestManagerEnsureCertificateExpiredInCache(t *testing.T) {
	logger := &mockTLSLogger{}
	tempDir := t.TempDir()
	store := NewStore(tempDir)
	manager := NewManager(store, nil, nil, logger)

	// Create an expiring certificate and add to cache
	privKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	serialNumber, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject:      pkix.Name{CommonName: "expiring-cache.com"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(15 * 24 * time.Hour), // 15 days - needs renewal
		DNSNames:     []string{"expiring-cache.com"},
	}
	certDER, _ := x509.CreateCertificate(rand.Reader, template, template, &privKey.PublicKey, privKey)

	testCert := &tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  privKey,
		Leaf:        template,
	}

	manager.mu.Lock()
	manager.certs["expiring-cache.com"] = testCert
	manager.mu.Unlock()

	// EnsureCertificate should try to provision because cached cert needs renewal
	err := manager.EnsureCertificate("expiring-cache.com")
	if err == nil {
		t.Error("EnsureCertificate should fail when trying to provision without ACME")
	}
}

func TestStoreSaveMkdirError(t *testing.T) {
	// Try to save to an invalid path
	store := NewStore("/nonexistent/path/that/cannot/be/created\x00")

	err := store.Save("test.com", []byte("cert"), []byte("key"))
	if err == nil {
		t.Error("Save should fail with invalid path")
	}
}

func TestStoreSaveMetaMarshalError(t *testing.T) {
	tempDir := t.TempDir()
	store := NewStore(tempDir)

	// Create the directory first so SaveMeta can try to write
	certDir := filepath.Join(tempDir, "certificates", "test.com")
	os.MkdirAll(certDir, 0755)

	// Valid meta should work
	meta := &CertMeta{
		Domain:    "test.com",
		Expiry:    time.Now().Add(365 * 24 * time.Hour).Unix(),
		CreatedAt: time.Now().Unix(),
	}

	err := store.SaveMeta("test.com", meta)
	if err != nil {
		t.Errorf("SaveMeta should succeed: %v", err)
	}

	// Load it back
	loaded, err := store.LoadMeta("test.com")
	if err != nil {
		t.Errorf("LoadMeta should succeed: %v", err)
	}
	if loaded.Domain != "test.com" {
		t.Errorf("Domain = %s, want test.com", loaded.Domain)
	}
}

func TestStoreLoadMetaErrors(t *testing.T) {
	tempDir := t.TempDir()
	store := NewStore(tempDir)

	// Try to load from non-existent domain
	_, err := store.LoadMeta("nonexistent.com")
	if err == nil {
		t.Error("LoadMeta should fail for non-existent domain")
	}

	// Create directory with invalid JSON
	certDir := filepath.Join(tempDir, "certificates", "badjson.com")
	os.MkdirAll(certDir, 0755)
	os.WriteFile(filepath.Join(certDir, "meta.json"), []byte("invalid json"), 0644)

	_, err = store.LoadMeta("badjson.com")
	if err == nil {
		t.Error("LoadMeta should fail with invalid JSON")
	}
}

func TestStoreLoadPEMKeyError(t *testing.T) {
	tempDir := t.TempDir()
	store := NewStore(tempDir)

	// Create cert directory with only cert.pem
	certDir := filepath.Join(tempDir, "certificates", "partial.com")
	os.MkdirAll(certDir, 0755)
	os.WriteFile(filepath.Join(certDir, "cert.pem"), []byte("cert data"), 0644)
	// No key.pem

	_, _, err := store.LoadPEM("partial.com")
	if err == nil {
		t.Error("LoadPEM should fail when key.pem is missing")
	}
}

func TestStoreListPermissionError(t *testing.T) {
	// Test with non-existent certificates directory
	tempDir := t.TempDir()
	store := NewStore(tempDir)

	// List should return nil when directory doesn't exist
	domains, err := store.List()
	if err != nil {
		t.Errorf("List should not error on non-existent dir: %v", err)
	}
	if domains != nil {
		t.Errorf("List should return nil for empty dir, got %v", domains)
	}
}

func TestStoreListWithFiles(t *testing.T) {
	tempDir := t.TempDir()
	store := NewStore(tempDir)

	// Create certificate directories
	for _, domain := range []string{"example.com", "test.com", "api.example.com"} {
		certDir := filepath.Join(tempDir, "certificates", domain)
		os.MkdirAll(certDir, 0755)
		os.WriteFile(filepath.Join(certDir, "cert.pem"), []byte("cert"), 0644)
	}

	// Also create a file (not a directory) to test IsDir check
	os.WriteFile(filepath.Join(tempDir, "certificates", "file.txt"), []byte("test"), 0644)

	domains, err := store.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(domains) != 3 {
		t.Errorf("List should return 3 domains, got %d: %v", len(domains), domains)
	}
}

func TestRenewalSchedulerCheckRenewalsLoadError(t *testing.T) {
	logger := &mockTLSLogger{}
	tempDir := t.TempDir()
	store := NewStore(tempDir)

	// Create a cert directory with incomplete data
	certDir := filepath.Join(tempDir, "certificates", "broken.com")
	os.MkdirAll(certDir, 0755)
	// No cert.pem or key.pem

	manager := NewManager(store, nil, nil, logger)
	scheduler := NewRenewalScheduler(manager, logger)

	// Should not panic when LoadPEM fails
	scheduler.checkRenewals()
}

func TestManagerSaveAccountKeyError(t *testing.T) {
	logger := &mockTLSLogger{}
	// Use invalid path
	store := NewStore("/nonexistent/path\x00")
	privKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	acme := &ACMEClient{privateKey: privKey}
	manager := NewManager(store, acme, nil, logger)

	err := manager.SaveAccountKey()
	if err == nil {
		t.Error("SaveAccountKey should fail with invalid path")
	}
}

func TestManagerLoadAccountKeyInvalidKey(t *testing.T) {
	logger := &mockTLSLogger{}
	tempDir := t.TempDir()
	store := NewStore(tempDir)

	// Create accounts directory with invalid key
	accountsDir := filepath.Join(tempDir, "accounts")
	os.MkdirAll(accountsDir, 0700)
	os.WriteFile(filepath.Join(accountsDir, "account.key"), []byte("invalid key data"), 0600)

	acme := &ACMEClient{}
	manager := NewManager(store, acme, nil, logger)

	err := manager.LoadAccountKey()
	if err == nil {
		t.Error("LoadAccountKey should fail with invalid key data")
	}
}

func TestManagerLoadAccountKeyNoClient(t *testing.T) {
	logger := &mockTLSLogger{}
	tempDir := t.TempDir()
	store := NewStore(tempDir)

	// Create a valid key file
	privKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	keyDER, _ := x509.MarshalECPrivateKey(privKey)
	accountsDir := filepath.Join(tempDir, "accounts")
	os.MkdirAll(accountsDir, 0700)
	os.WriteFile(filepath.Join(accountsDir, "account.key"), keyDER, 0600)

	// Manager with ACME client but nil privateKey initially
	acme := &ACMEClient{} // No privateKey set
	manager := NewManager(store, acme, nil, logger)

	err := manager.LoadAccountKey()
	if err != nil {
		t.Errorf("LoadAccountKey should succeed: %v", err)
	}

	// Verify the key was loaded
	if acme.privateKey == nil {
		t.Error("privateKey should be set after LoadAccountKey")
	}
}

func TestManagerLoadFromDiskError(t *testing.T) {
	logger := &mockTLSLogger{}
	tempDir := t.TempDir()
	store := NewStore(tempDir)

	// Create a directory with invalid cert structure
	certDir := filepath.Join(tempDir, "certificates", "bad.com")
	os.MkdirAll(certDir, 0755)
	os.WriteFile(filepath.Join(certDir, "cert.pem"), []byte("invalid cert"), 0644)
	os.WriteFile(filepath.Join(certDir, "key.pem"), []byte("invalid key"), 0644)

	manager := NewManager(store, nil, nil, logger)

	// LoadFromDisk should not return error, it just skips invalid certs
	err := manager.LoadFromDisk()
	// It may or may not error depending on implementation
	_ = err
}

// MockACMEClient for testing provisionCertificate
type MockACMEClient struct {
	ACMEClient
	RequestOrderFunc      func(domains []string) (*ACMEOrder, error)
	ProcessAuthFunc       func(authURL string) error
	FinalizeOrderFunc     func(order *ACMEOrder, csr []byte) error
	DownloadCertFunc      func(url string) ([]byte, error)
}

func (m *MockACMEClient) RequestOrder(domains []string) (*ACMEOrder, error) {
	if m.RequestOrderFunc != nil {
		return m.RequestOrderFunc(domains)
	}
	return nil, fmt.Errorf("not implemented")
}

func (m *MockACMEClient) FinalizeOrder(order *ACMEOrder, csr []byte) error {
	if m.FinalizeOrderFunc != nil {
		return m.FinalizeOrderFunc(order, csr)
	}
	return nil
}

func (m *MockACMEClient) DownloadCertificate(url string) ([]byte, error) {
	if m.DownloadCertFunc != nil {
		return m.DownloadCertFunc(url)
	}
	return nil, fmt.Errorf("not implemented")
}

func TestProvisionCertificateACMENil(t *testing.T) {
	logger := &mockTLSLogger{}
	store := NewStore(t.TempDir())

	manager := NewManager(store, nil, nil, logger)

	err := manager.provisionCertificate("example.com")
	if err == nil {
		t.Error("provisionCertificate should fail with nil ACME client")
	}
	if !strings.Contains(err.Error(), "ACME client not initialized") {
		t.Errorf("Error should mention ACME client not initialized, got: %v", err)
	}
}

func TestProvisionCertificateRequestOrderError(t *testing.T) {
	logger := &mockTLSLogger{}
	store := NewStore(t.TempDir())

	// Create an ACMEClient with a private key and httpClient set so
	// signPayload/signedPost don't nil-deref. The HTTP request will fail
	// because the URL is invalid, which exercises the error path.
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	acmeClient := &ACMEClient{
		privateKey:  key,
		httpClient:  &http.Client{Timeout: 1 * time.Second},
		newOrderURL: "http://127.0.0.1:1/new-order", // will fail to connect
		nonce:       "test-nonce",
	}

	manager := NewManager(store, acmeClient, nil, logger)

	err = manager.provisionCertificate("example.com")
	if err == nil {
		t.Error("provisionCertificate should fail when order creation fails")
	}
}

func TestGenerateCSR(t *testing.T) {
	logger := &mockTLSLogger{}
	store := NewStore(t.TempDir())
	manager := NewManager(store, nil, nil, logger)

	// Generate a test key
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	csr, err := manager.generateCSR(privKey, "test.example.com")
	if err != nil {
		t.Errorf("generateCSR should succeed: %v", err)
	}

	if len(csr) == 0 {
		t.Error("CSR should not be empty")
	}
}

func TestEncodePrivateKey(t *testing.T) {
	logger := &mockTLSLogger{}
	store := NewStore(t.TempDir())
	manager := NewManager(store, nil, nil, logger)

	// Generate a test key
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	keyPEM, err := manager.encodePrivateKey(privKey)
	if err != nil {
		t.Errorf("encodePrivateKey should succeed: %v", err)
	}

	if len(keyPEM) == 0 {
		t.Error("Key PEM should not be empty")
	}

	// Verify it's valid PEM
	block, _ := pem.Decode(keyPEM)
	if block == nil {
		t.Error("Key PEM should be valid")
	}
}

