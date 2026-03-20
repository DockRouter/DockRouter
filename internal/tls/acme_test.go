package tls

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewACMEClient(t *testing.T) {
	tests := []struct {
		name  string
		url   string
		email string
	}{
		{
			name:  "Let's Encrypt production",
			url:   LEProdURL,
			email: "test@example.com",
		},
		{
			name:  "Let's Encrypt staging",
			url:   LEStagingURL,
			email: "staging@example.com",
		},
		{
			name:  "ZeroSSL",
			url:   ZeroSSLURL,
			email: "zerossl@example.com",
		},
		{
			name:  "custom directory",
			url:   "https://acme.example.com/directory",
			email: "custom@example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewACMEClient(tt.url, tt.email)
			if client == nil {
				t.Fatal("NewACMEClient returned nil")
			}
			if client.directoryURL != tt.url {
				t.Errorf("directoryURL = %s, want %s", client.directoryURL, tt.url)
			}
			if client.email != tt.email {
				t.Errorf("email = %s, want %s", client.email, tt.email)
			}
			if client.httpClient == nil {
				t.Error("httpClient not initialized")
			}
		})
	}
}

func TestBase64URLEncode(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"f", "Zg"},
		{"fo", "Zm8"},
		{"foo", "Zm9v"},
		{"foob", "Zm9vYg"},
		{"fooba", "Zm9vYmE"},
		{"foobar", "Zm9vYmFy"},
		{"test data", "dGVzdCBkYXRh"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := base64URLEncode([]byte(tt.input))
			if result != tt.expected {
				t.Errorf("base64URLEncode(%q) = %q, want %q", tt.input, result, tt.expected)
			}

			// Verify no padding
			if len(result) > 0 && result[len(result)-1] == '=' {
				t.Error("Result should not contain padding")
			}
		})
	}
}

func TestBase64URLEncodeRoundTrip(t *testing.T) {
	testData := [][]byte{
		[]byte("hello world"),
		[]byte{0, 1, 2, 3, 4, 5},
		[]byte{255, 254, 253},
		make([]byte, 256),
	}

	for i := range testData {
		encoded := base64URLEncode(testData[i])
		decoded, err := base64.URLEncoding.WithPadding(base64.NoPadding).DecodeString(encoded)
		if err != nil {
			t.Errorf("Failed to decode: %v", err)
			continue
		}
		if string(decoded) != string(testData[i]) {
			t.Errorf("Round trip failed: got %v, want %v", decoded, testData[i])
		}
	}
}

func TestPadBytes(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		size     int
		expected []byte
	}{
		{
			name:     "short input",
			input:    []byte{1, 2, 3},
			size:     5,
			expected: []byte{0, 0, 1, 2, 3},
		},
		{
			name:     "exact size",
			input:    []byte{1, 2, 3, 4, 5},
			size:     5,
			expected: []byte{1, 2, 3, 4, 5},
		},
		{
			name:     "larger input",
			input:    []byte{1, 2, 3, 4, 5, 6, 7},
			size:     5,
			expected: []byte{3, 4, 5, 6, 7},
		},
		{
			name:     "empty input",
			input:    []byte{},
			size:     4,
			expected: []byte{0, 0, 0, 0},
		},
		{
			name:     "zero size",
			input:    []byte{1, 2, 3},
			size:     0,
			expected: []byte{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := padBytes(tt.input, tt.size)
			if len(result) != len(tt.expected) {
				t.Errorf("padBytes length = %d, want %d", len(result), len(tt.expected))
				return
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("padBytes[%d] = %d, want %d", i, result[i], tt.expected[i])
				}
			}
		})
	}
}

