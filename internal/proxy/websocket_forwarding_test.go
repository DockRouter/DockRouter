package proxy

import (
	"bufio"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// readUpgradeRequest runs sendUpgradeRequest against an in-memory pipe and
// returns what the backend would have received.
func readUpgradeRequest(t *testing.T, r *http.Request) string {
	t.Helper()

	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	got := make(chan string, 1)
	go func() {
		buf := make([]byte, 8192)
		n, err := serverConn.Read(buf)
		if err != nil {
			got <- ""
			return
		}
		got <- string(buf[:n])
	}()

	wp := NewWebSocketProxy(&mockLogger{})
	if err := wp.sendUpgradeRequest(clientConn, r, "backend:80"); err != nil {
		t.Fatalf("sendUpgradeRequest failed: %v", err)
	}

	select {
	case s := <-got:
		return s
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for upgrade request")
		return ""
	}
}

// A WebSocket backend that authenticates the socket needs the client's
// credentials. Forwarding only a fixed handshake allowlist dropped them.
func TestSendUpgradeRequestForwardsClientHeaders(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	req.Host = "app.example.com"
	req.RemoteAddr = "203.0.113.7:51234"
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	req.Header.Set("Cookie", "session=abc123")
	req.Header.Set("Authorization", "Bearer token-value")
	req.Header.Set("X-Tenant-Id", "acme")

	out := readUpgradeRequest(t, req)

	for _, want := range []string{
		"Cookie: session=abc123",
		"Authorization: Bearer token-value",
		"X-Tenant-Id: acme",
		"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("upgrade request missing %q\ngot:\n%s", want, out)
		}
	}
}

// The client's own X-Forwarded-For must not survive: it is set from the real
// connection so a client cannot claim someone else's address.
func TestSendUpgradeRequestOverridesForwardedHeaders(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	req.Host = "app.example.com"
	req.RemoteAddr = "203.0.113.7:51234"
	req.Header.Set("X-Forwarded-For", "10.0.0.1")
	req.Header.Set("X-Real-Ip", "10.0.0.1")

	out := readUpgradeRequest(t, req)

	if strings.Contains(out, "10.0.0.1") {
		t.Errorf("spoofed forwarding header was relayed\ngot:\n%s", out)
	}
	if !strings.Contains(out, "X-Forwarded-For: 203.0.113.7") {
		t.Errorf("X-Forwarded-For not set from RemoteAddr\ngot:\n%s", out)
	}
	if !strings.Contains(out, "X-Forwarded-Host: app.example.com") {
		t.Errorf("X-Forwarded-Host missing\ngot:\n%s", out)
	}
	if !strings.Contains(out, "X-Forwarded-Proto: http") {
		t.Errorf("X-Forwarded-Proto missing\ngot:\n%s", out)
	}
}

// Hop-by-hop headers are per-connection and must not be relayed.
func TestSendUpgradeRequestDropsHopByHopHeaders(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	req.Host = "app.example.com"
	req.Header.Set("Keep-Alive", "timeout=5")
	req.Header.Set("Proxy-Authorization", "Basic c2VjcmV0")
	req.Header.Set("Te", "trailers")

	out := readUpgradeRequest(t, req)

	for _, unwanted := range []string{"Keep-Alive:", "Proxy-Authorization:", "Te:"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("hop-by-hop header %q was relayed\ngot:\n%s", unwanted, out)
		}
	}
}

// A header value containing CRLF would otherwise smuggle a second request onto
// the upstream connection.
func TestSendUpgradeRequestRejectsHeaderInjection(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	req.Host = "app.example.com"
	// Bypass Header.Set, which rejects invalid values, to simulate a value that
	// reached us through a less strict path.
	req.Header["X-Evil"] = []string{"ok\r\nGET /admin HTTP/1.1"}

	out := readUpgradeRequest(t, req)

	if strings.Contains(out, "GET /admin") {
		t.Errorf("CRLF injection was relayed\ngot:\n%s", out)
	}
}

// hijackRecorder is a ResponseWriter whose Hijack returns a real connection.
type hijackRecorder struct {
	http.ResponseWriter
	conn net.Conn
}

func (h *hijackRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return h.conn, bufio.NewReadWriter(bufio.NewReader(h.conn), bufio.NewWriter(h.conn)), nil
}

// When one direction of the relay ends the other must not block forever: both
// connections are closed so the copy goroutines and sockets are released.
func TestServeHTTPReleasesBothDirections(t *testing.T) {
	backend, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer backend.Close()

	// Backend accepts, completes the handshake, then goes silent without ever
	// closing its side.
	backendReady := make(chan struct{})
	go func() {
		conn, err := backend.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		br := bufio.NewReader(conn)
		for {
			line, err := br.ReadString('\n')
			if err != nil {
				return
			}
			if strings.TrimSpace(line) == "" {
				break
			}
		}
		conn.Write([]byte("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n"))
		close(backendReady)
		// Block until the proxy closes us.
		io := make([]byte, 1)
		conn.Read(io)
	}()

	clientSide, proxySide := net.Pipe()

	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	req.Host = "app.example.com"
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")

	w := &hijackRecorder{ResponseWriter: httptest.NewRecorder(), conn: proxySide}
	wp := NewWebSocketProxy(&mockLogger{})

	done := make(chan error, 1)
	go func() {
		done <- wp.ServeHTTP(w, req, backend.Addr().String())
	}()

	select {
	case <-backendReady:
	case <-time.After(3 * time.Second):
		t.Fatal("backend never completed the handshake")
	}

	// Drain the 101 the proxy relays, then hang up as a browser tab would.
	buf := make([]byte, 256)
	clientSide.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := clientSide.Read(buf); err != nil {
		t.Fatalf("reading upgrade response: %v", err)
	}
	clientSide.Close()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("ServeHTTP did not return after the client hung up: " +
			"the backend->client copy goroutine is leaking")
	}
}
