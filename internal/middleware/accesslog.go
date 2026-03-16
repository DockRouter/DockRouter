// Package middleware provides HTTP middleware components
package middleware

import (
	"net/http"
	"time"
)

// AccessLog logs request details
func AccessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap ResponseWriter to capture status
		wrapped := &responseWriter{ResponseWriter: w}

		next.ServeHTTP(wrapped, r)

		duration := time.Since(start)

		// TODO: Use structured logger
		// TODO: Log to internal/log package
		_ = duration
		_ = wrapped.status
	})
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (w *responseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
