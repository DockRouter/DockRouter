package discovery

import (
	"testing"
)

// --- GetContainerIP edge cases ---

func TestGetContainerIPEmptyPreferredNetwork(t *testing.T) {
	// When preferred network is empty but exists
	detail := &ContainerDetail{
		Network: ContainerNetwork{
			Networks: map[string]NetworkInfo{
				"custom": {IPAddress: "192.168.1.5"},
			},
		},
	}

	// With empty preferred network, should use fallback logic
	result := GetContainerIP(detail, "")
	// Since "custom" is not in the fallback list, it should return any available IP
	if result == "" {
		t.Error("GetContainerIP should return an IP")
	}
}

func TestGetContainerIPMultipleNetworksPriority(t *testing.T) {
	// Test that bridge is preferred over other networks
	detail := &ContainerDetail{
		Network: ContainerNetwork{
			Networks: map[string]NetworkInfo{
				"other1":   {IPAddress: "10.0.0.5"},
				"bridge":   {IPAddress: "172.17.0.5"},
				"other2":   {IPAddress: "192.168.0.5"},
			},
		},
	}

	result := GetContainerIP(detail, "")
	// Should prefer bridge network
	if result != "172.17.0.5" {
		t.Errorf("GetContainerIP = %s, want 172.17.0.5 (bridge)", result)
	}
}

func TestGetContainerIPDockrouterNetPriority(t *testing.T) {
	// Test that dockrouter-net is preferred over bridge
	detail := &ContainerDetail{
		Network: ContainerNetwork{
			Networks: map[string]NetworkInfo{
				"bridge":         {IPAddress: "172.17.0.5"},
				"dockrouter-net": {IPAddress: "172.18.0.5"},
			},
		},
	}

	result := GetContainerIP(detail, "dockrouter-net")
	// When explicitly requesting dockrouter-net, should return that
	if result != "172.18.0.5" {
		t.Errorf("GetContainerIP = %s, want 172.18.0.5 (dockrouter-net)", result)
	}
}

func TestGetContainerIPDefaultNetworkPriority(t *testing.T) {
	// Test that default is preferred when explicitly requested
	detail := &ContainerDetail{
		Network: ContainerNetwork{
			Networks: map[string]NetworkInfo{
				"bridge":  {IPAddress: "172.17.0.5"},
				"default": {IPAddress: "172.19.0.5"},
			},
		},
	}

	result := GetContainerIP(detail, "default")
	// When explicitly requesting default, should return that
	if result != "172.19.0.5" {
		t.Errorf("GetContainerIP = %s, want 172.19.0.5 (default)", result)
	}
}

func TestGetContainerIPPreferredNetworkEmptyIP(t *testing.T) {
	// When preferred network has empty IP, should fall back
	detail := &ContainerDetail{
		Network: ContainerNetwork{
			Networks: map[string]NetworkInfo{
				"preferred": {IPAddress: ""},
				"bridge":    {IPAddress: "172.17.0.5"},
			},
		},
	}

	result := GetContainerIP(detail, "preferred")
	// Should fall back to bridge since preferred has empty IP
	if result != "172.17.0.5" {
		t.Errorf("GetContainerIP = %s, want 172.17.0.5 (fallback)", result)
	}
}

func TestGetContainerIPNoNetworks(t *testing.T) {
	// When no networks exist, should return empty string
	detail := &ContainerDetail{
		Network: ContainerNetwork{
			Networks: map[string]NetworkInfo{},
			IPAddress: "172.17.0.100",
		},
	}

	result := GetContainerIP(detail, "")
	// Should fall back to main IPAddress
	if result != "172.17.0.100" {
		t.Errorf("GetContainerIP = %s, want 172.17.0.100", result)
	}
}

