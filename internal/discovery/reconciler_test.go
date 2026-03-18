package discovery

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

type mockLogger struct{}

func (m *mockLogger) Debug(msg string, fields ...interface{}) {}
func (m *mockLogger) Info(msg string, fields ...interface{})  {}
func (m *mockLogger) Warn(msg string, fields ...interface{})  {}
func (m *mockLogger) Error(msg string, fields ...interface{}) {}

type mockRouteSink struct {
	mu      sync.RWMutex
	routes  map[string]*ContainerInfo
	removed []string
}

func newMockRouteSink() *mockRouteSink {
	return &mockRouteSink{
		routes: make(map[string]*ContainerInfo),
	}
}

func (m *mockRouteSink) AddRoute(info *ContainerInfo) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.routes[info.ID] = info
}

func (m *mockRouteSink) RemoveRoute(containerID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.routes, containerID)
	m.removed = append(m.removed, containerID)
}

func TestNewEngine(t *testing.T) {
	logger := &mockLogger{}
	client, _ := NewDockerClient("")
	sink := newMockRouteSink()

	engine := NewEngine(client, sink, logger)

	if engine == nil {
		t.Fatal("NewEngine returned nil")
	}
	if engine.client == nil {
		t.Error("client should not be nil")
	}
	if engine.events == nil {
		t.Error("events should not be nil")
	}
	if engine.poller == nil {
		t.Error("poller should not be nil")
	}
	if engine.containers == nil {
		t.Error("containers map should be initialized")
	}
}

func TestEngineGetContainers(t *testing.T) {
	logger := &mockLogger{}
	client, _ := NewDockerClient("")
	sink := newMockRouteSink()
	engine := NewEngine(client, sink, logger)

	// Empty initially
	containers := engine.GetContainers()
	if len(containers) != 0 {
		t.Errorf("GetContainers should return empty slice initially")
	}

	// Add a container
	engine.mu.Lock()
	engine.containers["abc123"] = &ContainerInfo{
		ID:      "abc123",
		Name:    "test-container",
		Address: "192.168.1.1:8080",
	}
	engine.mu.Unlock()

	containers = engine.GetContainers()
	if len(containers) != 1 {
		t.Errorf("GetContainers count = %d, want 1", len(containers))
	}
	if containers[0].ID != "abc123" {
		t.Errorf("Container ID = %s, want abc123", containers[0].ID)
	}
}

func TestEngineGetContainer(t *testing.T) {
	logger := &mockLogger{}
	client, _ := NewDockerClient("")
	sink := newMockRouteSink()
	engine := NewEngine(client, sink, logger)

	// Non-existent
	info := engine.GetContainer("nonexistent")
	if info != nil {
		t.Error("GetContainer should return nil for non-existent container")
	}

	// Add a container
	engine.mu.Lock()
	engine.containers["abc123"] = &ContainerInfo{
		ID:      "abc123",
		Name:    "test-container",
		Address: "192.168.1.1:8080",
	}
	engine.mu.Unlock()

	info = engine.GetContainer("abc123")
	if info == nil {
		t.Fatal("GetContainer returned nil")
	}
	if info.ID != "abc123" {
		t.Errorf("ID = %s, want abc123", info.ID)
	}
}

func TestNewEventStream(t *testing.T) {
	client, _ := NewDockerClient("")
	stream := NewEventStream(client)

	if stream == nil {
		t.Fatal("NewEventStream returned nil")
	}
	if stream.client == nil {
		t.Error("client should not be nil")
	}
}

func TestNewPoller(t *testing.T) {
	client, _ := NewDockerClient("")
	poller := NewPoller(client, 10*time.Second)

	if poller == nil {
		t.Fatal("NewPoller returned nil")
	}
	if poller.client == nil {
		t.Error("client should not be nil")
	}
	if poller.interval != 10*time.Second {
		t.Errorf("interval = %v, want 10s", poller.interval)
	}
}

