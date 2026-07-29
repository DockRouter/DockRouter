// Package middleware provides HTTP middleware components
package middleware

import (
	"bufio"
	"compress/gzip"
	"net"
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

		// Never compress a protocol upgrade or an event stream: gzip framing
		// would corrupt a WebSocket handshake and stall SSE delivery.
		if isLongLived(r) {
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

func (w *gzipResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return hijackThrough(w.ResponseWriter)
}

func (w *gzipResponseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *gzipResponseWriter) init() {
	if w.writer == nil {
		w.ResponseWriter.Header().Del("Content-Length")
		w.ResponseWriter.Header().Set("Content-Encoding", "gzip")
		w.writer = gzip.NewWriter(w.ResponseWriter)
	}
}

var compressibleTypes = map[string]bool{
	"text/":                             true,
	"application/json":                  true,
	"application/javascript":            true,
	"application/xml":                   true,
	"application/svg":                   true,
	"application/x-www-form-urlencoded": true,
}

func isCompressible(contentType string) bool {
	if contentType == "" {
		return true // default to compressing if unknown
	}
	ct := strings.ToLower(contentType)
	for prefix := range compressibleTypes {
		if strings.HasPrefix(ct, prefix) {
			return true
		}
	}
	return false
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
	// Check if content type is compressible
	if !isCompressible(w.ResponseWriter.Header().Get("Content-Type")) {
		w.ResponseWriter.WriteHeader(statusCode)
		return
	}
	w.init()
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	if w.writer == nil {
		return w.ResponseWriter.Write(b)
	}
	return w.writer.Write(b)
}

func (w *gzipResponseWriter) Flush() {
	if w.writer != nil {
		w.writer.Flush()
	}
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}
