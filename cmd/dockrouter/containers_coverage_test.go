package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DockRouter/dockrouter/internal/discovery"
	"github.com/DockRouter/dockrouter/internal/log"
)

// TestHandleContainersWithRealContainers tests handleContainers with populated discovery engine.
// We construct a discovery.Engine directly and inject containers into its map.
func TestHandleContainersWithRealContainers(t *testing.T) {
	logger := log.NewLogger(nil, log.LevelInfo)
	client, _ := discovery.NewDockerClient("")
	sink := &noopRouteSink{}

	engine := discovery.NewEngine(client, sink, &testDiscLogger{})

	// Inject containers directly into the engine's map
	engine.InjectContainerForTest(&discovery.ContainerInfo{
		ID:      "abc123def4567890123456789012345678901234567890123456789012345678",
		Name:    "api-service",
		Image:   "nginx:alpine",
		Address: "172.17.0.5:8080",
		Healthy: true,
		Labels: map[string]string{
			"dr.enable": "true",
			"dr.host":   "api.example.com",
			"dr.port":   "8080",
			"dr.tls":    "auto",
		},
		Config: &discovery.RouteConfig{
			Enabled: true,
			Host:    "api.example.com",
			Path:    "/",
			TLS:     "auto",
		},
	})

	engine.InjectContainerForTest(&discovery.ContainerInfo{
		ID:      "def456789abc1230123456789012345678901234567890123456789012345678",
		Name:    "web-frontend",
		Image:   "node:18",
		Address: "172.17.0.6:3000",
		Healthy: false,
		Labels: map[string]string{
			"dr.enable": "true",
			"dr.host":   "www.example.com",
		},
		Config: &discovery.RouteConfig{
			Enabled: true,
			Host:    "www.example.com",
			Path:    "/",
		},
	})

	app := &App{
		logger:          logger,
		discoveryEngine: engine,
	}

	req := httptest.NewRequest("GET", "/api/v1/containers", nil)
	rec := httptest.NewRecorder()
	app.handleContainers(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}

	var entries []map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&entries); err != nil {
		t.Fatalf("JSON decode error: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("entries count = %d, want 2", len(entries))
	}

	// Find each container by name (map iteration order is non-deterministic)
	var apiEntry, webEntry map[string]interface{}
	for _, e := range entries {
		switch e["name"] {
		case "api-service":
			apiEntry = e
		case "web-frontend":
			webEntry = e
		}
	}

	if apiEntry == nil {
		t.Fatal("api-service container not found")
	}
	if webEntry == nil {
		t.Fatal("web-frontend container not found")
	}

	if apiEntry["healthy"] != true {
		t.Errorf("api-service.healthy = %v", apiEntry["healthy"])
	}
	if apiEntry["host"] != "api.example.com" {
		t.Errorf("api-service.host = %v", apiEntry["host"])
	}
	if apiEntry["labels"] != float64(4) {
		t.Errorf("api-service.labels = %v, want 4", apiEntry["labels"])
	}

	if webEntry["status"] != "unhealthy" {
		t.Errorf("web-frontend.status = %v, want unhealthy", webEntry["status"])
	}
	if webEntry["healthy"] != false {
		t.Errorf("web-frontend.healthy = %v", webEntry["healthy"])
	}
}

// TestHandleContainersWithNilConfig tests container with nil config.
func TestHandleContainersWithNilConfig(t *testing.T) {
	logger := log.NewLogger(nil, log.LevelInfo)
	client, _ := discovery.NewDockerClient("")
	sink := &noopRouteSink{}

	engine := discovery.NewEngine(client, sink, &testDiscLogger{})

	engine.InjectContainerForTest(&discovery.ContainerInfo{
		ID:      "abc123def456",
		Name:    "no-config-app",
		Image:   "redis:latest",
		Address: "172.17.0.7:6379",
		Healthy: true,
		Labels:  map[string]string{},
		Config:  nil,
	})

	app := &App{
		logger:          logger,
		discoveryEngine: engine,
	}

	req := httptest.NewRequest("GET", "/api/v1/containers", nil)
	rec := httptest.NewRecorder()
	app.handleContainers(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}

	var entries []map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&entries); err != nil {
		t.Fatalf("JSON decode error: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("entries count = %d, want 1", len(entries))
	}

	// Host should be empty string when config is nil
	if entries[0]["host"] != "" {
		t.Errorf("entries[0].host = %v, want empty string", entries[0]["host"])
	}
}