func TestPollerStart(t *testing.T) {
	client, _ := NewDockerClient("")
	poller := NewPoller(client, 100*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start returns a channel
	ch := poller.Start(ctx)

	// Just verify it doesn't panic
	// The current implementation returns nil
	_ = ch

	cancel()
}

func TestPollerPollError(t *testing.T) {
	// Test that poll handles errors gracefully
	client, _ := NewDockerClient("")
	poller := NewPoller(client, 100*time.Millisecond)

	ch := make(chan []Container, 10)
	ctx := context.Background()

	// poll should handle errors without panicking
	poller.poll(ctx, ch)

	// Channel should be empty since error occurred (no real Docker)
	select {
	case <-ch:
		t.Error("expected no containers when error occurs")
	default:
		// Expected - channel is empty
	}
}

func TestPollerPollChannelFull(t *testing.T) {
	client, _ := NewDockerClient("")
	poller := NewPoller(client, 100*time.Millisecond)

	// Create a channel with buffer 0 (blocking)
	ch := make(chan []Container)
	ctx := context.Background()

	// This should not block forever due to select with default case
	done := make(chan struct{})
	go func() {
		poller.poll(ctx, ch)
		close(done)
	}()

	select {
	case <-done:
		// Expected - poll should complete without sending to full channel
	case <-time.After(100 * time.Millisecond):
		t.Error("poll should not block on full channel")
	}
}

func TestPollerPollContextCancelled(t *testing.T) {
	client, _ := NewDockerClient("")
	poller := NewPoller(client, 100*time.Millisecond)

	ch := make(chan []Container, 10)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	poller.poll(ctx, ch)

	// Channel should be empty since context is cancelled
	select {
	case <-ch:
		t.Error("expected no containers when context cancelled")
	default:
		// Expected - channel is empty
	}
}

func TestPollerDifferentIntervals(t *testing.T) {
	intervals := []time.Duration{
		1 * time.Second,
		5 * time.Second,
		30 * time.Second,
	}

	client, _ := NewDockerClient("")

	for _, interval := range intervals {
		t.Run(interval.String(), func(t *testing.T) {
			poller := NewPoller(client, interval)

			if poller.interval != interval {
				t.Errorf("interval: got %v, want %v", poller.interval, interval)
			}
		})
	}
}

func TestRouteConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  *RouteConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: &RouteConfig{
				Enabled: true,
				Host:    "example.com",
				Path:    "/",
				TLS:     "auto",
			},
			wantErr: false,
		},
		{
			name: "missing host",
			config: &RouteConfig{
				Enabled: true,
				Host:    "",
				Path:    "/",
			},
			wantErr: true,
		},
		{
			name: "disabled config",
			config: &RouteConfig{
				Enabled: false,
				Host:    "",
				Path:    "/",
			},
			wantErr: false,
		},
		{
			name: "invalid TLS mode",
			config: &RouteConfig{
				Enabled: true,
				Host:    "example.com",
				Path:    "/",
				TLS:     "invalid",
			},
			wantErr: true,
		},
		{
			name: "manual TLS without cert",
			config: &RouteConfig{
				Enabled: true,
				Host:    "example.com",
				Path:    "/",
				TLS:     "manual",
			},
			wantErr: true,
		},
		{
			name: "manual TLS with cert and key",
			config: &RouteConfig{
				Enabled:     true,
				Host:        "example.com",
				Path:        "/",
				TLS:         "manual",
				TLSCertFile: "/certs/cert.pem",
				TLSKeyFile:  "/certs/key.pem",
			},
			wantErr: false,
		},
		{
			name: "host with port",
			config: &RouteConfig{
				Enabled: true,
				Host:    "example.com:8080",
				Path:    "/",
				TLS:     "auto",
			},
			wantErr: true,
		},
		{
			name: "path without slash",
			config: &RouteConfig{
				Enabled: true,
				Host:    "example.com",
				Path:    "api",
				TLS:     "auto",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr && err == nil {
				t.Error("Validate should return error")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("Validate should not return error: %v", err)
			}
		})
	}
}

func TestEngineBuildContainerInfo(t *testing.T) {
	logger := &mockLogger{}
	client, _ := NewDockerClient("")
	sink := newMockRouteSink()
	engine := NewEngine(client, sink, logger)

	c := Container{
		ID:     "abc123",
		Names:  []string{"/test-container"},
		Labels: map[string]string{"dr.enable": "true", "dr.host": "example.com"},
	}

	detail := &ContainerDetail{
		ID:   "abc123",
		Name: "/test-container",
		State: ContainerState{
			Running: true,
			Healthy: true,
		},
		Network: ContainerNetwork{
			IPAddress: "172.17.0.5",
			Networks: map[string]NetworkInfo{
				"bridge": {IPAddress: "172.17.0.5"},
			},
		},
	}

	config := &RouteConfig{
		Enabled: true,
		Host:    "example.com",
		Path:    "/",
	}

	info := engine.buildContainerInfo(c, detail, config)

	if info == nil {
		t.Fatal("buildContainerInfo returned nil")
	}
	if info.ID != "abc123" {
		t.Errorf("ID = %s, want abc123", info.ID)
	}
	if info.Name != "test-container" {
		t.Errorf("Name = %s, want test-container", info.Name)
	}
	if !info.Healthy {
		t.Error("Healthy should be true")
	}
}

