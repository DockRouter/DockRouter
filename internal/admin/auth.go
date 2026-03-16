// Package admin provides the admin API and dashboard
package admin

import (
	"crypto/subtle"
	"net/http"
)

// Auth handles admin authentication
type Auth struct {
	username string
	password string
}

// NewAuth creates a new auth handler
func NewAuth(username, password string) *Auth {
	return &Auth{
		username: username,
		password: password,
	}
}

// Middleware returns auth middleware
func (a *Auth) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip auth if not configured
		if a.username == "" {
			next.ServeHTTP(w, r)
			return
		}

		user, pass, ok := r.BasicAuth()
		if !ok {
			unauthorized(w)
			return
		}

		if subtle.ConstantTimeCompare([]byte(user), []byte(a.username)) != 1 ||
			subtle.ConstantTimeCompare([]byte(pass), []byte(a.password)) != 1 {
			unauthorized(w)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Basic realm="DockRouter Admin"`)
	http.Error(w, "Unauthorized", http.StatusUnauthorized)
}
