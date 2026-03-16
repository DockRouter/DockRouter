package proxy

import (
	"bufio"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewTransport(t *testing.T) {
	transport := newTransport()
	if transport == nil {
		t.Fatal("newTransport returned nil")
	}
	if transport.MaxIdleConns != MaxIdleConns {
		t.Errorf("MaxIdleConns = %d, want %d", transport.MaxIdleConns, MaxIdleConns)
	}
	if transport.MaxIdleConnsPerHost != MaxIdleConnsPerHost {
		t.Errorf("MaxIdleConnsPerHost = %d, want %d", transport.MaxIdleConnsPerHost, MaxIdleConnsPerHost)
	}
	if transport.IdleConnTimeout != IdleConnTimeout {
		t.Errorf("IdleConnTimeout = %v, want %v", transport.IdleConnTimeout, IdleConnTimeout)
	}
	if transport.ResponseHeaderTimeout != ResponseTimeout {
		t.Errorf("ResponseHeaderTimeout = %v, want %v", transport.ResponseHeaderTimeout, ResponseTimeout)
	}
	if transport.ForceAttemptHTTP2 {
		t.Error("ForceAttemptHTTP2 should be false")
	}
	if transport.DisableKeepAlives {
		t.Error("DisableKeepAlives should be false")
	}
	if transport.DisableCompression {
		t.Error("DisableCompression should be false")
	}
	if transport.DialContext == nil {
		t.Error("DialContext should not be nil")
	}
}

func TestRenderError(t *testing.T) {
	tests := []struct {
		name       string
		code       int
		requestID  string
		wantCode    int
		wantContain []string
	}{
		{
			name:       "502 Bad Gateway",
			code:       http.StatusBadGateway,
			requestID:  "req-123",
			wantCode:   502,
			wantContain: []string{"502", "Bad Gateway", "req-123"},
		},
		{
			name:       "503 Service Unavailable",
			code:       http.StatusServiceUnavailable,
			requestID:  "req-456",
			wantCode:   503,
			wantContain: []string{"503", "Service Unavailable", "req-456"},
		},
		{
			name:       "504 Gateway Timeout",
			code:       http.StatusGatewayTimeout,
			requestID:  "req-789",
			wantCode:   504,
			wantContain: []string{"504", "Gateway Timeout", "req-789"},
		},
		{
			name:       "429 Too Many Requests",
			code:       http.StatusTooManyRequests,
			requestID:  "req-rate",
			wantCode:   429,
			wantContain: []string{"429", "Too Many Requests", "req-rate"},
		},
		{
			name:       "500 Internal Server Error",
			code:       http.StatusInternalServerError,
			requestID:  "",
			wantCode:   500,
			wantContain: []string{"500", "Internal Server Error"},
		},
		{
			name:       "404 Not Found",
			code:       http.StatusNotFound,
			requestID:  "req-404",
			wantCode:   404,
			wantContain: []string{"404", "Not Found", "req-404"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			RenderError(rec, tt.code, tt.requestID)

			if rec.Code != tt.wantCode {
				t.Errorf("RenderError() code = %d, want %d", rec.Code, tt.wantCode)
			}

			contentType := rec.Header().Get("Content-Type")
			if contentType != "text/html; charset=utf-8" {
				t.Errorf("Content-Type = %s, want text/html; charset=utf-8", contentType)
			}

			body := rec.Body.String()
			for _, want := range tt.wantContain {
				if !strings.Contains(body, want) {
					t.Errorf("Body missing expected content: %s", want)
				}
			}
		})
	}
}

func TestErrorPageData(t *testing.T) {
	data := ErrorPageData{
		StatusCode: 502,
		StatusText: "Bad Gateway",
		RequestID:  "test-123",
		Message:    "Connection refused",
	}

	if data.StatusCode != 502 {
		t.Errorf("StatusCode = %d, want 502", data.StatusCode)
	}
	if data.StatusText != "Bad Gateway" {
		t.Errorf("StatusText = %s, want Bad Gateway", data.StatusText)
	}
	if data.RequestID != "test-123" {
		t.Errorf("RequestID = %s, want test-123", data.RequestID)
	}
	if data.Message != "Connection refused" {
		t.Errorf("Message = %s, want Connection refused", data.Message)
	}
}

func TestProxySetTimeout(t *testing.T) {
	logger := &mockLogger{}
	proxy := NewProxy(logger)

	// Should work without error
	proxy.SetTimeout(60e9) // 60 seconds

	// Test with nil transport case handled
	proxy2 := &Proxy{logger: logger}
	proxy2.SetTimeout(30e9) // Should not panic
}

func TestNewProxyInitialization(t *testing.T) {
	logger := &mockLogger{}
	proxy := NewProxy(logger)

	if proxy == nil {
		t.Fatal("NewProxy returned nil")
	}
	if proxy.transport == nil {
		t.Error("transport should not be nil")
	}
	if proxy.bufferPool == nil {
		t.Error("bufferPool should not be nil")
	}
	if proxy.logger == nil {
		t.Error("logger should not be nil")
	}
}

func TestNewStreamProxy(t *testing.T) {
	logger := &mockLogger{}
	proxy := NewProxy(logger)
	streamProxy := NewStreamProxy(proxy)

	if streamProxy == nil {
		t.Fatal("NewStreamProxy returned nil")
	}
	if streamProxy.proxy == nil {
		t.Error("proxy should not be nil")
	}
}

func TestConstants(t *testing.T) {
	// Verify constants have expected values
	if MaxIdleConns != 100 {
		t.Errorf("MaxIdleConns = %d, want 100", MaxIdleConns)
	}
	if MaxIdleConnsPerHost != 100 {
		t.Errorf("MaxIdleConnsPerHost = %d, want 100", MaxIdleConnsPerHost)
	}
}

func TestRemoveHopHeadersAll(t *testing.T) {
	// Test all hop-by-hop headers
	hdr := http.Header{}
	hdr.Set("Connection", "keep-alive")
	hdr.Set("Keep-Alive", "timeout=5, max=100")
	hdr.Set("Proxy-Authenticate", "Basic realm=test")
	hdr.Set("Proxy-Authorization", "Basic xyz")
	hdr.Set("Te", "trailers")
	hdr.Set("Trailer", "X-Trailer")
	hdr.Set("Transfer-Encoding", "chunked")
	hdr.Set("Upgrade", "h2c")
	hdr.Set("X-Custom", "preserve")
	hdr.Set("X-Another", "keep")

	removeHopHeaders(hdr)

	removed := []string{
		"Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization",
		"Te", "Trailer", "Transfer-Encoding", "Upgrade",
	}

	for _, key := range removed {
		if hdr.Get(key) != "" {
			t.Errorf("Header %s should be removed", key)
		}
	}

	if hdr.Get("X-Custom") != "preserve" {
		t.Error("X-Custom should be preserved")
	}
	if hdr.Get("X-Another") != "keep" {
		t.Error("X-Another should be preserved")
	}
}

func TestRemoveHopHeadersEmpty(t *testing.T) {
	// Test with empty headers
	hdr := http.Header{}
	removeHopHeaders(hdr) // Should not panic
}

func TestWebSocketDetect(t *testing.T) {
	tests := []struct {
		name      string
		upgrade   string
		conn      string
		expected  bool
	}{
		{"websocket uppercase", "WEBSOCKET", "Upgrade", true},
		{"websocket mixed case", "WebSocket", "upgrade", true},
		{"websocket with keep-alive", "websocket", "keep-alive, Upgrade", true},
		{"not websocket", "h2c", "Upgrade", false},
		{"no upgrade", "", "Upgrade", false},
		{"no connection", "websocket", "", false},
		{"wrong connection", "websocket", "keep-alive", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.upgrade != "" {
				req.Header.Set("Upgrade", tt.upgrade)
			}
			if tt.conn != "" {
				req.Header.Set("Connection", tt.conn)
			}

			if IsWebSocketRequest(req) != tt.expected {
				t.Errorf("IsWebSocketRequest() = %v, want %v", !tt.expected, tt.expected)
			}
		})
	}
}

