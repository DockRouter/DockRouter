// Package discovery handles Docker container discovery
package discovery

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DockerAPIVersion is the Docker API version to use
const DockerAPIVersion = "v1.53"

// DockerClient communicates with Docker daemon via Unix socket
type DockerClient struct {
	socketPath string
	timeout    time.Duration
}

// NewDockerClient creates a new Docker API client
func NewDockerClient(socketPath string) (*DockerClient, error) {
	if socketPath == "" {
		socketPath = "/var/run/docker.sock"
	}
	return &DockerClient{
		socketPath: socketPath,
		timeout:    30 * time.Second,
	}, nil
}

// SetTimeout sets the HTTP timeout
func (c *DockerClient) SetTimeout(d time.Duration) {
	c.timeout = d
}

// doRequest performs an HTTP request over Unix socket
func (c *DockerClient) doRequest(ctx context.Context, method, path string) ([]byte, error) {
	// Create Unix socket connection
	conn, err := net.DialTimeout("unix", c.socketPath, c.timeout)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Docker socket: %w", err)
	}
	defer conn.Close()

	// Set deadline from context
	if deadline, ok := ctx.Deadline(); ok {
		conn.SetDeadline(deadline)
	} else {
		conn.SetDeadline(time.Now().Add(c.timeout))
	}

	// Build HTTP request
	url := fmt.Sprintf("http://localhost/%s%s", DockerAPIVersion, path)
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req = req.WithContext(ctx)

	// Send request
	if err := req.Write(conn); err != nil {
		return nil, fmt.Errorf("failed to write request: %w", err)
	}

	// Read response
	resp, err := http.ReadResponse(bufio.NewReader(conn), req)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	defer resp.Body.Close()

	// Check status
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Docker API error: %s - %s", resp.Status, string(body))
	}

	// Read body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return body, nil
}

// doStreamRequest performs a streaming HTTP request over Unix socket
func (c *DockerClient) doStreamRequest(ctx context.Context, method, path string) (io.ReadCloser, error) {
	// Create Unix socket connection
	conn, err := net.DialTimeout("unix", c.socketPath, c.timeout)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Docker socket: %w", err)
	}

	// Build HTTP request
	url := fmt.Sprintf("http://localhost/%s%s", DockerAPIVersion, path)
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req = req.WithContext(ctx)

	// Send request
	if err := req.Write(conn); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to write request: %w", err)
	}

	// Read response
	resp, err := http.ReadResponse(bufio.NewReader(conn), req)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Check status
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		conn.Close()
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Docker API error: %s - %s", resp.Status, string(body))
	}

	return &unixReadCloser{conn: conn, body: resp.Body}, nil
}

// unixReadCloser wraps connection and body for proper closing
type unixReadCloser struct {
	conn net.Conn
	body io.ReadCloser
}

func (r *unixReadCloser) Read(p []byte) (n int, err error) {
	return r.body.Read(p)
}

func (r *unixReadCloser) Close() error {
	r.body.Close()
	return r.conn.Close()
}

// Ping tests connectivity to Docker daemon
func (c *DockerClient) Ping(ctx context.Context) error {
	_, err := c.doRequest(ctx, http.MethodGet, "/_ping")
	return err
}

// Container represents a Docker container summary
type Container struct {
	ID         string            `json:"Id"`
	Names      []string          `json:"Names"`
	Image      string            `json:"Image"`
	State      string            `json:"State"`
	Status     string            `json:"Status"`
	Labels     map[string]string `json:"Labels"`
	Ports      []PortBinding     `json:"Ports"`
	HostConfig *HostConfig       `json:"HostConfig,omitempty"`
}

// HostConfig contains container host configuration
type HostConfig struct {
	NetworkMode string `json:"NetworkMode"`
}

// PortBinding represents a port mapping
type PortBinding struct {
	PrivatePort int    `json:"PrivatePort"`
	PublicPort  int    `json:"PublicPort"`
	Type        string `json:"Type"`
	IP          string `json:"IP,omitempty"`
}

// ListContainers returns all running containers
func (c *DockerClient) ListContainers(ctx context.Context) ([]Container, error) {
	// Add filters to only get running containers with dr.enable=true
	path := "/containers/json?status=running"

	body, err := c.doRequest(ctx, http.MethodGet, path)
	if err != nil {
		return nil, err
	}

	var containers []Container
	if err := json.Unmarshal(body, &containers); err != nil {
		return nil, fmt.Errorf("failed to parse containers: %w", err)
	}

	return containers, nil
}

// ListAllContainers returns all containers (including stopped)
func (c *DockerClient) ListAllContainers(ctx context.Context) ([]Container, error) {
	path := "/containers/json?all=true"

	body, err := c.doRequest(ctx, http.MethodGet, path)
	if err != nil {
		return nil, err
	}

	var containers []Container
	if err := json.Unmarshal(body, &containers); err != nil {
		return nil, fmt.Errorf("failed to parse containers: %w", err)
	}

	return containers, nil
}

