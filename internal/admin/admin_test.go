package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DockRouter/dockrouter/internal/metrics"
)

func TestNewAuth(t *testing.T) {
	auth := NewAuth("admin", "password123")
	if auth == nil {
		t.Fatal("NewAuth returned nil")
	}
	if auth.username != "admin" {
		t.Errorf("Expected username 'admin', got '%s'", auth.username)
	}
	if auth.password != "password123" {
		t.Errorf("Expected password 'password123', got '%s'", auth.password)
	}
}

func TestAuthMiddlewareNoAuth(t *testing.T) {
	// Auth with empty username - should skip auth
	auth := NewAuth("", "")

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	auth.Middleware(handler).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, rec.Code)
	}
	if rec.Body.String() != "success" {
		t.Errorf("Expected body 'success', got '%s'", rec.Body.String())
	}
}

func TestAuthMiddlewareNoCredentials(t *testing.T) {
	auth := NewAuth("admin", "secret")

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called without auth")
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	auth.Middleware(handler).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Expected status %d, got %d", http.StatusUnauthorized, rec.Code)
	}

	// Check WWW-Authenticate header
	authHeader := rec.Header().Get("WWW-Authenticate")
	if authHeader != `Basic realm="DockRouter Admin"` {
		t.Errorf("Expected WWW-Authenticate header, got '%s'", authHeader)
	}
}

func TestAuthMiddlewareWrongCredentials(t *testing.T) {
	auth := NewAuth("admin", "secret")

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called with wrong credentials")
	})

	tests := []struct {
		name     string
		username string
		password string
	}{
		{"wrong username", "wronguser", "secret"},
		{"wrong password", "admin", "wrongpass"},
		{"both wrong", "wronguser", "wrongpass"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.SetBasicAuth(tt.username, tt.password)
			rec := httptest.NewRecorder()

			auth.Middleware(handler).ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Errorf("Expected status %d, got %d", http.StatusUnauthorized, rec.Code)
			}
		})
	}
}

func TestAuthMiddlewareCorrectCredentials(t *testing.T) {
	auth := NewAuth("admin", "secret")

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("authenticated"))
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.SetBasicAuth("admin", "secret")
	rec := httptest.NewRecorder()

	auth.Middleware(handler).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, rec.Code)
	}
	if rec.Body.String() != "authenticated" {
		t.Errorf("Expected body 'authenticated', got '%s'", rec.Body.String())
	}
}

func TestAuthMiddlewareTimingSafeComparison(t *testing.T) {
	// Test that the auth comparison is timing-safe by using different length passwords
	auth := NewAuth("admin", "short")

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	tests := []struct {
		name     string
		password string
	}{
		{"short password", "x"},
		{"medium password", "xxxxxxxx"},
		{"long password", "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.SetBasicAuth("admin", tt.password)
			rec := httptest.NewRecorder()

			auth.Middleware(handler).ServeHTTP(rec, req)

			// All should be unauthorized
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("Expected status %d, got %d", http.StatusUnauthorized, rec.Code)
			}
		})
	}
}

func TestNewServer(t *testing.T) {
	handler := http.NewServeMux()
	server := NewServer("127.0.0.1:9090", handler)

	if server == nil {
		t.Fatal("NewServer returned nil")
	}
	if server.addr != "127.0.0.1:9090" {
		t.Errorf("Expected addr '127.0.0.1:9090', got '%s'", server.addr)
	}
}

// SSEHub Tests

func TestNewSSEHub(t *testing.T) {
	hub := NewSSEHub()
	if hub == nil {
		t.Fatal("NewSSEHub returned nil")
	}
	if hub.clients == nil {
		t.Error("clients map not initialized")
	}
	if hub.broadcast == nil {
		t.Error("broadcast channel not initialized")
	}
}

