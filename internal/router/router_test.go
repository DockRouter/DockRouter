package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// mockLogger implements Logger for testing
type mockRouterLogger struct{}

func (m *mockRouterLogger) Debug(msg string, fields ...interface{}) {}
func (m *mockRouterLogger) Info(msg string, fields ...interface{})  {}
func (m *mockRouterLogger) Warn(msg string, fields ...interface{})  {}
func (m *mockRouterLogger) Error(msg string, fields ...interface{}) {}

// mockProxy implements Proxy for testing
type mockProxy struct {
	lastTarget string
	err        error
}

func (m *mockProxy) ServeHTTP(w http.ResponseWriter, r *http.Request, target string) error {
	m.lastTarget = target
	if m.err != nil {
		// Mimic real proxy behavior: error handler writes response before returning error
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return m.err
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK from " + target))
	return nil
}

func TestNewRouter(t *testing.T) {
	table := NewTable()
	proxy := &mockProxy{}
	logger := &mockRouterLogger{}

	router := NewRouter(table, proxy, logger)
	if router == nil {
		t.Fatal("NewRouter returned nil")
	}
	if router.table == nil {
		t.Error("table should not be nil")
	}
	if router.proxy == nil {
		t.Error("proxy should not be nil")
	}
	if router.logger == nil {
		t.Error("logger should not be nil")
	}
}

func TestRouterServeHTTPNoMatch(t *testing.T) {
	table := NewTable()
	proxy := &mockProxy{}
	logger := &mockRouterLogger{}
	router := NewRouter(table, proxy, logger)

	req := httptest.NewRequest("GET", "http://example.com/path", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadGateway {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusBadGateway)
	}
	if !strings.Contains(w.Body.String(), "502") {
		t.Error("Response should contain 502 error code")
	}
	if !strings.Contains(w.Body.String(), "No route found") {
		t.Error("Response should contain 'No route found'")
	}
}

func TestRouterServeHTTPWithMatch(t *testing.T) {
	table := NewTable()
	proxy := &mockProxy{}
	logger := &mockRouterLogger{}
	router := NewRouter(table, proxy, logger)

	// Add a route
	route := &Route{
		Host:          "example.com",
		PathPrefix:    "/",
		ContainerID:   "container-1",
		ContainerName: "test-container",
		Backend:       NewBackendPool(RoundRobin),
	}
	route.Backend.Add(&BackendTarget{Address: "192.168.1.1:8080", Healthy: true})
	table.Add(route)

	req := httptest.NewRequest("GET", "http://example.com/path", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}
	if proxy.lastTarget != "192.168.1.1:8080" {
		t.Errorf("proxy.lastTarget = %s, want 192.168.1.1:8080", proxy.lastTarget)
	}
}

func TestRouterServeHTTPHostNormalization(t *testing.T) {
	table := NewTable()
	proxy := &mockProxy{}
	logger := &mockRouterLogger{}
	router := NewRouter(table, proxy, logger)

	// Add a route without port
	route := &Route{
		Host:          "example.com",
		PathPrefix:    "/",
		ContainerID:   "container-1",
		ContainerName: "test-container",
		Backend:       NewBackendPool(RoundRobin),
	}
	route.Backend.Add(&BackendTarget{Address: "192.168.1.1:8080", Healthy: true})
	table.Add(route)

	// Request with port in Host header
	req := httptest.NewRequest("GET", "http://example.com:8080/path", nil)
	req.Host = "example.com:8080"
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestRouterServeHTTPNoBackend(t *testing.T) {
	table := NewTable()
	proxy := &mockProxy{}
	logger := &mockRouterLogger{}
	router := NewRouter(table, proxy, logger)

	// Add a route with empty backend
	route := &Route{
		Host:          "example.com",
		PathPrefix:    "/",
		ContainerID:   "container-1",
		ContainerName: "test-container",
		Backend:       NewBackendPool(RoundRobin),
		// No backends added
	}
	table.Add(route)

	req := httptest.NewRequest("GET", "http://example.com/path", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
	if !strings.Contains(w.Body.String(), "503") {
		t.Error("Response should contain 503 error code")
	}
	if !strings.Contains(w.Body.String(), "No healthy backends") {
		t.Error("Response should contain 'No healthy backends'")
	}
}

func TestRouterServeHTTPProxyError(t *testing.T) {
	table := NewTable()
	proxy := &mockProxy{err: http.ErrHandlerTimeout}
	logger := &mockRouterLogger{}
	router := NewRouter(table, proxy, logger)

	// Add a route
	route := &Route{
		Host:          "example.com",
		PathPrefix:    "/",
		ContainerID:   "container-1",
		ContainerName: "test-container",
		Backend:       NewBackendPool(RoundRobin),
	}
	route.Backend.Add(&BackendTarget{Address: "192.168.1.1:8080", Healthy: true})
	table.Add(route)

	req := httptest.NewRequest("GET", "http://example.com/path", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadGateway {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusBadGateway)
	}
}

func TestRouterGetTable(t *testing.T) {
	table := NewTable()
	proxy := &mockProxy{}
	logger := &mockRouterLogger{}
	router := NewRouter(table, proxy, logger)

	result := router.GetTable()
	if result != table {
		t.Error("GetTable should return the same table instance")
	}
}

func TestBuildErrorPage(t *testing.T) {
	tests := []struct {
		code      int
		title     string
		message   string
		requestID string
		checks    []string
	}{
		{502, "Bad Gateway", "No route found", "req-123", []string{"502", "Bad Gateway", "No route found", "req-123", "Request ID"}},
		{503, "Service Unavailable", "No backends", "", []string{"503", "Service Unavailable", "No backends"}},
		{404, "Not Found", "Page not found", "", []string{"404", "Not Found", "Page not found"}},
		{500, "Internal Server Error", "Something went wrong", "abc-def", []string{"500", "Internal Server Error", "abc-def"}},
	}

	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			result := buildErrorPage(tt.code, tt.title, tt.message, tt.requestID)

			for _, check := range tt.checks {
				if !strings.Contains(result, check) {
					t.Errorf("Error page should contain %q", check)
				}
			}

			// Should be valid HTML
			if !strings.Contains(result, "<!DOCTYPE html>") {
				t.Error("Error page should start with DOCTYPE")
			}
			if !strings.Contains(result, "</html>") {
				t.Error("Error page should end with </html>")
			}
		})
	}
}

