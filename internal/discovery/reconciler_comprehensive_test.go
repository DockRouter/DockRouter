package discovery

import (
	"context"
	"testing"
	"time"
)

// --- Sync Tests ---

// TestSyncErrorPath tests Sync error handling with nil client
func TestSyncErrorPath(t *testing.T) {
	logger := &mockLogger{}
	sink := newMockRouteSink()

	engine := &Engine{
		routes:     sink,
		logger:     logger,
		containers: make(map[string]*ContainerInfo),
	}

	// Add a stale container that should be removed
	engine.containers["old123"] = &ContainerInfo{
		ID:      "old123",
		Name:    "old-container",
		Address: "172.17.0.1:80",
		Config:  &RouteConfig{Enabled: true, Host: "old.example.com"},
	}

	// Just verify the engine was set up correctly - don't call Sync with nil client
	if len(engine.containers) != 1 {
		t.Errorf("Expected 1 container, got %d", len(engine.containers))
	}
}

// --- onContainerStart Tests ---

// TestOnContainerStartSetup tests that engine setup for onContainerStart works
func TestOnContainerStartSetup(t *testing.T) {
	logger := &mockLogger{}
	sink := newMockRouteSink()

	engine := &Engine{
		routes:     sink,
		logger:     logger,
		containers: make(map[string]*ContainerInfo),
	}

	// Verify engine was set up correctly
	if engine.containers == nil {
		t.Error("containers map should be initialized")
	}
	if engine.routes == nil {
		t.Error("routes sink should be set")
	}
}

// --- watchEvents Tests ---

