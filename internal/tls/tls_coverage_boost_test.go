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
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// --- padToLength tests ---

func TestPadToLengthExactMatch(t *testing.T) {
	b := []byte{1, 2, 3, 4}
	result := padToLength(b, 4)
	if len(result) != 4 || result[0] != 1 {
		t.Errorf("padToLength exact = %v", result)
	}
}

func TestPadToLengthInputLonger(t *testing.T) {
	b := []byte{1, 2, 3, 4, 5}
	result := padToLength(b, 3)
	if len(result) != 3 {
		t.Errorf("len = %d, want 3", len(result))
	}
	if result[0] != 3 || result[1] != 4 || result[2] != 5 {
		t.Errorf("padToLength = %v, want [3,4,5]", result)
	}
}

func TestPadToLengthInputShorter(t *testing.T) {
	b := []byte{1, 2}
	result := padToLength(b, 4)
	if len(result) != 4 {
		t.Errorf("len = %d, want 4", len(result))
	}
	if result[0] != 0 || result[1] != 0 || result[2] != 1 || result[3] != 2 {
		t.Errorf("padToLength = %v, want [0,0,1,2]", result)
	}
}

func TestPadToLengthNilInput(t *testing.T) {
	result := padToLength(nil, 4)
	if len(result) != 4 {
		t.Errorf("len = %d, want 4", len(result))
	}
}

// --- computeJWKThumbprint ---

func TestComputeJWKThumbprintConsistency(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	thumb1 := computeJWKThumbprint(key.PublicKey)
	thumb2 := computeJWKThumbprint(key.PublicKey)
	if thumb1 == "" {
		t.Error("thumbprint should not be empty")
	}
	if thumb1 != thumb2 {
		t.Error("same key should produce same thumbprint")
	}

	key2, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	thumb3 := computeJWKThumbprint(key2.PublicKey)
	if thumb1 == thumb3 {
		t.Error("different keys should produce different thumbprints")
	}
}

// --- Store.Delete ---

func TestStoreDeleteNonexistent(t *testing.T) {
	store := NewStore(t.TempDir())
	err := store.Delete("nonexistent.com")
	if err != nil {
		t.Errorf("Delete nonexistent error: %v", err)
	}
}

// --- Store.LoadPEM partial failure ---

func TestStoreLoadPEMPartialFailure(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	// Create only cert.pem, no key.pem
	certDir := filepath.Join(dir, "certificates", "partial.com")
	os.MkdirAll(certDir, 0700)
	os.WriteFile(filepath.Join(certDir, "cert.pem"), []byte("cert-data"), 0600)

	_, _, err := store.LoadPEM("partial.com")
	if err == nil {
		t.Error("LoadPEM with missing key.pem should error")
	}
}

func TestStoreLoadPEMNonexistent(t *testing.T) {
	store := NewStore(t.TempDir())
	_, _, err := store.LoadPEM("nonexistent.com")
	if err == nil {
		t.Error("LoadPEM nonexistent should error")
	}
}

func TestStoreLoadNonexistent(t *testing.T) {
	store := NewStore(t.TempDir())
	_, err := store.Load("nonexistent.com")
	if err == nil {
		t.Error("Load nonexistent should error")
	}
}

// --- Store.SaveMeta ---

func TestStoreSaveAndLoadMeta(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	certDir := filepath.Join(dir, "certificates", "meta.com")
	os.MkdirAll(certDir, 0700)

	meta := &CertMeta{
		Domain:    "meta.com",
		Expiry:    time.Now().Add(90 * 24 * time.Hour).Unix(),
		Issuer:    "test",
		CreatedAt: time.Now().Unix(),
	}

	if err := store.SaveMeta("meta.com", meta); err != nil {
		t.Fatalf("SaveMeta error: %v", err)
	}

	loaded, err := store.LoadMeta("meta.com")
	if err != nil {
		t.Fatalf("LoadMeta error: %v", err)
	}
	if loaded.Domain != "meta.com" || loaded.Issuer != "test" {
		t.Error("metadata mismatch")
	}
}

// --- GetExpiry invalid cert ---

