// Package router handles HTTP routing
package router

import (
	"bufio"
	"bytes"
	"io"
	"net"
	"net/http"
)

// MaxReplayBodySize bounds how much of a request body is buffered in order to
// make a retry possible. Requests larger than this are streamed to a single
// backend instead of being held in memory, so large uploads stay cheap.
const MaxReplayBodySize = 1 << 20 // 1MB

// makeReplayable buffers a bounded request body so that a failed attempt can be
// replayed against another backend. Server requests never carry GetBody, so
// without this a POST would reach the second backend with an empty body.
//
// It reports whether the request can be retried. When the body is larger than
// MaxReplayBodySize the already-read prefix is stitched back in front of the
// remaining stream and the request is sent to a single backend only.
func makeReplayable(req *http.Request) (*http.Request, bool) {
	if req.Body == nil || req.Body == http.NoBody {
		return req, true
	}
	if req.GetBody != nil {
		return req, true
	}

	buf, err := io.ReadAll(io.LimitReader(req.Body, MaxReplayBodySize+1))
	if err != nil {
		return req, false
	}

	if len(buf) > MaxReplayBodySize {
		// Too large to replay; put the prefix back so nothing is lost.
		rest := req.Body
		req.Body = struct {
			io.Reader
			io.Closer
		}{io.MultiReader(bytes.NewReader(buf), rest), rest}
		return req, false
	}

	req.Body = io.NopCloser(bytes.NewReader(buf))
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(buf)), nil
	}
	return req, true
}

// failoverWriter wraps an http.ResponseWriter and records whether any part of
// the response has already reached the client.
//
// Responses are written straight through, so streaming responses (SSE, chunked
// bodies, large downloads) are forwarded incrementally instead of being buffered
// in memory. The trade-off is that once the first byte is committed the request
// can no longer be retried against a different backend — `committed` tracks
// exactly that boundary.
//
// Flush, Hijack and ReadFrom are forwarded to the underlying writer so that
// protocol upgrades (WebSocket) and explicit flushes keep working.
type failoverWriter struct {
	http.ResponseWriter
	committed bool
}

// WriteHeader commits the response status.
func (fw *failoverWriter) WriteHeader(code int) {
	fw.committed = true
	fw.ResponseWriter.WriteHeader(code)
}

// Write commits the response and forwards the bytes immediately.
func (fw *failoverWriter) Write(b []byte) (int, error) {
	fw.committed = true
	return fw.ResponseWriter.Write(b)
}

// Flush forwards a flush to the underlying writer when supported.
func (fw *failoverWriter) Flush() {
	if f, ok := fw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack takes over the connection, which is what WebSocket upgrades need.
// A successful hijack commits the response: the connection now belongs to the
// caller and no failover is possible.
func (fw *failoverWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := fw.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}
	conn, buf, err := h.Hijack()
	if err == nil {
		fw.committed = true
	}
	return conn, buf, err
}

// Unwrap exposes the underlying writer for http.ResponseController.
func (fw *failoverWriter) Unwrap() http.ResponseWriter { return fw.ResponseWriter }
