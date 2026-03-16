// Package tls handles TLS certificate management
package tls

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ACME directory URLs
const (
	LEProdURL    = "https://acme-v02.api.letsencrypt.org/directory"
	LEStagingURL = "https://acme-staging-v02.api.letsencrypt.org/directory"
	ZeroSSLURL   = "https://acme.zerossl.com/v2/DV90"
)

// ACMEClient implements the ACME protocol
type ACMEClient struct {
	directoryURL string
	email        string
	httpClient   *http.Client
	privateKey   *ecdsa.PrivateKey
	accountURL   string
	nonce        string

	// Directory endpoints
	newNonceURL   string
	newAccountURL string
	newOrderURL   string
}

// ACMEDirectory represents the ACME directory
type ACMEDirectory struct {
	NewNonce   string `json:"newNonce"`
	NewAccount string `json:"newAccount"`
	NewOrder   string `json:"newOrder"`
	RevokeCert string `json:"revokeCert"`
	KeyChange  string `json:"keyChange"`
}

// ACMEAccount represents an ACME account
type ACMEAccount struct {
	Status               string   `json:"status"`
	Contact              []string `json:"contact"`
	OrdersURL            string   `json:"orders,omitempty"`
	TermsOfServiceAgreed bool     `json:"termsOfServiceAgreed"`
}

// ACMEOrder represents a certificate order
type ACMEOrder struct {
	Status      string       `json:"status"`
	Expires     string       `json:"expires"`
	Identifiers []Identifier `json:"identifiers"`
	Authorizations []string  `json:"authorizations"`
	FinalizeURL string       `json:"finalize"`
	CertificateURL string    `json:"certificate,omitempty"`
}

// Identifier represents a domain identifier
type Identifier struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

// ACMEAuthorization represents an authorization
type ACMEAuthorization struct {
	Status     string       `json:"status"`
	Identifier Identifier   `json:"identifier"`
	Challenges []Challenge  `json:"challenges"`
	Expires    string       `json:"expires"`
}

// Challenge represents an ACME challenge
type Challenge struct {
	Type      string            `json:"type"`
	URL       string            `json:"url"`
	Token     string            `json:"token"`
	Status    string            `json:"status"`
	Validated string            `json:"validated,omitempty"`
	Error     *ACMEError        `json:"error,omitempty"`
}

// ACMEError represents an ACME error
type ACMEError struct {
	Type   string `json:"type"`
	Detail string `json:"detail"`
	Status int    `json:"status"`
}

func (e *ACMEError) Error() string {
	return fmt.Sprintf("ACME error: %s - %s", e.Type, e.Detail)
}