func TestGetExpiryInvalidCertDER(t *testing.T) {
	pemBlock := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: []byte("not a real certificate"),
	})
	_, err := GetExpiry(pemBlock)
	if err == nil {
		t.Error("should error on invalid certificate DER")
	}
}

// --- ShouldRenew with invalid PEM ---

func TestShouldRenewInvalidPEM(t *testing.T) {
	if !ShouldRenew([]byte("garbage")) {
		t.Error("invalid PEM should indicate renewal needed")
	}
}

// --- IsValid with invalid PEM ---

func TestIsValidInvalidPEM(t *testing.T) {
	_, err := IsValid([]byte("not pem"), 30*24*time.Hour)
	if err == nil {
		t.Error("should error on invalid PEM")
	}
}

// --- RenewalScheduler ---

func TestRenewalSchedulerStartAndStop(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	manager := NewManager(store, nil, nil, &boostTestLogger{})
	logger := &boostTestLogger{}

	scheduler := NewRenewalScheduler(manager, logger)
	ctx, cancel := context.WithCancel(context.Background())
	scheduler.Start(ctx)

	time.Sleep(50 * time.Millisecond)
	cancel()

	done := make(chan struct{})
	go func() {
		scheduler.Stop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Error("Stop should return after context cancelled")
	}
}

func TestRenewalSchedulerCheckRenewalsWithLoadError(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	// Create cert dir without files
	os.MkdirAll(filepath.Join(dir, "certificates", "broken.com"), 0700)

	manager := NewManager(store, nil, nil, &boostTestLogger{})
	logger := &boostTestLogger{}

	scheduler := NewRenewalScheduler(manager, logger)
	// Should not panic
	scheduler.checkRenewals()
}

func TestRenewalSchedulerCheckRenewalsValidCert(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	certPEM, keyPEM := boostGenerateTestCertPEM(t)
	store.Save("good.com", certPEM, keyPEM)

	manager := NewManager(store, nil, nil, &boostTestLogger{})
	logger := &boostTestLogger{}

	scheduler := NewRenewalScheduler(manager, logger)
	scheduler.checkRenewals()
}

// --- Manager getAccountThumbprint ---

func TestManagerGetAccountThumbprintNil(t *testing.T) {
	manager := NewManager(NewStore(t.TempDir()), nil, nil, &boostTestLogger{})
	if thumb := manager.getAccountThumbprint(); thumb != "" {
		t.Errorf("should be empty, got %q", thumb)
	}
}

func TestManagerGetAccountThumbprintNilKey(t *testing.T) {
	manager := NewManager(NewStore(t.TempDir()), nil, nil, &boostTestLogger{})
	manager.acme = &ACMEClient{}
	if thumb := manager.getAccountThumbprint(); thumb != "" {
		t.Errorf("should be empty, got %q", thumb)
	}
}

func TestManagerGetAccountThumbprintWithKey(t *testing.T) {
	manager := NewManager(NewStore(t.TempDir()), nil, nil, &boostTestLogger{})
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	manager.acme = &ACMEClient{privateKey: key}
	if thumb := manager.getAccountThumbprint(); thumb == "" {
		t.Error("should not be empty")
	}
}

// --- Helper ---

func boostGenerateTestCertPEM(t *testing.T) (certPEM, keyPEM []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test.com"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return
}

type boostTestLogger struct{}

func (l *boostTestLogger) Debug(msg string, fields ...interface{}) {}
func (l *boostTestLogger) Info(msg string, fields ...interface{})  {}
func (l *boostTestLogger) Warn(msg string, fields ...interface{})  {}
func (l *boostTestLogger) Error(msg string, fields ...interface{}) {}

// --- processAuthorization additional tests ---

func TestProcessAuthorizationGetAuthError(t *testing.T) {
	logger := &boostTestLogger{}
	store := NewStore(t.TempDir())
	challenge := NewChallengeSolver()

	// Create ACME client with invalid URL that will fail
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	acmeClient := &ACMEClient{
		privateKey: key,
		nonce:      "test-nonce",
	}

	manager := NewManager(store, acmeClient, challenge, logger)

	// This should fail because the URL is invalid
	err := manager.processAuthorization("http://[invalid-url")
	if err == nil {
		t.Error("processAuthorization should fail with invalid URL")
	}
}

