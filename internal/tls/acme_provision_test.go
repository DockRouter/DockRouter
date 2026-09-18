package tls

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestCheckResponseStatusSuccess tests 2xx status codes pass
func TestCheckResponseStatusSuccess(t *testing.T) {
	for _, code := range []int{200, 201, 204, 299} {
		t.Run(fmt.Sprintf("%d", code), func(t *testing.T) {
			resp := &http.Response{StatusCode: code, Body: http.NoBody}
			if err := checkResponseStatus(resp); err != nil {
				t.Errorf("checkResponseStatus(%d) = %v, want nil", code, err)
			}
		})
	}
}

// TestCheckResponseStatusACMEError tests non-2xx with ACME error body
func TestCheckResponseStatusACMEError(t *testing.T) {
	acmeErrBody := `{"type":"urn:ietf:params:acme:error:unauthorized","detail":"Invalid signature","status":403}`
	resp := &http.Response{
		StatusCode: 403,
		Body:       io.NopCloser(strings.NewReader(acmeErrBody)),
	}
	err := checkResponseStatus(resp)
	if err == nil {
		t.Fatal("expected error for 403")
	}
	if !strings.Contains(err.Error(), "unauthorized") {
		t.Errorf("error = %v, want ACME unauthorized error", err)
	}
	if !strings.Contains(err.Error(), "Invalid signature") {
		t.Errorf("error = %v, want ACME error detail", err)
	}
}

// TestCheckResponseStatusGenericError tests non-2xx with non-ACME body
func TestCheckResponseStatusGenericError(t *testing.T) {
	resp := &http.Response{
		StatusCode: 500,
		Body:       io.NopCloser(strings.NewReader("internal server error")),
	}
	err := checkResponseStatus(resp)
	if err == nil {
		t.Fatal("expected error for 500")
	}
	if !strings.Contains(err.Error(), "HTTP 500") {
		t.Errorf("error = %v, want HTTP 500", err)
	}
}

// TestCheckResponseStatusEmptyBody tests non-2xx with empty body
func TestCheckResponseStatusEmptyBody(t *testing.T) {
	resp := &http.Response{
		StatusCode: 400,
		Body:       io.NopCloser(strings.NewReader("")),
	}
	err := checkResponseStatus(resp)
	if err == nil {
		t.Fatal("expected error for 400")
	}
	if !strings.Contains(err.Error(), "HTTP 400") {
		t.Errorf("error = %v, want HTTP 400", err)
	}
}