func TestGetContainerIPAllNetworksEmpty(t *testing.T) {
	// When all networks have empty IPs
	detail := &ContainerDetail{
		Network: ContainerNetwork{
			Networks: map[string]NetworkInfo{
				"bridge": {IPAddress: ""},
				"custom": {IPAddress: ""},
			},
			IPAddress: "172.17.0.100",
		},
	}

	result := GetContainerIP(detail, "")
	// Should fall back to main IPAddress
	if result != "172.17.0.100" {
		t.Errorf("GetContainerIP = %s, want 172.17.0.100", result)
	}
}

func TestGetContainerIPDockrouterNetDefaultPriority(t *testing.T) {
	// dockrouter-net > default > bridge priority when explicitly requested
	detail := &ContainerDetail{
		Network: ContainerNetwork{
			Networks: map[string]NetworkInfo{
				"bridge":         {IPAddress: "172.17.0.5"},
				"default":        {IPAddress: "172.19.0.5"},
				"dockrouter-net": {IPAddress: "172.18.0.5"},
			},
		},
	}

	// Test explicit dockrouter-net request
	result := GetContainerIP(detail, "dockrouter-net")
	if result != "172.18.0.5" {
		t.Errorf("GetContainerIP = %s, want 172.18.0.5 (dockrouter-net)", result)
	}

	// Test explicit default request
	result = GetContainerIP(detail, "default")
	if result != "172.19.0.5" {
		t.Errorf("GetContainerIP = %s, want 172.19.0.5 (default)", result)
	}
}

func TestGetContainerIPFirstAvailableNetwork(t *testing.T) {
	// When none of the priority networks exist, return first available
	detail := &ContainerDetail{
		Network: ContainerNetwork{
			Networks: map[string]NetworkInfo{
				"custom1": {IPAddress: "10.0.0.5"},
				"custom2": {IPAddress: "10.0.0.6"},
			},
			IPAddress: "172.17.0.100",
		},
	}

	result := GetContainerIP(detail, "")
	// Should return first available network IP (iteration order not guaranteed, but one of them)
	if result != "10.0.0.5" && result != "10.0.0.6" {
		t.Errorf("GetContainerIP = %s, want one of the network IPs", result)
	}
}

// --- Network struct tests ---

func TestNetworkWithSubnets(t *testing.T) {
	network := Network{
		ID:     "net123",
		Name:   "my-network",
		Driver: "bridge",
		Scope:  "local",
		Subnets: []Subnet{
			{
				Subnet:  "172.20.0.0/16",
				Gateway: "172.20.0.1",
			},
			{
				Subnet:  "10.0.0.0/8",
				Gateway: "10.0.0.1",
			},
		},
	}

	if len(network.Subnets) != 2 {
		t.Errorf("Subnets count = %d, want 2", len(network.Subnets))
	}

	if network.Subnets[0].Subnet != "172.20.0.0/16" {
		t.Errorf("First subnet = %s", network.Subnets[0].Subnet)
	}
}

func TestNetworkEmptySubnets(t *testing.T) {
	network := Network{
		ID:      "net123",
		Name:    "empty-network",
		Driver:  "bridge",
		Scope:   "local",
		Subnets: []Subnet{},
	}

	if len(network.Subnets) != 0 {
		t.Errorf("Subnets should be empty, got %d", len(network.Subnets))
	}
}

// --- Container state tests ---

func TestContainerStateNotRunning(t *testing.T) {
	state := ContainerState{
		Status:    "exited",
		Running:   false,
		Healthy:   false,
		ExitCode:  1,
		StartedAt: "2024-01-01T00:00:00Z",
	}

	if state.Running {
		t.Error("Running should be false")
	}
	if state.ExitCode != 1 {
		t.Errorf("ExitCode = %d, want 1", state.ExitCode)
	}
}

func TestContainerStateRestarting(t *testing.T) {
	state := ContainerState{
		Status:    "restarting",
		Running:   true, // Still considered running while restarting
		Healthy:   false,
		ExitCode:  0,
		StartedAt: "2024-01-01T00:00:00Z",
	}

	if !state.Running {
		t.Error("Running should be true for restarting container")
	}
}

