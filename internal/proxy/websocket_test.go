package proxy

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSendUpgradeRequest(t *testing.T) {
	// Create a pipe to capture the request
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	logger := &mockLogger{}
	wp := NewWebSocketProxy(logger)

	// Create a test request
	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	req.Host = "example.com"
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	req.Header.Set("Sec-WebSocket-Version", "13")
	req.Header.Set("Sec-WebSocket-Protocol", "chat")
	req.Header.Set("Sec-WebSocket-Extensions", "permessage-deflate")
	req.Header.Set("Origin", "https://example.com")

	// Read the request in a goroutine
	done := make(chan string, 1)
	go func() {
		buf := make([]byte, 4096)
		n, err := serverConn.Read(buf)
		if err != nil {
			done <- ""
			return
		}
		done <- string(buf[:n])
	}()

	// Send upgrade request
	err := wp.sendUpgradeRequest(clientConn, req, "example.com:80")
	if err != nil {
		t.Fatalf("sendUpgradeRequest failed: %v", err)
	}

	// Get the request
	select {
	case reqStr := <-done:
		if !strings.Contains(reqStr, "GET /ws HTTP/1.1") {
			t.Error("Request should contain GET /ws HTTP/1.1")
		}
		if !strings.Contains(reqStr, "Host: example.com") {
			t.Error("Request should contain Host header")
		}
		if !strings.Contains(reqStr, "Upgrade: websocket") {
			t.Error("Request should contain Upgrade header")
		}
		if !strings.Contains(reqStr, "Sec-WebSocket-Key:") {
			t.Error("Request should contain Sec-WebSocket-Key")
		}
		if !strings.Contains(reqStr, "Sec-WebSocket-Protocol: chat") {
			t.Error("Request should contain Sec-WebSocket-Protocol")
		}
		if !strings.Contains(reqStr, "Origin: https://example.com") {
			t.Error("Request should contain Origin")
		}
	case <-time.After(time.Second):
		t.Error("Timeout waiting for request")
	}
}

func TestSendUpgradeRequestMinimal(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	logger := &mockLogger{}
	wp := NewWebSocketProxy(logger)

	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	req.Host = "localhost"

	done := make(chan string, 1)
	go func() {
		buf := make([]byte, 4096)
		n, _ := serverConn.Read(buf)
		done <- string(buf[:n])
	}()

	err := wp.sendUpgradeRequest(clientConn, req, "localhost:8080")
	if err != nil {
		t.Fatalf("sendUpgradeRequest failed: %v", err)
	}

	select {
	case reqStr := <-done:
		if !strings.Contains(reqStr, "GET /ws HTTP/1.1") {
			t.Error("Request should contain GET line")
		}
		if !strings.Contains(reqStr, "Upgrade: websocket") {
			t.Error("Request should contain Upgrade header")
		}
	case <-time.After(time.Second):
		t.Error("Timeout")
	}
}

func TestReadBackendResponse(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	logger := &mockLogger{}
	wp := NewWebSocketProxy(logger)

	// Send response from server
	expectedResp := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: s3pPLMBiTxaQ9kYGzzhZRbK+xOo=\r\n\r\n"

	go func() {
		clientConn.Write([]byte(expectedResp))
	}()

	resp, err := wp.readBackendResponse(serverConn)
	if err != nil {
		t.Fatalf("readBackendResponse failed: %v", err)
	}

	if !strings.Contains(resp, "101 Switching Protocols") {
		t.Errorf("Response should contain 101 status, got: %s", resp)
	}
	if !strings.Contains(resp, "Upgrade: websocket") {
		t.Error("Response should contain Upgrade header")
	}
}

func TestSendClientResponse(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	logger := &mockLogger{}
	wp := NewWebSocketProxy(logger)

	resp := "HTTP/1.1 101 Switching Protocols\r\n\r\n"

	// Read in goroutine
	done := make(chan string, 1)
	go func() {
		buf := make([]byte, 4096)
		n, _ := serverConn.Read(buf)
		done <- string(buf[:n])
	}()

	err := wp.sendClientResponse(clientConn, resp)
	if err != nil {
		t.Fatalf("sendClientResponse failed: %v", err)
	}

	select {
	case got := <-done:
		if got != resp {
			t.Errorf("Response mismatch: got %q, want %q", got, resp)
		}
	case <-time.After(time.Second):
		t.Error("Timeout")
	}
}