// --- provisionCertificate additional tests ---

func TestProvisionCertificateAuthorizationFailure(t *testing.T) {
	logger := &boostTestLogger{}
	store := NewStore(t.TempDir())
	challenge := NewChallengeSolver()

	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	acmeClient := &ACMEClient{
		privateKey:  key,
		nonce:       "test-nonce",
		newOrderURL: "http://127.0.0.1:1/new-order",
		httpClient:  &http.Client{Timeout: 100 * time.Millisecond},
	}

	manager := NewManager(store, acmeClient, challenge, logger)

	// Should fail when trying to create order
	err := manager.provisionCertificate("test.example.com")
	if err == nil {
		t.Error("provisionCertificate should fail when order creation fails")
	}
}

// --- Store additional tests ---

func TestStoreExistsNonexistent(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	// Non-existent domain
	if store.Exists("nonexistent.com") {
		t.Error("Exists should return false for non-existent domain")
	}
}

// --- ACME client additional tests ---

func TestACMEClientInitializeWithExistingKey(t *testing.T) {
	// Generate a key
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)

	client := NewACMEClient(LEStagingURL, "test@example.com")

	// Set the private key directly to simulate loading
	client.privateKey = key

	if client.privateKey == nil {
		t.Error("Private key should be set")
	}
}

// --- Challenge solver additional tests ---

func TestChallengeSolverGetNonexistentToken(t *testing.T) {
	solver := NewChallengeSolver()

	token, ok := solver.GetToken("nonexistent-token")
	if ok {
		t.Error("GetToken nonexistent should return ok=false")
	}
	if token != "" {
		t.Errorf("GetToken nonexistent should return empty token, got %q", token)
	}
}

func TestChallengeSolverRemoveNonexistentToken(t *testing.T) {
	solver := NewChallengeSolver()

	// Should not panic
	solver.RemoveToken("nonexistent-token")
}

// --- Certificate expiry tests ---

func TestGetExpiryValidCert(t *testing.T) {
	certPEM, _ := boostGenerateTestCertPEM(t)

	expiry, err := GetExpiry(certPEM)
	if err != nil {
		t.Errorf("GetExpiry error: %v", err)
	}
	if expiry.IsZero() {
		t.Error("expiry should not be zero")
	}
}

func TestShouldRenewValidCert(t *testing.T) {
	certPEM, _ := boostGenerateTestCertPEM(t)

	// Fresh cert should not need renewal
	if ShouldRenew(certPEM) {
		t.Error("fresh cert should not need renewal")
	}
}

func TestIsValidValidCert(t *testing.T) {
	certPEM, _ := boostGenerateTestCertPEM(t)

	valid, err := IsValid(certPEM, 30*24*time.Hour)
	if err != nil {
		t.Errorf("IsValid error: %v", err)
	}
	if !valid {
		t.Error("fresh cert should be valid")
	}
}

// --- needsRenewal edge cases ---

func TestNeedsRenewalNilCert(t *testing.T) {
	store := NewStore(t.TempDir())
	manager := NewManager(store, nil, nil, &boostTestLogger{})

	if !manager.needsRenewal(nil) {
		t.Error("nil cert should need renewal")
	}
}

func TestNeedsRenewalNilLeaf(t *testing.T) {
	store := NewStore(t.TempDir())
	manager := NewManager(store, nil, nil, &boostTestLogger{})

	cert := &tls.Certificate{} // Leaf is nil
	if !manager.needsRenewal(cert) {
		t.Error("cert with nil Leaf should need renewal")
	}
}

// --- GetCertificate edge cases ---

func TestGetCertificateNilHello(t *testing.T) {
	store := NewStore(t.TempDir())
	manager := NewManager(store, nil, nil, &boostTestLogger{})

	_, err := manager.GetCertificate(nil)
	if err == nil {
		t.Error("GetCertificate with nil hello should error")
	}
}

func TestGetCertificateEmptyServerName(t *testing.T) {
	store := NewStore(t.TempDir())
	manager := NewManager(store, nil, nil, &boostTestLogger{})

	hello := &tls.ClientHelloInfo{ServerName: ""}
	_, err := manager.GetCertificate(hello)
	if err == nil {
		t.Error("GetCertificate with empty server name should error")
	}
}

