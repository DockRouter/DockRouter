// Package middleware provides HTTP middleware components
package middleware

import (
	"fmt"
	"net/http"
	"runtime"
)

// recoveryResponseWriter wraps ResponseWriter to track whether headers were sent
type recoveryResponseWriter struct {
	http.ResponseWriter
	headersSent bool
}

func (w *recoveryResponseWriter) WriteHeader(code int) {
	w.headersSent = true
	w.ResponseWriter.WriteHeader(code)
}

func (w *recoveryResponseWriter) Write(b []byte) (int, error) {
	w.headersSent = true
	return w.ResponseWriter.Write(b)
}

// Recovery recovers from panics and returns a 500 error
func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rw := &recoveryResponseWriter{ResponseWriter: w}
		defer func() {
			if err := recover(); err != nil {
				buf := make([]byte, 4096)
				n := runtime.Stack(buf, false)
				stack := string(buf[:n])

				// Basic panic logging
				fmt.Printf("PANIC: %v\n%s\n", err, stack)
				if !rw.headersSent {
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				}
			}
		}()
		next.ServeHTTP(rw, r)
	})
}

// RecoveryWithLogger creates a recovery middleware with a structured logger
func RecoveryWithLogger(logger Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rw := &recoveryResponseWriter{ResponseWriter: w}
			defer func() {
				if err := recover(); err != nil {
					buf := make([]byte, 4096)
					n := runtime.Stack(buf, false)
					stack := string(buf[:n])

					logger.Info("recovered from panic",
						"error", fmt.Sprintf("%v", err),
						"stack", stack,
						"path", r.URL.Path,
						"method", r.Method,
					)
					if !rw.headersSent {
						http.Error(w, "Internal Server Error", http.StatusInternalServerError)
					}
				}
			}()
			next.ServeHTTP(rw, r)
		})
	}
}
