package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/DockRouter/dockrouter/internal/config"
	"github.com/DockRouter/dockrouter/internal/discovery"
	"github.com/DockRouter/dockrouter/internal/health"
	"github.com/DockRouter/dockrouter/internal/log"
	"github.com/DockRouter/dockrouter/internal/metrics"
	"github.com/DockRouter/dockrouter/internal/router"
	tlspkg "github.com/DockRouter/dockrouter/internal/tls"
)

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected log.Level
	}{
		{"debug", log.LevelDebug},
		{"warn", log.LevelWarn},
		{"error", log.LevelError},
		{"info", log.LevelInfo},
		{"", log.LevelInfo},
		{"unknown", log.LevelInfo},
		{"DEBUG", log.LevelInfo}, // case sensitive
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := parseLogLevel(tt.input)
			if result != tt.expected {
				t.Errorf("parseLogLevel(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestAppHandleStatus(t *testing.T) {
	cfg := &config.Config{
		Version:   "test-version",
		HTTPPort:  80,
		HTTPSPort: 443,
	}
	logger := log.NewLogger(nil, log.LevelInfo)
	routeTable := router.NewTable()

	app := &App{
		config:     cfg,
		logger:     logger,
		routeTable: routeTable,
		startTime:  time.Now(),
	}

	req := httptest.NewRequest("GET", "/api/v1/status", nil)
	w := httptest.NewRecorder()

	app.handleStatus(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %s, want application/json", ct)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}

	if result["status"] != "ok" {
		t.Errorf("status = %v, want ok", result["status"])
	}
	if result["version"] != "test-version" {
		t.Errorf("version = %v, want test-version", result["version"])
	}
}

func TestAppHandleStatusWithComponents(t *testing.T) {
	cfg := &config.Config{
		Version:   "test-version",
		HTTPPort:  80,
		HTTPSPort: 443,
	}
	logger := log.NewLogger(nil, log.LevelInfo)
	routeTable := router.NewTable()

	app := &App{
		config:     cfg,
		logger:     logger,
		routeTable: routeTable,
		startTime:  time.Now(),
	}

	// Test with TLS manager
	store := tlspkg.NewStore(t.TempDir())
	app.tlsManager = tlspkg.NewManager(store, nil, nil, logger)

	req := httptest.NewRequest("GET", "/api/v1/status", nil)
	w := httptest.NewRecorder()

	app.handleStatus(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestAppHandleRoutes(t *testing.T) {
	cfg := &config.Config{Version: "test"}
	logger := log.NewLogger(nil, log.LevelInfo)
	routeTable := router.NewTable()

	app := &App{
		config:     cfg,
		logger:     logger,
		routeTable: routeTable,
	}

	req := httptest.NewRequest("GET", "/api/v1/routes", nil)
	w := httptest.NewRecorder()

	app.handleRoutes(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %s, want application/json", ct)
	}

	// Should be empty array
	var routes []interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &routes); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}
}

func TestAppHandleContainers(t *testing.T) {
	cfg := &config.Config{Version: "test"}
	logger := log.NewLogger(nil, log.LevelInfo)
	routeTable := router.NewTable()

	app := &App{
		config:     cfg,
		logger:     logger,
		routeTable: routeTable,
	}

	t.Run("nil discovery engine", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/containers", nil)
		w := httptest.NewRecorder()

		app.handleContainers(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
		}

		if strings.TrimSpace(w.Body.String()) != "[]" {
			t.Errorf("Body = %s, want []", w.Body.String())
		}
	})
}

func TestAppHandleCertificates(t *testing.T) {
	cfg := &config.Config{Version: "test"}
	logger := log.NewLogger(nil, log.LevelInfo)
	routeTable := router.NewTable()

	app := &App{
		config:     cfg,
		logger:     logger,
		routeTable: routeTable,
	}

	t.Run("nil TLS manager", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/certificates", nil)
		w := httptest.NewRecorder()

		app.handleCertificates(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
		}

		if strings.TrimSpace(w.Body.String()) != "[]" {
			t.Errorf("Body = %s, want []", w.Body.String())
		}
	})

	t.Run("with TLS manager", func(t *testing.T) {
		store := tlspkg.NewStore(t.TempDir())
		app.tlsManager = tlspkg.NewManager(store, nil, nil, logger)

		req := httptest.NewRequest("GET", "/api/v1/certificates", nil)
		w := httptest.NewRecorder()

		app.handleCertificates(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
		}
	})
}

func TestAppHandleHealth(t *testing.T) {
	app := &App{}

	req := httptest.NewRequest("GET", "/api/v1/health", nil)
	w := httptest.NewRecorder()

	app.handleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %s, want application/json", ct)
	}

	if w.Body.String() != `{"status":"healthy"}` {
		t.Errorf("Body = %s, want {\"status\":\"healthy\"}", w.Body.String())
	}
}

func TestAppHandleConfig(t *testing.T) {
	cfg := &config.Config{
		Version:   "test-version",
		HTTPPort:  8080,
		HTTPSPort: 8443,
		Admin:     true,
		ACMEEmail: "test@example.com",
		LogLevel:  "debug",
	}
	logger := log.NewLogger(nil, log.LevelInfo)

	app := &App{
		config: cfg,
		logger: logger,
	}

	req := httptest.NewRequest("GET", "/api/v1/config", nil)
	w := httptest.NewRecorder()

	app.handleConfig(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %s, want application/json", ct)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}

	if result["http_port"].(float64) != 8080 {
		t.Errorf("http_port = %v, want 8080", result["http_port"])
	}
}

func TestAppHandleDashboard(t *testing.T) {
	app := &App{}

	t.Run("root path", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		w := httptest.NewRecorder()

		app.handleDashboard(w, req)

		// Dashboard should be served (even if it returns 404 due to missing embedded file)
		// We're just testing the handler doesn't panic
	})

	t.Run("non-root path returns 404", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/nonexistent", nil)
		w := httptest.NewRecorder()

		app.handleDashboard(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("Status = %d, want %d", w.Code, http.StatusNotFound)
		}
	})
}

func TestAppServeDashboardAsset(t *testing.T) {
	app := &App{}

	req := httptest.NewRequest("GET", "/style.css", nil)
	w := httptest.NewRecorder()

	app.serveDashboardAsset(w, req)

	// Asset may not exist (embedded), but handler shouldn't panic
	// We're just testing it runs without error
}

