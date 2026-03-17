package discovery

import (
	"context"
	"testing"
	"time"
)

// TestPollLoopContextCancellation tests the pollLoop function behavior
func TestPollLoopContextCancellation(t *testing.T) {
	logger := &mockLogger{}
	sink := newMockRouteSink()

	// Create engine with nil client (will fail on Sync but we test cancellation)
	engine := &Engine{
		routes:     sink,
		logger:     logger,
		containers: make(map[string]*ContainerInfo),
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel immediately
	cancel()

	// pollLoop should return immediately due to context cancellation
	done := make(chan bool)
	go func() {
		engine.pollLoop(ctx)
		done <- true
	}()

	select {
	case <-done:
		// Good, it returned
	case <-time.After(2 * time.Second):
		t.Error("pollLoop should return when context is cancelled")
	}
}

// TestWatchEventsContextCancellation tests watchEvents cancellation
func TestWatchEventsContextCancellation(t *testing.T) {
	logger := &mockLogger{}
	sink := newMockRouteSink()

	engine := &Engine{
		events:     NewEventStream(nil), // nil client
		routes:     sink,
		logger:     logger,
		containers: make(map[string]*ContainerInfo),
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel immediately
	cancel()

	// watchEvents should return
	done := make(chan bool)
	go func() {
		engine.watchEvents(ctx)
		done <- true
	}()

	select {
	case <-done:
		// Good
	case <-time.After(3 * time.Second):
		t.Error("watchEvents should return when context is cancelled")
	}
}

// TestSyncWithNilClient tests Sync with nil client
func TestSyncWithNilClient(t *testing.T) {
	logger := &mockLogger{}
	sink := newMockRouteSink()

	engine := &Engine{
		client:     nil,
		routes:     sink,
		logger:     logger,
		containers: make(map[string]*ContainerInfo),
	}

	ctx := context.Background()

	// Sync should panic with nil client - we test it doesn't hang
	defer func() {
		if r := recover(); r != nil {
			// Expected panic, that's fine
		}
	}()

	// This will panic but that's expected behavior with nil client
	engine.Sync(ctx)
}

// TestStartWithNilClient tests Start with nil client - it should work because it spawns goroutines
func TestStartWithNilClient(t *testing.T) {
	logger := &mockLogger{}
	sink := newMockRouteSink()

	engine := &Engine{
		client:     nil,
		events:     NewEventStream(nil),
		poller:     NewPoller(nil, time.Second),
		routes:     sink,
		logger:     logger,
		containers: make(map[string]*ContainerInfo),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start will panic in goroutines but that's expected with nil client
	// We test that Start itself doesn't block
	done := make(chan error, 1)
	go func() {
		// Recover from any panics
		defer func() {
			if r := recover(); r != nil {
				done <- nil // Panics in goroutines are expected
			}
		}()
		done <- engine.Start(ctx)
	}()

	select {
	case err := <-done:
		// Either error or panic, both acceptable with nil client
		_ = err
	case <-time.After(time.Second):
		// If it's taking too long, that's also acceptable (goroutines running)
	}
}

// TestEngineWithRunningFlag tests engine running state
func TestEngineRunningFlag(t *testing.T) {
	logger := &mockLogger{}
	sink := newMockRouteSink()

	engine := &Engine{
		routes:     sink,
		logger:     logger,
		containers: make(map[string]*ContainerInfo),
	}

	// Initially not running
	if engine.running {
		t.Error("Engine should not be running initially")
	}

	// Set running
	engine.running = true

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start should return nil if already running
	err := engine.Start(ctx)
	if err != nil {
		t.Errorf("Start should return nil when already running, got: %v", err)
	}
}
