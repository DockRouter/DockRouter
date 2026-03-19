package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
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

// --- doHealthCheck pattern test ---

func TestHealthCheckHTTPPattern(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/health" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"ok"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(server.URL + "/api/v1/health")
	if err != nil {
		t.Fatalf("health check request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

func TestHealthCheckHTTPFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(server.URL + "/api/v1/health")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Error("should not be OK")
	}
}

func TestHealthCheckConnectionRefused(t *testing.T) {
	client := &http.Client{Timeout: 500 * time.Millisecond}
	_, err := client.Get("http://127.0.0.1:19999/api/v1/health")
	if err == nil {
		t.Error("should error when server is not running")
	}
}

// --- printVersion variables test ---

func TestVersionVariables(t *testing.T) {
	old := version
	oldBT := buildTime
	defer func() {
		version = old
		buildTime = oldBT
	}()

	version = "1.2.3"
	buildTime = "2025-01-01"

	if version != "1.2.3" {
		t.Error("version")
	}
	if buildTime != "2025-01-01" {
		t.Error("buildTime")
	}
}

// --- shutdown ---

func TestShutdownNilServers(t *testing.T) {
	logger := log.NewLogger(nil, log.LevelInfo)
	app := &App{
		logger:      logger,
		httpServer:  nil,
		httpsServer: nil,
		adminServer: nil,
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	app.shutdown(ctx)
}

func TestShutdownWithServers(t *testing.T) {
	logger := log.NewLogger(nil, log.LevelInfo)

	httpLn, _ := net.Listen("tcp", "127.0.0.1:0")
	httpsLn, _ := net.Listen("tcp", "127.0.0.1:0")
	adminLn, _ := net.Listen("tcp", "127.0.0.1:0")

	httpServer := &http.Server{Handler: http.DefaultServeMux}
	httpsServer := &http.Server{Handler: http.DefaultServeMux}
	adminServer := &http.Server{Handler: http.DefaultServeMux}

	go httpServer.Serve(httpLn)
	go httpsServer.Serve(httpsLn)
	go adminServer.Serve(adminLn)

	app := &App{
		logger:      logger,
		httpServer:  httpServer,
		httpsServer: httpsServer,
		adminServer: adminServer,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	app.shutdown(ctx)
}

// --- buildMiddlewareChain ---

func TestBuildMiddlewareChainWithAccessLog(t *testing.T) {
	logger := log.NewLogger(nil, log.LevelInfo)
	app := &App{
		logger: logger,
		config: &config.Config{AccessLog: true},
	}

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := app.buildMiddlewareChain(inner)
	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d", rec.Code)
	}
}

func TestBuildMiddlewareChainWithoutAccessLog(t *testing.T) {
	logger := log.NewLogger(nil, log.LevelInfo)
	app := &App{
		logger: logger,
		config: &config.Config{AccessLog: false},
	}

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := app.buildMiddlewareChain(inner)
	if handler == nil {
		t.Fatal("nil handler")
	}
}

// --- buildHTTPHandler ---

func TestBuildHTTPHandlerHTTPSRedirect(t *testing.T) {
	logger := log.NewLogger(nil, log.LevelInfo)
	challengeSolver := tlspkg.NewChallengeSolver()

	app := &App{
		logger:          logger,
		config:          &config.Config{DefaultTLS: "auto"},
		challengeSolver: challengeSolver,
	}

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := app.buildHTTPHandler(inner)

	req := httptest.NewRequest("GET", "http://example.com/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMovedPermanently {
		t.Errorf("status = %d, want 301", rec.Code)
	}
	if loc := rec.Header().Get("Location"); !strings.HasPrefix(loc, "https://") {
		t.Errorf("Location = %s", loc)
	}
}

func TestBuildHTTPHandlerRedirectWithQuery(t *testing.T) {
	logger := log.NewLogger(nil, log.LevelInfo)
	challengeSolver := tlspkg.NewChallengeSolver()

	app := &App{
		logger:          logger,
		config:          &config.Config{DefaultTLS: "auto"},
		challengeSolver: challengeSolver,
	}

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	handler := app.buildHTTPHandler(inner)

	req := httptest.NewRequest("GET", "http://example.com/test?foo=bar", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if loc := rec.Header().Get("Location"); !strings.Contains(loc, "?foo=bar") {
		t.Errorf("Location = %s, should include query", loc)
	}
}

func TestBuildHTTPHandlerNoRedirectTLSOff(t *testing.T) {
	logger := log.NewLogger(nil, log.LevelInfo)
	challengeSolver := tlspkg.NewChallengeSolver()

	app := &App{
		logger:          logger,
		config:          &config.Config{DefaultTLS: "off"},
		challengeSolver: challengeSolver,
	}

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := app.buildHTTPHandler(inner)
	req := httptest.NewRequest("GET", "http://example.com/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestBuildHTTPHandlerXForwardedProto(t *testing.T) {
	logger := log.NewLogger(nil, log.LevelInfo)
	challengeSolver := tlspkg.NewChallengeSolver()

	app := &App{
		logger:          logger,
		config:          &config.Config{DefaultTLS: "auto"},
		challengeSolver: challengeSolver,
	}

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := app.buildHTTPHandler(inner)

	req := httptest.NewRequest("GET", "http://example.com/test", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

// --- handleContainers nil engine ---

func TestHandleContainersNilDiscovery(t *testing.T) {
	logger := log.NewLogger(nil, log.LevelInfo)
	app := &App{
		logger:          logger,
		discoveryEngine: nil,
	}

	req := httptest.NewRequest("GET", "/api/v1/containers", nil)
	rec := httptest.NewRecorder()
	app.handleContainers(rec, req)

	if rec.Body.String() != "[]" {
		t.Errorf("body = %s, want []", rec.Body.String())
	}
}

// --- handleContainers with discovery engine ---

func TestHandleContainersWithDiscoveryEngine(t *testing.T) {
	logger := log.NewLogger(nil, log.LevelInfo)

	app := &App{
		logger:          logger,
		discoveryEngine: nil, // Will use the nil case which returns "[]"
	}

	req := httptest.NewRequest("GET", "/api/v1/containers", nil)
	rec := httptest.NewRecorder()
	app.handleContainers(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d", rec.Code)
	}

	// Should return an empty array since no containers discovered
	body := rec.Body.String()
	if body != "[]" {
		t.Errorf("body should be empty array: %s", body)
	}
}

// --- handleRoutes empty ---

func TestHandleRoutesEmpty(t *testing.T) {
	logger := log.NewLogger(nil, log.LevelInfo)
	rt := router.NewTable()

	app := &App{
		logger:     logger,
		routeTable: rt,
	}

	req := httptest.NewRequest("GET", "/api/v1/routes", nil)
	rec := httptest.NewRecorder()
	app.handleRoutes(rec, req)

	if rec.Body.String() != "[]" {
		t.Errorf("body = %s, want []", rec.Body.String())
	}
}

// --- handleDashboard ---

func TestHandleDashboardRoot(t *testing.T) {
	logger := log.NewLogger(nil, log.LevelInfo)
	app := &App{logger: logger}

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	app.handleDashboard(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "DockRouter") {
		t.Error("should contain DockRouter")
	}
}

func TestHandleDashboardNotRoot(t *testing.T) {
	logger := log.NewLogger(nil, log.LevelInfo)
	app := &App{logger: logger}

	req := httptest.NewRequest("GET", "/other", nil)
	rec := httptest.NewRecorder()
	app.handleDashboard(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

// --- serveDashboardAsset ---

func TestServeDashboardAssetCSS(t *testing.T) {
	app := &App{logger: log.NewLogger(nil, log.LevelInfo)}

	req := httptest.NewRequest("GET", "/style.css", nil)
	rec := httptest.NewRecorder()
	app.serveDashboardAsset(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/css" {
		t.Errorf("Content-Type = %s", ct)
	}
}

func TestServeDashboardAssetJS(t *testing.T) {
	app := &App{logger: log.NewLogger(nil, log.LevelInfo)}

	req := httptest.NewRequest("GET", "/app.js", nil)
	rec := httptest.NewRecorder()
	app.serveDashboardAsset(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/javascript" {
		t.Errorf("Content-Type = %s", ct)
	}
}

func TestServeDashboardAssetNotFound(t *testing.T) {
	app := &App{logger: log.NewLogger(nil, log.LevelInfo)}

	req := httptest.NewRequest("GET", "/nonexistent.xyz", nil)
	rec := httptest.NewRecorder()
	app.serveDashboardAsset(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

// --- appRouteSink ---

func TestAppRouteSinkAddRouteComplete(t *testing.T) {
	logger := log.NewLogger(nil, log.LevelInfo)
	rt := router.NewTable()

	app := &App{
		logger:     logger,
		routeTable: rt,
	}
	sink := &appRouteSink{app: app}

	info := &discovery.ContainerInfo{
		ID:      "abc123def4567890123456789012345678901234567890123456789012345678",
		Name:    "test-app",
		Address: "172.17.0.5:8080",
		Healthy: true,
		Config: &discovery.RouteConfig{
			Enabled:     true,
			Host:        "test.example.com",
			Path:        "/api",
			LoadBalance: "roundrobin",
			Weight:      1,
			TLS:         "off",
			RateLimit:   discovery.RateLimitConfig{Enabled: true, Count: 100, Window: time.Minute, ByKey: "client_ip"},
			CORS:        discovery.CORSConfig{Enabled: true, Origins: []string{"*"}, Methods: []string{"GET"}},
			Compress:    true,
			StripPrefix: "/api",
			AddPrefix:   "/v2",
			MaxBody:     10 * 1024 * 1024,
			Retry:       3,
			CircuitBreaker: discovery.CircuitBreakerConfig{Enabled: true, Failures: 5, Window: 30 * time.Second},
			BasicAuthUsers: []discovery.BasicAuthUser{{Username: "admin", Hash: "hash"}},
		},
	}

	sink.AddRoute(info)

	routes := rt.List()
	if len(routes) != 1 {
		t.Fatalf("expected 1 route, got %d", len(routes))
	}
	if routes[0].Host != "test.example.com" {
		t.Errorf("host = %s", routes[0].Host)
	}
}

func TestAppRouteSinkAddRouteWithTLSAuto(t *testing.T) {
	logger := log.NewLogger(nil, log.LevelInfo)
	rt := router.NewTable()

	app := &App{
		logger:     logger,
		routeTable: rt,
		tlsManager: nil,
	}
	sink := &appRouteSink{app: app}

	info := &discovery.ContainerInfo{
		ID:      "def456789abc1230123456789012345678901234567890123456789012345678",
		Name:    "tls-app",
		Address: "172.17.0.6:443",
		Healthy: true,
		Config: &discovery.RouteConfig{
			Enabled:     true,
			Host:        "secure.example.com",
			LoadBalance: "roundrobin",
			Weight:      1,
			TLS:         "auto",
			TLSDomains:  []string{"secure.example.com"},
		},
	}

	sink.AddRoute(info)

	routes := rt.List()
	if len(routes) != 1 {
		t.Fatalf("expected 1 route, got %d", len(routes))
	}
	if routes[0].TLS.Mode != "auto" {
		t.Errorf("TLS mode = %s", routes[0].TLS.Mode)
	}
}

func TestAppRouteSinkRemoveRouteAndVerify(t *testing.T) {
	logger := log.NewLogger(nil, log.LevelInfo)
	rt := router.NewTable()

	app := &App{
		logger:     logger,
		routeTable: rt,
	}
	sink := &appRouteSink{app: app}

	containerID := "abc123def4567890123456789012345678901234567890123456789012345678"
	info := &discovery.ContainerInfo{
		ID:      containerID,
		Name:    "test-app",
		Address: "172.17.0.5:8080",
		Healthy: true,
		Config: &discovery.RouteConfig{
			Enabled: true, Host: "test.example.com",
			LoadBalance: "roundrobin", Weight: 1, TLS: "off",
		},
	}

	sink.AddRoute(info)
	if routes := rt.List(); len(routes) != 1 {
		t.Fatalf("expected 1 route before remove, got %d", len(routes))
	}
	sink.RemoveRoute(containerID)

	if routes := rt.List(); len(routes) != 0 {
		t.Errorf("expected 0 routes after remove, got %d", len(routes))
	}
}

// --- App.start cancelled context ---

func TestAppStartCancelledCtx(t *testing.T) {
	logger := log.NewLogger(nil, log.LevelInfo)
	rt := router.NewTable()
	challengeSolver := tlspkg.NewChallengeSolver()
	checker := health.NewChecker(10*time.Second, 5*time.Second)

	app := &App{
		logger:          logger,
		config:          &config.Config{HTTPPort: 0, HTTPSPort: 0, AdminPort: 0, AccessLog: true, Admin: true, DefaultTLS: "off"},
		routeTable:      rt,
		challengeSolver: challengeSolver,
		healthChecker:   checker,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	app.start(ctx)
}

// --- printVersion output test ---

func TestPrintVersionOutput(t *testing.T) {
	oldVersion := version
	oldBuildTime := buildTime
	defer func() {
		version = oldVersion
		buildTime = oldBuildTime
	}()

	version = "1.2.3-test"
	buildTime = "2025-01-15T10:30:00Z"

	// Capture stdout
	output := captureOutput(func() {
		printVersion()
	})

	if !strings.Contains(output, "1.2.3-test") {
		t.Errorf("output missing version: %s", output)
	}
	if !strings.Contains(output, "2025-01-15T10:30:00Z") {
		t.Errorf("output missing build time: %s", output)
	}
	if !strings.Contains(output, "DockRouter") {
		t.Errorf("output missing project name: %s", output)
	}
	if !strings.Contains(output, "github.com/DockRouter") {
		t.Errorf("output missing URL: %s", output)
	}
}

// captureOutput captures stdout during function execution
func captureOutput(f func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	f()

	w.Close()
	os.Stdout = old

	var buf strings.Builder
	io.Copy(&buf, r)
	return buf.String()
}

// --- handleCertificates tests ---

func TestHandleCertificatesNilManager(t *testing.T) {
	logger := log.NewLogger(nil, log.LevelInfo)
	app := &App{
		logger:     logger,
		tlsManager: nil,
	}

	req := httptest.NewRequest("GET", "/api/v1/certificates", nil)
	rec := httptest.NewRecorder()
	app.handleCertificates(rec, req)

	if rec.Body.String() != "[]" {
		t.Errorf("body = %s, want []", rec.Body.String())
	}
}

// --- App.initialize tests ---

func TestAppInitializeWithTLS(t *testing.T) {
	logger := log.NewLogger(nil, log.LevelInfo)
	app := &App{
		logger: logger,
		config: &config.Config{
			HTTPPort:    0,
			HTTPSPort:   0,
			ACMEEmail:   "", // Empty ACME email skips TLS manager initialization
			AccessLog:   false,
			LogLevel:    "info",
			DataDir:     t.TempDir(),
			DefaultTLS:  "off",
		},
	}

	err := app.initialize()
	if err != nil {
		t.Fatalf("initialize failed: %v", err)
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

// --- handleConfig test ---

func TestHandleConfig(t *testing.T) {
	logger := log.NewLogger(nil, log.LevelInfo)
	app := &App{
		logger: logger,
		config: &config.Config{
			HTTPPort:  8080,
			HTTPSPort: 8443,
			Admin:     true,
			ACMEEmail: "test@example.com",
			LogLevel:  "debug",
		},
	}

	req := httptest.NewRequest("GET", "/api/v1/config", nil)
	rec := httptest.NewRecorder()
	app.handleConfig(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "8080") {
		t.Errorf("body should contain HTTP port: %s", body)
	}
	if !strings.Contains(body, "8443") {
		t.Errorf("body should contain HTTPS port: %s", body)
	}
}

// --- handleHealth test ---

func TestHandleHealth(t *testing.T) {
	logger := log.NewLogger(nil, log.LevelInfo)
	app := &App{
		logger: logger,
	}

	req := httptest.NewRequest("GET", "/api/v1/health", nil)
	rec := httptest.NewRecorder()
	app.handleHealth(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d", rec.Code)
	}

	if !strings.Contains(rec.Body.String(), "healthy") {
		t.Errorf("body should contain healthy: %s", rec.Body.String())
	}
}

// --- handleMetrics test ---

func TestHandleMetrics(t *testing.T) {
	logger := log.NewLogger(nil, log.LevelInfo)
	app := &App{
		logger:  logger,
		metrics: metrics.NewCollector(),
	}

	req := httptest.NewRequest("GET", "/api/v1/metrics", nil)
	rec := httptest.NewRecorder()
	app.handleMetrics(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d", rec.Code)
	}

	if ct := rec.Header().Get("Content-Type"); ct != "text/plain" {
		t.Errorf("Content-Type = %s", ct)
	}
}
