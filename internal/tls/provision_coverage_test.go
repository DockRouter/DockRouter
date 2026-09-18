package tls

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// TestProvisionCertificateSaveFail tests provisionCertificate when store.Save fails.
func TestProvisionCertificateSaveFail(t *testing.T) {
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
				Identifier: Identifier{Type: "dns", Value: "savefail.example.com"},
				Challenges: []Challenge{
					{Type: "http-01", URL: server.URL + "/challenge/1", Token: "tok", Status: "valid"},
				},
			})
		case strings.Contains(r.URL.Path, "/challenge/"):
			json.NewEncoder(w).Encode(Challenge{Status: "valid"})
		case strings.Contains(r.URL.Path, "/finalize/"):
			body, _ := io.ReadAll(r.Body)
			payloadBytes, _ := decodeJWSPayload(strings.NewReader(string(body)))
			var fp struct {
				CSR string `json:"csr"`
			}
			json.Unmarshal(payloadBytes, &fp)
			csrDER, _ := base64.RawURLEncoding.DecodeString(fp.CSR)
			certPEM, _ := ca.signCSR(csrDER)
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
			if cert != nil {
				w.Write(cert)
			}
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	// Use a temp dir, then place a file where certificates subdir would go to cause Save failure
	tmpDir := t.TempDir()
	certDir := filepath.Join(tmpDir, "certificates")
	// Create a FILE (not directory) at the certificates path so MkdirAll inside Save fails
	os.WriteFile(certDir, []byte("blocker"), 0600)

	store := NewStore(tmpDir)
	logger := &mockTLSLogger{}

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

	err = manager.provisionCertificate("savefail.example.com")
	if err == nil {
		t.Fatal("provisionCertificate should fail when store.Save fails")
	}
}

// TestProvisionCertificatePollOrderFail tests the PollOrder failure path when cert URL is empty.
func TestProvisionCertificatePollOrderFail(t *testing.T) {
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
				Identifier: Identifier{Type: "dns", Value: "pollfail.example.com"},
				Challenges: []Challenge{
					{Type: "http-01", URL: server.URL + "/challenge/1", Token: "tok", Status: "valid"},
				},
			})
		case strings.Contains(r.URL.Path, "/challenge/"):
			json.NewEncoder(w).Encode(Challenge{Status: "valid"})
		case strings.Contains(r.URL.Path, "/finalize/"):
			// Return valid status but NO certificate URL — triggers PollOrder
			json.NewEncoder(w).Encode(ACMEOrder{
				Status:         "valid",
				FinalizeURL:    server.URL + "/finalize/1",
				CertificateURL: "", // empty — should trigger PollOrder
			})
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	store := NewStore(tmpDir)
	logger := &mockTLSLogger{}

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

	// PollOrder will return the same order (valid, no cert URL), which triggers
	// the "order completed but no certificate URL provided" error
	err := manager.provisionCertificate("pollfail.example.com")
	if err == nil {
		t.Fatal("provisionCertificate should fail when no certificate URL after poll")
	}
}

// TestProvisionCertificateNoCertURLAfterFinalize tests when finalize returns empty cert URL
// and the poll returns a valid order but still no cert URL.
func TestProvisionCertificateNoCertURLAfterFinalize(t *testing.T) {
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
				Identifier: Identifier{Type: "dns", Value: "nocert.example.com"},
				Challenges: []Challenge{
					{Type: "http-01", URL: server.URL + "/challenge/1", Token: "tok", Status: "valid"},
				},
			})
		case strings.Contains(r.URL.Path, "/challenge/"):
			json.NewEncoder(w).Encode(Challenge{Status: "valid"})
		case strings.Contains(r.URL.Path, "/finalize/"):
			json.NewEncoder(w).Encode(ACMEOrder{
				Status:         "valid",
				FinalizeURL:    server.URL + "/finalize/1",
				CertificateURL: "", // empty
			})
		default:
			// PollOrder hits this path — return valid but still no cert URL
			json.NewEncoder(w).Encode(ACMEOrder{
				Status:         "valid",
				FinalizeURL:    server.URL + "/finalize/1",
				CertificateURL: "",
			})
		}
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	store := NewStore(tmpDir)
	logger := &mockTLSLogger{}

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

	err := manager.provisionCertificate("nocert.example.com")
	if err == nil {
		t.Fatal("should fail when no certificate URL after finalize and poll")
	}
}

// TestGetExpiryBadPEM tests GetExpiry with invalid PEM data.
func TestGetExpiryBadPEM(t *testing.T) {
	_, err := GetExpiry([]byte("not PEM data"))
	if err == nil {
		t.Error("GetExpiry should fail with non-PEM data")
	}
}

// TestGetExpiryBadCert tests GetExpiry with valid PEM but invalid certificate.
func TestGetExpiryBadCert(t *testing.T) {
	badPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: []byte("not a real certificate"),
	})
	_, err := GetExpiry(badPEM)
	if err == nil {
		t.Error("GetExpiry should fail with invalid certificate data")
	}
}

// TestShouldRenewBadPEM tests ShouldRenew with invalid data.
func TestShouldRenewBadPEM(t *testing.T) {
	if !ShouldRenew([]byte("bad data")) {
		t.Error("ShouldRenew should return true for invalid PEM (cannot determine expiry)")
	}
}
