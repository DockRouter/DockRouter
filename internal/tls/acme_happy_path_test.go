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
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// miniCA holds a self-signed CA key+cert used by the mock ACME server
// to sign CSRs on the fly.
type miniCA struct {
	key  *ecdsa.PrivateKey
	cert *x509.Certificate
}

func newMiniCA() (*miniCA, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	sn, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	tmpl := &x509.Certificate{
		SerialNumber:          sn,
		Subject:               pkix.Name{CommonName: "Mock ACME CA"},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, err
	}

	return &miniCA{key: key, cert: cert}, nil
}

// signCSR parses a DER-encoded CSR, extracts the public key, and returns
// a PEM-encoded certificate signed by the CA.
func (ca *miniCA) signCSR(csrDER []byte) ([]byte, error) {
	csr, err := x509.ParseCertificateRequest(csrDER)
	if err != nil {
		return nil, fmt.Errorf("parse CSR: %w", err)
	}

	sn, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	tmpl := &x509.Certificate{
		SerialNumber: sn,
		Subject:      csr.Subject,
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     time.Now().Add(90 * 24 * time.Hour),
		DNSNames:     csr.DNSNames,
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, ca.cert, csr.PublicKey, ca.key)
	if err != nil {
		return nil, fmt.Errorf("create cert: %w", err)
	}

	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER}), nil
}

// decodeJWSPayload extracts and base64url-decodes the payload field from a JWS.
func decodeJWSPayload(body io.Reader) ([]byte, error) {
	var jws struct {
		Payload string `json:"payload"`
	}
	if err := json.NewDecoder(body).Decode(&jws); err != nil {
		return nil, err
	}
	return base64.RawURLEncoding.DecodeString(jws.Payload)
}

// TestProvisionCertificateHappyPath tests the full ACME provisioning flow:
// new-order → authorization (already valid) → finalize → download cert → save to disk → cache.
func TestProvisionCertificateHappyPath(t *testing.T) {
	ca, err := newMiniCA()
	if err != nil {
		t.Fatalf("create mini CA: %v", err)
	}

	var (
		mu         sync.Mutex
		signedCert []byte // PEM cert signed from CSR
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
				Identifier: Identifier{Type: "dns", Value: "test.example.com"},
				Challenges: []Challenge{
					{
						Type:   "http-01",
						URL:    server.URL + "/challenge/1",
						Token:  "test-token",
						Status: "valid",
					},
				},
			})

		case strings.Contains(r.URL.Path, "/challenge/"):
			json.NewEncoder(w).Encode(Challenge{
				Type:   "http-01",
				URL:    server.URL + "/challenge/1",
				Token:  "test-token",
				Status: "valid",
			})

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
			w.Header().Set("Content-Type", "application/pem-certificate-chain")
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

	err = manager.provisionCertificate("test.example.com")
	if err != nil {
		t.Fatalf("provisionCertificate happy path: %v", err)
	}

	// Verify certificate was saved to disk
	if !store.Exists("test.example.com") {
		t.Error("certificate should exist in store after provisioning")
	}

	// Verify certificate is cached in memory
	cached := manager.GetCachedCertificate("test.example.com")
	if cached == nil {
		t.Error("certificate should be cached in memory after provisioning")
	}

	// Verify metadata was saved
	meta, err := store.LoadMeta("test.example.com")
	if err != nil {
		t.Errorf("LoadMeta error: %v", err)
	}
	if meta.Domain != "test.example.com" {
		t.Errorf("meta.Domain = %q, want test.example.com", meta.Domain)
	}
	if meta.Expiry == 0 {
		t.Error("meta.Expiry should not be zero")
	}
}

// TestProvisionCertificateFinalizeError tests that provisionCertificate
// returns an error when the finalize step fails.
func TestProvisionCertificateFinalizeError(t *testing.T) {
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
				Identifier: Identifier{Type: "dns", Value: "test.example.com"},
				Challenges: []Challenge{
					{Type: "http-01", URL: server.URL + "/challenge/1", Token: "tok", Status: "valid"},
				},
			})

		case strings.Contains(r.URL.Path, "/challenge/"):
			json.NewEncoder(w).Encode(Challenge{Status: "valid"})

		case strings.Contains(r.URL.Path, "/finalize/"):
			w.WriteHeader(http.StatusInternalServerError)

		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	logger := &mockTLSLogger{}
	store := NewStore(t.TempDir())

	acmeKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	acme := &ACMEClient{
		privateKey:    acmeKey,
		httpClient:    server.Client(),
		newOrderURL:   server.URL + "/new-order",
		newNonceURL:   server.URL + "/nonce",
		newAccountURL: server.URL + "/account",
		nonce:         "test-nonce",
	}

	manager := NewManager(store, acme, NewChallengeSolver(), logger)

	err := manager.provisionCertificate("test.example.com")
	if err == nil {
		t.Fatal("provisionCertificate should fail when finalize returns 500")
	}
	if !strings.Contains(err.Error(), "finalize") {
		t.Errorf("error should mention finalize, got: %v", err)
	}
}

