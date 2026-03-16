// Package middleware provides HTTP middleware components
package middleware

import "net/http"

// StripPrefix removes a path prefix before forwarding
func StripPrefix(prefix string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.URL.Path = r.URL.Path[len(prefix):]
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
