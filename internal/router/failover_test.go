package router

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// hijackableWriter is a ResponseWriter that supports Hijack and Flush.
type hijackableWriter struct {
	*httptest.ResponseRecorder
	hijacked  bool
	flushed   bool
	hijackErr error
}

func (h *hijackableWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h.hijackErr != nil {
		return nil, nil, h.hijackErr
	}
	h.hijacked = true
	return nil, nil, nil
}

func (h *hijackableWriter) Flush() { h.flushed = true }

func TestFailoverWriterCommitTracking(t *testing.T) {
	t.Run("uncommitted before any write", func(t *testing.T) {
		fw := &failoverWriter{ResponseWriter: httptest.NewRecorder()}
		if fw.committed {
			t.Error("writer should start uncommitted")
		}
	})

	t.Run("WriteHeader commits", func(t *testing.T) {
		fw := &failoverWriter{ResponseWriter: httptest.NewRecorder()}
		fw.WriteHeader(http.StatusTeapot)
		if !fw.committed {
			t.Error("WriteHeader should commit the response")
		}
	})

	t.Run("Write commits and passes bytes through", func(t *testing.T) {
		rec := httptest.NewRecorder()
		fw := &failoverWriter{ResponseWriter: rec}
		if _, err := fw.Write([]byte("hello")); err != nil {
			t.Fatalf("Write: %v", err)
		}
		if !fw.committed {
			t.Error("Write should commit the response")
		}
		if rec.Body.String() != "hello" {
			t.Errorf("body = %q, want %q", rec.Body.String(), "hello")
		}
	})
}

func TestFailoverWriterForwardsOptionalInterfaces(t *testing.T) {
	t.Run("successful hijack commits", func(t *testing.T) {
		inner := &hijackableWriter{ResponseRecorder: httptest.NewRecorder()}
		fw := &failoverWriter{ResponseWriter: inner}

		if _, _, err := fw.Hijack(); err != nil {
			t.Fatalf("Hijack: %v", err)
		}
		if !inner.hijacked {
			t.Error("hijack was not forwarded to the underlying writer")
		}
		if !fw.committed {
			t.Error("a successful hijack must commit the response")
		}
	})

	t.Run("failed hijack does not commit", func(t *testing.T) {
		inner := &hijackableWriter{ResponseRecorder: httptest.NewRecorder(), hijackErr: io.ErrUnexpectedEOF}
		fw := &failoverWriter{ResponseWriter: inner}

		if _, _, err := fw.Hijack(); err == nil {
			t.Fatal("expected an error")
		}
		if fw.committed {
			t.Error("a failed hijack must leave the response uncommitted so failover stays possible")
		}
	})

	t.Run("hijack unsupported by underlying writer", func(t *testing.T) {
		fw := &failoverWriter{ResponseWriter: httptest.NewRecorder()}
		if _, _, err := fw.Hijack(); err == nil {
			t.Error("expected ErrNotSupported for a non-hijackable writer")
		}
	})

	t.Run("flush is forwarded", func(t *testing.T) {
		inner := &hijackableWriter{ResponseRecorder: httptest.NewRecorder()}
		fw := &failoverWriter{ResponseWriter: inner}
		fw.Flush()
		if !inner.flushed {
			t.Error("flush was not forwarded")
		}
	})

	t.Run("flush on non-flusher is a no-op", func(t *testing.T) {
		fw := &failoverWriter{ResponseWriter: nopWriter{}}
		fw.Flush() // must not panic
	})

	t.Run("Unwrap exposes the underlying writer", func(t *testing.T) {
		rec := httptest.NewRecorder()
		fw := &failoverWriter{ResponseWriter: rec}
		if fw.Unwrap() != http.ResponseWriter(rec) {
			t.Error("Unwrap should return the wrapped writer")
		}
	})
}

// nopWriter implements only http.ResponseWriter.
type nopWriter struct{}

func (nopWriter) Header() http.Header         { return http.Header{} }
func (nopWriter) Write(b []byte) (int, error) { return len(b), nil }
func (nopWriter) WriteHeader(int)             {}

func TestMakeReplayable(t *testing.T) {
	t.Run("nil body is replayable", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.Body = nil
		if _, ok := makeReplayable(req); !ok {
			t.Error("a request without a body should be replayable")
		}
	})

	t.Run("http.NoBody is replayable", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.Body = http.NoBody
		if _, ok := makeReplayable(req); !ok {
			t.Error("http.NoBody should be replayable")
		}
	})

	t.Run("existing GetBody is left alone", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/", strings.NewReader("abc"))
		called := false
		req.GetBody = func() (io.ReadCloser, error) {
			called = true
			return io.NopCloser(strings.NewReader("abc")), nil
		}
		got, ok := makeReplayable(req)
		if !ok {
			t.Fatal("should be replayable")
		}
		if got.GetBody == nil {
			t.Fatal("GetBody was dropped")
		}
		if _, err := got.GetBody(); err != nil || !called {
			t.Error("the original GetBody should have been preserved")
		}
	})

	t.Run("small body is buffered and rewindable", func(t *testing.T) {
		const payload = "small-payload"
		req := httptest.NewRequest("POST", "/", strings.NewReader(payload))
		req.GetBody = nil

		got, ok := makeReplayable(req)
		if !ok {
			t.Fatal("a small body should be replayable")
		}

		first, _ := io.ReadAll(got.Body)
		if string(first) != payload {
			t.Fatalf("first read = %q, want %q", first, payload)
		}

		rewound, err := got.GetBody()
		if err != nil {
			t.Fatalf("GetBody: %v", err)
		}
		second, _ := io.ReadAll(rewound)
		if string(second) != payload {
			t.Errorf("replayed body = %q, want %q", second, payload)
		}
	})

	t.Run("oversized body is not replayable but stays intact", func(t *testing.T) {
		payload := strings.Repeat("x", MaxReplayBodySize+1024)
		req := httptest.NewRequest("POST", "/", strings.NewReader(payload))
		req.GetBody = nil

		got, ok := makeReplayable(req)
		if ok {
			t.Error("a body over the limit must not be reported as replayable")
		}

		// Nothing may be lost: the buffered prefix is stitched back on.
		all, err := io.ReadAll(got.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if len(all) != len(payload) {
			t.Errorf("body length = %d, want %d", len(all), len(payload))
		}
		if string(all) != payload {
			t.Error("body content was corrupted")
		}
	})
}
