// Package middleware provides HTTP middleware components
package middleware

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

// Logger interface for access logging
type Logger interface {
	Info(msg string, fields ...interface{})
	Debug(msg string, fields ...interface{})
}

// AccessLog logs request details (basic version without logger)
func AccessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap ResponseWriter to capture status
		wrapped := newResponseWriter(w)

		next.ServeHTTP(wrapped, r)

		// Simple log output (use AccessLogWithLogger for structured logging)
		duration := time.Since(start)
		// Log to stdout in common log format
		fmt.Printf("[%s] %s %s %d %dms\n",
			time.Now().Format("2006-01-02T15:04:05.999"),
			r.Method,
			sanitizeLogField(r.URL.Path),
			wrapped.status,
			duration.Milliseconds(),
		)
	})
}

// AccessLogWithLogger logs request details with a structured logger
func AccessLogWithLogger(logger Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Wrap ResponseWriter to capture status and size
			wrapped := &accessLogResponseWriter{ResponseWriter: w, status: http.StatusOK}

			next.ServeHTTP(wrapped, r)

			duration := time.Since(start)

			logger.Info("request",
				"method", r.Method,
				"path", sanitizeLogField(r.URL.Path),
				"status", wrapped.status,
				"duration_ms", duration.Milliseconds(),
				"remote_addr", r.RemoteAddr,
				"user_agent", r.UserAgent(),
				"bytes_written", wrapped.bytesWritten,
			)
		})
	}
}

type accessLogResponseWriter struct {
	http.ResponseWriter
	status       int
	bytesWritten int
}

func (w *accessLogResponseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *accessLogResponseWriter) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	w.bytesWritten += n
	return n, err
}

func (w *accessLogResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return hijackThrough(w.ResponseWriter)
}

func (w *accessLogResponseWriter) Flush() { flushThrough(w.ResponseWriter) }

func (w *accessLogResponseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

type responseWriter struct {
	http.ResponseWriter
	status int
}

func newResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{ResponseWriter: w, status: http.StatusOK}
}

func (w *responseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return hijackThrough(w.ResponseWriter)
}

func (w *responseWriter) Flush() { flushThrough(w.ResponseWriter) }

func (w *responseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// sanitizeLogField escapes carriage return and newline characters to prevent
// log injection via malicious URL paths.
func sanitizeLogField(s string) string {
	s = strings.ReplaceAll(s, "\r", "\\r")
	s = strings.ReplaceAll(s, "\n", "\\n")
	return s
}