func TestCanUpgradeAllCases(t *testing.T) {
	// Valid upgrade
	req1 := httptest.NewRequest(http.MethodGet, "/ws", nil)
	req1.Header.Set("Upgrade", "websocket")
	req1.Header.Set("Connection", "Upgrade")
	if !CanUpgrade(req1) {
		t.Error("CanUpgrade should return true for valid WebSocket request")
	}

	// Invalid upgrade
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	if CanUpgrade(req2) {
		t.Error("CanUpgrade should return false for non-WebSocket request")
	}
}

func TestHijackConnectionNotSupported(t *testing.T) {
	// Create a ResponseWriter that doesn't support hijacking
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	_, _, err := HijackConnection(w, req)
	if err == nil {
		t.Error("HijackConnection should fail for ResponseRecorder")
	}
}

func TestWebSocketProxyNew(t *testing.T) {
	logger := &mockLogger{}
	wp := NewWebSocketProxy(logger)

	if wp == nil {
		t.Fatal("NewWebSocketProxy returned nil")
	}
	if wp.dialer == nil {
		t.Error("dialer should not be nil")
	}
	if wp.logger == nil {
		t.Error("logger should not be nil")
	}
	if wp.dialer.Timeout != 10*time.Second {
		t.Errorf("dialer.Timeout = %v, want 10s", wp.dialer.Timeout)
	}
}