func TestBuildErrorPageNoRequestID(t *testing.T) {
	result := buildErrorPage(502, "Bad Gateway", "No route", "")

	// Should not contain Request ID section
	if strings.Contains(result, "Request ID:") {
		t.Error("Error page should not contain 'Request ID:' when requestID is empty")
	}
}

func TestIntToStr(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{0, "0"},
		{1, "1"},
		{10, "10"},
		{100, "100"},
		{123, "123"},
		{502, "502"},
		{503, "503"},
		{999, "999"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := intToStr(tt.input)
			if result != tt.expected {
				t.Errorf("intToStr(%d) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestRouterServeHTTPCaseInsensitiveHost(t *testing.T) {
	table := NewTable()
	proxy := &mockProxy{}
	logger := &mockRouterLogger{}
	router := NewRouter(table, proxy, logger)

	// Add a route with lowercase host
	route := &Route{
		Host:          "example.com",
		PathPrefix:    "/",
		ContainerID:   "container-1",
		ContainerName: "test-container",
		Backend:       NewBackendPool(RoundRobin),
	}
	route.Backend.Add(&BackendTarget{Address: "192.168.1.1:8080", Healthy: true})
	table.Add(route)

	// Request with mixed case host
	req := httptest.NewRequest("GET", "http://Example.COM/path", nil)
	req.Host = "Example.COM"
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestRouterServeHTTPUnhealthyBackend(t *testing.T) {
	table := NewTable()
	proxy := &mockProxy{}
	logger := &mockRouterLogger{}
	router := NewRouter(table, proxy, logger)

	// Add a route with unhealthy backend
	route := &Route{
		Host:          "example.com",
		PathPrefix:    "/",
		ContainerID:   "container-1",
		ContainerName: "test-container",
		Backend:       NewBackendPool(RoundRobin),
	}
	route.Backend.Add(&BackendTarget{Address: "192.168.1.1:8080", Healthy: false})
	table.Add(route)

	req := httptest.NewRequest("GET", "http://example.com/path", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Should return 503 because no healthy backends
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

func TestRouterServeHTTPWithRequestID(t *testing.T) {
	table := NewTable()
	proxy := &mockProxy{}
	logger := &mockRouterLogger{}
	router := NewRouter(table, proxy, logger)

	req := httptest.NewRequest("GET", "http://example.com/path", nil)
	req.Header.Set("X-Request-Id", "test-request-123")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Response should include request ID
	if !strings.Contains(w.Body.String(), "test-request-123") {
		t.Error("Response should contain request ID")
	}
}

func TestNewRouterWithMiddleware(t *testing.T) {
	table := NewTable()
	proxy := &mockProxy{}
	logger := &mockRouterLogger{}
	builder := NewRouteMiddlewareBuilder()

	router := NewRouterWithMiddleware(table, proxy, logger, builder)
	if router == nil {
		t.Fatal("NewRouterWithMiddleware returned nil")
	}
	if router.table != table {
		t.Error("table should be set correctly")
	}
	if router.proxy != proxy {
		t.Error("proxy should be set correctly")
	}
	if router.logger != logger {
		t.Error("logger should be set correctly")
	}
	if router.middlewareBuilder != builder {
		t.Error("middlewareBuilder should be set correctly")
	}
	if router.maxRetries != 3 {
		t.Errorf("maxRetries = %d, want 3", router.maxRetries)
	}
}

func TestRouterSetMaxRetries(t *testing.T) {
	table := NewTable()
	proxy := &mockProxy{}
	logger := &mockRouterLogger{}
	router := NewRouter(table, proxy, logger)

	// Test setting valid value
	router.SetMaxRetries(10)
	if router.maxRetries != 10 {
		t.Errorf("maxRetries = %d, want 10", router.maxRetries)
	}

	// Test setting zero (should not change)
	router.SetMaxRetries(0)
	if router.maxRetries != 10 {
		t.Errorf("maxRetries should not change when set to 0, got %d", router.maxRetries)
	}

	// Test setting negative (should not change)
	router.SetMaxRetries(-5)
	if router.maxRetries != 10 {
		t.Errorf("maxRetries should not change when set to negative, got %d", router.maxRetries)
	}
}

func TestRouterCleanupRoute(t *testing.T) {
	table := NewTable()
	proxy := &mockProxy{}
	logger := &mockRouterLogger{}
	builder := NewRouteMiddlewareBuilder()
	router := NewRouterWithMiddleware(table, proxy, logger, builder)

	// Create a route with rate limiter and circuit breaker
	route := &Route{
		ID:          "cleanup-test-route",
		Host:        "example.com",
		PathPrefix:  "/",
		ContainerID: "container-1",
		Backend:     NewBackendPool(RoundRobin),
		MiddlewareConfig: MiddlewareConfig{
			RateLimit: RateLimitConfig{
				Enabled: true,
				Count:   10,
				Window:  0, // Use default
			},
			CircuitBreaker: CircuitBreakerConfig{
				Enabled:  true,
				Failures: 0, // Use default
				Window:   0, // Use default
			},
		},
	}
	route.Backend.Add(&BackendTarget{Address: "192.168.1.1:8080", Healthy: true})
	table.Add(route)

	// Build middleware chain to create rate limiter and circuit breaker
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	_ = builder.BuildChain(route, next)

	// Cleanup the route
	router.CleanupRoute("cleanup-test-route")

	// Should not panic and should clean up middleware state
	// The cleanup removes rate limiter and circuit breaker from the builder's maps
}