func TestParseECDSASignature(t *testing.T) {
	tests := []struct {
		name    string
		input   []byte
		wantLen int
		wantErr bool
	}{
		{
			name:    "invalid - too short",
			input:   []byte{0x30, 0x02},
			wantErr: true,
		},
		{
			name:    "invalid - wrong tag",
			input:   []byte{0x31, 0x08, 0x02, 0x01, 0x01, 0x02, 0x01, 0x02},
			wantErr: true,
		},
		{
			name:    "valid signature",
			input:   []byte{0x30, 0x08, 0x02, 0x01, 0x01, 0x02, 0x01, 0x02},
			wantLen: 64, // 32 + 32
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseECDSASignature(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("Expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}
			if len(result) != tt.wantLen {
				t.Errorf("Result length = %d, want %d", len(result), tt.wantLen)
			}
		})
	}
}

func TestACMEError(t *testing.T) {
	err := &ACMEError{
		Type:   "urn:ietf:params:acme:error:unauthorized",
		Detail: "Invalid signature",
		Status: 403,
	}

	expected := "ACME error: urn:ietf:params:acme:error:unauthorized - Invalid signature"
	if err.Error() != expected {
		t.Errorf("Error() = %s, want %s", err.Error(), expected)
	}
}

func TestIdentifier(t *testing.T) {
	ident := Identifier{
		Type:  "dns",
		Value: "example.com",
	}

	if ident.Type != "dns" {
		t.Errorf("Type = %s, want dns", ident.Type)
	}
	if ident.Value != "example.com" {
		t.Errorf("Value = %s, want example.com", ident.Value)
	}
}

func TestChallenge(t *testing.T) {
	ch := Challenge{
		Type:   "http-01",
		URL:    "https://acme.example.com/challenge/123",
		Token:  "abc123",
		Status: "pending",
	}

	if ch.Type != "http-01" {
		t.Errorf("Type = %s, want http-01", ch.Type)
	}
	if ch.Token != "abc123" {
		t.Errorf("Token = %s, want abc123", ch.Token)
	}
}

func TestACMEDirectory(t *testing.T) {
	dir := ACMEDirectory{
		NewNonce:   "https://acme.example.com/nonce",
		NewAccount: "https://acme.example.com/account",
		NewOrder:   "https://acme.example.com/order",
		RevokeCert: "https://acme.example.com/revoke",
		KeyChange:  "https://acme.example.com/key-change",
	}

	if dir.NewNonce == "" {
		t.Error("NewNonce should not be empty")
	}
}

func TestACMEOrder(t *testing.T) {
	order := ACMEOrder{
		Status:         "pending",
		Expires:        "2024-12-31T23:59:59Z",
		Identifiers:    []Identifier{{Type: "dns", Value: "example.com"}},
		Authorizations: []string{"https://acme.example.com/auth/1"},
		FinalizeURL:    "https://acme.example.com/finalize/1",
	}

	if order.Status != "pending" {
		t.Errorf("Status = %s, want pending", order.Status)
	}
	if len(order.Identifiers) != 1 {
		t.Errorf("Identifiers count = %d, want 1", len(order.Identifiers))
	}
}

func TestACMEAccount(t *testing.T) {
	account := ACMEAccount{
		Status:               "valid",
		Contact:              []string{"mailto:test@example.com"},
		OrdersURL:            "https://acme.example.com/orders/1",
		TermsOfServiceAgreed: true,
	}

	if account.Status != "valid" {
		t.Errorf("Status = %s, want valid", account.Status)
	}
	if !account.TermsOfServiceAgreed {
		t.Error("TermsOfServiceAgreed should be true")
	}
}

func TestACMEAuthorization(t *testing.T) {
	auth := ACMEAuthorization{
		Status:     "valid",
		Identifier: Identifier{Type: "dns", Value: "example.com"},
		Challenges: []Challenge{
			{Type: "http-01", Status: "valid"},
			{Type: "dns-01", Status: "pending"},
		},
	}

	if auth.Status != "valid" {
		t.Errorf("Status = %s, want valid", auth.Status)
	}
	if len(auth.Challenges) != 2 {
		t.Errorf("Challenges count = %d, want 2", len(auth.Challenges))
	}
}

func TestACMEClientJWK(t *testing.T) {
	// Test with valid private key
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	client := NewACMEClient(LEStagingURL, "test@example.com")
	client.privateKey = privKey

	jwk := client.jwk()
	if jwk == nil {
		t.Fatal("jwk returned nil")
	}
	if jwk["kty"] != "EC" {
		t.Errorf("kty = %v, want EC", jwk["kty"])
	}
	if jwk["crv"] != "P-256" {
		t.Errorf("crv = %v, want P-256", jwk["crv"])
	}
	if jwk["x"] == nil {
		t.Error("x should not be nil")
	}
	if jwk["y"] == nil {
		t.Error("y should not be nil")
	}
}

func TestACMEClientFetchDirectory(t *testing.T) {
	// Create a test server that returns a valid directory
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"newNonce": "https://acme.example.com/acme/new-nonce",
			"newAccount": "https://acme.example.com/acme/new-acct",
			"newOrder": "https://acme.example.com/acme/new-order",
			"revokeCert": "https://acme.example.com/acme/revoke-cert",
			"keyChange": "https://acme.example.com/acme/key-change"
		}`))
	}))
	defer server.Close()

	client := NewACMEClient(server.URL, "test@example.com")
	err := client.fetchDirectory()
	if err != nil {
		t.Fatalf("fetchDirectory failed: %v", err)
	}

	if client.newNonceURL != "https://acme.example.com/acme/new-nonce" {
		t.Errorf("newNonceURL = %s, want https://acme.example.com/acme/new-nonce", client.newNonceURL)
	}
	if client.newAccountURL != "https://acme.example.com/acme/new-acct" {
		t.Errorf("newAccountURL = %s, want https://acme.example.com/acme/new-acct", client.newAccountURL)
	}
	if client.newOrderURL != "https://acme.example.com/acme/new-order" {
		t.Errorf("newOrderURL = %s, want https://acme.example.com/acme/new-order", client.newOrderURL)
	}
}

func TestACMEClientFetchNonce(t *testing.T) {
	// Create a test server that returns a nonce
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "HEAD" {
			t.Errorf("Expected HEAD request, got %s", r.Method)
		}
		w.Header().Set("Replay-Nonce", "test-nonce-12345")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewACMEClient("", "test@example.com")
	client.newNonceURL = server.URL

	err := client.fetchNonce()
	if err != nil {
		t.Fatalf("fetchNonce failed: %v", err)
	}

	if client.nonce != "test-nonce-12345" {
		t.Errorf("nonce = %s, want test-nonce-12345", client.nonce)
	}
}

func TestACMEClientFetchDirectoryInvalidJSON(t *testing.T) {
	// Create a test server that returns invalid JSON
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`invalid json`))
	}))
	defer server.Close()

	client := NewACMEClient(server.URL, "test@example.com")
	err := client.fetchDirectory()
	if err == nil {
		t.Error("fetchDirectory should return error for invalid JSON")
	}
}

func TestACMEClientFetchDirectoryConnectionError(t *testing.T) {
	client := NewACMEClient("http://localhost:59999/nonexistent", "test@example.com")
	err := client.fetchDirectory()
	if err == nil {
		t.Error("fetchDirectory should return error for connection failure")
	}
}

func TestACMEClientFetchNonceConnectionError(t *testing.T) {
	client := NewACMEClient("", "test@example.com")
	client.newNonceURL = "http://localhost:59999/nonexistent"
	err := client.fetchNonce()
	if err == nil {
		t.Error("fetchNonce should return error for connection failure")
	}
}

func TestACMEClientHTTPTimeout(t *testing.T) {
	client := NewACMEClient(LEStagingURL, "test@example.com")
	if client.httpClient.Timeout != 30*time.Second {
		t.Errorf("HTTP client timeout = %v, want 30s", client.httpClient.Timeout)
	}
}

func TestACMEClientInitialize(t *testing.T) {
	// Create mock server for directory and nonce
	directoryCalled := false
	nonceCalled := false
	accountCalled := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/directory" {
			directoryCalled = true
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{
				"newNonce": "http://` + r.Host + `/nonce",
				"newAccount": "http://` + r.Host + `/account",
				"newOrder": "http://` + r.Host + `/order"
			}`))
		} else if r.URL.Path == "/nonce" {
			nonceCalled = true
			w.Header().Set("Replay-Nonce", "test-nonce-123")
			w.WriteHeader(http.StatusOK)
		} else if r.URL.Path == "/account" {
			accountCalled = true
			w.Header().Set("Location", "http://"+r.Host+"/account/123")
			w.Header().Set("Replay-Nonce", "test-nonce-456")
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"status": "valid"}`))
		}
	}))
	defer server.Close()

	client := NewACMEClient(server.URL, "test@example.com")
	err := client.Initialize()
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	if !directoryCalled {
		t.Error("Directory not fetched")
	}
	if !nonceCalled {
		t.Error("Nonce not fetched")
	}
	if !accountCalled {
		t.Error("Account not created")
	}
	if client.accountURL == "" {
		t.Error("Account URL not set")
	}
}

func TestACMEClientSignedGet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// signedGet now uses POST-as-GET (RFC 8555)
		if r.Method != "POST" {
			t.Errorf("Method = %s, want POST (POST-as-GET)", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/jose+json" {
			t.Errorf("Content-Type = %s, want application/jose+json", r.Header.Get("Content-Type"))
		}
		w.Header().Set("Replay-Nonce", "new-nonce")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "ok"}`))
	}))
	defer server.Close()

	privKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	client := NewACMEClient(LEStagingURL, "test@example.com")
	client.privateKey = privKey
	client.nonce = "initial-nonce"

	resp, err := client.signedGet(server.URL)
	if err != nil {
		t.Fatalf("signedGet failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Status = %d, want 200", resp.StatusCode)
	}
}