// ContainerDetail represents detailed container info
type ContainerDetail struct {
	ID         string                 `json:"Id"`
	Name       string                 `json:"Name"`
	State      ContainerState         `json:"State"`
	Config     ContainerConfig        `json:"Config"`
	Network    ContainerNetwork       `json:"NetworkSettings"`
	HostConfig ContainerHostConfig    `json:"HostConfig"`
}

// ContainerState holds container state info
type ContainerState struct {
	Status    string `json:"Status"`
	Running   bool   `json:"Running"`
	Healthy   bool   `json:"Healthy,omitempty"`
	ExitCode  int    `json:"ExitCode"`
	StartedAt string `json:"StartedAt"`
}

// ContainerConfig holds container configuration
type ContainerConfig struct {
	Labels map[string]string `json:"Labels"`
	Image  string            `json:"Image"`
}

// ContainerNetwork holds network settings
type ContainerNetwork struct {
	Networks map[string]NetworkInfo `json:"Networks"`
	Ports    PortMap                `json:"Ports"`
	IPAddress string                `json:"IPAddress"`
}

// NetworkInfo contains container network details
type NetworkInfo struct {
	IPAddress string `json:"IPAddress"`
	Gateway   string `json:"Gateway"`
}

// PortMap holds port mappings
type PortMap map[string][]PortBinding

// ContainerHostConfig holds host configuration
type ContainerHostConfig struct {
	NetworkMode string `json:"NetworkMode"`
}

// InspectContainer returns detailed info for a container
func (c *DockerClient) InspectContainer(ctx context.Context, id string) (*ContainerDetail, error) {
	path := fmt.Sprintf("/containers/%s/json", id)

	body, err := c.doRequest(ctx, http.MethodGet, path)
	if err != nil {
		return nil, err
	}

	var detail ContainerDetail
	if err := json.Unmarshal(body, &detail); err != nil {
		return nil, fmt.Errorf("failed to parse container detail: %w", err)
	}

	return &detail, nil
}

// Event represents a Docker container event
type Event struct {
	Type      string          `json:"Type"`
	Action    string          `json:"Action"`
	Actor     EventActor      `json:"Actor"`
	Time      int64           `json:"time"`
	TimeNano  int64           `json:"timeNano,omitempty"`
}

// EventActor holds the actor info in an event
type EventActor struct {
	ID         string            `json:"ID"`
	Attributes map[string]string `json:"Attributes"`
}

// EventsStream starts streaming Docker events
func (c *DockerClient) EventsStream(ctx context.Context, filters map[string]string) (<-chan Event, error) {
	// Build filter query - Docker API expects filters as JSON with array values
	// e.g., {"type":["container"],"event":["start","stop","die"]}
	apiFilters := make(map[string][]string)
	for k, v := range filters {
		// Split comma-separated values into array
		apiFilters[k] = strings.Split(v, ",")
	}

	filterJSON, _ := json.Marshal(apiFilters)
	path := fmt.Sprintf("/events?filters=%s", url.QueryEscape(string(filterJSON)))

	stream, err := c.doStreamRequest(ctx, http.MethodGet, path)
	if err != nil {
		return nil, err
	}

	events := make(chan Event, 100)

	go func() {
		defer close(events)
		defer stream.Close()

		decoder := json.NewDecoder(stream)
		for {
			select {
			case <-ctx.Done():
				return
			default:
				var event Event
				if err := decoder.Decode(&event); err != nil {
					if err == io.EOF || strings.Contains(err.Error(), "closed") {
						return
					}
					continue
				}
				events <- event
			}
		}
	}()

	return events, nil
}

// Network represents a Docker network
type Network struct {
	ID      string   `json:"Id"`
	Name    string   `json:"Name"`
	Driver  string   `json:"Driver"`
	Scope   string   `json:"Scope"`
	Subnets []Subnet `json:"IPAM,omitempty"`
}

// Subnet holds network subnet info
type Subnet struct {
	Subnet string `json:"Subnet"`
	Gateway string `json:"Gateway"`
}

// ListNetworks returns all Docker networks
func (c *DockerClient) ListNetworks(ctx context.Context) ([]Network, error) {
	body, err := c.doRequest(ctx, http.MethodGet, "/networks")
	if err != nil {
		return nil, err
	}

	var networks []Network
	if err := json.Unmarshal(body, &networks); err != nil {
		return nil, fmt.Errorf("failed to parse networks: %w", err)
	}

	return networks, nil
}

// GetContainerIP returns the IP address for a container
func GetContainerIP(detail *ContainerDetail, preferredNetwork string) string {
	// If there's a preferred network, try it first
	if preferredNetwork != "" {
		if net, ok := detail.Network.Networks[preferredNetwork]; ok && net.IPAddress != "" {
			return net.IPAddress
		}
	}

	// Try common networks
	for _, name := range []string{"bridge", "dockrouter-net", "default"} {
		if net, ok := detail.Network.Networks[name]; ok && net.IPAddress != "" {
			return net.IPAddress
		}
	}

	// Return any available IP
	for _, net := range detail.Network.Networks {
		if net.IPAddress != "" {
			return net.IPAddress
		}
	}

	// Fallback to main IP
	return detail.Network.IPAddress
}