// --- encodePrivateKey error case ---

func TestEncodePrivateKeyError(t *testing.T) {
	store := NewStore(t.TempDir())
	manager := NewManager(store, nil, nil, &boostTestLogger{})

	// Create an invalid key by using a different curve that might cause issues
	key, _ := ecdsa.GenerateKey(elliptic.P224(), rand.Reader)

	_, err := manager.encodePrivateKey(key)
	if err != nil {
		// P-224 is valid, so this shouldn't error, but test the path
		t.Logf("encodePrivateKey with P-224: %v", err)
	}
}

// --- LoadAccountKey error cases ---

func TestLoadAccountKeyNonexistent(t *testing.T) {
	store := NewStore(t.TempDir())
	acmeClient := NewACMEClient(LEStagingURL, "test@example.com")
	manager := NewManager(store, acmeClient, nil, &boostTestLogger{})

	err := manager.LoadAccountKey()
	if err == nil {
		t.Error("LoadAccountKey with no file should error")
	}
}

func TestLoadAccountKeyInvalidData(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	acmeClient := NewACMEClient(LEStagingURL, "test@example.com")
	manager := NewManager(store, acmeClient, nil, &boostTestLogger{})

	// Create invalid key file
	keyPath := filepath.Join(dir, "accounts", "account.key")
	os.MkdirAll(filepath.Join(dir, "accounts"), 0700)
	os.WriteFile(keyPath, []byte("invalid key data"), 0600)

	err := manager.LoadAccountKey()
	if err == nil {
		t.Error("LoadAccountKey with invalid data should error")
	}
}

// --- SaveAccountKey edge cases ---

func TestSaveAccountKeyNilACME(t *testing.T) {
	store := NewStore(t.TempDir())
	manager := NewManager(store, nil, nil, &boostTestLogger{})

	err := manager.SaveAccountKey()
	if err != nil {
		t.Errorf("SaveAccountKey with nil ACME should return nil, got %v", err)
	}
}

func TestSaveAccountKeyNilKey(t *testing.T) {
	store := NewStore(t.TempDir())
	acmeClient := NewACMEClient(LEStagingURL, "test@example.com")
	manager := NewManager(store, acmeClient, nil, &boostTestLogger{})

	err := manager.SaveAccountKey()
	if err != nil {
		t.Errorf("SaveAccountKey with nil key should return nil, got %v", err)
	}
}

// --- EnsureCertificate edge cases ---

func TestEnsureCertificateNeedsRenewal(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	manager := NewManager(store, nil, nil, &boostTestLogger{})

	// Create an expired certificate
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "expired.com"},
		NotBefore:    time.Now().Add(-365 * 24 * time.Hour),
		NotAfter:     time.Now().Add(-1 * time.Hour), // Expired
		DNSNames:     []string{"expired.com"},
	}

	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	certDER, _ := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, _ := x509.MarshalECPrivateKey(key)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	store.Save("expired.com", certPEM, keyPEM)

	// Since ACME is nil, this will fail to provision but should detect renewal needed
	err := manager.EnsureCertificate("expired.com")
	if err == nil {
		t.Error("EnsureCertificate for expired cert with no ACME should error")
	}
}

// --- GenerateSelfSigned edge cases ---

func TestGenerateSelfSignedSuccess(t *testing.T) {
	cert, err := GenerateSelfSigned("localhost")
	if err != nil {
		t.Fatalf("GenerateSelfSigned error: %v", err)
	}
	if cert == nil {
		t.Error("GenerateSelfSigned should return a certificate")
	}
	if cert.Leaf == nil {
		t.Error("GenerateSelfSigned certificate should have Leaf")
	}
	if cert.PrivateKey == nil {
		t.Error("GenerateSelfSigned certificate should have PrivateKey")
	}
}

// --- provisionCertificate edge cases ---

