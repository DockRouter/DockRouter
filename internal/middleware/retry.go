// Package middleware provides HTTP middleware components
package middleware

import "net/http"

// Retry retries failed requests to alternative backends
// Note: The actual retry logic is implemented in the router package (router.go)
// which has direct access to the backend pool for trying alternative backends.
// This middleware is a placeholder for potential future use.
func Retry(maxRetries int) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r)
		})
	}
}