func TestACMEClientSignedPost(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/jose+json" {
			t.Errorf("Content-Type = %s, want application/jose+json", r.Header.Get("Content-Type"))
		}
		w.Header().Set("Replay-Nonce", "new-nonce")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "ok"}`))
	}))
	defer server.Close()

	privKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	client := NewACMEClient(LEStagingURL, "test@example.com")
	client.privateKey = privKey
	client.nonce = "initial-nonce"

	resp, err := client.signedPost(server.URL, map[string]string{"test": "data"})
	if err != nil {
		t.Fatalf("signedPost failed: %v", err)
	}
	defer resp.Body.Close()

	if client.nonce != "new-nonce" {
		t.Errorf("Nonce not updated: %s", client.nonce)
	}
}

func TestACMEClientSignPayload(t *testing.T) {
	privKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	client := NewACMEClient(LEStagingURL, "test@example.com")
	client.privateKey = privKey
	client.nonce = "test-nonce"

	jws, err := client.signPayload(map[string]string{"key": "value"}, "https://example.com/test")
	if err != nil {
		t.Fatalf("signPayload failed: %v", err)
	}

	if jws["protected"] == nil {
		t.Error("Missing protected header")
	}
	if jws["payload"] == nil {
		t.Error("Missing payload")
	}
	if jws["signature"] == nil {
		t.Error("Missing signature")
	}
}

func TestACMEClientSignPayloadWithAccount(t *testing.T) {
	privKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	client := NewACMEClient(LEStagingURL, "test@example.com")
	client.privateKey = privKey
	client.nonce = "test-nonce"
	client.accountURL = "https://acme.example.com/account/123"

	jws, err := client.signPayload(map[string]string{"key": "value"}, "https://example.com/test")
	if err != nil {
		t.Fatalf("signPayload failed: %v", err)
	}

	// When account URL is set, it should use kid instead of jwk
	if jws["protected"] == nil {
		t.Error("Missing protected header")
	}
}

func TestACMEClientRequestOrder(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Replay-Nonce", "order-nonce")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{
			"status": "pending",
			"expires": "2024-12-31T23:59:59Z",
			"identifiers": [{"type": "dns", "value": "example.com"}],
			"authorizations": ["https://acme.example.com/auth/1"],
			"finalize": "https://acme.example.com/finalize/1"
		}`))
	}))
	defer server.Close()

	privKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	client := NewACMEClient(LEStagingURL, "test@example.com")
	client.privateKey = privKey
	client.nonce = "test-nonce"
	client.newOrderURL = server.URL

	order, err := client.RequestOrder([]string{"example.com"})
	if err != nil {
		t.Fatalf("RequestOrder failed: %v", err)
	}

	if order.Status != "pending" {
		t.Errorf("Status = %s, want pending", order.Status)
	}
	if len(order.Identifiers) != 1 {
		t.Errorf("Identifiers count = %d, want 1", len(order.Identifiers))
	}
}

