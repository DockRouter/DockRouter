package tls

import (
	"context"
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
