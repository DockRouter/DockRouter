//go:build integration
// +build integration

package discovery

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"testing"
	"time"
)

// TestDockerClientWithMockSocket tests DockerClient with a mock Unix socket server
// This test requires the integration build tag to avoid running in CI

func TestDockerClientWithMockSocket_Ping(t *testing.T) {
	// Create a temporary Unix socket
	socketPath := "/tmp/dockrouter_test_" + time.Now().Format("20060102150405") + ".sock"

	// Start mock server
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Skipf("Cannot create Unix socket: %v (requires Unix-like OS)", err)
	}
	defer os.Remove(socketPath)
	defer listener.Close()

	// Handle connections
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go handleMockDockerConnection(conn)
		}
	}()

	// Create client
	client, err := NewDockerClient(socketPath)
	if err != nil {
		t.Fatalf("NewDockerClient failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = client.Ping(ctx)
	if err != nil {
		t.Errorf("Ping failed: %v", err)
	}
}

func handleMockDockerConnection(conn net.Conn) {
	defer conn.Close()

	reader := bufio.NewReader(conn)
	for {
		req, err := http.ReadRequest(reader)
		if err != nil {
			return
		}

		// Handle different paths
		var respBody []byte
		var statusCode int = http.StatusOK

		switch {
		case req.URL.Path == "/v1.53/_ping":
			respBody = []byte("OK")
		case req.URL.Path == "/v1.53/containers/json":
			containers := []Container{
				{
					ID:     "abc123def4567890123456789012345678901234567890123456789012345678",
					Names:  []string{"/test-container"},
					Image:  "nginx:latest",
					State:  "running",
					Status: "Up 1 hour",
					Labels: map[string]string{"dr.enable": "true", "dr.host": "test.example.com"},
				},
			}
			respBody, _ = json.Marshal(containers)
		case req.URL.Path == "/v1.53/networks":
			networks := []Network{
				{
					ID:     "net123",
					Name:   "bridge",
					Driver: "bridge",
					Scope:  "local",
				},
			}
			respBody, _ = json.Marshal(networks)
		case len(req.URL.Path) > 30 && req.URL.Path[:30] == "/v1.53/containers/":
			// Inspect container
			detail := &ContainerDetail{
				ID:   req.URL.Path[30:94],
				Name: "/test-container",
				State: ContainerState{
					Status:  "running",
					Running: true,
					Healthy: true,
				},
				Network: ContainerNetwork{
					IPAddress: "172.17.0.5",
					Networks: map[string]NetworkInfo{
						"bridge": {IPAddress: "172.17.0.5", Gateway: "172.17.0.1"},
					},
				},
				Config: ContainerConfig{
					Image: "nginx:latest",
					Labels: map[string]string{
						"dr.enable": "true",
						"dr.host":   "test.example.com",
					},
				},
			}
			respBody, _ = json.Marshal(detail)
		default:
			statusCode = http.StatusNotFound
			respBody = []byte("Not found")
		}

		// Write response
		resp := http.Response{
			StatusCode: statusCode,
			Header:     make(http.Header),
			Body:       io.NopCloser(nil),
		}
		resp.Header.Set("Content-Type", "application/json")
		resp.Write(conn)
		if len(respBody) > 0 {
			conn.Write(respBody)
		}
	}
}

func TestDockerClientWithMockSocket_ListContainers(t *testing.T) {
	socketPath := "/tmp/dockrouter_test_list_" + time.Now().Format("20060102150405") + ".sock"

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Skipf("Cannot create Unix socket: %v", err)
	}
	defer os.Remove(socketPath)
	defer listener.Close()

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go handleMockDockerConnection(conn)
		}
	}()

	client, _ := NewDockerClient(socketPath)
	ctx := context.Background()

	containers, err := client.ListContainers(ctx)
	if err != nil {
		t.Errorf("ListContainers failed: %v", err)
	}
	if len(containers) != 1 {
		t.Errorf("Expected 1 container, got %d", len(containers))
	}
}

func TestDockerClientWithMockSocket_InspectContainer(t *testing.T) {
	socketPath := "/tmp/dockrouter_test_inspect_" + time.Now().Format("20060102150405") + ".sock"

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Skipf("Cannot create Unix socket: %v", err)
	}
	defer os.Remove(socketPath)
	defer listener.Close()

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go handleMockDockerConnection(conn)
		}
	}()

	client, _ := NewDockerClient(socketPath)
	ctx := context.Background()

	containerID := "abc123def4567890123456789012345678901234567890123456789012345678"
	detail, err := client.InspectContainer(ctx, containerID)
	if err != nil {
		t.Errorf("InspectContainer failed: %v", err)
	}
	if detail == nil {
		t.Fatal("Detail should not be nil")
	}
	if detail.Name != "/test-container" {
		t.Errorf("Name = %s, want /test-container", detail.Name)
	}
}

func TestDockerClientWithMockSocket_ListNetworks(t *testing.T) {
	socketPath := "/tmp/dockrouter_test_nets_" + time.Now().Format("20060102150405") + ".sock"

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Skipf("Cannot create Unix socket: %v", err)
	}
	defer os.Remove(socketPath)
	defer listener.Close()

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go handleMockDockerConnection(conn)
		}
	}()

	client, _ := NewDockerClient(socketPath)
	ctx := context.Background()

	networks, err := client.ListNetworks(ctx)
	if err != nil {
		t.Errorf("ListNetworks failed: %v", err)
	}
	if len(networks) != 1 {
		t.Errorf("Expected 1 network, got %d", len(networks))
	}
}