// NewACMEClient creates a new ACME client
func NewACMEClient(directoryURL, email string) *ACMEClient {
	return &ACMEClient{
		directoryURL: directoryURL,
		email:        email,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Initialize fetches the directory and creates/gets account
func (c *ACMEClient) Initialize() error {
	// Fetch directory
	if err := c.fetchDirectory(); err != nil {
		return fmt.Errorf("failed to fetch directory: %w", err)
	}

	// Generate account key if not exists
	if c.privateKey == nil {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return fmt.Errorf("failed to generate account key: %w", err)
		}
		c.privateKey = key
	}

	// Get nonce
	if err := c.fetchNonce(); err != nil {
		return fmt.Errorf("failed to get nonce: %w", err)
	}

	// Create or get account
	if err := c.createOrGetAccount(); err != nil {
		return fmt.Errorf("failed to create account: %w", err)
	}

	return nil
}

// fetchDirectory fetches the ACME directory
func (c *ACMEClient) fetchDirectory() error {
	resp, err := c.httpClient.Get(c.directoryURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var dir ACMEDirectory
	if err := json.NewDecoder(resp.Body).Decode(&dir); err != nil {
		return err
	}

	c.newNonceURL = dir.NewNonce
	c.newAccountURL = dir.NewAccount
	c.newOrderURL = dir.NewOrder

	return nil
}

// fetchNonce gets a fresh nonce
func (c *ACMEClient) fetchNonce() error {
	resp, err := c.httpClient.Head(c.newNonceURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	c.nonce = resp.Header.Get("Replay-Nonce")
	return nil
}

// createOrGetAccount creates a new account or gets existing
func (c *ACMEClient) createOrGetAccount() error {
	payload := map[string]interface{}{
		"termsOfServiceAgreed": true,
		"contact":              []string{"mailto:" + c.email},
	}

	resp, err := c.signedPost(c.newAccountURL, payload)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Store account URL from Location header
	if loc := resp.Header.Get("Location"); loc != "" {
		c.accountURL = loc
	}

	return nil
}

// RequestOrder creates a new certificate order
func (c *ACMEClient) RequestOrder(domains []string) (*ACMEOrder, error) {
	identifiers := make([]Identifier, len(domains))
	for i, d := range domains {
		identifiers[i] = Identifier{Type: "dns", Value: d}
	}

	payload := map[string]interface{}{
		"identifiers": identifiers,
	}

	resp, err := c.signedPost(c.newOrderURL, payload)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var order ACMEOrder
	if err := json.NewDecoder(resp.Body).Decode(&order); err != nil {
		return nil, err
	}

	return &order, nil
}

// GetAuthorization fetches an authorization
func (c *ACMEClient) GetAuthorization(url string) (*ACMEAuthorization, error) {
	resp, err := c.signedGet(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var auth ACMEAuthorization
	if err := json.NewDecoder(resp.Body).Decode(&auth); err != nil {
		return nil, err
	}

	return &auth, nil
}

// GetChallenge fetches a challenge
func (c *ACMEClient) GetChallenge(url string) (*Challenge, error) {
	resp, err := c.signedGet(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var ch Challenge
	if err := json.NewDecoder(resp.Body).Decode(&ch); err != nil {
		return nil, err
	}

	return &ch, nil
}

// TriggerChallenge triggers challenge validation
func (c *ACMEClient) TriggerChallenge(url string) (*Challenge, error) {
	resp, err := c.signedPost(url, map[string]interface{}{})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var ch Challenge
	if err := json.NewDecoder(resp.Body).Decode(&ch); err != nil {
		return nil, err
	}

	// Update nonce
	if nonce := resp.Header.Get("Replay-Nonce"); nonce != "" {
		c.nonce = nonce
	}

	return &ch, nil
}

// FinalizeOrder finalizes the order with CSR
func (c *ACMEClient) FinalizeOrder(order *ACMEOrder, csr []byte) error {
	payload := map[string]interface{}{
		"csr": base64URLEncode(csr),
	}

	resp, err := c.signedPost(order.FinalizeURL, payload)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var finalized ACMEOrder
	if err := json.NewDecoder(resp.Body).Decode(&finalized); err != nil {
		return err
	}

	order.Status = finalized.Status
	order.CertificateURL = finalized.CertificateURL

	return nil
}

// DownloadCertificate downloads the issued certificate
func (c *ACMEClient) DownloadCertificate(url string) ([]byte, error) {
	resp, err := c.signedGet(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}

// PollOrder waits for order to reach desired status
func (c *ACMEClient) PollOrder(orderURL string, desiredStatus string, timeout time.Duration) (*ACMEOrder, error) {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		resp, err := c.signedGet(orderURL)
		if err != nil {
			return nil, err
		}

		var order ACMEOrder
		if err := json.NewDecoder(resp.Body).Decode(&order); err != nil {
			resp.Body.Close()
			return nil, err
		}
		resp.Body.Close()

		if order.Status == desiredStatus {
			return &order, nil
		}

		if order.Status == "invalid" {
			return nil, fmt.Errorf("order is invalid")
		}

		time.Sleep(2 * time.Second)
	}

	return nil, fmt.Errorf("polling timed out")
}

// HTTP request helpers

func (c *ACMEClient) signedGet(url string) (*http.Response, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "DockRouter/1.0")
	return c.httpClient.Do(req)
}

func (c *ACMEClient) signedPost(url string, payload interface{}) (*http.Response, error) {
	// Create JWS
	jws, err := c.signPayload(payload, url)
	if err != nil {
		return nil, err
	}

	body, err := json.Marshal(jws)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/jose+json")
	req.Header.Set("User-Agent", "DockRouter/1.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	// Update nonce from response
	if nonce := resp.Header.Get("Replay-Nonce"); nonce != "" {
		c.nonce = nonce
	}

	return resp, nil
}

// signPayload creates a JWS signature
func (c *ACMEClient) signPayload(payload interface{}, url string) (map[string]interface{}, error) {
	// Encode payload
	var payloadBytes []byte
	var err error
	if payload != nil {
		payloadBytes, err = json.Marshal(payload)
		if err != nil {
			return nil, err
		}
	}
	encodedPayload := base64URLEncode(payloadBytes)

	// Create protected header
	protected := map[string]interface{}{
		"alg": "ES256",
		"url": url,
		"nonce": c.nonce,
	}

	if c.accountURL != "" {
		protected["kid"] = c.accountURL
	} else {
		protected["jwk"] = c.jwk()
	}

	protectedBytes, err := json.Marshal(protected)
	if err != nil {
		return nil, err
	}
	encodedProtected := base64URLEncode(protectedBytes)

	// Sign
	signingInput := encodedProtected + "." + encodedPayload
	hash := sha256.Sum256([]byte(signingInput))
	signature, err := c.privateKey.Sign(rand.Reader, hash[:], crypto.SHA256)
	if err != nil {
		return nil, err
	}

	// Convert DER to R,S
	rs, err := parseECDSASignature(signature)
	if err != nil {
		return nil, err
	}

	// Build JWS
	return map[string]interface{}{
		"protected": encodedProtected,
		"payload":   encodedPayload,
		"signature": base64URLEncode(rs),
	}, nil
}

// jwk returns the JWK for the account key
func (c *ACMEClient) jwk() map[string]interface{} {
	return map[string]interface{}{
		"crv": "P-256",
		"kty": "EC",
		"x":   base64URLEncode(c.privateKey.PublicKey.X.Bytes()),
		"y":   base64URLEncode(c.privateKey.PublicKey.Y.Bytes()),
	}
}

// base64URLEncode encodes bytes using URL-safe base64 without padding
func base64URLEncode(data []byte) string {
	return strings.TrimRight(base64.URLEncoding.EncodeToString(data), "=")
}

// parseECDSASignature parses DER-encoded ECDSA signature to R||S
func parseECDSASignature(der []byte) ([]byte, error) {
	// Simple DER parser for ECDSA signature
	// Format: 0x30 <len> 0x02 <r_len> <r> 0x02 <s_len> <s>
	if len(der) < 8 || der[0] != 0x30 {
		return nil, fmt.Errorf("invalid DER signature")
	}

	// Find R
	rLen := int(der[3])
	r := der[4 : 4+rLen]

	// Find S
	sStart := 4 + rLen
	sLen := int(der[sStart+1])
	s := der[sStart+2 : sStart+2+sLen]

	// Pad to 32 bytes
	rPadded := padBytes(r, 32)
	sPadded := padBytes(s, 32)

	return append(rPadded, sPadded...), nil
}

func padBytes(b []byte, size int) []byte {
	if len(b) >= size {
		return b[len(b)-size:]
	}
	padded := make([]byte, size)
	copy(padded[size-len(b):], b)
	return padded
}
