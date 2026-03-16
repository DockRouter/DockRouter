// Package middleware provides HTTP middleware components
package middleware

import "net/http"

// Retry retries failed requests to alternative backends
func Retry(maxRetries int) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// TODO: Implement retry logic with backend pool
			next.ServeHTTP(w, r)
		})
	}
}
