// Package middleware provides HTTP middleware components
package middleware

import (
	"compress/gzip"
	"net/http"
	"strings"
)

// Compress provides gzip compression
func Compress(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if client accepts gzip
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}

		// Check if content type is compressible
		// Skip images, videos, etc.

		wrapped := &gzipResponseWriter{ResponseWriter: w}
		next.ServeHTTP(wrapped, r)
		// Close gzip writer if it was initialized
		if wrapped.writer != nil {
			wrapped.writer.Close()
		}
	})
}

type gzipResponseWriter struct {
	http.ResponseWriter
	writer      *gzip.Writer
	wroteHeader bool
}

func (w *gzipResponseWriter) init() {
	if w.writer == nil {
		w.ResponseWriter.Header().Del("Content-Length")
		w.ResponseWriter.Header().Set("Content-Encoding", "gzip")
		w.writer = gzip.NewWriter(w.ResponseWriter)
	}
}

func (w *gzipResponseWriter) WriteHeader(statusCode int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	// Don't compress responses with no body
	if statusCode == http.StatusNoContent || statusCode == http.StatusNotModified {
		w.ResponseWriter.WriteHeader(statusCode)
		return
	}
	w.init()
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	w.init()
	return w.writer.Write(b)
}
