// Package middleware provides HTTP middleware components
package middleware

import (
	"bufio"
	"net"
	"net/http"
)

// Middleware that wraps http.ResponseWriter hides the optional interfaces the
// underlying writer implements. Without forwarding them, a WebSocket upgrade
// cannot hijack the connection and an SSE stream cannot flush, so both break as
// soon as any wrapping middleware is in the chain.
//
// These helpers keep that forwarding to one line per wrapper.

// hijackThrough forwards a hijack request to the underlying writer.
func hijackThrough(w http.ResponseWriter) (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := w.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, http.ErrNotSupported
}

// flushThrough forwards a flush to the underlying writer when it supports one.
func flushThrough(w http.ResponseWriter) {
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}