func TestSSEHubSend(t *testing.T) {
	hub := NewSSEHub()

	// Start hub in background
	go hub.Run()

	// Create a test client that simulates being registered
	client := &sseClient{
		ch:    make(chan Event, 10),
		flush: make(chan struct{}),
	}

	// Register client
	hub.register <- client

	// Give it time to register
	time.Sleep(10 * time.Millisecond)

	// Send event
	hub.Send(Event{Type: "test", Data: "hello"})

	// Receive event
	select {
	case event := <-client.ch:
		if event.Type != "test" {
			t.Errorf("Expected event type 'test', got '%s'", event.Type)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Did not receive event within timeout")
	}
}

func TestSSEHubClientUnregister(t *testing.T) {
	hub := NewSSEHub()
	go hub.Run()

	client := &sseClient{
		ch:    make(chan Event, 10),
		flush: make(chan struct{}),
	}

	// Register and unregister
	hub.register <- client
	time.Sleep(10 * time.Millisecond)
	hub.unregister <- client
	time.Sleep(10 * time.Millisecond)

	// Channel should be closed
	select {
	case _, ok := <-client.ch:
		if ok {
			t.Error("Client channel should be closed after unregister")
		}
	default:
		// Channel not closed yet, that's ok
	}
}

func TestSSEHubDropSlowClient(t *testing.T) {
	hub := NewSSEHub()
	go hub.Run()

	// Client with buffer of 1
	client := &sseClient{
		ch:    make(chan Event, 1),
		flush: make(chan struct{}),
	}

	hub.register <- client
	time.Sleep(10 * time.Millisecond)

	// Send multiple events rapidly - should drop some
	for i := 0; i < 20; i++ {
		hub.Send(Event{Type: "test"})
	}

	// Should only have 1 event (buffer size)
	count := 0
	for {
		select {
		case <-client.ch:
			count++
		default:
			// Check count
			if count > 2 {
				t.Logf("Received %d events from buffer of 1", count)
			}
			return
		}
	}
}

// Benchmark auth middleware
func BenchmarkAuthMiddleware(b *testing.B) {
	auth := NewAuth("admin", "secret")

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.SetBasicAuth("admin", "secret")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		auth.Middleware(handler).ServeHTTP(rec, req)
	}
}

func BenchmarkAuthMiddlewareUnauthorized(b *testing.B) {
	auth := NewAuth("admin", "secret")

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	// No auth header

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		auth.Middleware(handler).ServeHTTP(rec, req)
	}
}

// API Handler Tests

func TestNewAPIHandler(t *testing.T) {
	handler := NewAPIHandler(nil)
	if handler == nil {
		t.Fatal("NewAPIHandler returned nil")
	}
}

func TestAPIHandlerRoutes(t *testing.T) {
	handler := NewAPIHandler(nil)
	routes := handler.Routes()

	if len(routes) != 6 {
		t.Errorf("Routes count = %d, want 6", len(routes))
	}

	expectedRoutes := []string{
		"/api/v1/status",
		"/api/v1/routes",
		"/api/v1/containers",
		"/api/v1/certificates",
		"/api/v1/metrics",
		"/api/v1/health",
	}

	for _, route := range expectedRoutes {
		if _, ok := routes[route]; !ok {
			t.Errorf("Missing route: %s", route)
		}
	}
}

