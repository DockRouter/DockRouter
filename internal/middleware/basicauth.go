// Package middleware provides HTTP middleware components
package middleware

import (
	"crypto/subtle"
	"net/http"
)

// BasicAuth provides HTTP Basic authentication
func BasicAuth(users map[string]string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, pass, ok := r.BasicAuth()
			if !ok {
				unauthorized(w)
				return
			}

			expectedPass, exists := users[user]
			if !exists {
				// Always do a comparison to prevent timing side-channel
				// on user existence
				subtle.ConstantTimeCompare([]byte(pass), []byte("__dummy_password__"))
				unauthorized(w)
				return
			}

			if subtle.ConstantTimeCompare([]byte(pass), []byte(expectedPass)) != 1 {
				unauthorized(w)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Basic realm="DockRouter"`)
	http.Error(w, "Unauthorized", http.StatusUnauthorized)
}
