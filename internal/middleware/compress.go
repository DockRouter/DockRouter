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

		gw := gzip.NewWriter(w)
		defer gw.Close()

		w.Header().Set("Content-Encoding", "gzip")
		wrapped := &gzipResponseWriter{ResponseWriter: w, writer: gw}
		next.ServeHTTP(wrapped, r)
	})
}

type gzipResponseWriter struct {
	http.ResponseWriter
	writer *gzip.Writer
}

func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	return w.writer.Write(b)
}