func TestACMEClientGetAuthorization(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"status": "pending",
			"identifier": {"type": "dns", "value": "example.com"},
			"challenges": [
				{"type": "http-01", "url": "https://acme.example.com/challenge/1", "token": "abc123", "status": "pending"}
			]
		}`))
	}))
	defer server.Close()

	privKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	client := NewACMEClient(LEStagingURL, "test@example.com")
	client.privateKey = privKey
	client.nonce = "test-nonce"

	auth, err := client.GetAuthorization(server.URL)
	if err != nil {
		t.Fatalf("GetAuthorization failed: %v", err)
	}

	if auth.Status != "pending" {
		t.Errorf("Status = %s, want pending", auth.Status)
	}
	if len(auth.Challenges) != 1 {
		t.Errorf("Challenges count = %d, want 1", len(auth.Challenges))
	}
}

func TestACMEClientGetChallenge(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"type": "http-01",
			"url": "https://acme.example.com/challenge/1",
			"token": "abc123",
			"status": "pending"
		}`))
	}))
	defer server.Close()

	privKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	client := NewACMEClient(LEStagingURL, "test@example.com")
	client.privateKey = privKey
	client.nonce = "test-nonce"

	ch, err := client.GetChallenge(server.URL)
	if err != nil {
		t.Fatalf("GetChallenge failed: %v", err)
	}

	if ch.Type != "http-01" {
		t.Errorf("Type = %s, want http-01", ch.Type)
	}
	if ch.Token != "abc123" {
		t.Errorf("Token = %s, want abc123", ch.Token)
	}
}