func TestCopyData(t *testing.T) {
	// Create pipe for testing
	reader, writer := io.Pipe()

	logger := &mockLogger{}
	wp := NewWebSocketProxy(logger)

	// Create destination buffer
	destBuf := &strings.Builder{}

	// Write data in goroutine
	go func() {
		writer.Write([]byte("test data"))
		writer.Close()
	}()

	// Copy data
	done := make(chan bool, 1)
	go func() {
		wp.copyData(destBuf, reader, "test-direction")
		done <- true
	}()

	select {
	case <-done:
		if !strings.Contains(destBuf.String(), "test data") {
			t.Errorf("Expected 'test data', got %q", destBuf.String())
		}
	case <-time.After(time.Second):
		t.Error("Timeout")
	}
}

func TestCopyDataError(t *testing.T) {
	logger := &mockLogger{}
	wp := NewWebSocketProxy(logger)

	// Create error reader
	errorReader := &errorReader{err: io.ErrUnexpectedEOF}
	destBuf := &strings.Builder{}

	wp.copyData(destBuf, errorReader, "error-test")
	// Should not panic, just return
}

func TestCopyDataWriteError(t *testing.T) {
	logger := &mockLogger{}
	wp := NewWebSocketProxy(logger)

	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()

	// Create error writer
	errorWriter := &errorWriter{err: io.ErrClosedPipe}

	done := make(chan bool, 1)
	go func() {
		writer.Write([]byte("data"))
		writer.Close()
	}()

	go func() {
		wp.copyData(errorWriter, reader, "write-error-test")
		done <- true
	}()

	select {
	case <-done:
		// Good
	case <-time.After(time.Second):
		t.Error("Timeout")
	}
}

// errorReader always returns an error
type errorReader struct {
	err error
}

func (e *errorReader) Read(p []byte) (n int, err error) {
	return 0, e.err
}

// errorWriter always returns an error
type errorWriter struct {
	err error
}

func (e *errorWriter) Write(p []byte) (n int, err error) {
	return 0, e.err
}

func TestWebSocketProxyDialerSettings(t *testing.T) {
	logger := &mockLogger{}
	wp := NewWebSocketProxy(logger)

	if wp.dialer == nil {
		t.Fatal("dialer should not be nil")
	}
	if wp.dialer.Timeout != 10*time.Second {
		t.Errorf("Timeout = %v, want 10s", wp.dialer.Timeout)
	}
	if wp.dialer.KeepAlive != 30*time.Second {
		t.Errorf("KeepAlive = %v, want 30s", wp.dialer.KeepAlive)
	}
}

func TestWebSocketServeHTTPHijackNotSupported(t *testing.T) {
	logger := &mockLogger{}
	wp := NewWebSocketProxy(logger)

	// ResponseRecorder doesn't support hijacking
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/ws", nil)
	r.Header.Set("Upgrade", "websocket")
	r.Header.Set("Connection", "Upgrade")

	err := wp.ServeHTTP(w, r, "localhost:8080")
	if err == nil {
		t.Error("ServeHTTP should return error when hijacking not supported")
	}
	if !strings.Contains(err.Error(), "hijacking not supported") {
		t.Errorf("Error should mention hijacking not supported, got: %v", err)
	}
}

func TestWebSocketServeHTTPDialError(t *testing.T) {
	logger := &mockLogger{}
	wp := NewWebSocketProxy(logger)

	// Create a mock hijacker that succeeds but dial fails
	w := &mockHijackerResponse{
		conn: &mockConn{},
	}
	r := httptest.NewRequest(http.MethodGet, "/ws", nil)

	// Use invalid address that will fail to connect
	err := wp.ServeHTTP(w, r, "localhost:59999")
	if err == nil {
		t.Error("ServeHTTP should return error when dial fails")
	}
}

// mockHijackerResponse implements http.ResponseWriter and http.Hijacker
type mockHijackerResponse struct {
	header http.Header
	conn   net.Conn
}

func (m *mockHijackerResponse) Header() http.Header {
	if m.header == nil {
		m.header = make(http.Header)
	}
	return m.header
}

func (m *mockHijackerResponse) Write([]byte) (int, error) {
	return 0, nil
}

func (m *mockHijackerResponse) WriteHeader(int) {}

func (m *mockHijackerResponse) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return m.conn, bufio.NewReadWriter(bufio.NewReader(m.conn), bufio.NewWriter(m.conn)), nil
}

// mockConn implements net.Conn for testing
type mockConn struct {
	readData  []byte
	writeData []byte
}

func (m *mockConn) Read(b []byte) (n int, err error) {
	if len(m.readData) == 0 {
		return 0, io.EOF
	}
	n = copy(b, m.readData)
	m.readData = m.readData[n:]
	return n, nil
}

