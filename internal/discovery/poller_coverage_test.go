package discovery

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

// TestPollerPollSuccessViaMock tests the poll method with a successful ListContainers.
func TestPollerPollSuccessViaMock(t *testing.T) {
	mock, err := newUnixMockServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1.53/containers/json" {
			containers := []Container{
				{ID: "abc123", Names: []string{"/app1"}, State: "running"},
				{ID: "def456", Names: []string{"/app2"}, State: "running"},
			}
			json.NewEncoder(w).Encode(containers)
			return
		}
		json.NewEncoder(w).Encode([]Container{})
	}))
	if err != nil {
		t.Skipf("Cannot create unix socket mock: %v", err)
	}
	defer mock.Close()

	client, _ := NewDockerClient(mock.socket)
	defer client.Close()

	poller := NewPoller(client, 10*time.Second)

	ch := make(chan []Container, 10)
	ctx := context.Background()

	poller.poll(ctx, ch)

	select {
	case containers := <-ch:
		if len(containers) != 2 {
			t.Errorf("containers count = %d, want 2", len(containers))
		}
		if containers[0].ID != "abc123" {
			t.Errorf("containers[0].ID = %s, want abc123", containers[0].ID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for poll result")
	}
}

// TestPollerPollErrorViaMock tests poll when ListContainers fails.
func TestPollerPollErrorViaMock(t *testing.T) {
	mock, err := newUnixMockServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	if err != nil {
		t.Skipf("Cannot create unix socket mock: %v", err)
	}
	defer mock.Close()

	client, _ := NewDockerClient(mock.socket)
	defer client.Close()

	poller := NewPoller(client, 10*time.Second)

	ch := make(chan []Container, 10)
	ctx := context.Background()

	poller.poll(ctx, ch)

	// Should return without sending to channel
	select {
	case <-ch:
		t.Error("should not send on error")
	case <-time.After(200 * time.Millisecond):
		// Expected — no data sent
	}
}

// TestPollerPollChannelFullViaMock tests poll when output channel is full.
func TestPollerPollChannelFullViaMock(t *testing.T) {
	mock, err := newUnixMockServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		containers := []Container{{ID: "abc123", State: "running"}}
		json.NewEncoder(w).Encode(containers)
	}))
	if err != nil {
		t.Skipf("Cannot create unix socket mock: %v", err)
	}
	defer mock.Close()

	client, _ := NewDockerClient(mock.socket)
	defer client.Close()

	poller := NewPoller(client, 10*time.Second)

	// Create a full channel (capacity 1, already has data)
	ch := make(chan []Container, 1)
	ch <- []Container{{ID: "existing"}}

	ctx := context.Background()
	poller.poll(ctx, ch)

	// Should skip (default case) since channel is full
	// Channel should still have only the original item
	select {
	case c := <-ch:
		if c[0].ID != "existing" {
			t.Errorf("should have original item, got %s", c[0].ID)
		}
	default:
		t.Error("channel should have original item")
	}
}

// TestPollerPollCancelledContext tests poll with cancelled context.
func TestPollerPollCancelledContext(t *testing.T) {
	mock, err := newUnixMockServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		containers := []Container{{ID: "abc123", State: "running"}}
		json.NewEncoder(w).Encode(containers)
	}))
	if err != nil {
		t.Skipf("Cannot create unix socket mock: %v", err)
	}
	defer mock.Close()

	client, _ := NewDockerClient(mock.socket)
	defer client.Close()

	poller := NewPoller(client, 10*time.Second)

	ch := make(chan []Container, 10)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	poller.poll(ctx, ch)

	// With cancelled context, ListContainers may fail or succeed
	// If it succeeds, the select should pick ctx.Done()
	select {
	case <-ch:
		// Might get data if ListContainers succeeded before context check
	case <-time.After(200 * time.Millisecond):
		// Also fine — context was cancelled
	}
}

// TestPollerStartIntegration tests the full Start → poll → context cancel flow.
func TestPollerStartIntegration(t *testing.T) {
	mock, err := newUnixMockServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]Container{{ID: "abc123", State: "running"}})
	}))
	if err != nil {
		t.Skipf("Cannot create unix socket mock: %v", err)
	}
	defer mock.Close()

	client, _ := NewDockerClient(mock.socket)
	defer client.Close()

	poller := NewPoller(client, 50*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	ch := poller.Start(ctx)

	// Should get at least one poll result
	select {
	case containers := <-ch:
		if len(containers) != 1 {
			t.Errorf("containers count = %d, want 1", len(containers))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for poll result")
	}

	// Wait for context to expire
	<-ctx.Done()

	// Drain any remaining items and wait for channel to close
drainLoop:
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				break drainLoop // channel closed — expected
			}
			// More data, keep draining
		case <-time.After(500 * time.Millisecond):
			t.Fatal("channel should close after context cancellation")
		}
	}
}

// TestNewPollerValues tests the constructor values.
func TestNewPollerValues(t *testing.T) {
	client, _ := NewDockerClient("/tmp/test.sock")
	defer client.Close()

	poller := NewPoller(client, 5*time.Second)
	if poller.client != client {
		t.Error("client not set")
	}
	if poller.interval != 5*time.Second {
		t.Error("interval not set")
	}
}