func TestEngineBuildContainerInfoWithAddress(t *testing.T) {
	logger := &mockLogger{}
	client, _ := NewDockerClient("")
	sink := newMockRouteSink()
	engine := NewEngine(client, sink, logger)

	c := Container{
		ID:     "abc123",
		Names:  []string{"/test-container"},
		Labels: map[string]string{},
	}

	detail := &ContainerDetail{
		ID:   "abc123",
		Name: "/test-container",
		State: ContainerState{
			Running: true,
		},
		Network: ContainerNetwork{},
	}

	config := &RouteConfig{
		Enabled: true,
		Host:    "example.com",
		Address: "192.168.1.100:8080",
	}

	info := engine.buildContainerInfo(c, detail, config)

	if info.Address != "192.168.1.100:8080" {
		t.Errorf("Address = %s, want 192.168.1.100:8080", info.Address)
	}
}

func TestEngineStartAlreadyRunning(t *testing.T) {
	logger := &mockLogger{}
	sink := newMockRouteSink()

	// Create engine directly without using NewEngine
	engine := &Engine{
		client:     nil,
		events:     nil,
		poller:     nil,
		routes:     sink,
		logger:     logger,
		containers: make(map[string]*ContainerInfo),
	}

	ctx := context.Background()

	// Set running to true
	engine.mu.Lock()
	engine.running = true
	engine.mu.Unlock()

	// Start should return nil (already running)
	err := engine.Start(ctx)
	if err != nil {
		t.Errorf("Start when already running should return nil, got %v", err)
	}
}

func TestEngineOnContainerStop(t *testing.T) {
	logger := &mockLogger{}
	sink := newMockRouteSink()

	// Create engine directly
	engine := &Engine{
		routes:     sink,
		logger:     logger,
		containers: make(map[string]*ContainerInfo),
	}

	// Add a container to the engine
	engine.mu.Lock()
	engine.containers["container-1"] = &ContainerInfo{
		ID:      "container-1",
		Name:    "test-container",
		Address: "192.168.1.1:8080",
		Config:  &RouteConfig{Host: "example.com"},
	}
	engine.mu.Unlock()

	// Stop the container
	engine.onContainerStop("container-1")

	// Verify container was removed
	engine.mu.RLock()
	_, exists := engine.containers["container-1"]
	engine.mu.RUnlock()

	if exists {
		t.Error("Container should be removed after onContainerStop")
	}

	// Verify RemoveRoute was called
	if len(sink.removed) != 1 || sink.removed[0] != "container-1" {
		t.Errorf("RemoveRoute should be called with container-1, got %v", sink.removed)
	}
}

func TestEngineOnContainerStopNonExistent(t *testing.T) {
	logger := &mockLogger{}
	sink := newMockRouteSink()

	// Create engine directly
	engine := &Engine{
		routes:     sink,
		logger:     logger,
		containers: make(map[string]*ContainerInfo),
	}

	// Stop a container that doesn't exist - should not panic
	engine.onContainerStop("non-existent")

	// RemoveRoute should not be called
	if len(sink.removed) != 0 {
		t.Errorf("RemoveRoute should not be called, got %v", sink.removed)
	}
}