// TestCheckResponseStatusACMEErrorNoDetail tests non-2xx with ACME JSON but empty detail
func TestCheckResponseStatusACMEErrorNoDetail(t *testing.T) {
	body := `{"type":"urn:ietf:params:acme:error:malformed","detail":""}`
	resp := &http.Response{
		StatusCode: 400,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
	err := checkResponseStatus(resp)
	if err == nil {
		t.Fatal("expected error for 400")
	}
	// Should fall back to generic error since detail is empty
	if !strings.Contains(err.Error(), "HTTP 400") {
		t.Errorf("error = %v, want HTTP 400 fallback", err)
	}
}

// TestProvisionCertificateOrderCreation tests that provisionCertificate
// creates an order via the ACME client
func TestProvisionCertificateOrderCreation(t *testing.T) {
	logger := &mockTLSLogger{}
	store := NewStore(t.TempDir())

	// Create mock ACME server that returns order with authorization
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Replay-Nonce", "test-nonce")
		if strings.Contains(r.URL.Path, "/new-order") {
			json.NewEncoder(w).Encode(ACMEOrder{
				Status:         "pending",
				Authorizations: []string{server.URL + "/authz/1"},
				FinalizeURL:    server.URL + "/finalize/1",
			})
		} else {
			// Authz/challenge paths - return errors to stop the flow early
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer server.Close()

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

	// This should create order but fail at authorization
	err := manager.provisionCertificate("test.example.com")
	if err == nil {
		t.Error("expected error when auth fails")
	}
	// The error should be about authorization, not order creation
	if !strings.Contains(err.Error(), "authorization") {
		t.Errorf("error = %v, want authorization error", err)
	}
}

// TestProcessAuthorizationSuccess tests the authorization flow with mock server
func TestProcessAuthorizationSuccess(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Replay-Nonce", "test-nonce")
		if strings.Contains(r.URL.Path, "/authz/") {
			json.NewEncoder(w).Encode(ACMEAuthorization{
				Status:     "pending",
				Identifier: Identifier{Type: "dns", Value: "test.com"},
				Challenges: []Challenge{
					{
						Type:   "http-01",
						URL:    server.URL + "/challenge/1",
						Token:  "test-token",
						Status: "pending",
					},
				},
			})
		} else if strings.Contains(r.URL.Path, "/challenge/") {
			json.NewEncoder(w).Encode(Challenge{
				Type:   "http-01",
				URL:    server.URL + "/challenge/1",
				Token:  "test-token",
				Status: "valid",
			})
		}
	}))
	defer server.Close()

	logger := &mockTLSLogger{}
	store := NewStore(t.TempDir())
	challenge := NewChallengeSolver()

	acmeKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	acme := &ACMEClient{
		privateKey:    acmeKey,
		httpClient:    server.Client(),
		newOrderURL:   server.URL + "/new-order",
		newNonceURL:   server.URL + "/nonce",
		newAccountURL: server.URL + "/account",
		nonce:         "test-nonce",
	}

	manager := NewManager(store, acme, challenge, logger)

	err := manager.processAuthorization(server.URL + "/authz/1")
	if err != nil {
		t.Fatalf("processAuthorization error: %v", err)
	}
}

// TestProcessAuthorizationInvalidChallenge tests auth flow when challenge fails
func TestProcessAuthorizationInvalidChallenge(t *testing.T) {
	var server *httptest.Server
	callCount := 0
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Replay-Nonce", "test-nonce")
		if strings.Contains(r.URL.Path, "/authz/") {
			json.NewEncoder(w).Encode(ACMEAuthorization{
				Status:     "pending",
				Identifier: Identifier{Type: "dns", Value: "test.com"},
				Challenges: []Challenge{
					{
						Type:   "http-01",
						URL:    server.URL + "/challenge/1",
						Token:  "test-token",
						Status: "pending",
					},
				},
			})
		} else if strings.Contains(r.URL.Path, "/challenge/") {
			callCount++
			if callCount == 1 {
				json.NewEncoder(w).Encode(Challenge{
					Type:   "http-01",
					URL:    server.URL + "/challenge/1",
					Token:  "test-token",
					Status: "pending",
				})
			} else {
				json.NewEncoder(w).Encode(Challenge{
					Type:   "http-01",
					URL:    server.URL + "/challenge/1",
					Token:  "test-token",
					Status: "invalid",
					Error:  &ACMEError{Type: "urn:ietf:params:acme:error:unauthorized", Detail: "challenge failed"},
				})
			}
		}
	}))
	defer server.Close()

	logger := &mockTLSLogger{}
	store := NewStore(t.TempDir())
	challenge := NewChallengeSolver()

	acmeKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	acme := &ACMEClient{
		privateKey:    acmeKey,
		httpClient:    server.Client(),
		newOrderURL:   server.URL + "/new-order",
		newNonceURL:   server.URL + "/nonce",
		newAccountURL: server.URL + "/account",
		nonce:         "test-nonce",
	}

	manager := NewManager(store, acme, challenge, logger)

	err := manager.processAuthorization(server.URL + "/authz/1")
	if err == nil {
		t.Fatal("processAuthorization should fail with invalid challenge")
	}
	if !strings.Contains(err.Error(), "challenge failed") {
		t.Errorf("error = %v, want challenge failed", err)
	}
}