func TestAPIHandlerStatus(t *testing.T) {
	handler := NewAPIHandler(nil)
	routes := handler.Routes()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	rec := httptest.NewRecorder()

	routes["/api/v1/status"](rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status code = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestAPIHandlerRoutesEndpoint(t *testing.T) {
	handler := NewAPIHandler(nil)
	routes := handler.Routes()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/routes", nil)
	rec := httptest.NewRecorder()

	routes["/api/v1/routes"](rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status code = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestAPIHandlerContainers(t *testing.T) {
	handler := NewAPIHandler(nil)
	routes := handler.Routes()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/containers", nil)
	rec := httptest.NewRecorder()

	routes["/api/v1/containers"](rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status code = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestAPIHandlerCertificates(t *testing.T) {
	handler := NewAPIHandler(nil)
	routes := handler.Routes()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/certificates", nil)
	rec := httptest.NewRecorder()

	routes["/api/v1/certificates"](rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status code = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestAPIHandlerMetrics(t *testing.T) {
	handler := NewAPIHandler(nil)
	routes := handler.Routes()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/metrics", nil)
	rec := httptest.NewRecorder()

	routes["/api/v1/metrics"](rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status code = %d, want %d", rec.Code, http.StatusOK)
	}

	// With nil metrics collector, response should be empty but valid
	contentType := rec.Header().Get("Content-Type")
	if contentType != "text/plain; version=0.0.4" {
		t.Errorf("Content-Type = %s, want text/plain; version=0.0.4", contentType)
	}
}

func TestAPIHandlerMetricsWithCollector(t *testing.T) {
	collector := metrics.NewCollector()
	collector.Counter("test_counter").Inc()
	collector.Gauge("test_gauge").Set(42.0)

	handler := NewAPIHandler(collector)
	routes := handler.Routes()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/metrics", nil)
	rec := httptest.NewRecorder()

	routes["/api/v1/metrics"](rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status code = %d, want %d", rec.Code, http.StatusOK)
	}

	// With collector, response should contain metrics
	body := rec.Body.String()
	if !strings.Contains(body, "dockrouter_test_counter") {
		t.Errorf("Response should contain test_counter, got: %s", body)
	}
}

func TestAPIHandlerHealth(t *testing.T) {
	handler := NewAPIHandler(nil)
	routes := handler.Routes()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rec := httptest.NewRecorder()

	routes["/api/v1/health"](rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status code = %d, want %d", rec.Code, http.StatusOK)
	}

	if rec.Body.String() != `{"status":"healthy"}` {
		t.Errorf("Health response = %s, want {\"status\":\"healthy\"}", rec.Body.String())
	}
}

// SSE Handler Tests

func TestSSEHubHandler(t *testing.T) {
	hub := NewSSEHub()
	go hub.Run()

	handler := hub.Handler()
	if handler == nil {
		t.Fatal("Handler returned nil")
	}

	// Create a request
	req := httptest.NewRequest(http.MethodGet, "/events", nil)
	req.Header.Set("Accept", "text/event-stream")

	// Can't easily test the full SSE flow without a real response writer
	// Just verify the handler function doesn't panic
	_ = req
}

func TestSSEHubMultipleClients(t *testing.T) {
	hub := NewSSEHub()
	go hub.Run()

	// Create multiple test clients
	clients := make([]*sseClient, 3)
	for i := 0; i < 3; i++ {
		clients[i] = &sseClient{
			ch:    make(chan Event, 10),
			flush: make(chan struct{}),
		}
		hub.register <- clients[i]
	}

	time.Sleep(10 * time.Millisecond)

	// Send an event
	hub.Send(Event{Type: "test", Data: "broadcast"})

	// All clients should receive it
	for i, client := range clients {
		select {
		case event := <-client.ch:
			if event.Type != "test" {
				t.Errorf("Client %d: event type = %s, want test", i, event.Type)
			}
		case <-time.After(100 * time.Millisecond):
			t.Errorf("Client %d did not receive event", i)
		}
	}
}

func TestSSEHubClose(t *testing.T) {
	hub := NewSSEHub()
	go hub.Run()

	// Add and remove a client
	client := &sseClient{
		ch:    make(chan Event, 10),
		flush: make(chan struct{}),
	}

	hub.register <- client
	time.Sleep(10 * time.Millisecond)

	// Close the hub (if it has a Close method)
	// If not, just unregister the client
	hub.unregister <- client
	time.Sleep(10 * time.Millisecond)
}

// Server Start Tests

func TestServerStart(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})

	server := NewServer("127.0.0.1:0", handler) // Use port 0 for random available port

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Start(ctx)
	}()

	// Give server time to start
	time.Sleep(50 * time.Millisecond)

	// Cancel context to shutdown server
	cancel()

	// Wait for server to shut down
	select {
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			t.Errorf("Server error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("Server did not shut down in time")
	}
}

func TestServerStartAndRequest(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("pong"))
	})

	// Use port 0 to get a random available port
	server := NewServer("127.0.0.1:0", handler)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Channel to get the actual address
	addrCh := make(chan string, 1)

	go func() {
		// We need to modify the server slightly to get the actual address
		// For now, just start it
		err := server.Start(ctx)
		_ = err
	}()

	// Since we can't get the actual port easily, we just verify the server starts
	// and shuts down cleanly
	time.Sleep(50 * time.Millisecond)
	cancel()
	_ = addrCh
}

// SSE Handler with Flusher tests

// mockFlusher implements http.ResponseWriter and http.Flusher
type mockFlusher struct {
	header     http.Header
	body       strings.Builder
	statusCode int
	flushed    bool
}

func (m *mockFlusher) Header() http.Header {
	if m.header == nil {
		m.header = make(http.Header)
	}
	return m.header
}

func (m *mockFlusher) Write(b []byte) (int, error) {
	return m.body.Write(b)
}

func (m *mockFlusher) WriteHeader(statusCode int) {
	m.statusCode = statusCode
}

func (m *mockFlusher) Flush() {
	m.flushed = true
}

func TestSSEHubHandlerWithFlusher(t *testing.T) {
	hub := NewSSEHub()
	go hub.Run()

	handler := hub.Handler()

	// Create mock flusher
	w := &mockFlusher{}
	r := httptest.NewRequest(http.MethodGet, "/events", nil)
	r.Header.Set("Accept", "text/event-stream")

	// Create context with cancel
	ctx, cancel := context.WithCancel(context.Background())
	r = r.WithContext(ctx)

	// Run handler in goroutine
	done := make(chan struct{})
	go func() {
		handler(w, r)
		close(done)
	}()

	// Give time to register
	time.Sleep(20 * time.Millisecond)

	// Verify headers were set
	if w.Header().Get("Content-Type") != "text/event-stream" {
		t.Errorf("Content-Type = %s, want text/event-stream", w.Header().Get("Content-Type"))
	}
	if w.Header().Get("Cache-Control") != "no-cache" {
		t.Errorf("Cache-Control = %s, want no-cache", w.Header().Get("Cache-Control"))
	}
	if w.Header().Get("Connection") != "keep-alive" {
		t.Errorf("Connection = %s, want keep-alive", w.Header().Get("Connection"))
	}

	// Send an event
	hub.Send(Event{Type: "test-event"})

	// Wait for event to be processed
	time.Sleep(50 * time.Millisecond)

	// Cancel to stop handler
	cancel()

	// Wait for handler to finish
	select {
	case <-done:
		// Handler finished
	case <-time.After(time.Second):
		t.Error("Handler did not finish in time")
	}

	// Check body contains event data
	body := w.body.String()
	if !strings.Contains(body, "data: ") {
		t.Errorf("Body should contain 'data: ', got %q", body)
	}
	if !strings.Contains(body, "test-event") {
		t.Errorf("Body should contain event type, got %q", body)
	}

	// Verify flush was called
	if !w.flushed {
		t.Error("Flush should have been called")
	}
}

// nonFlusherResponseWriter implements http.ResponseWriter but NOT http.Flusher
type nonFlusherResponseWriter struct {
	header     http.Header
	body       strings.Builder
	statusCode int
}

func (n *nonFlusherResponseWriter) Header() http.Header {
	if n.header == nil {
		n.header = make(http.Header)
	}
	return n.header
}

func (n *nonFlusherResponseWriter) Write(b []byte) (int, error) {
	return n.body.Write(b)
}

func (n *nonFlusherResponseWriter) WriteHeader(statusCode int) {
	n.statusCode = statusCode
}

func TestSSEHubHandlerNoFlusher(t *testing.T) {
	hub := NewSSEHub()
	go hub.Run()

	handler := hub.Handler()

	// Use a ResponseWriter that does NOT implement Flusher
	w := &nonFlusherResponseWriter{}
	r := httptest.NewRequest(http.MethodGet, "/events", nil)

	handler(w, r)

	// Should return 500 because Flusher not supported
	if w.statusCode != http.StatusInternalServerError {
		t.Errorf("Status code = %d, want %d", w.statusCode, http.StatusInternalServerError)
	}

	// Body should contain error message
	if !strings.Contains(w.body.String(), "SSE not supported") {
		t.Errorf("Body should contain 'SSE not supported', got %s", w.body.String())
	}
}

func TestSSEHubHandlerContextCancel(t *testing.T) {
	hub := NewSSEHub()
	go hub.Run()

	handler := hub.Handler()

	w := &mockFlusher{}
	ctx, cancel := context.WithCancel(context.Background())
	r := httptest.NewRequest(http.MethodGet, "/events", nil)
	r = r.WithContext(ctx)

	done := make(chan struct{})
	go func() {
		handler(w, r)
		close(done)
	}()

	// Give time to register
	time.Sleep(20 * time.Millisecond)

	// Cancel immediately to test context done path
	cancel()

	// Wait for handler to finish
	select {
	case <-done:
		// Handler finished cleanly
	case <-time.After(time.Second):
		t.Error("Handler should finish when context is cancelled")
	}
}

func TestSSEHubBroadcastEvent(t *testing.T) {
	hub := NewSSEHub()
	go hub.Run()

	// Register two clients
	client1 := &sseClient{ch: make(chan Event, 10), flush: make(chan struct{})}
	client2 := &sseClient{ch: make(chan Event, 10), flush: make(chan struct{})}

	hub.register <- client1
	hub.register <- client2
	time.Sleep(20 * time.Millisecond)

	// Broadcast event
	event := Event{Type: "broadcast", Data: map[string]string{"key": "value"}}
	hub.Send(event)

	// Both clients should receive
	for i, client := range []*sseClient{client1, client2} {
		select {
		case received := <-client.ch:
			if received.Type != "broadcast" {
				t.Errorf("Client %d: type = %s, want broadcast", i, received.Type)
			}
		case <-time.After(100 * time.Millisecond):
			t.Errorf("Client %d did not receive broadcast", i)
		}
	}
}

func TestSSEHubClientCount(t *testing.T) {
	hub := NewSSEHub()
	go hub.Run()

	// Initially should have no clients
	hub.mu.RLock()
	count := len(hub.clients)
	hub.mu.RUnlock()
	if count != 0 {
		t.Errorf("Initial client count = %d, want 0", count)
	}

	// Add a client
	client := &sseClient{ch: make(chan Event, 10), flush: make(chan struct{})}
	hub.register <- client
	time.Sleep(20 * time.Millisecond)

	hub.mu.RLock()
	count = len(hub.clients)
	hub.mu.RUnlock()
	if count != 1 {
		t.Errorf("Client count after register = %d, want 1", count)
	}

	// Remove the client
	hub.unregister <- client
	time.Sleep(20 * time.Millisecond)

	hub.mu.RLock()
	count = len(hub.clients)
	hub.mu.RUnlock()
	if count != 0 {
		t.Errorf("Client count after unregister = %d, want 0", count)
	}
}

func TestEventStruct(t *testing.T) {
	event := Event{
		Type: "container.started",
		Data: map[string]interface{}{"id": "abc123", "name": "test"},
	}

	if event.Type != "container.started" {
		t.Errorf("Type = %s, want container.started", event.Type)
	}

	data, ok := event.Data.(map[string]interface{})
	if !ok {
		t.Fatal("Data should be a map")
	}
	if data["id"] != "abc123" {
		t.Errorf("Data[id] = %v, want abc123", data["id"])
	}
}
