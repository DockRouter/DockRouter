// Package middleware provides HTTP middleware components
package middleware

import (
	"net/http"
	"strings"
)

// StripPrefix removes a path prefix before forwarding
func StripPrefix(prefix string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.URL.Path, prefix) {
				http.NotFound(w, r)
				return
			}
			r.URL.Path = r.URL.Path[len(prefix):]
			if r.URL.RawPath != "" {
				if strings.HasPrefix(r.URL.RawPath, prefix) {
					r.URL.RawPath = r.URL.RawPath[len(prefix):]
				}
			}
			if r.URL.Path == "" {
				r.URL.Path = "/"
			}
			next.ServeHTTP(w, r)
		})
	}
}

// AddPrefix adds a path prefix before forwarding
func AddPrefix(prefix string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.URL.Path = prefix + r.URL.Path
			next.ServeHTTP(w, r)
		})
	}
}