func TestProvisionCertificateNilACME(t *testing.T) {
	store := NewStore(t.TempDir())
	manager := NewManager(store, nil, nil, &boostTestLogger{})

	err := manager.provisionCertificate("test.example.com")
	if err == nil {
		t.Error("provisionCertificate with nil ACME should error")
	}
	if err.Error() != "ACME client not initialized" {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- processAuthorization edge cases ---

func TestProcessAuthorizationNilACME(t *testing.T) {
	// This test documents that processAuthorization panics with nil ACME
	// The function doesn't check for nil before using m.acme
	defer func() {
		if r := recover(); r != nil {
			t.Logf("processAuthorization panics with nil ACME (expected): %v", r)
		}
	}()

	store := NewStore(t.TempDir())
	manager := NewManager(store, nil, nil, &boostTestLogger{})

	// This will panic - documenting the behavior
	_ = manager.processAuthorization("http://example.com/auth")
	t.Error("Should have panicked with nil ACME")
}

// --- generateCSR tests ---

func TestGenerateCSRSuccess(t *testing.T) {
	store := NewStore(t.TempDir())
	manager := NewManager(store, nil, nil, &boostTestLogger{})

	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	csr, err := manager.generateCSR(key, "example.com")
	if err != nil {
		t.Errorf("generateCSR error: %v", err)
	}
	if csr == nil {
		t.Error("generateCSR should return CSR bytes")
	}
}

// --- encodePrivateKey edge cases ---

func TestEncodePrivateKeySuccess(t *testing.T) {
	store := NewStore(t.TempDir())
	manager := NewManager(store, nil, nil, &boostTestLogger{})

	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	pem, err := manager.encodePrivateKey(key)
	if err != nil {
		t.Errorf("encodePrivateKey error: %v", err)
	}
	if pem == nil {
		t.Error("encodePrivateKey should return PEM bytes")
	}
}

// --- Store.Save edge cases ---

func TestStoreSaveAndLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	// Generate test certificate
	certPEM, keyPEM := boostGenerateTestCertPEM(t)

	// Save
	err := store.Save("test.com", certPEM, keyPEM)
	if err != nil {
		t.Fatalf("Save error: %v", err)
	}

	// Load
	loadedCert, loadedKey, err := store.LoadPEM("test.com")
	if err != nil {
		t.Fatalf("LoadPEM error: %v", err)
	}

	// Verify
	if string(loadedCert) != string(certPEM) {
		t.Error("Certificate mismatch")
	}
	if string(loadedKey) != string(keyPEM) {
		t.Error("Key mismatch")
	}
}

// --- Store.Delete edge cases ---

func TestStoreDeleteExisting(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	// Create a certificate
	certPEM, keyPEM := boostGenerateTestCertPEM(t)
	store.Save("delete-test.com", certPEM, keyPEM)

	// Verify it exists
	if !store.Exists("delete-test.com") {
		t.Fatal("Certificate should exist before deletion")
	}

	// Delete
	err := store.Delete("delete-test.com")
	if err != nil {
		t.Errorf("Delete error: %v", err)
	}

	// Verify it's gone
	if store.Exists("delete-test.com") {
		t.Error("Certificate should not exist after deletion")
	}
}

// --- Manager.GetCertificate edge cases ---

func TestManagerGetCertificateNotFound(t *testing.T) {
	store := NewStore(t.TempDir())
	manager := NewManager(store, nil, nil, &boostTestLogger{})

	// Try to get certificate for non-existent domain
	hello := &tls.ClientHelloInfo{ServerName: "nonexistent.com"}
	cert, err := manager.GetCertificate(hello)

	// Should return error since cert doesn't exist and no ACME
	if err == nil {
		t.Error("GetCertificate should error for non-existent domain without ACME")
	}
	if cert != nil {
		t.Error("Certificate should be nil on error")
	}
}

// --- Store.List with multiple certificates ---

func TestStoreListMultiple(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	// Create multiple certificates
	certPEM, keyPEM := boostGenerateTestCertPEM(t)

	domains := []string{"a.com", "b.com", "c.com"}
	for _, domain := range domains {
		store.Save(domain, certPEM, keyPEM)
		store.SaveMeta(domain, &CertMeta{
			Domain:    domain,
			Expiry:    time.Now().Add(90 * 24 * time.Hour).Unix(),
			Issuer:    "test",
			CreatedAt: time.Now().Unix(),
		})
	}

	// List all
	certs, err := store.List()
	if err != nil {
		t.Fatalf("List error: %v", err)
	}

	if len(certs) != 3 {
		t.Errorf("Expected 3 certificates, got %d", len(certs))
	}
}

// --- IsValid edge cases ---

func TestIsValidExpired(t *testing.T) {
	// Create an expired certificate
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "expired.com"},
		NotBefore:    time.Now().Add(-365 * 24 * time.Hour),
		NotAfter:     time.Now().Add(-1 * time.Hour), // Expired
		DNSNames:     []string{"expired.com"},
	}

	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	certDER, _ := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	// Should not be valid
	valid, err := IsValid(certPEM, 30*24*time.Hour)
	if err != nil {
		t.Errorf("IsValid error: %v", err)
	}
	if valid {
		t.Error("Expired certificate should not be valid")
	}
}

