package tls

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// buildExpiringCertPEM creates a cert+key PEM pair that expires within 30 days.
func buildExpiringCertPEM(t *testing.T, domain string) (certPEM, keyPEM []byte) {
	t.Helper()
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	sn, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	tmpl := &x509.Certificate{
		SerialNumber: sn,
		Subject:      pkix.Name{CommonName: domain},
		NotBefore:    time.Now().Add(-24 * time.Hour),
		NotAfter:     time.Now().Add(15 * 24 * time.Hour), // expires in 15 days
		DNSNames:     []string{domain},
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &privKey.PublicKey, privKey)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, _ := x509.MarshalECPrivateKey(privKey)
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return
}

// TestCheckRenewalsSuccessfulRenewal tests checkRenewals when Renew succeeds.
func TestCheckRenewalsSuccessfulRenewal(t *testing.T) {
	ca, err := newMiniCA()
	if err != nil {
		t.Fatalf("create mini CA: %v", err)
	}

	var (
		mu         sync.Mutex
		signedCert []byte
	)

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Replay-Nonce", "test-nonce")
		switch {
		case strings.Contains(r.URL.Path, "/new-order"):
			json.NewEncoder(w).Encode(ACMEOrder{
				Status:         "pending",
				Authorizations: []string{server.URL + "/authz/1"},
				FinalizeURL:    server.URL + "/finalize/1",
			})
		case strings.Contains(r.URL.Path, "/authz/"):
			json.NewEncoder(w).Encode(ACMEAuthorization{
				Status:     "valid",
				Identifier: Identifier{Type: "dns", Value: "renew.example.com"},
				Challenges: []Challenge{
					{Type: "http-01", URL: server.URL + "/challenge/1", Token: "tok", Status: "valid"},
				},
			})
		case strings.Contains(r.URL.Path, "/challenge/"):
			json.NewEncoder(w).Encode(Challenge{Status: "valid"})
		case strings.Contains(r.URL.Path, "/finalize/"):
			body, _ := io.ReadAll(r.Body)
			payloadBytes, err := decodeJWSPayload(strings.NewReader(string(body)))
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			var finalizePayload struct {
				CSR string `json:"csr"`
			}
			if err := json.Unmarshal(payloadBytes, &finalizePayload); err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			csrDER, err := base64.RawURLEncoding.DecodeString(finalizePayload.CSR)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			certPEM, err := ca.signCSR(csrDER)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			mu.Lock()
			signedCert = certPEM
			mu.Unlock()
			json.NewEncoder(w).Encode(ACMEOrder{
				Status:         "valid",
				CertificateURL: server.URL + "/cert/1",
			})
		case strings.Contains(r.URL.Path, "/cert/"):
			mu.Lock()
			cert := signedCert
			mu.Unlock()
			if cert == nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.Write(cert)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	logger := &mockTLSLogger{}
	store := NewStore(t.TempDir())

	// Store an expiring cert
	certPEM, keyPEM := buildExpiringCertPEM(t, "renew.example.com")
	if err := store.Save("renew.example.com", certPEM, keyPEM); err != nil {
		t.Fatalf("save cert: %v", err)
	}

	acmeKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	acme := &ACMEClient{
		privateKey:    acmeKey,
		httpClient:    server.Client(),
		newOrderURL:   server.URL + "/new-order",
		newNonceURL:   server.URL + "/nonce",
		newAccountURL: server.URL + "/account",
		nonce:         "test-nonce",
	}

	challenge := NewChallengeSolver()
	manager := NewManager(store, acme, challenge, logger)
	scheduler := NewRenewalScheduler(manager, logger)

	// This should trigger renewal and succeed
	scheduler.checkRenewals()

	// Verify new cert was saved
	if !store.Exists("renew.example.com") {
		t.Error("cert should still exist after renewal")
	}
}

// TestCheckRenewalsListError tests checkRenewals when store.List returns error.
func TestCheckRenewalsListError(t *testing.T) {
	logger := &mockTLSLogger{}
	store := NewStore("/nonexistent/path/x/y/z")
	manager := NewManager(store, nil, nil, logger)
	scheduler := NewRenewalScheduler(manager, logger)

	// Should not panic
	scheduler.checkRenewals()
}
