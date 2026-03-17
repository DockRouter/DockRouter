package discovery

import (
	"context"
	"io"
	"net"
	"testing"
	"time"
)

func TestNewDockerClient(t *testing.T) {
	tests := []struct {
		name      string
		socketPath string
		expected  string
	}{
		{
			name:      "default socket path",
			socketPath: "",
			expected:  "/var/run/docker.sock",
		},
		{
			name:      "custom socket path",
			socketPath: "/custom/docker.sock",
			expected:  "/custom/docker.sock",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewDockerClient(tt.socketPath)
			if err != nil {
				t.Fatalf("NewDockerClient failed: %v", err)
			}
			if client.socketPath != tt.expected {
				t.Errorf("socketPath = %s, want %s", client.socketPath, tt.expected)
			}
			if client.timeout != 30*time.Second {
				t.Errorf("timeout = %v, want 30s", client.timeout)
			}
		})
	}
}

func TestDockerClientSetTimeout(t *testing.T) {
	client, _ := NewDockerClient("")
	client.SetTimeout(60 * time.Second)

	if client.timeout != 60*time.Second {
		t.Errorf("timeout = %v, want 60s", client.timeout)
	}
}

func TestDockerClientPingConnectionError(t *testing.T) {
	client, _ := NewDockerClient("/nonexistent/docker.sock")
	ctx := context.Background()

	err := client.Ping(ctx)
	if err == nil {
		t.Error("Ping should fail with nonexistent socket")
	}
}

func TestDockerClientListContainersConnectionError(t *testing.T) {
	client, _ := NewDockerClient("/nonexistent/docker.sock")
	ctx := context.Background()

	_, err := client.ListContainers(ctx)
	if err == nil {
		t.Error("ListContainers should fail with nonexistent socket")
	}
}

func TestDockerClientListAllContainersConnectionError(t *testing.T) {
	client, _ := NewDockerClient("/nonexistent/docker.sock")
	ctx := context.Background()

	_, err := client.ListAllContainers(ctx)
	if err == nil {
		t.Error("ListAllContainers should fail with nonexistent socket")
	}
}

func TestDockerClientInspectContainerConnectionError(t *testing.T) {
	client, _ := NewDockerClient("/nonexistent/docker.sock")
	ctx := context.Background()

	_, err := client.InspectContainer(ctx, "abc123")
	if err == nil {
		t.Error("InspectContainer should fail with nonexistent socket")
	}
}

func TestDockerClientListNetworksConnectionError(t *testing.T) {
	client, _ := NewDockerClient("/nonexistent/docker.sock")
	ctx := context.Background()

	_, err := client.ListNetworks(ctx)
	if err == nil {
		t.Error("ListNetworks should fail with nonexistent socket")
	}
}

func TestDockerClientEventsStreamConnectionError(t *testing.T) {
	client, _ := NewDockerClient("/nonexistent/docker.sock")
	ctx := context.Background()

	_, err := client.EventsStream(ctx, map[string]string{"type": "container"})
	if err == nil {
		t.Error("EventsStream should fail with nonexistent socket")
	}
}

func TestGetContainerIP(t *testing.T) {
	tests := []struct {
		name             string
		detail           *ContainerDetail
		preferredNetwork string
		expected         string
	}{
		{
			name: "preferred network",
			detail: &ContainerDetail{
				Network: ContainerNetwork{
					Networks: map[string]NetworkInfo{
						"custom": {IPAddress: "172.20.0.5"},
						"bridge": {IPAddress: "172.17.0.5"},
					},
				},
			},
			preferredNetwork: "custom",
			expected:         "172.20.0.5",
		},
		{
			name: "bridge network fallback",
			detail: &ContainerDetail{
				Network: ContainerNetwork{
					Networks: map[string]NetworkInfo{
						"bridge": {IPAddress: "172.17.0.5"},
					},
				},
			},
			preferredNetwork: "",
			expected:         "172.17.0.5",
		},
		{
			name: "dockrouter-net network",
			detail: &ContainerDetail{
				Network: ContainerNetwork{
					Networks: map[string]NetworkInfo{
						"dockrouter-net": {IPAddress: "172.18.0.5"},
					},
				},
			},
			preferredNetwork: "",
			expected:         "172.18.0.5",
		},
		{
			name: "any available IP",
			detail: &ContainerDetail{
				Network: ContainerNetwork{
					IPAddress: "172.17.0.100",
					Networks: map[string]NetworkInfo{
						"other": {IPAddress: "172.19.0.5"},
					},
				},
			},
			preferredNetwork: "",
			expected:         "172.19.0.5",
		},
		{
			name: "fallback to main IP",
			detail: &ContainerDetail{
				Network: ContainerNetwork{
					IPAddress: "172.17.0.100",
					Networks:  map[string]NetworkInfo{},
				},
			},
			preferredNetwork: "",
			expected:         "172.17.0.100",
		},
		{
			name: "preferred network not found",
			detail: &ContainerDetail{
				Network: ContainerNetwork{
					Networks: map[string]NetworkInfo{
						"bridge": {IPAddress: "172.17.0.5"},
					},
				},
			},
			preferredNetwork: "nonexistent",
			expected:         "172.17.0.5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetContainerIP(tt.detail, tt.preferredNetwork)
			if result != tt.expected {
				t.Errorf("GetContainerIP() = %s, want %s", result, tt.expected)
			}
		})
	}
}

