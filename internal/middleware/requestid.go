// Package middleware provides HTTP middleware components
package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
)

// RequestIDHeader is the header name for request IDs
const RequestIDHeader = "X-Request-Id"

// RequestID generates a unique request ID and adds it to headers
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Use existing ID if present
		id := r.Header.Get(RequestIDHeader)
		if id == "" {
			id = generateID()
			r.Header.Set(RequestIDHeader, id)
		}
		w.Header().Set(RequestIDHeader, id)
		next.ServeHTTP(w, r)
	})
}

func generateID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}