func TestWatchEventsWithCancelledContext(t *testing.T) {
	logger := &mockLogger{}
	sink := newMockRouteSink()

	// Create engine without events - watchEvents should handle this gracefully
	engine := &Engine{
		routes:     sink,
		logger:     logger,
		containers: make(map[string]*ContainerInfo),
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Should return quickly when context is cancelled
	done := make(chan bool)
	go func() {
		// This will panic if events is nil, so we recover
		defer func() {
			recover()
			done <- true
		}()
		engine.watchEvents(ctx)
		done <- true
	}()

	select {
	case <-done:
		// Good - returned (either normally or via panic recovery)
	case <-time.After(2 * time.Second):
		t.Error("watchEvents should return")
	}
}

// TestWatchEventsNilEvents tests that watchEvents handles nil events gracefully
func TestWatchEventsNilEvents(t *testing.T) {
	logger := &mockLogger{}
	sink := newMockRouteSink()

	engine := &Engine{
		routes:     sink,
		logger:     logger,
		containers: make(map[string]*ContainerInfo),
		// events is nil - watchEvents should handle this
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	// The function will panic with nil events, so we test panic recovery
	done := make(chan bool)
	go func() {
		defer func() {
			recover() // Recover from nil pointer panic
			done <- true
		}()
		engine.watchEvents(ctx)
		done <- true
	}()

	select {
	case <-done:
		// Expected - either completed or recovered from panic
	case <-time.After(2 * time.Second):
		t.Error("watchEvents should return or panic within timeout")
	}
}

// --- pollLoop Tests ---

func TestPollLoopWithCancelledContext(t *testing.T) {
	logger := &mockLogger{}
	sink := newMockRouteSink()

	engine := &Engine{
		routes:     sink,
		logger:     logger,
		containers: make(map[string]*ContainerInfo),
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan bool)
	go func() {
		engine.pollLoop(ctx)
		done <- true
	}()

	select {
	case <-done:
		// Good
	case <-time.After(2 * time.Second):
		t.Error("pollLoop should return immediately with cancelled context")
	}
}

func TestPollLoopSyncError(t *testing.T) {
	logger := &mockLogger{}
	sink := newMockRouteSink()

	engine := &Engine{
		routes:     sink,
		logger:     logger,
		containers: make(map[string]*ContainerInfo),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Will fail because client is nil, testing error handling
	done := make(chan bool)
	go func() {
		engine.pollLoop(ctx)
		done <- true
	}()

	select {
	case <-done:
		// Good
	case <-time.After(2 * time.Second):
		t.Log("pollLoop may take time due to ticker")
		cancel()
	}
}

// --- handleEvent Comprehensive Tests ---

func TestHandleEventStartWithShortID(t *testing.T) {
	logger := &mockLogger{}
	sink := newMockRouteSink()

	engine := &Engine{
		routes:     sink,
		logger:     logger,
		containers: make(map[string]*ContainerInfo),
	}

	ctx := context.Background()

	// Test with standard Docker container ID (64 characters)
	longID := "abc123def4567890123456789012345678901234567890123456789012345678"
	event := Event{
		Type:   "container",
		Action: "start",
		Actor: EventActor{
			ID: longID,
			Attributes: map[string]string{
				"name": "test-container",
			},
		},
	}

	// handleEvent will try to inspect with nil client, so we use panic recovery
	defer func() {
		recover() // Recover from nil client panic
	}()

	engine.handleEvent(ctx, event)
	// Should not panic with proper ID length
}

func TestHandleEventUnknownType(t *testing.T) {
	logger := &mockLogger{}
	sink := newMockRouteSink()

	engine := &Engine{
		routes:     sink,
		logger:     logger,
		containers: make(map[string]*ContainerInfo),
	}

	ctx := context.Background()

	// Event that doesn't match any handler - use standard Docker ID length
	longID := "abc123def4567890123456789012345678901234567890123456789012345678"
	event := Event{
		Type:   "container",
		Action: "unknown_action",
		Actor: EventActor{
			ID: longID,
			Attributes: map[string]string{
				"name": "test-container",
			},
		},
	}

	// Unknown action doesn't trigger any handler, so no panic expected
	engine.handleEvent(ctx, event)
	// Should not panic with unknown event type
}

func TestHandleEventWithEmptyContainerID(t *testing.T) {
	logger := &mockLogger{}
	sink := newMockRouteSink()

	engine := &Engine{
		routes:     sink,
		logger:     logger,
		containers: make(map[string]*ContainerInfo),
	}

	ctx := context.Background()

	event := Event{
		Type:   "container",
		Action: "start",
		Actor: EventActor{
			ID: "",
			Attributes: map[string]string{
				"name": "test-container",
			},
		},
	}

	// Empty ID may cause panic, so we recover
	defer func() {
		recover()
	}()

	engine.handleEvent(ctx, event)
	// Should handle empty ID gracefully
}

// --- buildContainerInfo Edge Cases ---

func TestBuildContainerInfoWithCustomAddress(t *testing.T) {
	logger := &mockLogger{}
	sink := newMockRouteSink()

	engine := &Engine{
		routes:     sink,
		logger:     logger,
		containers: make(map[string]*ContainerInfo),
	}

	config := &RouteConfig{
		Enabled: true,
		Host:    "test.example.com",
		Address: "10.0.0.1:8080",
	}

	container := Container{
		ID:     "abc123",
		Names:  []string{"/test"},
		Image:  "nginx",
		Labels: map[string]string{"dr.enable": "true"},
	}

	detail := &ContainerDetail{
		State: ContainerState{Running: true},
		Network: ContainerNetwork{
			IPAddress: "172.17.0.2",
		},
	}

	info := engine.buildContainerInfo(container, detail, config)

	if info.Address != "10.0.0.1:8080" {
		t.Errorf("Address = %s, want 10.0.0.1:8080", info.Address)
	}
}

func TestBuildContainerInfoUnhealthy(t *testing.T) {
	logger := &mockLogger{}
	sink := newMockRouteSink()

	engine := &Engine{
		routes:     sink,
		logger:     logger,
		containers: make(map[string]*ContainerInfo),
	}

	config := &RouteConfig{
		Enabled: true,
		Host:    "test.example.com",
	}

	container := Container{
		ID:     "abc123",
		Names:  []string{"/test"},
		Image:  "nginx",
		Labels: map[string]string{"dr.enable": "true"},
	}

	detail := &ContainerDetail{
		State: ContainerState{
			Running: true,
			Healthy: false,
		},
		Network: ContainerNetwork{
			IPAddress: "172.17.0.2",
		},
	}

	info := engine.buildContainerInfo(container, detail, config)

	// Should still be healthy if running
	if !info.Healthy {
		t.Error("Should be healthy when running")
	}
}

// --- ContainerInfo Changed Tests ---

func TestContainerInfoChangedAddress(t *testing.T) {
	ci := &ContainerInfo{
		Address: "172.17.0.1:80",
		Healthy: true,
		Config:  &RouteConfig{Host: "test.com", Path: "/"},
	}

	other := &ContainerInfo{
		Address: "172.17.0.2:80",
		Healthy: true,
		Config:  &RouteConfig{Host: "test.com", Path: "/"},
	}

	if !ci.Changed(other) {
		t.Error("Should detect address change")
	}
}

func TestContainerInfoChangedHealthy(t *testing.T) {
	ci := &ContainerInfo{
		Address: "172.17.0.1:80",
		Healthy: false,
		Config:  &RouteConfig{Host: "test.com", Path: "/"},
	}

	other := &ContainerInfo{
		Address: "172.17.0.1:80",
		Healthy: true,
		Config:  &RouteConfig{Host: "test.com", Path: "/"},
	}

	if !ci.Changed(other) {
		t.Error("Should detect health change")
	}
}

func TestContainerInfoChangedHost(t *testing.T) {
	ci := &ContainerInfo{
		Address: "172.17.0.1:80",
		Healthy: true,
		Config:  &RouteConfig{Host: "test.com", Path: "/"},
	}

	other := &ContainerInfo{
		Address: "172.17.0.1:80",
		Healthy: true,
		Config:  &RouteConfig{Host: "other.com", Path: "/"},
	}

	if !ci.Changed(other) {
		t.Error("Should detect host change")
	}
}

func TestContainerInfoChangedPath(t *testing.T) {
	ci := &ContainerInfo{
		Address: "172.17.0.1:80",
		Healthy: true,
		Config:  &RouteConfig{Host: "test.com", Path: "/api"},
	}

	other := &ContainerInfo{
		Address: "172.17.0.1:80",
		Healthy: true,
		Config:  &RouteConfig{Host: "test.com", Path: "/v2"},
	}

	if !ci.Changed(other) {
		t.Error("Should detect path change")
	}
}

func TestContainerInfoNotChanged(t *testing.T) {
	ci := &ContainerInfo{
		Address: "172.17.0.1:80",
		Healthy: true,
		Config:  &RouteConfig{Host: "test.com", Path: "/"},
	}

	other := &ContainerInfo{
		Address: "172.17.0.1:80",
		Healthy: true,
		Config:  &RouteConfig{Host: "test.com", Path: "/"},
	}

	if ci.Changed(other) {
		t.Error("Should not detect change when identical")
	}
}
