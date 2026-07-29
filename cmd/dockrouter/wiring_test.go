package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DockRouter/dockrouter/internal/config"
	"github.com/DockRouter/dockrouter/internal/discovery"
	"github.com/DockRouter/dockrouter/internal/log"
	"github.com/DockRouter/dockrouter/internal/metrics"
	"github.com/DockRouter/dockrouter/internal/router"
	tlspkg "github.com/DockRouter/dockrouter/internal/tls"
)

func testApp(cfg *config.Config) *App {
	return &App{
		config:     cfg,
		logger:     log.NewLogger(nil, log.LevelError),
		routeTable: router.NewTable(),
		metrics:    metrics.NewCollector(),
		startTime:  time.Now(),
	}
}

// A route that opted out of TLS must be served over plain HTTP rather than
// redirected to a port that holds no certificate for it.
func TestShouldRedirectToHTTPSRespectsRouteMode(t *testing.T) {
	tests := []struct {
		name       string
		routeMode  string
		globalMode string
		want       bool
	}{
		{"route opts out", "off", "auto", false},
		{"route opts in", "auto", "off", true},
		{"route manual", "manual", "off", true},
		{"route inherits global auto", "", "auto", true},
		{"route inherits global off", "", "off", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := testApp(&config.Config{DefaultTLS: tt.globalMode})
			route := &router.Route{TLS: router.TLSConfig{Mode: tt.routeMode}}

			if got := app.shouldRedirectToHTTPS(route); got != tt.want {
				t.Errorf("shouldRedirectToHTTPS() = %v, want %v", got, tt.want)
			}
		})
	}
}

// End-to-end through buildHTTPHandler: dr.tls=off must reach the backend
// handler instead of receiving a 301.
func TestBuildHTTPHandlerDoesNotRedirectTLSOffRoute(t *testing.T) {
	app := testApp(&config.Config{DefaultTLS: "auto"})
	app.challengeSolver = tlspkg.NewChallengeSolver()

	pool := router.NewBackendPool(router.RoundRobin)
	app.routeTable.Upsert(&router.Route{
		Host: "plain.example.com", PathPrefix: "/",
		Backend: pool,
		TLS:     router.TLSConfig{Mode: "off"},
	}, &router.BackendTarget{Address: "10.0.0.1:80", ContainerID: "c1", Healthy: true})

	reached := false
	handler := app.buildHTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.Host = "plain.example.com"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code == http.StatusMovedPermanently {
		t.Error("a dr.tls=off route was redirected to HTTPS")
	}
	if !reached {
		t.Errorf("request never reached the handler (status %d)", rec.Code)
	}
}

func TestHandleReady(t *testing.T) {
	t.Run("ready without discovery configured", func(t *testing.T) {
		app := testApp(&config.Config{})
		rec := httptest.NewRecorder()
		app.handleReady(rec, httptest.NewRequest("GET", "/ready", nil))

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want 200", rec.Code)
		}

		var body map[string]interface{}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if body["ready"] != true {
			t.Errorf("ready = %v, want true", body["ready"])
		}
	})

	t.Run("not ready while discovery is down", func(t *testing.T) {
		app := testApp(&config.Config{})
		// A discovery engine that was created but never started successfully,
		// which is what happens when the Docker socket is unreachable.
		app.discoveryEngine = discovery.NewEngine(nil, nil, log.NewLogger(nil, log.LevelError))

		rec := httptest.NewRecorder()
		app.handleReady(rec, httptest.NewRequest("GET", "/ready", nil))

		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("status = %d, want 503 while discovery is down", rec.Code)
		}
	})
}

// Probe endpoints must stay reachable without credentials so that the Docker
// HEALTHCHECK and orchestrator probes do not fail against an authenticated
// admin server.
func TestAdminProbesBypassAuthButAPIDoesNot(t *testing.T) {
	app := testApp(&config.Config{
		Admin: true, AdminUser: "admin", AdminPass: "secret",
	})
	handler := app.buildAdminHandler()

	tests := []struct {
		path       string
		wantStatus int
	}{
		{"/health", http.StatusOK},
		{"/ready", http.StatusOK},
		{"/api/v1/status", http.StatusUnauthorized},
		{"/api/v1/health", http.StatusUnauthorized},
		{"/metrics", http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest("GET", tt.path, nil))
			if rec.Code != tt.wantStatus {
				t.Errorf("GET %s = %d, want %d", tt.path, rec.Code, tt.wantStatus)
			}
		})
	}
}

func TestMetricsIncludeStateGauges(t *testing.T) {
	app := testApp(&config.Config{})

	pool := router.NewBackendPool(router.RoundRobin)
	app.routeTable.Upsert(&router.Route{
		Host: "app.example.com", PathPrefix: "/", Backend: pool,
	}, &router.BackendTarget{Address: "10.0.0.1:80", ContainerID: "c1", Healthy: true})

	pool.RecordRequest("10.0.0.1:80")
	pool.RecordFailure("10.0.0.1:80")

	rec := httptest.NewRecorder()
	app.handleMetrics(rec, httptest.NewRequest("GET", "/metrics", nil))

	body := rec.Body.String()
	for _, want := range []string{
		"dockrouter_routes_total 1",
		"dockrouter_backend_requests_total 1",
		"dockrouter_backend_errors_total 1",
		"dockrouter_containers_total",
		"dockrouter_certificates_total",
		"dockrouter_active_connections",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics output missing %q\ngot:\n%s", want, body)
		}
	}
}

func TestHealthCheckEndpointHonoursAdminPort(t *testing.T) {
	tests := []struct {
		name       string
		port, bind string
		want       string
	}{
		{"no override uses default", "", "", healthCheckURL},
		{"custom port", "9999", "", "http://localhost:9999/health"},
		{"custom bind and port", "9999", "192.168.1.5", "http://192.168.1.5:9999/health"},
		{"wildcard bind resolves to localhost", "9999", "0.0.0.0", "http://localhost:9999/health"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("DR_ADMIN_PORT", tt.port)
			t.Setenv("DR_ADMIN_BIND", tt.bind)

			if got := healthCheckEndpoint(); got != tt.want {
				t.Errorf("healthCheckEndpoint() = %q, want %q", got, tt.want)
			}
		})
	}
}