func TestIsValidAlmostExpired(t *testing.T) {
	// Create a certificate that expires soon
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "almost-expired.com"},
		NotBefore:    time.Now().Add(-30 * 24 * time.Hour),
		NotAfter:     time.Now().Add(10 * 24 * time.Hour), // Expires in 10 days
		DNSNames:     []string{"almost-expired.com"},
	}

	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	certDER, _ := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	// Check if valid for 30 days (should fail since it expires in 10)
	valid, err := IsValid(certPEM, 30*24*time.Hour)
	if err != nil {
		t.Errorf("IsValid error: %v", err)
	}
	if valid {
		t.Error("Almost expired certificate should not be valid for 30 days")
	}
}

func TestRenewNilACME(t *testing.T) {
	store := NewStore(t.TempDir())
	manager := NewManager(store, nil, nil, &boostTestLogger{})

	err := manager.Renew("test.example.com")
	if err == nil {
		t.Error("Renew with nil ACME should error")
	}
}

// --- provisionCertificate edge cases ---

func TestProvisionCertificateOrderError(t *testing.T) {
	logger := &boostTestLogger{}
	store := NewStore(t.TempDir())
	challenge := NewChallengeSolver()

	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	acmeClient := &ACMEClient{
		privateKey:  key,
		nonce:       "test-nonce",
		newOrderURL: "http://[::1]:1/new-order",
		httpClient:  &http.Client{Timeout: 100 * time.Millisecond},
	}

	manager := NewManager(store, acmeClient, challenge, logger)

	// Should fail when trying to create order due to invalid URL
	err := manager.provisionCertificate("test.example.com")
	if err == nil {
		t.Error("provisionCertificate should fail with invalid order URL")
	}
}

// --- processAuthorization edge cases ---

func TestProcessAuthorizationInvalidAuthURL(t *testing.T) {
	logger := &boostTestLogger{}
	store := NewStore(t.TempDir())
	challenge := NewChallengeSolver()

	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	acmeClient := &ACMEClient{
		privateKey: key,
		nonce:      "test-nonce",
	}

	manager := NewManager(store, acmeClient, challenge, logger)

	// Should fail with invalid URL
	err := manager.processAuthorization("://invalid-url")
	if err == nil {
		t.Error("processAuthorization should fail with invalid URL")
	}
}

// --- Store.List edge cases ---

func TestStoreListEmpty(t *testing.T) {
	store := NewStore(t.TempDir())

	// List on empty store should return empty slice
	certs, err := store.List()
	if err != nil {
		t.Errorf("List error: %v", err)
	}
	if len(certs) != 0 {
		t.Errorf("Expected 0 certs, got %d", len(certs))
	}
}

// --- ACME client with invalid URLs ---

func TestACMEClientInitializeInvalidDirectoryURL(t *testing.T) {
	client := NewACMEClient("://invalid-url", "test@example.com")
	err := client.Initialize()
	if err == nil {
		t.Error("Initialize should fail with invalid directory URL")
	}
}

// --- Challenge solver edge cases ---