// TestProvisionCertificateNoCertURL tests provisionCertificate when
// the order completes but no certificate URL is provided and polling also fails.
func TestProvisionCertificateNoCertURL(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Replay-Nonce", "test-nonce")

		switch {
		case strings.Contains(r.URL.Path, "/new-order"):
			json.NewEncoder(w).Encode(ACMEOrder{
				Status:         "pending",
				Authorizations: []string{server.URL + "/authz/1"},
				FinalizeURL:    server.URL + "/finalize/1",
				// No CertificateURL
			})

		case strings.Contains(r.URL.Path, "/authz/"):
			json.NewEncoder(w).Encode(ACMEAuthorization{
				Status:     "valid",
				Identifier: Identifier{Type: "dns", Value: "test.example.com"},
				Challenges: []Challenge{
					{Type: "http-01", URL: server.URL + "/challenge/1", Token: "tok", Status: "valid"},
				},
			})

		case strings.Contains(r.URL.Path, "/challenge/"):
			json.NewEncoder(w).Encode(Challenge{Status: "valid"})

		case strings.Contains(r.URL.Path, "/finalize/"):
			// Return order with no certificate URL
			json.NewEncoder(w).Encode(ACMEOrder{
				Status:         "valid",
				CertificateURL: "",
			})

		case strings.Contains(r.URL.Path, "/poll/"):
			// Polling returns order with no cert URL
			json.NewEncoder(w).Encode(ACMEOrder{
				Status:         "valid",
				CertificateURL: "",
			})

		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	logger := &mockTLSLogger{}
	store := NewStore(t.TempDir())

	acmeKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	acme := &ACMEClient{
		privateKey:    acmeKey,
		httpClient:    server.Client(),
		newOrderURL:   server.URL + "/new-order",
		newNonceURL:   server.URL + "/nonce",
		newAccountURL: server.URL + "/account",
		nonce:         "test-nonce",
	}

	manager := NewManager(store, acme, NewChallengeSolver(), logger)

	err := manager.provisionCertificate("test.example.com")
	if err == nil {
		t.Fatal("provisionCertificate should fail when no certificate URL provided")
	}
	// Either "polling timed out" or "no certificate URL" depending on the code path
	t.Logf("error (expected): %v", err)
}

// TestProvisionCertificateDownloadError tests provisionCertificate when
// the finalize succeeds but certificate download fails.
func TestProvisionCertificateDownloadError(t *testing.T) {
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
				Identifier: Identifier{Type: "dns", Value: "test.example.com"},
				Challenges: []Challenge{
					{Type: "http-01", URL: server.URL + "/challenge/1", Token: "tok", Status: "valid"},
				},
			})

		case strings.Contains(r.URL.Path, "/challenge/"):
			json.NewEncoder(w).Encode(Challenge{Status: "valid"})

		case strings.Contains(r.URL.Path, "/finalize/"):
			json.NewEncoder(w).Encode(ACMEOrder{
				Status:         "valid",
				CertificateURL: server.URL + "/cert/1",
			})

		case strings.Contains(r.URL.Path, "/cert/"):
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"type":"urn:ietf:params:acme:error:unauthorized","detail":"certificate not found"}`))

		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	logger := &mockTLSLogger{}
	store := NewStore(t.TempDir())

	acmeKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	acme := &ACMEClient{
		privateKey:    acmeKey,
		httpClient:    server.Client(),
		newOrderURL:   server.URL + "/new-order",
		newNonceURL:   server.URL + "/nonce",
		newAccountURL: server.URL + "/account",
		nonce:         "test-nonce",
	}

	manager := NewManager(store, acme, NewChallengeSolver(), logger)

	err := manager.provisionCertificate("test.example.com")
	if err == nil {
		t.Fatal("provisionCertificate should fail when cert download returns 404")
	}
	if !strings.Contains(err.Error(), "download") {
		t.Errorf("error should mention download, got: %v", err)
	}
}
