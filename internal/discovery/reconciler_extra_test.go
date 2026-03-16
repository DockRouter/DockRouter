package discovery

import (
	"context"
	"testing"
)

func TestEngineHandleEventStop(t *testing.T) {
	logger := &mockLogger{}
	sink := newMockRouteSink()

	// Create engine directly with nil client (we're only testing stop event)
	engine := &Engine{
		client:     nil,
		routes:     sink,
		logger:     logger,
		containers: make(map[string]*ContainerInfo),
	}

	// Add a container with proper Docker ID length (64 chars)
	containerID := "abc123def456789012345678901234567890123456789012345678901234"
	engine.mu.Lock()
	engine.containers[containerID] = &ContainerInfo{
		ID:      containerID,
		Name:    "test-container",
		Address: "192.168.1.1:8080",
		Config:  &RouteConfig{Host: "example.com"},
	}
	engine.mu.Unlock()

	// Create stop event
	event := Event{
		Type:   "container",
		Action: "stop",
		Actor: EventActor{
			ID: containerID,
		},
	}

	// Handle the event
	engine.handleEvent(context.Background(), event)

	// Verify container was removed
	engine.mu.RLock()
	_, exists := engine.containers[containerID]
	engine.mu.RUnlock()

	if exists {
		t.Error("Container should be removed after stop event")
	}

	if len(sink.removed) != 1 || sink.removed[0] != containerID {
		t.Errorf("RemoveRoute should be called, got %v", sink.removed)
	}
}

func TestEngineHandleEventDie(t *testing.T) {
	logger := &mockLogger{}
	sink := newMockRouteSink()

	engine := &Engine{
		client:     nil,
		routes:     sink,
		logger:     logger,
		containers: make(map[string]*ContainerInfo),
	}

	// Add a container with proper Docker ID length
	containerID := "def456789012345678901234567890123456789012345678901234567890"
	engine.mu.Lock()
	engine.containers[containerID] = &ContainerInfo{
		ID:      containerID,
		Name:    "another-container",
		Address: "192.168.1.2:8080",
		Config:  &RouteConfig{Host: "test.com"},
	}
	engine.mu.Unlock()

	// Create die event
	event := Event{
		Type:   "container",
		Action: "die",
		Actor: EventActor{
			ID: containerID,
		},
	}

	engine.handleEvent(context.Background(), event)

	// Verify container was removed
	engine.mu.RLock()
	_, exists := engine.containers[containerID]
	engine.mu.RUnlock()

	if exists {
		t.Error("Container should be removed after die event")
	}
}

func TestEngineHandleEventNonContainer(t *testing.T) {
	logger := &mockLogger{}
	sink := newMockRouteSink()

	engine := &Engine{
		routes:     sink,
		logger:     logger,
		containers: make(map[string]*ContainerInfo),
	}

	// Create non-container event (should be ignored)
	event := Event{
		Type:   "image",
		Action: "pull",
		Actor: EventActor{
			ID: "sha256:abc123def456789012345678901234567890123456789012345678901234",
		},
	}

	// Should not panic
	engine.handleEvent(context.Background(), event)

	// No routes should be removed
	if len(sink.removed) != 0 {
		t.Errorf("No routes should be removed for non-container event, got %v", sink.removed)
	}
}

// Note: Start and health events require a valid Docker client because
// onContainerStart calls InspectContainer. These cannot be tested with nil client.

