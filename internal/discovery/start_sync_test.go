package discovery

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

// TestEngineStartWithSyncSuccess tests Start with a successful initial Sync.
func TestEngineStartWithSyncSuccess(t *testing.T) {
	mock, err := newUnixMockServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1.53/containers/json":
			containers := []Container{
				{
					ID:     "abc123def456",
					Names:  []string{"/test-app"},
					Image:  "nginx:latest",
					State:  "running",
					Labels: map[string]string{"dr.enable": "true", "dr.host": "test.example.com"},
				},
			}
			json.NewEncoder(w).Encode(containers)
		case "/v1.53/containers/abc123def456/json":
			json.NewEncoder(w).Encode(ContainerDetail{
				State:   ContainerState{Running: true, Healthy: true},
				Network: ContainerNetwork{IPAddress: "172.17.0.5"},
			})
		case "/v1.53/events":
			// Keep connection open
			time.Sleep(5 * time.Second)
		default:
			json.NewEncoder(w).Encode([]Container{})
		}
	}))
	if err != nil {
		t.Skipf("Cannot create unix socket mock: %v", err)
	}
	defer mock.Close()

	client, _ := NewDockerClient(mock.socket)
	defer client.Close()

	logger := &mockDiscoveryLogger{}
	sink := newMockRouteSink()
	engine := NewEngine(client, sink, logger)

	// Use short interval for testing
	engine.interval = 100 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	err = engine.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Verify container was discovered
	containers := engine.GetContainers()
	if len(containers) != 1 {
		t.Errorf("containers count = %d, want 1", len(containers))
	}

	// Verify route was added
	sink.mu.RLock()
	routeCount := len(sink.routes)
	sink.mu.RUnlock()
	if routeCount != 1 {
		t.Errorf("routes added = %d, want 1", routeCount)
	}
	sink.mu.RLock()
	info := sink.routes["abc123def456"]
	sink.mu.RUnlock()
	if info != nil && info.Config.Host != "test.example.com" {
		t.Errorf("host = %s, want test.example.com", info.Config.Host)
	}
}

// TestEngineStartSyncError tests Start when initial Sync fails.
func TestEngineStartSyncError(t *testing.T) {
	mock, err := newUnixMockServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	if err != nil {
		t.Skipf("Cannot create unix socket mock: %v", err)
	}
	defer mock.Close()

	client, _ := NewDockerClient(mock.socket)
	defer client.Close()

	logger := &mockDiscoveryLogger{}
	sink := newMockRouteSink()
	engine := NewEngine(client, sink, logger)

	ctx := context.Background()
	err = engine.Start(ctx)
	if err == nil {
		t.Fatal("Start should fail when Sync fails")
	}

	// A failed Start must not leave the engine claiming to be running: the
	// /ready probe reads IsRunning and would advertise a router that has no
	// way to discover any routes.
	if engine.IsRunning() {
		t.Error("IsRunning() = true after a failed Start, want false")
	}

	// And a later Start must still be able to succeed rather than short-circuit
	// on the stale flag.
	if err := engine.Start(ctx); err == nil {
		t.Error("second Start should have reported the same sync failure")
	}
}

// TestIntToStrNegative tests the negative number path in intToStr.
func TestIntToStrNegative(t *testing.T) {
	result := intToStr(-42)
	if result != "-42" {
		t.Errorf("intToStr(-42) = %q, want -42", result)
	}
}

// TestIntToStrZero tests zero.
func TestIntToStrZero(t *testing.T) {
	result := intToStr(0)
	if result != "0" {
		t.Errorf("intToStr(0) = %q, want 0", result)
	}
}

// TestIntToStrLarge tests a large number.
func TestIntToStrLarge(t *testing.T) {
	result := intToStr(8080)
	if result != "8080" {
		t.Errorf("intToStr(8080) = %q, want 8080", result)
	}
}

// TestDetectPortWithPublishedPort tests detectPort with public port binding.
func TestDetectPortWithPublishedPort(t *testing.T) {
	ports := []PortBinding{
		{PublicPort: 8080, PrivatePort: 80},
		{PublicPort: 0, PrivatePort: 443},
	}
	result := detectPort(ports, nil)
	if result != 80 {
		t.Errorf("detectPort = %d, want 80", result)
	}
}

// TestDetectPortEmpty tests detectPort with no ports.
func TestDetectPortEmpty(t *testing.T) {
	result := detectPort(nil, nil)
	if result != 0 {
		t.Errorf("detectPort = %d, want 0", result)
	}
}

// TestExtractNameVariants tests extractName with various inputs.
func TestExtractNameVariants(t *testing.T) {
	tests := []struct {
		names []string
		want  string
	}{
		{[]string{"/my-app"}, "my-app"},
		{[]string{}, ""},
		{nil, ""},
		{[]string{"no-slash"}, "no-slash"},
	}

	for _, tt := range tests {
		got := extractName(tt.names)
		if got != tt.want {
			t.Errorf("extractName(%v) = %q, want %q", tt.names, got, tt.want)
		}
	}
}