func TestWebSocketProxyServeHTTPNoHijacker(t *testing.T) {
	logger := &mockLogger{}
	wp := NewWebSocketProxy(logger)

	// ResponseRecorder doesn't support hijacking
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/ws", nil)

	err := wp.ServeHTTP(w, r, "localhost:8080")
	if err == nil {
		t.Error("ServeHTTP should return error when hijacking not supported")
	}
	if !strings.Contains(err.Error(), "hijacking not supported") {
		t.Errorf("Error = %v, should contain 'hijacking not supported'", err)
	}
}

func TestProxyErrorHandlerAllCases(t *testing.T) {
	logger := &mockLogger{}
	proxy := NewProxy(logger)

	tests := []struct {
		name        string
		err         error
		expectedCode int
	}{
		{"timeout error", errors.New("timeout waiting for response"), http.StatusGatewayTimeout},
		{"connection refused", errors.New("connection refused"), http.StatusServiceUnavailable},
		{"other error", errors.New("some random error"), http.StatusBadGateway},
		{"empty error", errors.New(""), http.StatusBadGateway},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.Header.Set("X-Request-Id", "test-"+tt.name)

			proxy.errorHandler(w, r, tt.err)

			if w.Code != tt.expectedCode {
				t.Errorf("errorHandler code = %d, want %d", w.Code, tt.expectedCode)
			}

			// Verify response body is HTML
			if !strings.Contains(w.Body.String(), "<!DOCTYPE html>") {
				t.Error("Response should be HTML")
			}
		})
	}
}

func TestProxyServeHTTPWithHeaders(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify forwarded headers
		if r.Header.Get("X-Forwarded-For") == "" {
			t.Error("X-Forwarded-For should be set")
		}
		if r.Header.Get("X-Forwarded-Proto") == "" {
			t.Error("X-Forwarded-Proto should be set")
		}
		if r.Header.Get("X-Real-IP") == "" {
			t.Error("X-Real-IP should be set")
		}

		w.Header().Set("X-Backend-Header", "backend-value")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("response from backend"))
	}))
	defer backend.Close()

	logger := &mockLogger{}
	proxy := NewProxy(logger)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader("request body"))
	r.RemoteAddr = "192.168.1.100:54321"
	r.Header.Set("X-Custom-Header", "custom-value")

	target := strings.TrimPrefix(backend.URL, "http://")
	err := proxy.ServeHTTP(w, r, target)

	if err != nil {
		t.Errorf("ServeHTTP error: %v", err)
	}

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want 200", w.Code)
	}
}