func TestUnixReadCloser(t *testing.T) {
	// Test unixReadCloser - just verify it implements io.ReadCloser
	r := &unixReadCloser{}
	_ = io.Reader(r)
	_ = io.Closer(r)
}

func TestContainerStruct(t *testing.T) {
	c := Container{
		ID:      "abc123def456",
		Names:   []string{"/my-container"},
		Image:   "nginx:latest",
		State:   "running",
		Status:  "Up 2 hours",
		Labels:  map[string]string{"dr.enable": "true"},
		Ports:   []PortBinding{{PrivatePort: 80, PublicPort: 8080, Type: "tcp"}},
	}

	if c.ID != "abc123def456" {
		t.Errorf("ID = %s", c.ID)
	}
	if len(c.Names) != 1 {
		t.Errorf("Names count = %d", len(c.Names))
	}
}

func TestContainerDetailStruct(t *testing.T) {
	detail := ContainerDetail{
		ID:   "abc123",
		Name: "/my-container",
		State: ContainerState{
			Status:  "running",
			Running: true,
			Healthy: true,
		},
		Config: ContainerConfig{
			Labels: map[string]string{"dr.enable": "true"},
			Image:  "nginx:latest",
		},
		Network: ContainerNetwork{
			IPAddress: "172.17.0.5",
			Networks: map[string]NetworkInfo{
				"bridge": {IPAddress: "172.17.0.5", Gateway: "172.17.0.1"},
			},
		},
	}

	if detail.ID != "abc123" {
		t.Errorf("ID = %s", detail.ID)
	}
	if !detail.State.Running {
		t.Error("State.Running should be true")
	}
}

func TestEventStruct(t *testing.T) {
	event := Event{
		Type:   "container",
		Action: "start",
		Actor: EventActor{
			ID: "container123",
			Attributes: map[string]string{
				"name":  "my-container",
				"image": "nginx:latest",
			},
		},
		Time:     1609459200,
		TimeNano: 1609459200000000000,
	}

	if event.Type != "container" {
		t.Errorf("Type = %s", event.Type)
	}
	if event.Actor.ID != "container123" {
		t.Errorf("Actor.ID = %s", event.Actor.ID)
	}
}

func TestNetworkStruct(t *testing.T) {
	network := Network{
		ID:     "net123",
		Name:   "bridge",
		Driver: "bridge",
		Scope:  "local",
	}

	if network.ID != "net123" {
		t.Errorf("ID = %s", network.ID)
	}
}

func TestPortBindingStruct(t *testing.T) {
	port := PortBinding{
		PrivatePort: 80,
		PublicPort:  8080,
		Type:        "tcp",
		IP:          "0.0.0.0",
	}

	if port.PrivatePort != 80 {
		t.Errorf("PrivatePort = %d", port.PrivatePort)
	}
}

func TestDetectPort(t *testing.T) {
	tests := []struct {
		name     string
		ports    []PortBinding
		detail   *ContainerDetail
		expected int
	}{
		{
			name: "with published port",
			ports: []PortBinding{
				{PrivatePort: 80, PublicPort: 8080, Type: "tcp"},
			},
			detail:   &ContainerDetail{},
			expected: 80,
		},
		{
			name:     "no published ports",
			ports:    []PortBinding{},
			detail:   &ContainerDetail{},
			expected: 0,
		},
		{
			name: "multiple ports",
			ports: []PortBinding{
				{PrivatePort: 443, PublicPort: 8443, Type: "tcp"},
				{PrivatePort: 80, PublicPort: 8080, Type: "tcp"},
			},
			detail:   &ContainerDetail{},
			expected: 443,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectPort(tt.ports, tt.detail)
			if result != tt.expected {
				t.Errorf("detectPort() = %d, want %d", result, tt.expected)
			}
		})
	}
}

