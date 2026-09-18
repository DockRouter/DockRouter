package discovery

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

// TestListAllContainersWithUnixMock tests ListAllContainers via Unix socket mock
func TestListAllContainersWithUnixMock(t *testing.T) {
	mock, err := newUnixMockServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1.53/containers/json" && r.URL.Query().Get("all") == "true" {
			containers := []Container{
				{
					ID:     "abc123def456",
					Names:  []string{"/running-app"},
					Image:  "nginx:latest",
					State:  "running",
					Status: "Up 2 hours",
					Labels: map[string]string{
						"dr.enable": "true",
						"dr.host":   "app.example.com",
					},
				},
				{
					ID:     "def456abc789",
					Names:  []string{"/stopped-app"},
					Image:  "redis:latest",
					State:  "exited",
					Status: "Exited (0) 1 hour ago",
					Labels: map[string]string{},
				},
			}
			json.NewEncoder(w).Encode(containers)
		} else {
			json.NewEncoder(w).Encode([]Container{})
		}
	}))
	if err != nil {
		t.Skipf("Cannot create unix socket mock: %v", err)
	}
	defer mock.Close()

	client, _ := NewDockerClient(mock.socket)
	defer client.Close()

	ctx := context.Background()
	containers, err := client.ListAllContainers(ctx)
	if err != nil {
		t.Fatalf("ListAllContainers error: %v", err)
	}
	if len(containers) != 2 {
		t.Fatalf("container count = %d, want 2", len(containers))
	}

	// Verify running container
	if containers[0].State != "running" {
		t.Errorf("state[0] = %s, want running", containers[0].State)
	}
	if containers[0].Labels["dr.host"] != "app.example.com" {
		t.Errorf("host label = %s, want app.example.com", containers[0].Labels["dr.host"])
	}

	// Verify stopped container
	if containers[1].State != "exited" {
		t.Errorf("state[1] = %s, want exited", containers[1].State)
	}
}

// TestListNetworksWithUnixMock tests ListNetworks via Unix socket mock
func TestListNetworksWithUnixMock(t *testing.T) {
	mock, err := newUnixMockServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1.53/networks" {
			networks := []Network{
				{
					ID:     "net123",
					Name:   "bridge",
					Driver: "bridge",
					Scope:  "local",
					Subnets: []Subnet{
						{Subnet: "172.17.0.0/16", Gateway: "172.17.0.1"},
					},
				},
				{
					ID:     "net456",
					Name:   "host",
					Driver: "host",
					Scope:  "local",
				},
			}
			json.NewEncoder(w).Encode(networks)
		} else {
			json.NewEncoder(w).Encode([]Network{})
		}
	}))
	if err != nil {
		t.Skipf("Cannot create unix socket mock: %v", err)
	}
	defer mock.Close()

	client, _ := NewDockerClient(mock.socket)
	defer client.Close()

	ctx := context.Background()
	networks, err := client.ListNetworks(ctx)
	if err != nil {
		t.Fatalf("ListNetworks error: %v", err)
	}
	if len(networks) != 2 {
		t.Fatalf("network count = %d, want 2", len(networks))
	}

	if networks[0].Name != "bridge" {
		t.Errorf("network[0].Name = %s, want bridge", networks[0].Name)
	}
	if len(networks[0].Subnets) != 1 {
		t.Errorf("network[0] subnets = %d, want 1", len(networks[0].Subnets))
	}
	if networks[0].Subnets[0].Subnet != "172.17.0.0/16" {
		t.Errorf("subnet = %s, want 172.17.0.0/16", networks[0].Subnets[0].Subnet)
	}
	if networks[0].Subnets[0].Gateway != "172.17.0.1" {
		t.Errorf("gateway = %s, want 172.17.0.1", networks[0].Subnets[0].Gateway)
	}
}

// TestPingWithUnixMock tests Ping via Unix socket mock
func TestPingWithUnixMock(t *testing.T) {
	mock, err := newUnixMockServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1.53/_ping" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("OK"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	if err != nil {
		t.Skipf("Cannot create unix socket mock: %v", err)
	}
	defer mock.Close()

	client, _ := NewDockerClient(mock.socket)
	defer client.Close()

	ctx := context.Background()
	if err := client.Ping(ctx); err != nil {
		t.Fatalf("Ping error: %v", err)
	}
}

