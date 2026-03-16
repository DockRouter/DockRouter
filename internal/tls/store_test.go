package tls

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func generateTestCertificate(t *testing.T, domain string) (certPEM, keyPEM []byte) {
	t.Helper()

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: domain,
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(90 * 24 * time.Hour), // 90 days
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{domain},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatal(err)
	}

	certPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	keyPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(priv),
	})

	return certPEM, keyPEM
}

func TestStoreSaveAndLoad(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "dockrouter-tls-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store := NewStore(tmpDir)
	domain := "example.com"

	certPEM, keyPEM := generateTestCertificate(t, domain)

	// Test Save
	err = store.Save(domain, certPEM, keyPEM)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify files exist
	certPath := filepath.Join(tmpDir, "certificates", domain, "cert.pem")
	keyPath := filepath.Join(tmpDir, "certificates", domain, "key.pem")

	if _, err := os.Stat(certPath); os.IsNotExist(err) {
		t.Error("cert.pem was not created")
	}
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		t.Error("key.pem was not created")
	}

	// Test Load
	cert, err := store.Load(domain)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cert == nil {
		t.Error("Load returned nil certificate")
	}

	// Test LoadPEM
	loadedCertPEM, loadedKeyPEM, err := store.LoadPEM(domain)
	if err != nil {
		t.Fatalf("LoadPEM failed: %v", err)
	}
	if string(loadedCertPEM) != string(certPEM) {
		t.Error("Loaded cert PEM does not match original")
	}
	if string(loadedKeyPEM) != string(keyPEM) {
		t.Error("Loaded key PEM does not match original")
	}
}

func TestStoreExists(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "dockrouter-tls-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store := NewStore(tmpDir)

	// Should not exist initially
	if store.Exists("example.com") {
		t.Error("Exists returned true for non-existent certificate")
	}

	// Create certificate
	certPEM, keyPEM := generateTestCertificate(t, "example.com")
	if err := store.Save("example.com", certPEM, keyPEM); err != nil {
		t.Fatal(err)
	}

	// Should exist now
	if !store.Exists("example.com") {
		t.Error("Exists returned false for existing certificate")
	}
}

func TestStoreDelete(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "dockrouter-tls-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store := NewStore(tmpDir)
	domain := "example.com"

	// Create certificate
	certPEM, keyPEM := generateTestCertificate(t, domain)
	if err := store.Save(domain, certPEM, keyPEM); err != nil {
		t.Fatal(err)
	}

	if !store.Exists(domain) {
		t.Fatal("Certificate should exist before delete")
	}

	// Delete
	if err := store.Delete(domain); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Should not exist after delete
	if store.Exists(domain) {
		t.Error("Certificate still exists after delete")
	}
}

