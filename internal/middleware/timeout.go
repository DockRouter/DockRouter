package middleware

import (
	"context"
	"net/http"
	"strings"
	"time"
)

// Timeout returns a middleware that limits request duration.
//
// Long-lived responses are exempt: a WebSocket upgrade or a Server-Sent Events
// stream is supposed to outlive any request deadline, and cancelling their
// context would drop the connection mid-stream.
func Timeout(duration time.Duration) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isLongLived(r) {
				next.ServeHTTP(w, r)
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), duration)
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// isLongLived reports whether a request is expected to produce a streaming or
// upgraded response that must not be bounded by a request timeout.
func isLongLived(r *http.Request) bool {
	if strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade") {
		return true
	}
	if r.Header.Get("Upgrade") != "" {
		return true
	}
	// SSE clients advertise the stream they expect back.
	return strings.Contains(r.Header.Get("Accept"), "text/event-stream")
}