func TestExtractNameFromDetail(t *testing.T) {
	tests := []struct {
		name     string
		detail   *ContainerDetail
		expected string
	}{
		{
			name:     "with leading slash",
			detail:   &ContainerDetail{Name: "/my-container"},
			expected: "my-container",
		},
		{
			name:     "without slash",
			detail:   &ContainerDetail{Name: "my-container"},
			expected: "my-container",
		},
		{
			name:     "empty name",
			detail:   &ContainerDetail{Name: ""},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractNameFromDetail(tt.detail)
			if result != tt.expected {
				t.Errorf("extractNameFromDetail() = %s, want %s", result, tt.expected)
			}
		})
	}
}

// unixReadCloser tests

func TestUnixReadCloserRead(t *testing.T) {
	// Create a pipe to simulate connection
	pr, pw := io.Pipe()

	body := io.NopCloser(pr)
	conn := &mockConn{}

	reader := &unixReadCloser{
		conn: conn,
		body: body,
	}

	// Write data in goroutine
	go func() {
		pw.Write([]byte("test data"))
		pw.Close()
	}()

	// Read data
	buf := make([]byte, 100)
	n, err := reader.Read(buf)
	if err != nil {
		t.Errorf("Read error: %v", err)
	}
	if string(buf[:n]) != "test data" {
		t.Errorf("Read = %q, want 'test data'", buf[:n])
	}
}

func TestUnixReadCloserClose(t *testing.T) {
	pr, _ := io.Pipe()
	body := io.NopCloser(pr)
	conn := &mockConn{}

	reader := &unixReadCloser{
		conn: conn,
		body: body,
	}

	err := reader.Close()
	if err != nil {
		t.Errorf("Close error: %v", err)
	}
}

func TestUnixReadCloserReadError(t *testing.T) {
	// Create a reader that returns an error
	errorReader := &errorReaderCloser{err: io.ErrUnexpectedEOF}

	reader := &unixReadCloser{
		conn: &mockConn{},
		body: errorReader,
	}

	buf := make([]byte, 100)
	_, err := reader.Read(buf)
	if err == nil {
		t.Error("Read should return error")
	}
}

type errorReaderCloser struct {
	err error
}

func (e *errorReaderCloser) Read(p []byte) (n int, err error) {
	return 0, e.err
}

func (e *errorReaderCloser) Close() error {
	return nil
}

// mockConn implements net.Conn for testing
type mockConn struct{}

func (m *mockConn) Read(b []byte) (n int, err error)  { return 0, nil }
func (m *mockConn) Write(b []byte) (n int, err error) { return len(b), nil }
func (m *mockConn) Close() error                       { return nil }
func (m *mockConn) LocalAddr() net.Addr               { return nil }
func (m *mockConn) RemoteAddr() net.Addr              { return nil }
func (m *mockConn) SetDeadline(t time.Time) error     { return nil }
func (m *mockConn) SetReadDeadline(t time.Time) error  { return nil }
func (m *mockConn) SetWriteDeadline(t time.Time) error { return nil }

func TestEventStreamSubscribe(t *testing.T) {
	client, _ := NewDockerClient("")
	stream := NewEventStream(client)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// This will fail because there's no Docker socket, but we verify the method doesn't panic
	_, err := stream.Subscribe(ctx)
	if err == nil {
		// Success - events channel returned
		cancel()
	} else {
		// Expected - Docker socket not available
		// Verify error contains socket connection message
		if err.Error() == "" {
			t.Error("Error should have a message")
		}
	}
}

func TestEventStreamSubscribeWithFilters(t *testing.T) {
	client, _ := NewDockerClient("")
	stream := NewEventStream(client)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	filters := map[string]string{
		"event": "start,stop",
	}

	_, err := stream.SubscribeWithFilters(ctx, filters)
	if err == nil {
		cancel()
	} else {
		// Expected - Docker socket not available
		// Verify type filter was added
		if filters["type"] != "container" {
			t.Error("SubscribeWithFilters should add type=container filter")
		}
	}
}

func TestEventStreamSubscribeLifecycle(t *testing.T) {
	client, _ := NewDockerClient("")
	stream := NewEventStream(client)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, err := stream.SubscribeLifecycle(ctx)
	if err == nil {
		cancel()
	} else {
		// Expected - Docker socket not available
		_ = err
	}
}

