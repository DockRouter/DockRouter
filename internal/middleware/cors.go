// Package middleware provides HTTP middleware components
package middleware

import (
	"net/http"
	"strings"
)

// CORSConfig holds CORS configuration
type CORSConfig struct {
	Origins     []string
	Methods     []string
	Headers     []string
	Credentials bool
}

// CORS handles Cross-Origin Resource Sharing
func CORS(config CORSConfig) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin == "" {
				next.ServeHTTP(w, r)
				return
			}

			// Check if origin is allowed
			allowed := false
			for _, o := range config.Origins {
				if o == "*" || o == origin {
					allowed = true
					break
				}
			}

			// Always set Vary: Origin for proper cache behavior
			w.Header().Set("Vary", "Origin")

			if allowed {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				if config.Credentials {
					w.Header().Set("Access-Control-Allow-Credentials", "true")
				}
				if len(config.Methods) > 0 {
					w.Header().Set("Access-Control-Allow-Methods", strings.Join(config.Methods, ", "))
				}
				if len(config.Headers) > 0 {
					w.Header().Set("Access-Control-Allow-Headers", strings.Join(config.Headers, ", "))
				}
				w.Header().Set("Access-Control-Max-Age", "86400")
			}

			// Handle preflight
			if r.Method == http.MethodOptions {
				if !allowed {
					http.Error(w, "Forbidden", http.StatusForbidden)
					return
				}
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