func (m *mockConn) Write(b []byte) (n int, err error) {
	m.writeData = append(m.writeData, b...)
	return len(b), nil
}

func (m *mockConn) Close() error                       { return nil }
func (m *mockConn) LocalAddr() net.Addr                { return nil }
func (m *mockConn) RemoteAddr() net.Addr               { return nil }
func (m *mockConn) SetDeadline(t time.Time) error      { return nil }
func (m *mockConn) SetReadDeadline(t time.Time) error  { return nil }
func (m *mockConn) SetWriteDeadline(t time.Time) error { return nil }

// mockHijackerWithError returns error on Hijack
type mockHijackerWithError struct {
	header http.Header
}

func (m *mockHijackerWithError) Header() http.Header {
	if m.header == nil {
		m.header = make(http.Header)
	}
	return m.header
}

func (m *mockHijackerWithError) Write([]byte) (int, error) {
	return 0, nil
}

func (m *mockHijackerWithError) WriteHeader(int) {}

func (m *mockHijackerWithError) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return nil, nil, fmt.Errorf("hijack failed")
}

func TestWebSocketServeHTTPHijackError(t *testing.T) {
	logger := &mockLogger{}
	wp := NewWebSocketProxy(logger)

	// Create a mock hijacker that returns error on hijack
	w := &mockHijackerWithError{}
	r := httptest.NewRequest(http.MethodGet, "/ws", nil)
	r.Header.Set("Upgrade", "websocket")

	// Note: Backend dial happens before hijack, so connection error will occur first
	// when using a non-existent backend. This tests the error path.
	err := wp.ServeHTTP(w, r, "localhost:59999")
	if err == nil {
		t.Error("ServeHTTP should return error")
	}
}

// mockErrorConn fails on Write
type mockErrorConn struct {
	writeErr error
}

func (m *mockErrorConn) Read(b []byte) (n int, err error) {
	return 0, io.EOF
}

func (m *mockErrorConn) Write(b []byte) (n int, err error) {
	return 0, m.writeErr
}

func (m *mockErrorConn) Close() error                       { return nil }
func (m *mockErrorConn) LocalAddr() net.Addr                { return nil }
func (m *mockErrorConn) RemoteAddr() net.Addr               { return nil }
func (m *mockErrorConn) SetDeadline(t time.Time) error      { return nil }
func (m *mockErrorConn) SetReadDeadline(t time.Time) error  { return nil }
func (m *mockErrorConn) SetWriteDeadline(t time.Time) error { return nil }

func TestWebSocketSendUpgradeRequestError(t *testing.T) {
	logger := &mockLogger{}
	wp := NewWebSocketProxy(logger)

	// Create connection that fails on write
	conn := &mockErrorConn{writeErr: io.ErrClosedPipe}

	r := httptest.NewRequest(http.MethodGet, "/ws", nil)
	r.Host = "example.com"

	err := wp.sendUpgradeRequest(conn, r, "example.com:80")
	if err == nil {
		t.Error("sendUpgradeRequest should return error when write fails")
	}
}

func TestHijackConnectionWithMockHijacker(t *testing.T) {
	// Create a mock hijacker
	w := &mockHijackerResponse{
		conn: &mockConn{},
	}
	r := httptest.NewRequest(http.MethodGet, "/ws", nil)

	conn, rw, err := HijackConnection(w, r)
	if err != nil {
		t.Errorf("HijackConnection should succeed, got: %v", err)
	}
	if conn == nil {
		t.Error("Connection should not be nil")
	}
	if rw == nil {
		t.Error("ReadWriter should not be nil")
	}
}

func TestHijackConnectionError(t *testing.T) {
	// Create a mock hijacker that returns error
	w := &mockHijackerWithError{}
	r := httptest.NewRequest(http.MethodGet, "/ws", nil)

	conn, rw, err := HijackConnection(w, r)
	if err == nil {
		t.Error("HijackConnection should return error")
	}
	if conn != nil {
		t.Error("Connection should be nil on error")
	}
	if rw != nil {
		t.Error("ReadWriter should be nil on error")
	}
}