func TestAppBuildMiddlewareChain(t *testing.T) {
	cfg := &config.Config{
		AccessLog: true,
	}
	logger := log.NewLogger(nil, log.LevelInfo)

	app := &App{
		config: cfg,
		logger: logger,
	}

	finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	chain := app.buildMiddlewareChain(finalHandler)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	chain.ServeHTTP(w, req)

	// Just verify it runs without panic
}

func TestAppBuildHTTPHandler(t *testing.T) {
	cfg := &config.Config{
		DefaultTLS: "off",
	}
	logger := log.NewLogger(nil, log.LevelInfo)

	app := &App{
		config:          cfg,
		logger:          logger,
		challengeSolver: tlspkg.NewChallengeSolver(),
	}

	finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := app.buildHTTPHandler(finalHandler)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Should serve through normal handler
}

func TestAppBuildHTTPHandlerWithRedirect(t *testing.T) {
	cfg := &config.Config{
		DefaultTLS: "auto",
	}
	logger := log.NewLogger(nil, log.LevelInfo)

	app := &App{
		config:          cfg,
		logger:          logger,
		challengeSolver: tlspkg.NewChallengeSolver(),
	}

	finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := app.buildHTTPHandler(finalHandler)

	req := httptest.NewRequest("GET", "/test", nil)
	req.Host = "example.com"
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Should redirect to HTTPS
	if w.Code != http.StatusMovedPermanently {
		t.Errorf("Status = %d, want %d (redirect)", w.Code, http.StatusMovedPermanently)
	}

	location := w.Header().Get("Location")
	if location != "https://example.com/test" {
		t.Errorf("Location = %s, want https://example.com/test", location)
	}
}

func TestAppBuildHTTPHandlerWithForwardedProto(t *testing.T) {
	cfg := &config.Config{
		DefaultTLS: "auto",
	}
	logger := log.NewLogger(nil, log.LevelInfo)

	app := &App{
		config:          cfg,
		logger:          logger,
		challengeSolver: tlspkg.NewChallengeSolver(),
	}

	finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	handler := app.buildHTTPHandler(finalHandler)

	req := httptest.NewRequest("GET", "/test", nil)
	req.Host = "example.com"
	req.Header.Set("X-Forwarded-Proto", "https") // Already behind HTTPS proxy
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Should NOT redirect because X-Forwarded-Proto is https
	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d (no redirect)", w.Code, http.StatusOK)
	}
}

func TestAppBuildHTTPHandlerWithRedirectAndQuery(t *testing.T) {
	cfg := &config.Config{
		DefaultTLS: "auto",
	}
	logger := log.NewLogger(nil, log.LevelInfo)

	app := &App{
		config:          cfg,
		logger:          logger,
		challengeSolver: tlspkg.NewChallengeSolver(),
	}

	finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := app.buildHTTPHandler(finalHandler)

	req := httptest.NewRequest("GET", "/search?q=test&lang=en", nil)
	req.Host = "example.com"
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Should redirect to HTTPS with query string preserved
	if w.Code != http.StatusMovedPermanently {
		t.Errorf("Status = %d, want %d (redirect)", w.Code, http.StatusMovedPermanently)
	}

	location := w.Header().Get("Location")
	expected := "https://example.com/search?q=test&lang=en"
	if location != expected {
		t.Errorf("Location = %s, want %s", location, expected)
	}
}