func TestContainerStatePaused(t *testing.T) {
	state := ContainerState{
		Status:    "paused",
		Running:   true, // Paused containers are still running
		Healthy:   true,
		ExitCode:  0,
		StartedAt: "2024-01-01T00:00:00Z",
	}

	if !state.Running {
		t.Error("Running should be true for paused container")
	}
	if !state.Healthy {
		t.Error("Healthy should be true")
	}
}

// --- ContainerConfig edge cases ---

func TestContainerConfigEmptyLabels(t *testing.T) {
	config := ContainerConfig{
		Labels: map[string]string{},
		Image:  "nginx:latest",
	}

	if len(config.Labels) != 0 {
		t.Errorf("Labels should be empty, got %d", len(config.Labels))
	}
}

func TestContainerConfigNilLabels(t *testing.T) {
	config := ContainerConfig{
		Labels: nil,
		Image:  "nginx:latest",
	}

	if config.Labels != nil {
		t.Error("Labels should be nil")
	}
}

// --- ContainerDetail edge cases ---

func TestContainerDetailWithHostConfig(t *testing.T) {
	detail := ContainerDetail{
		ID:   "abc123",
		Name: "/test-container",
		HostConfig: ContainerHostConfig{
			NetworkMode: "host",
		},
	}

	if detail.HostConfig.NetworkMode != "host" {
		t.Errorf("NetworkMode = %s, want host", detail.HostConfig.NetworkMode)
	}
}

func TestContainerDetailWithEmptyHostConfig(t *testing.T) {
	detail := ContainerDetail{
		ID:         "abc123",
		Name:       "/test-container",
		HostConfig: ContainerHostConfig{},
	}

	if detail.HostConfig.NetworkMode != "" {
		t.Errorf("NetworkMode = %s, want empty", detail.HostConfig.NetworkMode)
	}
}

// --- PortMap tests ---

func TestPortMapMultiplePorts(t *testing.T) {
	portMap := PortMap{
		"80/tcp":  {{PrivatePort: 80, PublicPort: 8080, Type: "tcp"}},
		"443/tcp": {{PrivatePort: 443, PublicPort: 8443, Type: "tcp"}},
		"53/udp":  {{PrivatePort: 53, PublicPort: 5353, Type: "udp"}},
	}

	if len(portMap) != 3 {
		t.Errorf("PortMap length = %d, want 3", len(portMap))
	}

	if len(portMap["80/tcp"]) != 1 {
		t.Errorf("80/tcp bindings = %d, want 1", len(portMap["80/tcp"]))
	}
}

func TestPortMapEmpty(t *testing.T) {
	portMap := PortMap{}

	if len(portMap) != 0 {
		t.Errorf("PortMap should be empty, got %d", len(portMap))
	}
}

// --- EventActor tests ---

func TestEventActorWithAttributes(t *testing.T) {
	actor := EventActor{
		ID: "container123",
		Attributes: map[string]string{
			"name":  "my-container",
			"image": "nginx:latest",
			"dr.enable": "true",
		},
	}

	if actor.ID != "container123" {
		t.Errorf("ID = %s", actor.ID)
	}

	if len(actor.Attributes) != 3 {
		t.Errorf("Attributes count = %d, want 3", len(actor.Attributes))
	}
}

func TestEventActorEmptyAttributes(t *testing.T) {
	actor := EventActor{
		ID:         "container123",
		Attributes: map[string]string{},
	}

	if len(actor.Attributes) != 0 {
		t.Errorf("Attributes should be empty, got %d", len(actor.Attributes))
	}
}

// --- Subnet tests ---

func TestSubnetIPv6(t *testing.T) {
	subnet := Subnet{
		Subnet:  "2001:db8::/64",
		Gateway: "2001:db8::1",
	}

	if subnet.Subnet != "2001:db8::/64" {
		t.Errorf("Subnet = %s", subnet.Subnet)
	}
}

func TestSubnetEmpty(t *testing.T) {
	subnet := Subnet{
		Subnet:  "",
		Gateway: "",
	}

	if subnet.Subnet != "" {
		t.Error("Subnet should be empty")
	}
}