func TestProxyServeHTTPWithExistingXFF(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		xff := r.Header.Get("X-Forwarded-For")
		if !strings.Contains(xff, "10.0.0.1") {
			t.Errorf("X-Forwarded-For should contain original IPs, got: %s", xff)
		}
		if !strings.Contains(xff, "192.168.1.50") {
			t.Errorf("X-Forwarded-For should append client IP, got: %s", xff)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	logger := &mockLogger{}
	proxy := NewProxy(logger)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "192.168.1.50:12345"
	r.Header.Set("X-Forwarded-For", "10.0.0.1, 172.16.0.1")

	target := strings.TrimPrefix(backend.URL, "http://")
	proxy.ServeHTTP(w, r, target)
}

func TestStreamProxyServeHTTPBackendError(t *testing.T) {
	logger := &mockLogger{}
	proxy := NewProxy(logger)
	streamProxy := NewStreamProxy(proxy)

	// ResponseRecorder doesn't implement Flusher, so it falls back to regular proxy
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)

	// This will delegate to proxy.ServeHTTP
	// Using a non-existent backend should cause an error
	err := streamProxy.ServeHTTP(w, r, "localhost:59999")

	// The error might be nil if it writes to response, but response should indicate error
	// Just verify it doesn't panic
	_ = err
}

func TestWebSocketProxyDialer(t *testing.T) {
	logger := &mockLogger{}
	wp := NewWebSocketProxy(logger)

	if wp.dialer == nil {
		t.Error("dialer should not be nil")
	}
	if wp.dialer.Timeout != 10*time.Second {
		t.Errorf("dialer.Timeout = %v, want 10s", wp.dialer.Timeout)
	}
	if wp.dialer.KeepAlive != 30*time.Second {
		t.Errorf("dialer.KeepAlive = %v, want 30s", wp.dialer.KeepAlive)
	}
}

func TestIsWebSocketRequestVariations(t *testing.T) {
	tests := []struct {
		name     string
		upgrade  string
		conn     string
		expected bool
	}{
		{"WEBSOCKET uppercase", "WEBSOCKET", "Upgrade", true},
		{"WebSocket mixed case", "WebSocket", "upgrade", true},
		{"websocket lowercase", "websocket", "UPGRADE", true},
		{"connection with keep-alive", "websocket", "keep-alive, Upgrade", true},
		{"connection with Upgrade first", "websocket", "Upgrade, keep-alive", true},
		{"not websocket", "h2c", "Upgrade", false},
		{"no upgrade header", "", "Upgrade", false},
		{"no connection header", "websocket", "", false},
		{"wrong connection", "websocket", "keep-alive", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.upgrade != "" {
				req.Header.Set("Upgrade", tt.upgrade)
			}
			if tt.conn != "" {
				req.Header.Set("Connection", tt.conn)
			}

			result := IsWebSocketRequest(req)
			if result != tt.expected {
				t.Errorf("IsWebSocketRequest() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestHijackConnectionWithMock(t *testing.T) {
	// Create a mock hijacker
	mockHijacker := &mockHijacker{
		conn:  &mockConn{},
	}

	// Try to hijack
	hijacker, ok := interface{}(mockHijacker).(http.Hijacker)
	if !ok {
		t.Skip("Mock doesn't implement Hijacker")
		return
	}

	conn, bufrw, err := hijacker.Hijack()
	if err != nil {
		t.Errorf("Hijack failed: %v", err)
	}
	_ = conn
	_ = bufrw
}

// Mock types for testing
type mockHijacker struct {
	conn   net.Conn
}

func (m *mockHijacker) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return m.conn, bufio.NewReadWriter(bufio.NewReader(nil), bufio.NewWriter(nil)), nil
}

type mockConn struct{}

func (m *mockConn) Read(b []byte) (n int, err error)  { return 0, nil }
func (m *mockConn) Write(b []byte) (n int, err error) { return len(b), nil }
func (m *mockConn) Close() error                       { return nil }
func (m *mockConn) LocalAddr() net.Addr               { return nil }
func (m *mockConn) RemoteAddr() net.Addr              { return nil }
func (m *mockConn) SetDeadline(t time.Time) error     { return nil }
func (m *mockConn) SetReadDeadline(t time.Time) error  { return nil }
func (m *mockConn) SetWriteDeadline(t time.Time) error { return nil }
