// Package tls handles TLS certificate management
package tls

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"net/http"
	"sync"
)

// ChallengeSolver handles HTTP-01 ACME challenges
type ChallengeSolver struct {
	mu     sync.RWMutex
	tokens map[string]string // token -> keyAuthorization

	// Handler path
	pathPrefix string
}

// NewChallengeSolver creates a new challenge solver
func NewChallengeSolver() *ChallengeSolver {
	return &ChallengeSolver{
		tokens:     make(map[string]string),
		pathPrefix: "/.well-known/acme-challenge/",
	}
}

// SetToken stores a challenge token
func (s *ChallengeSolver) SetToken(token, keyAuth string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens[token] = keyAuth
}

// GetToken retrieves a challenge token
func (s *ChallengeSolver) GetToken(token string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	keyAuth, ok := s.tokens[token]
	return keyAuth, ok
}

// RemoveToken removes a challenge token
func (s *ChallengeSolver) RemoveToken(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.tokens, token)
}

// Handler returns an HTTP handler for challenge requests
func (s *ChallengeSolver) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract token from path
		token := r.URL.Path[len(s.pathPrefix):]
		if token == "" {
			http.Error(w, "Missing token", http.StatusBadRequest)
			return
		}

		// Get key authorization
		keyAuth, ok := s.GetToken(token)
		if !ok {
			http.Error(w, "Token not found", http.StatusNotFound)
			return
		}

		// Return key authorization
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write([]byte(keyAuth))
	}
}

// Matches checks if a request path is a challenge request
func (s *ChallengeSolver) Matches(path string) bool {
	return len(path) > len(s.pathPrefix) &&
		path[:len(s.pathPrefix)] == s.pathPrefix
}

// PathPrefix returns the ACME challenge path prefix
func (s *ChallengeSolver) PathPrefix() string {
	return s.pathPrefix
}

// ComputeKeyAuthorization computes the key authorization for a token
func ComputeKeyAuthorization(token string, accountKey *AccountKey) string {
	// Key authorization = token + '.' + base64url(sha256(jwk))
	jwk := accountKey.JWK()
	jwkJSON, _ := encodeJSON(jwk)
	hash := sha256.Sum256(jwkJSON)
	thumbprint := base64URLEncode(hash[:])
	return token + "." + thumbprint
}

// AccountKey represents an ACME account key
type AccountKey struct {
	key *ecdsa.PrivateKey
}

func encodeJSON(v interface{}) ([]byte, error) {
	return jsonMarshal(v)
}

func jsonMarshal(v interface{}) ([]byte, error) {
	// Simple JSON marshal without importing encoding/json
	// This is a placeholder - use encoding/json in real implementation
	buf := make([]byte, 0, 256)
	return buf, nil
}

// JWK returns the JSON Web Key
func (k *AccountKey) JWK() map[string]interface{} {
	if k.key == nil {
		return nil
	}
	return map[string]interface{}{
		"crv": "P-256",
		"kty": "EC",
		"x":   base64URLEncode(k.key.PublicKey.X.Bytes()),
		"y":   base64URLEncode(k.key.PublicKey.Y.Bytes()),
	}
}