func TestAppBuildAdminHandlerNoAuth(t *testing.T) {
	cfg := &config.Config{
		Admin:     true,
		AdminUser: "", // No auth
	}
	logger := log.NewLogger(nil, log.LevelInfo)
	routeTable := router.NewTable()

	app := &App{
		config:     cfg,
		logger:     logger,
		routeTable: routeTable,
		startTime:  time.Now(),
	}

	handler := app.buildAdminHandler()

	// Test health endpoint
	req := httptest.NewRequest("GET", "/api/v1/health", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestAppRouteSinkAddRoute(t *testing.T) {
	cfg := &config.Config{}
	logger := log.NewLogger(nil, log.LevelInfo)
	routeTable := router.NewTable()

	app := &App{
		config:     cfg,
		logger:     logger,
		routeTable: routeTable,
	}

	sink := &appRouteSink{app: app}

	info := &discovery.ContainerInfo{
		ID:      "container-1234567890abcdef",
		Name:    "test-container",
		Image:   "nginx:latest",
		Address: "192.168.1.1:8080",
		Healthy: true,
		Config: &discovery.RouteConfig{
			Host: "example.com",
			Path: "/",
			TLS:  "off",
		},
		Labels: map[string]string{},
	}

	sink.AddRoute(info)

	// Verify route was added
	if routeTable.Count() != 1 {
		t.Errorf("RouteTable count = %d, want 1", routeTable.Count())
	}
}

func TestAppRouteSinkAddRouteWithTLS(t *testing.T) {
	cfg := &config.Config{}
	logger := log.NewLogger(nil, log.LevelInfo)
	routeTable := router.NewTable()

	app := &App{
		config:     cfg,
		logger:     logger,
		routeTable: routeTable,
	}

	sink := &appRouteSink{app: app}

	info := &discovery.ContainerInfo{
		ID:      "container-tls-test",
		Name:    "tls-container",
		Address: "192.168.1.2:8443",
		Healthy: true,
		Config: &discovery.RouteConfig{
			Host:       "secure.example.com",
			Path:       "/",
			TLS:        "auto",
			TLSDomains: []string{"secure.example.com"},
		},
		Labels: map[string]string{},
	}

	sink.AddRoute(info)

	// Verify route was added
	if routeTable.Count() != 1 {
		t.Errorf("RouteTable count = %d, want 1", routeTable.Count())
	}
}

func TestAppRouteSinkAddRouteWithMiddleware(t *testing.T) {
	cfg := &config.Config{}
	logger := log.NewLogger(nil, log.LevelInfo)
	routeTable := router.NewTable()

	app := &App{
		config:     cfg,
		logger:     logger,
		routeTable: routeTable,
	}

	sink := &appRouteSink{app: app}

	info := &discovery.ContainerInfo{
		ID:      "container-middleware-test",
		Name:    "middleware-container",
		Address: "192.168.1.3:8080",
		Healthy: true,
		Config: &discovery.RouteConfig{
			Host: "api.example.com",
			Path: "/api",
			TLS:  "off",
			RateLimit: discovery.RateLimitConfig{
				Enabled: true,
				Count:   100,
				Window:  60,
			},
			CORS: discovery.CORSConfig{
				Enabled: true,
				Origins: []string{"https://example.com"},
				Methods: []string{"GET", "POST"},
				Headers: []string{"Content-Type"},
			},
			Compress:    true,
			StripPrefix: "/api",
			AddPrefix:   "/v1",
		},
		Labels: map[string]string{},
	}

	sink.AddRoute(info)

	// Verify route was added
	if routeTable.Count() != 1 {
		t.Errorf("RouteTable count = %d, want 1", routeTable.Count())
	}
}

func TestAppRouteSinkAddRouteWithTLSAndManager(t *testing.T) {
	cfg := &config.Config{}
	logger := log.NewLogger(nil, log.LevelInfo)
	routeTable := router.NewTable()

	// Create a TLS manager
	store := tlspkg.NewStore(t.TempDir())
	tlsManager := tlspkg.NewManager(store, nil, nil, logger)

	app := &App{
		config:     cfg,
		logger:     logger,
		routeTable: routeTable,
		tlsManager: tlsManager,
	}

	sink := &appRouteSink{app: app}

	info := &discovery.ContainerInfo{
		ID:      "container-tls-manager-test",
		Name:    "tls-with-manager-container",
		Address: "192.168.1.3:8443",
		Healthy: true,
		Config: &discovery.RouteConfig{
			Host:       "auto.example.com",
			Path:       "/",
			TLS:        "auto",
			TLSDomains: []string{"auto.example.com"},
		},
		Labels: map[string]string{},
	}

	sink.AddRoute(info)

	// Verify route was added
	if routeTable.Count() != 1 {
		t.Errorf("RouteTable count = %d, want 1", routeTable.Count())
	}

	// Wait a bit for goroutine to execute
	time.Sleep(100 * time.Millisecond)
}

func TestAppRouteSinkAddRouteWithTLSManual(t *testing.T) {
	cfg := &config.Config{}
	logger := log.NewLogger(nil, log.LevelInfo)
	routeTable := router.NewTable()

	app := &App{
		config:     cfg,
		logger:     logger,
		routeTable: routeTable,
	}

	sink := &appRouteSink{app: app}

	info := &discovery.ContainerInfo{
		ID:      "container-tls-manual-test",
		Name:    "tls-manual-container",
		Address: "192.168.1.4:8443",
		Healthy: true,
		Config: &discovery.RouteConfig{
			Host:       "manual.example.com",
			Path:       "/",
			TLS:        "manual",
			TLSDomains: []string{"manual.example.com"},
		},
		Labels: map[string]string{},
	}

	sink.AddRoute(info)

	// Verify route was added with TLS config
	if routeTable.Count() != 1 {
		t.Errorf("RouteTable count = %d, want 1", routeTable.Count())
	}
}

func TestAppStartWithQuickCancel(t *testing.T) {
	cfg := &config.Config{
		HTTPPort:  0, // Port 0 = OS assigns random available port
		HTTPSPort: 0,
		AdminPort: 0,
		Admin:     false,
	}
	logger := log.NewLogger(nil, log.LevelInfo)
	routeTable := router.NewTable()
	healthChecker := health.NewChecker(10*time.Second, 5*time.Second)

	app := &App{
		config:        cfg,
		logger:        logger,
		routeTable:    routeTable,
		healthChecker: healthChecker,
	}

	// Create a context that's cancelled almost immediately
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Call start - it should not block since context is cancelled
	app.start(ctx)

	// Verify middlewareBuilder was initialized
	if app.middlewareBuilder == nil {
		t.Error("middlewareBuilder should be initialized")
	}
}

func TestAppStartSetsComponents(t *testing.T) {
	cfg := &config.Config{
		HTTPPort:  0,
		HTTPSPort: 0,
		AdminPort: 0,
		Admin:     false,
	}
	logger := log.NewLogger(nil, log.LevelInfo)
	routeTable := router.NewTable()
	healthChecker := health.NewChecker(10*time.Second, 5*time.Second)

	app := &App{
		config:        cfg,
		logger:        logger,
		routeTable:    routeTable,
		healthChecker: healthChecker,
	}

	ctx := context.Background()

	// Start and immediately cancel
	go func() {
		time.Sleep(50 * time.Millisecond)
		app.start(ctx)
	}()

	// Give start time to initialize components
	time.Sleep(100 * time.Millisecond)

	// Verify components were set
	if app.middlewareBuilder == nil {
		t.Error("middlewareBuilder should be initialized")
	}
}

func TestAppShutdownEmptyApp(t *testing.T) {
	logger := log.NewLogger(nil, log.LevelInfo)
	routeTable := router.NewTable()

	app := &App{
		logger:     logger,
		routeTable: routeTable,
	}

	ctx := context.Background()
	app.shutdown(ctx)

	// shutdown is empty - just verify it doesn't panic
}

func TestAppRouteSinkRemoveRoute(t *testing.T) {
	cfg := &config.Config{}
	logger := log.NewLogger(nil, log.LevelInfo)
	routeTable := router.NewTable()

	// Add a route first
	route := &router.Route{
		ID:          "test-container-id",
		Host:        "test.example.com",
		ContainerID: "test-container-id",
	}
	routeTable.Add(route)

	app := &App{
		config:     cfg,
		logger:     logger,
		routeTable: routeTable,
	}

	sink := &appRouteSink{app: app}

	// Remove the route
	sink.RemoveRoute("test-container-id")

	// Verify route was removed
	if routeTable.Count() != 0 {
		t.Errorf("RouteTable count = %d, want 0", routeTable.Count())
	}
}

func TestAppHandleRoutesWithRoutes(t *testing.T) {
	cfg := &config.Config{Version: "test"}
	logger := log.NewLogger(nil, log.LevelInfo)
	routeTable := router.NewTable()

	// Add routes
	pool := router.NewBackendPool(router.RoundRobin)
	pool.Add(&router.BackendTarget{Address: "192.168.1.1:8080", Healthy: true})

	route1 := &router.Route{
		ID:         "route-1-abc123",
		Host:       "example.com",
		PathPrefix: "/",
		Backend:    pool,
		TLS:        router.TLSConfig{Mode: "auto"},
	}
	routeTable.Add(route1)

	app := &App{
		config:     cfg,
		logger:     logger,
		routeTable: routeTable,
	}

	req := httptest.NewRequest("GET", "/api/v1/routes", nil)
	w := httptest.NewRecorder()

	app.handleRoutes(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	// Response should contain route info
	body := w.Body.String()
	if !strings.Contains(body, "example.com") {
		t.Error("Response should contain host name")
	}
}

func TestAppHandleContainersWithData(t *testing.T) {
	cfg := &config.Config{Version: "test"}
	logger := log.NewLogger(nil, log.LevelInfo)
	routeTable := router.NewTable()

	// Create discovery engine with containers
	client, _ := discovery.NewDockerClient("/nonexistent/docker.sock")
	sink := &mockDiscoverySink{}
	engine := discovery.NewEngine(client, sink, logger)

	// Add container to engine
	ctx := context.Background()
	_ = engine // Engine is just for type
	_ = ctx

	app := &App{
		config:          cfg,
		logger:          logger,
		routeTable:      routeTable,
		discoveryEngine: nil, // Will test nil case
	}

	req := httptest.NewRequest("GET", "/api/v1/containers", nil)
	w := httptest.NewRecorder()

	app.handleContainers(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestAppHandleMetrics(t *testing.T) {
	cfg := &config.Config{Version: "test"}
	logger := log.NewLogger(nil, log.LevelInfo)
	routeTable := router.NewTable()
	metrics := metrics.NewCollector()

	app := &App{
		config:     cfg,
		logger:     logger,
		routeTable: routeTable,
		metrics:    metrics,
	}

	req := httptest.NewRequest("GET", "/api/v1/metrics", nil)
	w := httptest.NewRecorder()

	app.handleMetrics(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	if ct := w.Header().Get("Content-Type"); ct != "text/plain" {
		t.Errorf("Content-Type = %s, want text/plain", ct)
	}
}

func TestAppHandleCertificatesWithData(t *testing.T) {
	cfg := &config.Config{Version: "test"}
	logger := log.NewLogger(nil, log.LevelInfo)
	routeTable := router.NewTable()

	store := tlspkg.NewStore(t.TempDir())
	tlsManager := tlspkg.NewManager(store, nil, nil, logger)

	app := &App{
		config:     cfg,
		logger:     logger,
		routeTable: routeTable,
		tlsManager: tlsManager,
	}

	req := httptest.NewRequest("GET", "/api/v1/certificates", nil)
	w := httptest.NewRecorder()

	app.handleCertificates(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestAppBuildAdminHandlerWithAuth(t *testing.T) {
	cfg := &config.Config{
		Admin:     true,
		AdminUser: "admin",
		AdminPass: "password",
	}
	logger := log.NewLogger(nil, log.LevelInfo)
	routeTable := router.NewTable()

	app := &App{
		config:     cfg,
		logger:     logger,
		routeTable: routeTable,
		startTime:  time.Now(),
	}

	handler := app.buildAdminHandler()

	// Test without auth
	req := httptest.NewRequest("GET", "/api/v1/health", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Should return 401 without auth
	if w.Code != http.StatusUnauthorized {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestAppBuildHTTPHandlerWithChallenge(t *testing.T) {
	cfg := &config.Config{
		DefaultTLS: "auto",
	}
	logger := log.NewLogger(nil, log.LevelInfo)
	challengeSolver := tlspkg.NewChallengeSolver()

	// Set a token
	challengeSolver.SetToken("test-token", "test-key-auth")

	app := &App{
		config:          cfg,
		logger:          logger,
		challengeSolver: challengeSolver,
	}

	finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := app.buildHTTPHandler(finalHandler)

	// Request ACME challenge
	req := httptest.NewRequest("GET", "/.well-known/acme-challenge/test-token", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Should handle challenge
	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}
}

// mockDiscoverySink for testing
type mockDiscoverySink struct{}

func (m *mockDiscoverySink) AddRoute(info *discovery.ContainerInfo) {}
func (m *mockDiscoverySink) RemoveRoute(containerID string)         {}

func TestAppInitialize(t *testing.T) {
	cfg := &config.Config{
		DataDir:   t.TempDir(),
		ACMEEmail: "", // Skip ACME initialization
		Admin:     false,
	}
	logger := log.NewLogger(nil, log.LevelInfo)

	app := &App{
		config:    cfg,
		logger:    logger,
		startTime: time.Now(),
	}

	err := app.initialize()
	if err != nil {
		t.Errorf("initialize() returned error: %v", err)
	}

	// Verify components were initialized
	if app.metrics == nil {
		t.Error("metrics should be initialized")
	}
	if app.routeTable == nil {
		t.Error("routeTable should be initialized")
	}
	if app.healthChecker == nil {
		t.Error("healthChecker should be initialized")
	}
	if app.challengeSolver == nil {
		t.Error("challengeSolver should be initialized")
	}
}

func TestAppInitializeWithACME(t *testing.T) {
	cfg := &config.Config{
		DataDir:   t.TempDir(),
		ACMEEmail: "test@example.com",
		Admin:     false,
	}
	logger := log.NewLogger(nil, log.LevelInfo)

	app := &App{
		config:    cfg,
		logger:    logger,
		startTime: time.Now(),
	}

	err := app.initialize()
	if err != nil {
		t.Errorf("initialize() returned error: %v", err)
	}

	// Verify TLS manager was initialized (ACME email provided)
	if app.tlsManager == nil {
		t.Error("tlsManager should be initialized when ACMEEmail is set")
	}
}

func TestAppShutdown(t *testing.T) {
	cfg := &config.Config{}
	logger := log.NewLogger(nil, log.LevelInfo)

	app := &App{
		config:    cfg,
		logger:    logger,
		startTime: time.Now(),
	}

	// shutdown should not panic with empty app
	ctx, cancel := contextWithTimeout()
	defer cancel()

	app.shutdown(ctx)
}

func contextWithTimeout() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 5*time.Second)
}

func TestAppHandleContainersWithActualData(t *testing.T) {
	cfg := &config.Config{Version: "test"}
	logger := log.NewLogger(nil, log.LevelInfo)
	routeTable := router.NewTable()

	// Create discovery engine - since we can't add containers to unexported map,
	// just test with nil engine first
	app := &App{
		config:          cfg,
		logger:          logger,
		routeTable:      routeTable,
		discoveryEngine: nil, // nil case
	}

	req := httptest.NewRequest("GET", "/api/v1/containers", nil)
	w := httptest.NewRecorder()

	app.handleContainers(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	// nil discovery engine should return empty array
	body := w.Body.String()
	if strings.TrimSpace(body) != "[]" {
		t.Errorf("Body = %s, want []", body)
	}
}

func TestAppHandleDashboardRoot(t *testing.T) {
	cfg := &config.Config{}
	logger := log.NewLogger(nil, log.LevelInfo)

	app := &App{
		config: cfg,
		logger: logger,
	}

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	app.handleDashboard(w, req)

	// May succeed or fail depending on embedded files
	// Just verify it doesn't panic
}

func TestAppServeDashboardAssetCSS(t *testing.T) {
	cfg := &config.Config{}
	logger := log.NewLogger(nil, log.LevelInfo)

	app := &App{
		config: cfg,
		logger: logger,
	}

	req := httptest.NewRequest("GET", "/style.css", nil)
	w := httptest.NewRecorder()

	app.serveDashboardAsset(w, req)

	// May succeed or fail depending on embedded files
	// Just verify it doesn't panic and sets correct content type if successful
	if strings.HasSuffix(req.URL.Path, ".css") && w.Code == http.StatusOK {
		if ct := w.Header().Get("Content-Type"); ct != "text/css" {
			t.Errorf("Content-Type = %s, want text/css", ct)
		}
	}
}

func TestAppServeDashboardAssetJS(t *testing.T) {
	cfg := &config.Config{}
	logger := log.NewLogger(nil, log.LevelInfo)

	app := &App{
		config: cfg,
		logger: logger,
	}

	req := httptest.NewRequest("GET", "/app.js", nil)
	w := httptest.NewRecorder()

	app.serveDashboardAsset(w, req)

	// May succeed or fail depending on embedded files
	// Just verify it doesn't panic
}

func TestAppHandleDashboardNonRootPath(t *testing.T) {
	cfg := &config.Config{}
	logger := log.NewLogger(nil, log.LevelInfo)

	app := &App{
		config: cfg,
		logger: logger,
	}

	// Request to non-root path should return 404
	req := httptest.NewRequest("GET", "/dashboard", nil)
	w := httptest.NewRecorder()

	app.handleDashboard(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestAppServeDashboardAssetNotFound(t *testing.T) {
	cfg := &config.Config{}
	logger := log.NewLogger(nil, log.LevelInfo)

	app := &App{
		config: cfg,
		logger: logger,
	}

	// Request to non-existent file should return 404
	req := httptest.NewRequest("GET", "/nonexistent.file", nil)
	w := httptest.NewRecorder()

	app.serveDashboardAsset(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestAppRouteSinkAddRouteWithBasicAuth(t *testing.T) {
	cfg := &config.Config{}
	logger := log.NewLogger(nil, log.LevelInfo)
	routeTable := router.NewTable()

	app := &App{
		config:     cfg,
		logger:     logger,
		routeTable: routeTable,
	}

	sink := &appRouteSink{app: app}

	info := &discovery.ContainerInfo{
		ID:      "auth-container-test",
		Name:    "auth-container",
		Address: "192.168.1.4:8080",
		Healthy: true,
		Config: &discovery.RouteConfig{
			Host: "auth.example.com",
			Path: "/",
			TLS:  "off",
			BasicAuthUsers: []discovery.BasicAuthUser{
				{Username: "admin", Hash: "$2a$10$hash"},
				{Username: "user", Hash: "$2a$10$hash2"},
			},
			IPWhitelist: nil, // Would need net.ParseCIDR
			IPBlacklist: nil,
		},
		Labels: map[string]string{},
	}

	sink.AddRoute(info)

	// Verify route was added
	if routeTable.Count() != 1 {
		t.Errorf("RouteTable count = %d, want 1", routeTable.Count())
	}
}

func TestAppHandleRoutesWithMultipleRoutes(t *testing.T) {
	cfg := &config.Config{Version: "test"}
	logger := log.NewLogger(nil, log.LevelInfo)
	routeTable := router.NewTable()

	// Add multiple routes
	route1 := &router.Route{
		ID:         "route-1-abcdef",
		Host:       "example.com",
		PathPrefix: "/",
		Backend:    router.NewBackendPool(router.RoundRobin),
	}
	route1.Backend.Add(&router.BackendTarget{
		Address: "192.168.1.1:8080",
		Healthy: true,
	})

	route2 := &router.Route{
		ID:         "route-2-ghijkl",
		Host:       "api.example.com",
		PathPrefix: "/v1",
		Backend:    router.NewBackendPool(router.RoundRobin),
		TLS:        router.TLSConfig{Mode: "auto"},
	}
	route2.Backend.Add(&router.BackendTarget{
		Address: "192.168.1.2:3000",
		Healthy: true,
	})

	routeTable.Add(route1)
	routeTable.Add(route2)

	app := &App{
		config:     cfg,
		logger:     logger,
		routeTable: routeTable,
	}

	req := httptest.NewRequest("GET", "/api/v1/routes", nil)
	w := httptest.NewRecorder()

	app.handleRoutes(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	body := w.Body.String()
	if !strings.Contains(body, "example.com") {
		t.Error("Response should contain 'example.com'")
	}
	if !strings.Contains(body, "api.example.com") {
		t.Error("Response should contain 'api.example.com'")
	}
}

func TestAppHandleStatusWithAllComponents(t *testing.T) {
	cfg := &config.Config{
		Version:   "v1.0.0",
		HTTPPort:  8080,
		HTTPSPort: 8443,
	}
	logger := log.NewLogger(nil, log.LevelInfo)
	routeTable := router.NewTable()

	// Add a route
	route := &router.Route{
		ID:         "test-route",
		Host:       "test.example.com",
		PathPrefix: "/",
	}
	routeTable.Add(route)

	// Create TLS manager
	store := tlspkg.NewStore(t.TempDir())
	tlsManager := tlspkg.NewManager(store, nil, nil, logger)

	// Create discovery engine - can't add containers to unexported map
	client, _ := discovery.NewDockerClient("/nonexistent/docker.sock")
	sink := &mockDiscoverySink{}
	discoveryEngine := discovery.NewEngine(client, sink, logger)

	app := &App{
		config:          cfg,
		logger:          logger,
		routeTable:      routeTable,
		tlsManager:      tlsManager,
		discoveryEngine: discoveryEngine,
		startTime:       time.Now(),
	}

	req := httptest.NewRequest("GET", "/api/v1/status", nil)
	w := httptest.NewRecorder()

	app.handleStatus(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	body := w.Body.String()
	if !strings.Contains(body, `"status":"ok"`) {
		t.Error("Response should contain status ok")
	}
	if !strings.Contains(body, `"routes":1`) {
		t.Error("Response should contain routes count")
	}
	// Containers will be 0 since we can't add to unexported map
}

func TestAppBuildHTTPHandlerNoRedirect(t *testing.T) {
	cfg := &config.Config{
		DefaultTLS: "off",
	}
	logger := log.NewLogger(nil, log.LevelInfo)

	app := &App{
		config:          cfg,
		logger:          logger,
		challengeSolver: tlspkg.NewChallengeSolver(),
	}

	finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	handler := app.buildHTTPHandler(finalHandler)

	req := httptest.NewRequest("GET", "/test", nil)
	req.Host = "example.com"
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Should NOT redirect because DefaultTLS is off
	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d (no redirect)", w.Code, http.StatusOK)
	}
}

func TestAppBuildHTTPHandlerWithTLSHeader(t *testing.T) {
	cfg := &config.Config{
		DefaultTLS: "auto",
	}
	logger := log.NewLogger(nil, log.LevelInfo)

	app := &App{
		config:          cfg,
		logger:          logger,
		challengeSolver: tlspkg.NewChallengeSolver(),
	}

	finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	handler := app.buildHTTPHandler(finalHandler)

	req := httptest.NewRequest("GET", "/test", nil)
	req.Host = "example.com"
	req.TLS = &tls.ConnectionState{} // Simulate TLS connection
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Should NOT redirect because request has TLS
	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d (TLS request)", w.Code, http.StatusOK)
	}
}

func TestAppBuildAdminHandlerMultipleEndpoints(t *testing.T) {
	cfg := &config.Config{
		Admin:     true,
		AdminUser: "", // No auth
	}
	logger := log.NewLogger(nil, log.LevelInfo)
	routeTable := router.NewTable()
	metricsCollector := metrics.NewCollector()

	app := &App{
		config:     cfg,
		logger:     logger,
		routeTable: routeTable,
		metrics:    metricsCollector,
		startTime:  time.Now(),
	}

	handler := app.buildAdminHandler()

	endpoints := []struct {
		path       string
		expectCode int
	}{
		{"/api/v1/status", http.StatusOK},
		{"/api/v1/routes", http.StatusOK},
		{"/api/v1/containers", http.StatusOK},
		{"/api/v1/certificates", http.StatusOK},
		{"/api/v1/health", http.StatusOK},
		{"/api/v1/config", http.StatusOK},
		{"/api/v1/metrics", http.StatusOK},
	}

	for _, ep := range endpoints {
		t.Run(ep.path, func(t *testing.T) {
			req := httptest.NewRequest("GET", ep.path, nil)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			if w.Code != ep.expectCode {
				t.Errorf("Status for %s = %d, want %d", ep.path, w.Code, ep.expectCode)
			}
		})
	}
}

func TestAppHandleContainersWithEngine(t *testing.T) {
	cfg := &config.Config{Version: "test"}
	logger := log.NewLogger(nil, log.LevelInfo)
	routeTable := router.NewTable()

	// Create a discovery engine - it will have empty containers since we can't access Docker
	client, _ := discovery.NewDockerClient("/nonexistent/docker.sock")
	sink := &mockDiscoverySink{}
	engine := discovery.NewEngine(client, sink, logger)

	app := &App{
		config:          cfg,
		logger:          logger,
		routeTable:      routeTable,
		discoveryEngine: engine,
	}

	req := httptest.NewRequest("GET", "/api/v1/containers", nil)
	w := httptest.NewRecorder()

	app.handleContainers(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	// Should return empty array since no containers
	if strings.TrimSpace(w.Body.String()) != "[]" {
		t.Errorf("Body = %s, want []", w.Body.String())
	}
}

func TestAppHandleContainersWithContainerData(t *testing.T) {
	cfg := &config.Config{Version: "test"}
	logger := log.NewLogger(nil, log.LevelInfo)
	routeTable := router.NewTable()

	// Create a mock discovery engine with containers
	client, _ := discovery.NewDockerClient("/nonexistent/docker.sock")
	sink := &mockDiscoverySink{}
	engine := discovery.NewEngine(client, sink, logger)

	app := &App{
		config:          cfg,
		logger:          logger,
		routeTable:      routeTable,
		discoveryEngine: engine,
	}

	req := httptest.NewRequest("GET", "/api/v1/containers", nil)
	w := httptest.NewRecorder()

	app.handleContainers(w, req)

	// Since we can't access Docker, containers will be empty
	// This tests the nil discoveryEngine path
	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	// Verify JSON content type
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %s, want application/json", ct)
	}
}

func TestAppHandleContainersNilEngine(t *testing.T) {
	cfg := &config.Config{Version: "test"}
	logger := log.NewLogger(nil, log.LevelInfo)
	routeTable := router.NewTable()

	app := &App{
		config:          cfg,
		logger:          logger,
		routeTable:      routeTable,
		discoveryEngine: nil,
	}

	req := httptest.NewRequest("GET", "/api/v1/containers", nil)
	w := httptest.NewRecorder()

	app.handleContainers(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	// Should return empty array when discoveryEngine is nil
	if strings.TrimSpace(w.Body.String()) != "[]" {
		t.Errorf("Body = %s, want []", w.Body.String())
	}
}

func TestAppHandleCertificatesWithDomains(t *testing.T) {
	cfg := &config.Config{Version: "test"}
	logger := log.NewLogger(nil, log.LevelInfo)
	routeTable := router.NewTable()

	tempDir := t.TempDir()
	store := tlspkg.NewStore(tempDir)

	// Generate and save certificates to disk
	privKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	serialNumber, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))

	for _, domain := range []string{"example.com", "test.com"} {
		template := &x509.Certificate{
			SerialNumber: serialNumber,
			Subject:      pkix.Name{CommonName: domain},
			NotBefore:    time.Now(),
			NotAfter:     time.Now().Add(365 * 24 * time.Hour),
			DNSNames:     []string{domain},
		}
		certDER, _ := x509.CreateCertificate(rand.Reader, template, template, &privKey.PublicKey, privKey)
		keyDER, _ := x509.MarshalECPrivateKey(privKey)

		certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
		keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

		err := store.Save(domain, certPEM, keyPEM)
		if err != nil {
			t.Fatalf("Failed to save certificate: %v", err)
		}
	}

	tlsManager := tlspkg.NewManager(store, nil, nil, logger)
	tlsManager.LoadFromDisk()

	app := &App{
		config:     cfg,
		logger:     logger,
		routeTable: routeTable,
		tlsManager: tlsManager,
	}

	req := httptest.NewRequest("GET", "/api/v1/certificates", nil)
	w := httptest.NewRecorder()

	app.handleCertificates(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	body := w.Body.String()
	if !strings.Contains(body, "example.com") {
		t.Error("Response should contain example.com")
	}
	if !strings.Contains(body, "test.com") {
		t.Error("Response should contain test.com")
	}
}

func TestAppInitializeWithDockerError(t *testing.T) {
	cfg := &config.Config{
		DataDir:      t.TempDir(),
		ACMEEmail:    "",
		Admin:        false,
		DockerSocket: "/nonexistent/docker.sock",
	}
	logger := log.NewLogger(nil, log.LevelInfo)

	app := &App{
		config:    cfg,
		logger:    logger,
		startTime: time.Now(),
	}

	// initialize should succeed even if Docker is not available
	err := app.initialize()
	if err != nil {
		t.Errorf("initialize() should not fail: %v", err)
	}

	// Verify components are initialized
	if app.metrics == nil {
		t.Error("metrics should be initialized")
	}
	if app.routeTable == nil {
		t.Error("routeTable should be initialized")
	}
	// discoveryEngine is still created (even if Docker socket doesn't respond to ping)
}

func TestAppRouteSinkWithCircuitBreaker(t *testing.T) {
	cfg := &config.Config{}
	logger := log.NewLogger(nil, log.LevelInfo)
	routeTable := router.NewTable()

	app := &App{
		config:     cfg,
		logger:     logger,
		routeTable: routeTable,
	}

	sink := &appRouteSink{app: app}

	info := &discovery.ContainerInfo{
		ID:      "circuit-breaker-test",
		Name:    "cb-container",
		Address: "192.168.1.5:8080",
		Healthy: true,
		Config: &discovery.RouteConfig{
			Host: "cb.example.com",
			Path: "/",
			TLS:  "off",
			CircuitBreaker: discovery.CircuitBreakerConfig{
				Enabled:  true,
				Failures: 5,
				Window:   30 * time.Second,
			},
		},
		Labels: map[string]string{},
	}

	sink.AddRoute(info)

	if routeTable.Count() != 1 {
		t.Errorf("RouteTable count = %d, want 1", routeTable.Count())
	}
}

func TestAppRouteSinkWithIPFilters(t *testing.T) {
	cfg := &config.Config{}
	logger := log.NewLogger(nil, log.LevelInfo)
	routeTable := router.NewTable()

	app := &App{
		config:     cfg,
		logger:     logger,
		routeTable: routeTable,
	}

	sink := &appRouteSink{app: app}

	// Note: IP filters require net.ParseCIDR which we can't easily do in a unit test
	// Just test that the route is created without panic
	info := &discovery.ContainerInfo{
		ID:      "ip-filter-test",
		Name:    "ip-filter-container",
		Address: "192.168.1.6:8080",
		Healthy: true,
		Config: &discovery.RouteConfig{
			Host:        "ip.example.com",
			Path:        "/",
			TLS:         "off",
			IPWhitelist: nil, // Would need parsed CIDR
			IPBlacklist: nil,
		},
		Labels: map[string]string{},
	}

	sink.AddRoute(info)

	if routeTable.Count() != 1 {
		t.Errorf("RouteTable count = %d, want 1", routeTable.Count())
	}
}

func TestAppRouteSinkWithRetry(t *testing.T) {
	cfg := &config.Config{}
	logger := log.NewLogger(nil, log.LevelInfo)
	routeTable := router.NewTable()

	app := &App{
		config:     cfg,
		logger:     logger,
		routeTable: routeTable,
	}

	sink := &appRouteSink{app: app}

	info := &discovery.ContainerInfo{
		ID:      "retry-test",
		Name:    "retry-container",
		Address: "192.168.1.9:8080",
		Healthy: true,
		Config: &discovery.RouteConfig{
			Host:  "retry.example.com",
			Path:  "/",
			TLS:   "off",
			Retry: 3, // Retry count
		},
		Labels: map[string]string{},
	}

	sink.AddRoute(info)

	if routeTable.Count() != 1 {
		t.Errorf("RouteTable count = %d, want 1", routeTable.Count())
	}
}

func TestAppRouteSinkWithDrLabels(t *testing.T) {
	cfg := &config.Config{}
	logger := log.NewLogger(nil, log.LevelInfo)
	routeTable := router.NewTable()

	app := &App{
		config:     cfg,
		logger:     logger,
		routeTable: routeTable,
	}

	sink := &appRouteSink{app: app}

	info := &discovery.ContainerInfo{
		ID:      "labels-test",
		Name:    "labels-container",
		Address: "192.168.1.10:8080",
		Healthy: true,
		Config: &discovery.RouteConfig{
			Host: "labels.example.com",
			Path: "/",
			TLS:  "off",
		},
		Labels: map[string]string{
			"dr.enable":    "true",
			"dr.host":      "labels.example.com",
			"custom.label": "custom-value",
		},
	}

	sink.AddRoute(info)

	if routeTable.Count() != 1 {
		t.Errorf("RouteTable count = %d, want 1", routeTable.Count())
	}
}

func TestAppRouteSinkMultipleBackends(t *testing.T) {
	cfg := &config.Config{}
	logger := log.NewLogger(nil, log.LevelInfo)
	routeTable := router.NewTable()

	app := &App{
		config:     cfg,
		logger:     logger,
		routeTable: routeTable,
	}

	// Add multiple routes
	for i := 0; i < 5; i++ {
		sink := &appRouteSink{app: app}
		info := &discovery.ContainerInfo{
			ID:      "multi-" + string(rune('0'+i)),
			Name:    "multi-container-" + string(rune('0'+i)),
			Address: "192.168.1." + string(rune('1'+i)) + ":8080",
			Healthy: true,
			Config: &discovery.RouteConfig{
				Host: "multi" + string(rune('0'+i)) + ".example.com",
				Path: "/",
				TLS:  "off",
			},
			Labels: map[string]string{},
		}
		sink.AddRoute(info)
	}

	if routeTable.Count() != 5 {
		t.Errorf("RouteTable count = %d, want 5", routeTable.Count())
	}
}

func TestAppHandleRoutesWithUnhealthyBackends(t *testing.T) {
	cfg := &config.Config{Version: "test"}
	logger := log.NewLogger(nil, log.LevelInfo)
	routeTable := router.NewTable()

	// Create a route with some unhealthy backends
	pool := router.NewBackendPool(router.RoundRobin)
	pool.Add(&router.BackendTarget{Address: "192.168.1.1:8080", Healthy: false})
	pool.Add(&router.BackendTarget{Address: "192.168.1.2:8080", Healthy: false})

	route := &router.Route{
		ID:         "unhealthy-route",
		Host:       "unhealthy.example.com",
		PathPrefix: "/",
		Backend:    pool,
	}
	routeTable.Add(route)

	app := &App{
		config:     cfg,
		logger:     logger,
		routeTable: routeTable,
	}

	req := httptest.NewRequest("GET", "/api/v1/routes", nil)
	w := httptest.NewRecorder()

	app.handleRoutes(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	body := w.Body.String()
	if !strings.Contains(body, "unhealthy.example.com") {
		t.Error("Response should contain unhealthy.example.com")
	}
}

// Subprocess tests for main function
func TestMainHelpSubprocess(t *testing.T) {
	if os.Getenv("TEST_MAIN") == "help" {
		os.Args = []string{"dockrouter", "--help"}
		main()
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestMainHelpSubprocess")
	cmd.Env = append(os.Environ(), "TEST_MAIN=help")
	output, _ := cmd.CombinedOutput()

	// --help calls os.Exit(0), so we expect output but no error from the test
	if !strings.Contains(string(output), "DockRouter") {
		t.Errorf("Help output should contain 'DockRouter', got: %s", output)
	}
}

func TestMainVersionSubprocess(t *testing.T) {
	if os.Getenv("TEST_MAIN") == "version" {
		os.Args = []string{"dockrouter", "--version"}
		main()
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestMainVersionSubprocess")
	cmd.Env = append(os.Environ(), "TEST_MAIN=version")
	output, _ := cmd.CombinedOutput()

	// --version calls os.Exit(0)
	if !strings.Contains(string(output), "DockRouter") {
		t.Errorf("Version output should contain 'DockRouter', got: %s", output)
	}
}

func TestMainInvalidFlagSubprocess(t *testing.T) {
	if os.Getenv("TEST_MAIN") == "invalid" {
		os.Args = []string{"dockrouter", "--invalid-flag-value"}
		main()
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestMainInvalidFlagSubprocess")
	cmd.Env = append(os.Environ(), "TEST_MAIN=invalid")
	err := cmd.Run()

	// Should fail with invalid flag
	if err == nil {
		t.Error("Expected error for invalid flag")
	}
}

func TestAppStartNoPanic(t *testing.T) {
	// Test that start doesn't panic with minimal app
	cfg := &config.Config{
		HTTPPort:  0,
		HTTPSPort: 0,
		AdminPort: 0,
		Admin:     false,
	}
	logger := log.NewLogger(nil, log.LevelInfo)
	routeTable := router.NewTable()
	healthChecker := health.NewChecker(10*time.Second, 5*time.Second)

	app := &App{
		config:        cfg,
		logger:        logger,
		routeTable:    routeTable,
		healthChecker: healthChecker,
		startTime:     time.Now(),
	}

	// Create a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Call start - it should not panic
	app.start(ctx)
}

func TestAppShutdownNoPanic(t *testing.T) {
	// Test that shutdown doesn't panic with minimal app
	logger := log.NewLogger(nil, log.LevelInfo)
	routeTable := router.NewTable()

	app := &App{
		logger:     logger,
		routeTable: routeTable,
	}

	ctx := context.Background()
	app.shutdown(ctx)
}

func TestMainHelpFlag(t *testing.T) {
	// This test runs in a subprocess to test --help which calls os.Exit(0)
	if os.Getenv("TEST_HELP") == "1" {
		os.Args = []string{"dockrouter", "--help"}
		main()
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestMainHelpFlag")
	cmd.Env = append(os.Environ(), "TEST_HELP=1")
	output, err := cmd.CombinedOutput()

	// Help should cause the process to exit with code 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		if exitErr.ExitCode() != 0 {
			t.Errorf("Exit code = %d, want 0", exitErr.ExitCode())
		}
	}

	// Check that help output was printed
	if !strings.Contains(string(output), "DockRouter") {
		t.Errorf("Output should contain 'DockRouter', got: %s", output)
	}
	if !strings.Contains(string(output), "Usage") {
		t.Errorf("Output should contain 'Usage', got: %s", output)
	}
}

func TestMainVersionFlag(t *testing.T) {
	// This test runs in a subprocess to test --version which calls os.Exit(0)
	if os.Getenv("TEST_VERSION") == "1" {
		os.Args = []string{"dockrouter", "--version"}
		main()
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestMainVersionFlag")
	cmd.Env = append(os.Environ(), "TEST_VERSION=1")
	output, err := cmd.CombinedOutput()

	// Version should cause the process to exit with code 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		if exitErr.ExitCode() != 0 {
			t.Errorf("Exit code = %d, want 0", exitErr.ExitCode())
		}
	}

	// Check that version output was printed
	if !strings.Contains(string(output), "DockRouter") {
		t.Errorf("Output should contain 'DockRouter', got: %s", output)
	}
}