func TestStoreList(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "dockrouter-tls-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store := NewStore(tmpDir)

	// List empty store
	list, err := store.List()
	if err != nil {
		t.Fatalf("List failed on empty store: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("Expected empty list, got %d items", len(list))
	}

	// Add multiple certificates
	domains := []string{"example.com", "test.com", "api.example.com"}
	for _, domain := range domains {
		certPEM, keyPEM := generateTestCertificate(t, domain)
		if err := store.Save(domain, certPEM, keyPEM); err != nil {
			t.Fatal(err)
		}
	}

	// List should contain all domains
	list, err = store.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(list) != len(domains) {
		t.Errorf("Expected %d items, got %d", len(domains), len(list))
	}
}

func TestStoreMeta(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "dockrouter-tls-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store := NewStore(tmpDir)
	domain := "example.com"

	// Create certificate first
	certPEM, keyPEM := generateTestCertificate(t, domain)
	if err := store.Save(domain, certPEM, keyPEM); err != nil {
		t.Fatal(err)
	}

	// Save meta
	meta := &CertMeta{
		Domain:    domain,
		Expiry:    time.Now().Add(90 * 24 * time.Hour).Unix(),
		Issuer:    "Let's Encrypt",
		CreatedAt: time.Now().Unix(),
	}
	if err := store.SaveMeta(domain, meta); err != nil {
		t.Fatalf("SaveMeta failed: %v", err)
	}

	// Load meta
	loadedMeta, err := store.LoadMeta(domain)
	if err != nil {
		t.Fatalf("LoadMeta failed: %v", err)
	}
	if loadedMeta.Domain != meta.Domain {
		t.Errorf("Domain mismatch: got %s, want %s", loadedMeta.Domain, meta.Domain)
	}
	if loadedMeta.Issuer != meta.Issuer {
		t.Errorf("Issuer mismatch: got %s, want %s", loadedMeta.Issuer, meta.Issuer)
	}
}

func TestGetExpiry(t *testing.T) {
	certPEM, _ := generateTestCertificate(t, "example.com")

	expiry, err := GetExpiry(certPEM)
	if err != nil {
		t.Fatalf("GetExpiry failed: %v", err)
	}

	// Should be approximately 90 days from now
	expectedExpiry := time.Now().Add(90 * 24 * time.Hour)
	diff := expectedExpiry.Sub(expiry)
	if diff < 0 {
		diff = -diff
	}

	// Allow 1 minute tolerance
	if diff > time.Minute {
		t.Errorf("Expiry mismatch: got %v, expected approximately %v", expiry, expectedExpiry)
	}
}

func TestGetExpiryInvalidPEM(t *testing.T) {
	_, err := GetExpiry([]byte("not valid PEM"))
	if err != ErrNoPEMData {
		t.Errorf("Expected ErrNoPEMData, got %v", err)
	}
}

func TestShouldRenew(t *testing.T) {
	// Certificate that expires in 90 days - should not need renewal
	certPEM, _ := generateTestCertificate(t, "example.com")
	if ShouldRenew(certPEM) {
		t.Error("Should not renew certificate with 90 days left")
	}
}

func TestIsValid(t *testing.T) {
	certPEM, _ := generateTestCertificate(t, "example.com")

	// Certificate valid for 90 days, renewBefore = 30 days
	valid, err := IsValid(certPEM, 30*24*time.Hour)
	if err != nil {
		t.Fatalf("IsValid failed: %v", err)
	}
	if !valid {
		t.Error("Certificate should be valid with 90 days left and 30 day renew threshold")
	}

	// Certificate valid for 90 days, renewBefore = 100 days
	valid, err = IsValid(certPEM, 100*24*time.Hour)
	if err != nil {
		t.Fatalf("IsValid failed: %v", err)
	}
	if valid {
		t.Error("Certificate should need renewal with 90 days left and 100 day renew threshold")
	}
}

func TestLoadNonExistent(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "dockrouter-tls-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store := NewStore(tmpDir)

	_, err = store.Load("nonexistent.com")
	if err == nil {
		t.Error("Load should fail for non-existent certificate")
	}
}

func TestLoadPEMNonExistent(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "dockrouter-tls-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store := NewStore(tmpDir)

	_, _, err = store.LoadPEM("nonexistent.com")
	if err == nil {
		t.Error("LoadPEM should fail for non-existent certificate")
	}
}

func TestLoadMetaNonExistent(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "dockrouter-tls-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store := NewStore(tmpDir)

	_, err = store.LoadMeta("nonexistent.com")
	if err == nil {
		t.Error("LoadMeta should fail for non-existent metadata")
	}
}

func TestLoadMetaInvalidJSON(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "dockrouter-tls-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store := NewStore(tmpDir)
	domain := "example.com"

	// Create certificate first
	certPEM, keyPEM := generateTestCertificate(t, domain)
	if err := store.Save(domain, certPEM, keyPEM); err != nil {
		t.Fatal(err)
	}

	// Write invalid JSON to meta.json
	metaPath := filepath.Join(tmpDir, "certificates", domain, "meta.json")
	if err := os.WriteFile(metaPath, []byte("invalid json {"), 0600); err != nil {
		t.Fatal(err)
	}

	_, err = store.LoadMeta(domain)
	if err == nil {
		t.Error("LoadMeta should fail with invalid JSON")
	}
}

func TestShouldRenewExpired(t *testing.T) {
	// Certificate that has already expired
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "expired.com"},
		NotBefore:    time.Now().Add(-200 * 24 * time.Hour),
		NotAfter:     time.Now().Add(-100 * 24 * time.Hour), // Expired 100 days ago
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatal(err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	if !ShouldRenew(certPEM) {
		t.Error("Should renew expired certificate")
	}
}

func TestShouldRenewError(t *testing.T) {
	// Invalid PEM should return true (needs renewal)
	if !ShouldRenew([]byte("invalid pem")) {
		t.Error("ShouldRenew should return true for invalid PEM")
	}
}

func TestIsValidError(t *testing.T) {
	valid, err := IsValid([]byte("invalid pem"), 30*24*time.Hour)
	if err == nil {
		t.Error("IsValid should return error for invalid PEM")
	}
	if valid {
		t.Error("IsValid should return false for invalid PEM")
	}
}

func TestGetExpiryInvalidCertificate(t *testing.T) {
	// Valid PEM block but invalid certificate bytes
	invalidCert := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: []byte("not a valid certificate"),
	})

	_, err := GetExpiry(invalidCert)
	if err == nil {
		t.Error("GetExpiry should return error for invalid certificate bytes")
	}
}

func TestSaveMetaWithoutCertificate(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "dockrouter-tls-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store := NewStore(tmpDir)
	domain := "example.com"

	meta := &CertMeta{
		Domain:    domain,
		Expiry:    time.Now().Add(90 * 24 * time.Hour).Unix(),
		CreatedAt: time.Now().Unix(),
	}

	// SaveMeta should fail if certificate directory doesn't exist
	err = store.SaveMeta(domain, meta)
	if err == nil {
		t.Error("SaveMeta should fail when certificate directory doesn't exist")
	}
}

func TestStoreListNonExistentDir(t *testing.T) {
	// Use a directory that doesn't exist
	store := NewStore("/nonexistent/path/that/does/not/exist")

	list, err := store.List()
	if err != nil {
		t.Errorf("List should not error on non-existent dir: %v", err)
	}
	if list != nil {
		t.Error("List should return nil for non-existent directory")
	}
}

func TestStoreConcurrentAccess(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "dockrouter-tls-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store := NewStore(tmpDir)
	done := make(chan bool)

	// Concurrent saves
	for i := 0; i < 5; i++ {
		go func(idx int) {
			domain := "example" + string(rune('0'+idx)) + ".com"
			certPEM, keyPEM := generateTestCertificate(t, domain)
			store.Save(domain, certPEM, keyPEM)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 5; i++ {
		<-done
	}
}

// Benchmark for certificate operations
func BenchmarkStoreSave(b *testing.B) {
	tmpDir, err := os.MkdirTemp("", "dockrouter-tls-bench")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store := NewStore(tmpDir)
	certPEM, keyPEM := generateTestCertificate(&testing.T{}, "example.com")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.Save("example.com", certPEM, keyPEM)
	}
}

func BenchmarkStoreLoad(b *testing.B) {
	tmpDir, err := os.MkdirTemp("", "dockrouter-tls-bench")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store := NewStore(tmpDir)
	certPEM, keyPEM := generateTestCertificate(&testing.T{}, "example.com")
	store.Save("example.com", certPEM, keyPEM)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.Load("example.com")
	}
}