func TestEngineBuildContainerInfoWithPort(t *testing.T) {
	logger := &mockLogger{}
	sink := newMockRouteSink()

	// Create engine directly
	engine := &Engine{
		routes:     sink,
		logger:     logger,
		containers: make(map[string]*ContainerInfo),
	}

	c := Container{
		ID:     "abc123",
		Names:  []string{"/test-container"},
		Labels: map[string]string{},
		Ports:  []PortBinding{{PrivatePort: 8080, PublicPort: 80, Type: "tcp"}},
	}

	detail := &ContainerDetail{
		ID:   "abc123",
		Name: "/test-container",
		State: ContainerState{
			Running: true,
			Healthy: true,
		},
		Network: ContainerNetwork{
			IPAddress: "172.17.0.5",
			Networks: map[string]NetworkInfo{
				"bridge": {IPAddress: "172.17.0.5"},
			},
		},
	}

	config := &RouteConfig{
		Enabled: true,
		Host:    "example.com",
		Port:    3000,
	}

	info := engine.buildContainerInfo(c, detail, config)

	// Port should be from config
	if !strings.HasSuffix(info.Address, ":3000") {
		t.Errorf("Address should end with :3000, got %s", info.Address)
	}
}

func TestEngineBuildContainerInfoHealthy(t *testing.T) {
	logger := &mockLogger{}
	sink := newMockRouteSink()

	// Create engine directly
	engine := &Engine{
		routes:     sink,
		logger:     logger,
		containers: make(map[string]*ContainerInfo),
	}

	tests := []struct {
		name          string
		healthy       bool
		running       bool
		expectHealthy bool
	}{
		{"healthy container", true, true, true},
		{"running but not healthy", false, true, true},
		{"not running", false, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Container{
				ID:     "abc123",
				Names:  []string{"/test-container"},
				Labels: map[string]string{},
			}

			detail := &ContainerDetail{
				ID:   "abc123",
				Name: "/test-container",
				State: ContainerState{
					Running: tt.running,
					Healthy: tt.healthy,
				},
				Network: ContainerNetwork{
					IPAddress: "172.17.0.5",
				},
			}

			config := &RouteConfig{
				Enabled: true,
				Host:    "example.com",
			}

			info := engine.buildContainerInfo(c, detail, config)

			if info.Healthy != tt.expectHealthy {
				t.Errorf("Healthy = %v, want %v", info.Healthy, tt.expectHealthy)
			}
		})
	}
}

func TestContainerInfoChangedAllFields(t *testing.T) {
	base := &ContainerInfo{
		ID:      "abc123",
		Name:    "test",
		Address: "192.168.1.1:8080",
		Healthy: true,
		Config:  &RouteConfig{Host: "example.com", Path: "/api"},
	}

	tests := []struct {
		name     string
		other    *ContainerInfo
		expected bool
	}{
		{
			name:     "identical",
			other:    &ContainerInfo{Address: "192.168.1.1:8080", Healthy: true, Config: &RouteConfig{Host: "example.com", Path: "/api"}},
			expected: false,
		},
		{
			name:     "address changed",
			other:    &ContainerInfo{Address: "192.168.1.2:8080", Healthy: true, Config: &RouteConfig{Host: "example.com", Path: "/api"}},
			expected: true,
		},
		{
			name:     "healthy changed",
			other:    &ContainerInfo{Address: "192.168.1.1:8080", Healthy: false, Config: &RouteConfig{Host: "example.com", Path: "/api"}},
			expected: true,
		},
		{
			name:     "host changed",
			other:    &ContainerInfo{Address: "192.168.1.1:8080", Healthy: true, Config: &RouteConfig{Host: "other.com", Path: "/api"}},
			expected: true,
		},
		{
			name:     "path changed",
			other:    &ContainerInfo{Address: "192.168.1.1:8080", Healthy: true, Config: &RouteConfig{Host: "example.com", Path: "/v2"}},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := base.Changed(tt.other)
			if result != tt.expected {
				t.Errorf("Changed() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestEngineSyncError tests that Sync returns an error when Docker is not available
func TestEngineSyncError(t *testing.T) {
	logger := &mockLogger{}
	client, _ := NewDockerClient("/nonexistent/docker.sock")
	sink := newMockRouteSink()
	engine := NewEngine(client, sink, logger)

	ctx := context.Background()
	err := engine.Sync(ctx)
	if err == nil {
		t.Error("Sync should return error when Docker socket is not available")
	}
}

// TestEngineStartError tests that Start returns an error when Docker is not available
func TestEngineStartError(t *testing.T) {
	logger := &mockLogger{}
	client, _ := NewDockerClient("/nonexistent/docker.sock")
	sink := newMockRouteSink()
	engine := NewEngine(client, sink, logger)

	ctx := context.Background()
	err := engine.Start(ctx)
	if err == nil {
		t.Error("Start should return error when Docker socket is not available")
	}
}