func TestACMEClientTriggerChallenge(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Replay-Nonce", "triggered-nonce")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"type": "http-01",
			"url": "https://acme.example.com/challenge/1",
			"token": "abc123",
			"status": "processing"
		}`))
	}))
	defer server.Close()

	privKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	client := NewACMEClient(LEStagingURL, "test@example.com")
	client.privateKey = privKey
	client.nonce = "test-nonce"

	ch, err := client.TriggerChallenge(server.URL)
	if err != nil {
		t.Fatalf("TriggerChallenge failed: %v", err)
	}

	if ch.Status != "processing" {
		t.Errorf("Status = %s, want processing", ch.Status)
	}
	if client.nonce != "triggered-nonce" {
		t.Errorf("Nonce not updated")
	}
}

func TestACMEClientFinalizeOrder(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"status": "valid",
			"certificate": "https://acme.example.com/cert/1"
		}`))
	}))
	defer server.Close()

	privKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	client := NewACMEClient(LEStagingURL, "test@example.com")
	client.privateKey = privKey
	client.nonce = "test-nonce"

	order := &ACMEOrder{
		FinalizeURL: server.URL,
	}
	csr := []byte("dummy-csr-data")

	err := client.FinalizeOrder(order, csr)
	if err != nil {
		t.Fatalf("FinalizeOrder failed: %v", err)
	}

	if order.Status != "valid" {
		t.Errorf("Status = %s, want valid", order.Status)
	}
	if order.CertificateURL != "https://acme.example.com/cert/1" {
		t.Errorf("CertificateURL not set")
	}
}

func TestACMEClientDownloadCertificate(t *testing.T) {
	certPEM := `-----BEGIN CERTIFICATE-----
MIIBkTCB+wIJAKHBfpLxAAAAADANBgkqhkiG9w0BAQsFADANMQswCQYDVQQDDAJj
-----END CERTIFICATE-----`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(certPEM))
	}))
	defer server.Close()

	privKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	client := NewACMEClient(LEStagingURL, "test@example.com")
	client.privateKey = privKey
	client.nonce = "test-nonce"

	data, err := client.DownloadCertificate(server.URL)
	if err != nil {
		t.Fatalf("DownloadCertificate failed: %v", err)
	}

	if string(data) != certPEM {
		t.Error("Certificate data mismatch")
	}
}

func TestACMEClientPollOrder(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount < 3 {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status": "pending"}`))
		} else {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status": "valid", "certificate": "https://acme.example.com/cert/1"}`))
		}
	}))
	defer server.Close()

	privKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	client := NewACMEClient(LEStagingURL, "test@example.com")
	client.privateKey = privKey
	client.nonce = "test-nonce"

	order, err := client.PollOrder(server.URL, "valid", 10*time.Second)
	if err != nil {
		t.Fatalf("PollOrder failed: %v", err)
	}

	if order.Status != "valid" {
		t.Errorf("Status = %s, want valid", order.Status)
	}
}

func TestACMEClientPollOrderInvalid(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "invalid"}`))
	}))
	defer server.Close()

	privKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	client := NewACMEClient(LEStagingURL, "test@example.com")
	client.privateKey = privKey
	client.nonce = "test-nonce"

	_, err := client.PollOrder(server.URL, "valid", 5*time.Second)
	if err == nil {
		t.Error("PollOrder should return error for invalid order")
	}
}

func TestACMEClientPollOrderTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "pending"}`))
	}))
	defer server.Close()

	privKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	client := NewACMEClient(LEStagingURL, "test@example.com")
	client.privateKey = privKey
	client.nonce = "test-nonce"

	_, err := client.PollOrder(server.URL, "valid", 100*time.Millisecond)
	if err == nil {
		t.Error("PollOrder should timeout")
	}
}

func TestACMEClientCreateOrGetAccount(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "https://acme.example.com/account/123")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"status": "valid"}`))
	}))
	defer server.Close()

	privKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	client := NewACMEClient(LEStagingURL, "test@example.com")
	client.privateKey = privKey
	client.nonce = "test-nonce"
	client.newAccountURL = server.URL

	err := client.createOrGetAccount()
	if err != nil {
		t.Fatalf("createOrGetAccount failed: %v", err)
	}

	if client.accountURL != "https://acme.example.com/account/123" {
		t.Errorf("Account URL = %s", client.accountURL)
	}
}