// TestEventsStreamWithUnixMock tests EventsStream via Unix socket mock
func TestEventsStreamWithUnixMock(t *testing.T) {
	eventJSON1 := `{"Type":"container","Action":"start","Actor":{"ID":"abc123","Attributes":{"name":"test-app"}},"time":1700000000}`
	eventJSON2 := `{"Type":"container","Action":"stop","Actor":{"ID":"def456","Attributes":{"name":"other-app"}},"time":1700000001}`

	mock, err := newUnixMockServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1.53/events" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(eventJSON1 + "\n"))
			w.Write([]byte(eventJSON2 + "\n"))
			// Close after sending events
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	if err != nil {
		t.Skipf("Cannot create unix socket mock: %v", err)
	}
	defer mock.Close()

	client, _ := NewDockerClient(mock.socket)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	events, err := client.EventsStream(ctx, map[string]string{
		"type":  "container",
		"event": "start,stop",
	})
	if err != nil {
		t.Fatalf("EventsStream error: %v", err)
	}

	// Read first event
	select {
	case event, ok := <-events:
		if !ok {
			t.Fatal("events channel closed unexpectedly")
		}
		if event.Type != "container" {
			t.Errorf("event.Type = %s, want container", event.Type)
		}
		if event.Action != "start" {
			t.Errorf("event.Action = %s, want start", event.Action)
		}
		if event.Actor.ID != "abc123" {
			t.Errorf("event.Actor.ID = %s, want abc123", event.Actor.ID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for first event")
	}

	// Read second event
	select {
	case event, ok := <-events:
		if !ok {
			return // channel closed, that's fine
		}
		if event.Action != "stop" {
			t.Errorf("event.Action = %s, want stop", event.Action)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for second event")
	}
}

// TestEventsStreamCancelledContext tests that EventsStream returns when context is cancelled
func TestEventsStreamCancelledContext(t *testing.T) {
	mock, err := newUnixMockServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Keep connection open but don't send anything
		time.Sleep(5 * time.Second)
	}))
	if err != nil {
		t.Skipf("Cannot create unix socket mock: %v", err)
	}
	defer mock.Close()

	client, _ := NewDockerClient(mock.socket)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	events, err := client.EventsStream(ctx, nil)
	if err != nil {
		t.Fatalf("EventsStream error: %v", err)
	}

	// Should get no events before context cancellation
	select {
	case _, ok := <-events:
		if ok {
			t.Error("unexpected event received")
		}
		// Channel closed due to context cancellation — expected
	case <-time.After(3 * time.Second):
		t.Fatal("events channel should close when context is cancelled")
	}
}

// TestDoRequestErrorStatus tests doRequest with non-2xx response
func TestDoRequestErrorStatus(t *testing.T) {
	mock, err := newUnixMockServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal server error"))
	}))
	if err != nil {
		t.Skipf("Cannot create unix socket mock: %v", err)
	}
	defer mock.Close()

	client, _ := NewDockerClient(mock.socket)
	defer client.Close()

	ctx := context.Background()
	_, err = client.ListContainers(ctx)
	if err == nil {
		t.Fatal("ListContainers should fail with 500 response")
	}
}

// TestValidateContainerID tests the container ID validator
func TestValidateContainerID(t *testing.T) {
	tests := []struct {
		id    string
		valid bool
	}{
		{"abc123def456", true},
		{"ABC123DEF456", true},
		{"a1b2c3d4e5f6", true},
		{"", false},
		{"abc-123", false}, // dash not allowed
		{"abc_123", false}, // underscore not allowed
		{"abc.123", false}, // dot not allowed
		{"abc 123", false}, // space not allowed
		{"abc/123", false}, // slash not allowed
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			err := validateContainerID(tt.id)
			if tt.valid && err != nil {
				t.Errorf("validateContainerID(%q) = %v, want nil", tt.id, err)
			}
			if !tt.valid && err == nil {
				t.Errorf("validateContainerID(%q) = nil, want error", tt.id)
			}
		})
	}
}

// TestSetTimeout tests the client timeout setter
func TestSetTimeout(t *testing.T) {
	client, _ := NewDockerClient("/tmp/test.sock")
	defer client.Close()

	if client.timeout != 30*time.Second {
		t.Errorf("default timeout = %v, want 30s", client.timeout)
	}

	client.SetTimeout(5 * time.Second)
	if client.timeout != 5*time.Second {
		t.Errorf("timeout after SetTimeout = %v, want 5s", client.timeout)
	}
}

// TestInspectContainerInvalidID tests that InspectContainer rejects invalid IDs
func TestInspectContainerInvalidID(t *testing.T) {
	client, _ := NewDockerClient("/tmp/test.sock")
	defer client.Close()

	ctx := context.Background()

	_, err := client.InspectContainer(ctx, "invalid-id-with-dash")
	if err == nil {
		t.Error("InspectContainer should reject invalid container ID")
	}

	_, err = client.InspectContainer(ctx, "")
	if err == nil {
		t.Error("InspectContainer should reject empty container ID")
	}
}
