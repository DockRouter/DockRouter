package proxy

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIsWebSocketRequest(t *testing.T) {
	tests := []struct {
		name     string
		headers  map[string]string
		expected bool
	}{
		{
			name: "valid websocket request",
			headers: map[string]string{
				"Upgrade":    "websocket",
				"Connection": "Upgrade",
			},
			expected: true,
		},
		{
			name: "valid websocket request lowercase",
			headers: map[string]string{
				"Upgrade":    "WEBSOCKET",
				"Connection": "keep-alive, Upgrade",
			},
			expected: true,
		},
		{
			name: "missing upgrade header",
			headers: map[string]string{
				"Connection": "Upgrade",
			},
			expected: false,
		},
		{
			name: "missing connection header",
			headers: map[string]string{
				"Upgrade": "websocket",
			},
			expected: false,
		},
		{
			name: "wrong upgrade type",
			headers: map[string]string{
				"Upgrade":    "h2c",
				"Connection": "Upgrade",
			},
			expected: false,
		},
		{
			name:     "no headers",
			headers:  map[string]string{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/ws", nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			result := IsWebSocketRequest(req)
			if result != tt.expected {
				t.Errorf("IsWebSocketRequest() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestCanUpgrade(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")

	if !CanUpgrade(req) {
		t.Error("CanUpgrade should return true for valid WebSocket request")
	}

	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	if CanUpgrade(req2) {
		t.Error("CanUpgrade should return false for non-WebSocket request")
	}
}

func TestNewProxy(t *testing.T) {
	logger := &mockLogger{}
	proxy := NewProxy(logger)

	if proxy == nil {
		t.Fatal("NewProxy returned nil")
	}
	if proxy.transport == nil {
		t.Error("Proxy transport not initialized")
	}
	if proxy.bufferPool == nil {
		t.Error("Proxy buffer pool not initialized")
	}
}

func TestBuildErrorPage(t *testing.T) {
	tests := []struct {
		code      int
		title     string
		message   string
		requestID string
		check     func(string) bool
	}{
		{
			code:      502,
			title:     "Bad Gateway",
			message:   "connection refused",
			requestID: "abc123",
			check: func(html string) bool {
				return containsAll(html, "502", "Bad Gateway", "abc123")
			},
		},
		{
			code:      503,
			title:     "Service Unavailable",
			message:   "backend unavailable",
			requestID: "",
			check: func(html string) bool {
				return containsAll(html, "503", "Service Unavailable") &&
					!contains(html, "Request ID:")
			},
		},
		{
			code:      504,
			title:     "Gateway Timeout",
			message:   "timeout",
			requestID: "xyz789",
			check: func(html string) bool {
				return containsAll(html, "504", "Gateway Timeout", "xyz789")
			},
		},
		{
			code:      429,
			title:     "Too Many Requests",
			message:   "rate limit exceeded",
			requestID: "rate123",
			check: func(html string) bool {
				return containsAll(html, "429", "Too Many Requests", "rate123")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			html := buildErrorPage(tt.code, tt.title, tt.message, tt.requestID)
			if !tt.check(html) {
				t.Errorf("Error page HTML doesn't match expected content: %s", html[:200])
			}
		})
	}
}

func TestRemoveHopHeaders(t *testing.T) {
	hdr := http.Header{}
	hdr.Set("Connection", "keep-alive")
	hdr.Set("Keep-Alive", "timeout=5")
	hdr.Set("Transfer-Encoding", "chunked")
	hdr.Set("X-Custom", "value")

	removeHopHeaders(hdr)

	if hdr.Get("Connection") != "" {
		t.Error("Connection header should be removed")
	}
	if hdr.Get("Keep-Alive") != "" {
		t.Error("Keep-Alive header should be removed")
	}
	if hdr.Get("Transfer-Encoding") != "" {
		t.Error("Transfer-Encoding header should be removed")
	}
	if hdr.Get("X-Custom") != "value" {
		t.Error("X-Custom header should be preserved")
	}
}

func TestNewWebSocketProxy(t *testing.T) {
	logger := &mockLogger{}
	wsp := NewWebSocketProxy(logger)

	if wsp == nil {
		t.Fatal("NewWebSocketProxy returned nil")
	}
	if wsp.dialer == nil {
		t.Error("WebSocket dialer not initialized")
	}
}

func TestProxyErrorHandler(t *testing.T) {
	logger := &mockLogger{}
	proxy := NewProxy(logger)

	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{
			name:       "connection refused",
			err:        errors.New("connection refused"),
			wantStatus: http.StatusServiceUnavailable,
		},
		{
			name:       "timeout",
			err:        errors.New("request timeout exceeded"),
			wantStatus: http.StatusGatewayTimeout,
		},
		{
			name:       "other error",
			err:        errors.New("network error"),
			wantStatus: http.StatusBadGateway,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)
			req.Header.Set("X-Request-Id", "test-123")
			w := httptest.NewRecorder()

			proxy.errorHandler(w, req, tt.err)

			if w.Code != tt.wantStatus {
				t.Errorf("Status = %d, want %d", w.Code, tt.wantStatus)
			}
			if !strings.Contains(w.Body.String(), "test-123") {
				t.Error("Response should contain request ID")
			}
		})
	}
}

func TestNewBufferPool(t *testing.T) {
	pool := newBufferPool()
	if pool == nil {
		t.Fatal("newBufferPool returned nil")
	}

	buf := pool.Get()
	if buf == nil {
		t.Error("Get returned nil buffer")
	}

	pool.Put(buf)
}

// Mock logger for testing
type mockLogger struct{}

func (m *mockLogger) Debug(msg string, fields ...interface{}) {}
func (m *mockLogger) Info(msg string, fields ...interface{})  {}
func (m *mockLogger) Warn(msg string, fields ...interface{})  {}
func (m *mockLogger) Error(msg string, fields ...interface{}) {}

// Helper function
func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !contains(s, sub) {
			return false
		}
	}
	return true
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		(len(s) > 0 && len(sub) > 0 && findSubstring(s, sub)))
}

func findSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