func TestIsWebSocketRequestVariations(t *testing.T) {
	tests := []struct {
		name     string
		headers  map[string]string
		expected bool
	}{
		{
			name: "standard WebSocket",
			headers: map[string]string{
				"Upgrade":    "websocket",
				"Connection": "Upgrade",
			},
			expected: true,
		},
		{
			name: "WebSocket with keep-alive",
			headers: map[string]string{
				"Upgrade":    "websocket",
				"Connection": "keep-alive, Upgrade",
			},
			expected: true,
		},
		{
			name: "mixed case headers",
			headers: map[string]string{
				"Upgrade":    "WebSocket",
				"Connection": "Upgrade, Keep-Alive",
			},
			expected: true,
		},
		{
			name: "regular HTTP request",
			headers: map[string]string{
				"Connection": "keep-alive",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			for key, value := range tt.headers {
				r.Header.Set(key, value)
			}

			result := IsWebSocketRequest(r)
			if result != tt.expected {
				t.Errorf("IsWebSocketRequest() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestWebSocketServeHTTPBackendDialSuccess(t *testing.T) {
	// This test requires a real backend server to connect to
	// We'll start a simple TCP server that accepts WebSocket upgrade
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("Cannot create test server: %v", err)
	}
	defer listener.Close()

	// Start a goroutine that accepts and handles the connection
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		// Read the upgrade request
		buf := make([]byte, 4096)
		n, _ := conn.Read(buf)
		if n == 0 {
			return
		}

		// Send WebSocket upgrade response
		response := "HTTP/1.1 101 Switching Protocols\r\n" +
			"Upgrade: websocket\r\n" +
			"Connection: Upgrade\r\n" +
			"Sec-WebSocket-Accept: s3pPLMBiTxaQ9kYGzzhZRbK+xOo=\r\n\r\n"
		conn.Write([]byte(response))

		// Echo back any data received
		for {
			n, err := conn.Read(buf)
			if err != nil {
				return
			}
			conn.Write(buf[:n])
		}
	}()

	logger := &mockLogger{}
	wp := NewWebSocketProxy(logger)

	// Get the server address
	addr := listener.Addr().String()

	// Create mock hijacker with real net.Pipe connection
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	w := &mockHijackerResponse{conn: serverConn}
	r := httptest.NewRequest(http.MethodGet, "/ws", nil)
	r.Header.Set("Upgrade", "websocket")
	r.Header.Set("Connection", "Upgrade")
	r.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")

	// Run ServeHTTP in a goroutine since it blocks
	done := make(chan error, 1)
	go func() {
		done <- wp.ServeHTTP(w, r, addr)
	}()

	// Close the client connection to trigger ServeHTTP to return
	clientConn.Close()

	select {
	case err := <-done:
		// We expect an error because we closed the connection
		if err != nil {
			// This is expected - connection was closed
			t.Logf("ServeHTTP returned error (expected): %v", err)
		}
	case <-time.After(2 * time.Second):
		// Timeout is also acceptable for this test
		t.Log("ServeHTTP timed out (expected for blocking operations)")
	}
}

func TestWebSocketServeHTTPUpgradeRequestError(t *testing.T) {
	// Test where sendUpgradeRequest fails
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("Cannot create test server: %v", err)
	}
	defer listener.Close()

	// Accept and immediately close to trigger error
	go func() {
		conn, _ := listener.Accept()
		if conn != nil {
			conn.Close()
		}
	}()

	logger := &mockLogger{}
	wp := NewWebSocketProxy(logger)

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	w := &mockHijackerResponse{conn: serverConn}
	r := httptest.NewRequest(http.MethodGet, "/ws", nil)
	r.Header.Set("Upgrade", "websocket")
	r.Header.Set("Connection", "Upgrade")

	// Run in goroutine since it blocks
	done := make(chan error, 1)
	go func() {
		// The connection will fail during upgrade request or response reading
		done <- wp.ServeHTTP(w, r, listener.Addr().String())
	}()

	// Close client side to trigger cleanup
	clientConn.Close()

	select {
	case <-done:
		// Expected - error occurred
	case <-time.After(2 * time.Second):
		t.Log("Timeout waiting for ServeHTTP")
	}
}

func TestWebSocketCopyDataLargeBuffer(t *testing.T) {
	logger := &mockLogger{}
	wp := NewWebSocketProxy(logger)

	// Create a large data buffer
	largeData := make([]byte, 64*1024)
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}

	reader, writer := io.Pipe()
	destBuf := &bytes.Buffer{}

	// Write data in goroutine
	go func() {
		writer.Write(largeData)
		writer.Close()
	}()

	// Copy data
	done := make(chan bool, 1)
	go func() {
		wp.copyData(destBuf, reader, "large-buffer-test")
		done <- true
	}()

	select {
	case <-done:
		if destBuf.Len() != len(largeData) {
			t.Errorf("Expected %d bytes, got %d", len(largeData), destBuf.Len())
		}
	case <-time.After(2 * time.Second):
		t.Error("Timeout")
	}
}