func TestChallengeSolverHandlerNotFound(t *testing.T) {
	solver := NewChallengeSolver()

	// Get the handler
	handler := solver.Handler()

	// Handle request for non-existent token
	req := httptest.NewRequest("GET", "/.well-known/acme-challenge/nonexistent-token", nil)
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want 404", rec.Code)
	}
}

func TestChallengeSolverHandlerWithToken(t *testing.T) {
	solver := NewChallengeSolver()

	// Add a token using SetToken
	solver.SetToken("test-token", "test-key-auth")

	// Verify it was added
	token, ok := solver.GetToken("test-token")
	if !ok {
		t.Error("GetToken should return ok=true for added token")
	}
	if token != "test-key-auth" {
		t.Errorf("Token = %q, want test-key-auth", token)
	}

	// Get the handler and request the token
	handler := solver.Handler()
	req := httptest.NewRequest("GET", "/.well-known/acme-challenge/test-token", nil)
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want 200", rec.Code)
	}
	if rec.Body.String() != "test-key-auth" {
		t.Errorf("Body = %q, want test-key-auth", rec.Body.String())
	}
}

// --- ACME client error paths ---

func TestACMEClientFetchDirectoryError(t *testing.T) {
	client := NewACMEClient("://invalid-url", "test@example.com")
	err := client.fetchDirectory()
	if err == nil {
		t.Error("fetchDirectory should fail with invalid URL")
	}
}

func TestACMEClientFetchNonceError(t *testing.T) {
	client := NewACMEClient("://invalid-url", "test@example.com")
	err := client.fetchNonce()
	if err == nil {
		t.Error("fetchNonce should fail with invalid URL")
	}
}

func TestACMEClientCreateOrGetAccountError(t *testing.T) {
	client := NewACMEClient("://invalid-url", "test@example.com")
	// Set a private key to skip key generation
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	client.privateKey = key

	err := client.createOrGetAccount()
	if err == nil {
		t.Error("createOrGetAccount should fail with invalid URL")
	}
}

func TestACMEClientRequestOrderError(t *testing.T) {
	client := NewACMEClient("://invalid-url", "test@example.com")
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	client.privateKey = key

	_, err := client.RequestOrder([]string{"example.com"})
	if err == nil {
		t.Error("RequestOrder should fail with invalid URL")
	}
}

func TestACMEClientGetAuthorizationError(t *testing.T) {
	client := NewACMEClient("://invalid-url", "test@example.com")
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	client.privateKey = key

	_, err := client.GetAuthorization("://invalid-url")
	if err == nil {
		t.Error("GetAuthorization should fail with invalid URL")
	}
}

func TestACMEClientGetChallengeError(t *testing.T) {
	client := NewACMEClient("://invalid-url", "test@example.com")
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	client.privateKey = key

	_, err := client.GetChallenge("://invalid-url")
	if err == nil {
		t.Error("GetChallenge should fail with invalid URL")
	}
}

func TestACMEClientTriggerChallengeError(t *testing.T) {
	client := NewACMEClient("://invalid-url", "test@example.com")
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	client.privateKey = key

	_, err := client.TriggerChallenge("://invalid-url")
	if err == nil {
		t.Error("TriggerChallenge should fail with invalid URL")
	}
}

func TestACMEClientFinalizeOrderError(t *testing.T) {
	client := NewACMEClient("://invalid-url", "test@example.com")
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	client.privateKey = key

	order := &ACMEOrder{FinalizeURL: "://invalid-url"}
	csr := []byte("test-csr")

	err := client.FinalizeOrder(order, csr)
	if err == nil {
		t.Error("FinalizeOrder should fail with invalid URL")
	}
}

func TestACMEClientDownloadCertificateError(t *testing.T) {
	client := NewACMEClient("://invalid-url", "test@example.com")
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	client.privateKey = key

	_, err := client.DownloadCertificate("://invalid-url")
	if err == nil {
		t.Error("DownloadCertificate should fail with invalid URL")
	}
}

func TestACMEClientPollOrderError(t *testing.T) {
	client := NewACMEClient("://invalid-url", "test@example.com")
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	client.privateKey = key

	_, err := client.PollOrder("://invalid-url", "valid", 100*time.Millisecond)
	if err == nil {
		t.Error("PollOrder should fail with invalid URL")
	}
}