func TestEventStreamSubscribeAddsTypeFilter(t *testing.T) {
	// Test that SubscribeWithFilters adds type filter if missing
	client, _ := NewDockerClient("")
	stream := NewEventStream(client)

	ctx := context.Background()
	filters := map[string]string{
		"event": "start",
	}

	// Call SubscribeWithFilters - it will fail due to no socket,
	// but we can verify it adds the type filter
	_, _ = stream.SubscribeWithFilters(ctx, filters)

	if filters["type"] != "container" {
		t.Error("SubscribeWithFilters should add type=container filter when missing")
	}
}

func TestEventStreamSubscribePreservesExistingType(t *testing.T) {
	// Test that SubscribeWithFilters preserves existing type filter
	client, _ := NewDockerClient("")
	stream := NewEventStream(client)

	ctx := context.Background()
	filters := map[string]string{
		"type":  "image",
		"event": "pull",
	}

	_, _ = stream.SubscribeWithFilters(ctx, filters)

	// Should keep original type
	if filters["type"] != "image" {
		t.Error("SubscribeWithFilters should preserve existing type filter")
	}
}

func TestDockerClientInspectContainerError(t *testing.T) {
	client, _ := NewDockerClient("/nonexistent/docker.sock")
	ctx := context.Background()

	_, err := client.InspectContainer(ctx, "nonexistent")
	if err == nil {
		t.Error("InspectContainer should fail with nonexistent socket")
	}
}

func TestDockerClientEventsStreamError(t *testing.T) {
	client, _ := NewDockerClient("/nonexistent/docker.sock")
	ctx := context.Background()

	_, err := client.EventsStream(ctx, map[string]string{"type": "container"})
	if err == nil {
		t.Error("EventsStream should fail with nonexistent socket")
	}
}

func TestContainerHostConfigStruct(t *testing.T) {
	config := ContainerHostConfig{
		NetworkMode: "host",
	}

	if config.NetworkMode != "host" {
		t.Errorf("NetworkMode = %s, want host", config.NetworkMode)
	}
}

func TestContainerStateStruct(t *testing.T) {
	state := ContainerState{
		Status:    "running",
		Running:   true,
		Healthy:   true,
		ExitCode:  0,
		StartedAt: "2024-01-01T00:00:00Z",
	}

	if !state.Running {
		t.Error("Running should be true")
	}
	if !state.Healthy {
		t.Error("Healthy should be true")
	}
	if state.Status != "running" {
		t.Errorf("Status = %s, want running", state.Status)
	}
}

func TestContainerConfigStruct(t *testing.T) {
	config := ContainerConfig{
		Labels: map[string]string{"dr.enable": "true"},
		Image:  "nginx:latest",
	}

	if config.Image != "nginx:latest" {
		t.Errorf("Image = %s, want nginx:latest", config.Image)
	}
	if config.Labels["dr.enable"] != "true" {
		t.Error("Labels[dr.enable] should be true")
	}
}

func TestContainerNetworkStruct(t *testing.T) {
	network := ContainerNetwork{
		IPAddress: "172.17.0.5",
		Networks: map[string]NetworkInfo{
			"bridge": {IPAddress: "172.17.0.5", Gateway: "172.17.0.1"},
		},
	}

	if network.IPAddress != "172.17.0.5" {
		t.Errorf("IPAddress = %s, want 172.17.0.5", network.IPAddress)
	}
	if len(network.Networks) != 1 {
		t.Errorf("Networks count = %d, want 1", len(network.Networks))
	}
}

func TestPortMapStruct(t *testing.T) {
	portMap := PortMap{
		"80/tcp": {{PrivatePort: 80, PublicPort: 8080, Type: "tcp"}},
	}

	if len(portMap) != 1 {
		t.Errorf("PortMap length = %d, want 1", len(portMap))
	}
}

func TestSubnetStruct(t *testing.T) {
	subnet := Subnet{
		Subnet: "172.17.0.0/16",
		Gateway: "172.17.0.1",
	}

	if subnet.Subnet != "172.17.0.0/16" {
		t.Errorf("Subnet = %s, want 172.17.0.0/16", subnet.Subnet)
	}
}

func TestNetworkInfoStruct(t *testing.T) {
	info := NetworkInfo{
		IPAddress: "172.17.0.5",
		Gateway:   "172.17.0.1",
	}

	if info.IPAddress != "172.17.0.5" {
		t.Errorf("IPAddress = %s, want 172.17.0.5", info.IPAddress)
	}
	if info.Gateway != "172.17.0.1" {
		t.Errorf("Gateway = %s, want 172.17.0.1", info.Gateway)
	}
}