// TestHandleContainersWithNoDRLabels tests container with non-dr labels.
func TestHandleContainersWithNoDRLabels(t *testing.T) {
	logger := log.NewLogger(nil, log.LevelInfo)
	client, _ := discovery.NewDockerClient("")
	sink := &noopRouteSink{}

	engine := discovery.NewEngine(client, sink, &testDiscLogger{})

	engine.InjectContainerForTest(&discovery.ContainerInfo{
		ID:      "xyz789abc456",
		Name:    "plain-app",
		Image:   "busybox:latest",
		Address: "172.17.0.8:80",
		Healthy: true,
		Labels: map[string]string{
			"com.docker.compose.service": "plain",
			"maintainer":                 "devops",
		},
		Config: &discovery.RouteConfig{
			Enabled: true,
			Host:    "plain.example.com",
		},
	})

	app := &App{
		logger:          logger,
		discoveryEngine: engine,
	}

	req := httptest.NewRequest("GET", "/api/v1/containers", nil)
	rec := httptest.NewRecorder()
	app.handleContainers(rec, req)

	var entries []map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&entries)

	// drLabelCount should be 0 since no labels start with "dr."
	if entries[0]["labels"] != float64(0) {
		t.Errorf("entries[0].labels = %v, want 0", entries[0]["labels"])
	}
}

// noopRouteSink is a route sink that does nothing (for testing).
type noopRouteSink struct{}

func (n *noopRouteSink) AddRoute(info *discovery.ContainerInfo) {}
func (n *noopRouteSink) RemoveRoute(containerID string)         {}

// testDiscLogger is a logger for discovery tests.
type testDiscLogger struct{}

func (t *testDiscLogger) Debug(msg string, fields ...interface{}) {}
func (t *testDiscLogger) Info(msg string, fields ...interface{})  {}
func (t *testDiscLogger) Warn(msg string, fields ...interface{})  {}
func (t *testDiscLogger) Error(msg string, fields ...interface{}) {}

// TestHandleContainersContentType tests Content-Type header.
func TestHandleContainersContentType(t *testing.T) {
	logger := log.NewLogger(nil, log.LevelInfo)

	app := &App{
		logger:          logger,
		discoveryEngine: nil,
	}

	req := httptest.NewRequest("GET", "/api/v1/containers", nil)
	rec := httptest.NewRecorder()
	app.handleContainers(rec, req)

	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %s, want application/json", ct)
	}
}

// TestHandleContainersWithMixedLabels tests container with mixed dr/non-dr labels.
func TestHandleContainersWithMixedLabels(t *testing.T) {
	logger := log.NewLogger(nil, log.LevelInfo)
	client, _ := discovery.NewDockerClient("")
	sink := &noopRouteSink{}

	engine := discovery.NewEngine(client, sink, &testDiscLogger{})

	engine.InjectContainerForTest(&discovery.ContainerInfo{
		ID:      "mixed123abc456",
		Name:    "mixed-app",
		Image:   "alpine:latest",
		Address: "172.17.0.9:9090",
		Healthy: true,
		Labels: map[string]string{
			"dr.enable":            "true",
			"dr.host":              "mixed.example.com",
			"com.docker.compose":   "test",
			"org.label-schema.vcs": "https://github.com/test",
		},
		Config: &discovery.RouteConfig{
			Enabled: true,
			Host:    "mixed.example.com",
		},
	})

	app := &App{
		logger:          logger,
		discoveryEngine: engine,
	}

	req := httptest.NewRequest("GET", "/api/v1/containers", nil)
	rec := httptest.NewRecorder()
	app.handleContainers(rec, req)

	var entries []map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&entries)

	// Should count only 2 dr.* labels
	if entries[0]["labels"] != float64(2) {
		t.Errorf("entries[0].labels = %v, want 2", entries[0]["labels"])
	}
}
