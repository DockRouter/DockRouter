package discovery

import (
	"testing"
	"time"
)

func TestIsStartEvent(t *testing.T) {
	tests := []struct {
		name     string
		event    Event
		expected bool
	}{
		{
			name:     "start event",
			event:    Event{Type: "container", Action: "start"},
			expected: true,
		},
		{
			name:     "stop event",
			event:    Event{Type: "container", Action: "stop"},
			expected: false,
		},
		{
			name:     "die event",
			event:    Event{Type: "container", Action: "die"},
			expected: false,
		},
		{
			name:     "non-container event",
			event:    Event{Type: "image", Action: "start"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsStartEvent(tt.event)
			if result != tt.expected {
				t.Errorf("IsStartEvent() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestIsStopEvent(t *testing.T) {
	tests := []struct {
		name     string
		event    Event
		expected bool
	}{
		{
			name:     "stop event",
			event:    Event{Type: "container", Action: "stop"},
			expected: true,
		},
		{
			name:     "die event",
			event:    Event{Type: "container", Action: "die"},
			expected: true,
		},
		{
			name:     "start event",
			event:    Event{Type: "container", Action: "start"},
			expected: false,
		},
		{
			name:     "non-container event",
			event:    Event{Type: "network", Action: "stop"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsStopEvent(tt.event)
			if result != tt.expected {
				t.Errorf("IsStopEvent() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestIsHealthEvent(t *testing.T) {
	tests := []struct {
		name     string
		event    Event
		expected bool
	}{
		{
			name:     "health_status event",
			event:    Event{Type: "container", Action: "health_status"},
			expected: true,
		},
		{
			name:     "start event",
			event:    Event{Type: "container", Action: "start"},
			expected: false,
		},
		{
			name:     "non-container event",
			event:    Event{Type: "volume", Action: "health_status"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsHealthEvent(tt.event)
			if result != tt.expected {
				t.Errorf("IsHealthEvent() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestGetContainerID(t *testing.T) {
	event := Event{
		Type:   "container",
		Action: "start",
		Actor: EventActor{
			ID: "abc123def456",
		},
	}

	id := GetContainerID(event)
	if id != "abc123def456" {
		t.Errorf("GetContainerID() = %s, want abc123def456", id)
	}
}

func TestGetContainerName(t *testing.T) {
	tests := []struct {
		name     string
		event    Event
		expected string
	}{
		{
			name: "with name attribute",
			event: Event{
				Type:   "container",
				Action: "start",
				Actor: EventActor{
					ID: "abc123",
					Attributes: map[string]string{
						"name": "my-container",
					},
				},
			},
			expected: "my-container",
		},
		{
			name: "without attributes",
			event: Event{
				Type:   "container",
				Action: "start",
				Actor: EventActor{
					ID: "abc123",
				},
			},
			expected: "",
		},
		{
			name: "with nil attributes",
			event: Event{
				Type:   "container",
				Action: "start",
				Actor: EventActor{
					ID:         "abc123",
					Attributes: nil,
				},
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetContainerName(tt.event)
			if result != tt.expected {
				t.Errorf("GetContainerName() = %s, want %s", result, tt.expected)
			}
		})
	}
}

func TestGetContainerImage(t *testing.T) {
	tests := []struct {
		name     string
		event    Event
		expected string
	}{
		{
			name: "with image attribute",
			event: Event{
				Type:   "container",
				Action: "start",
				Actor: EventActor{
					ID: "abc123",
					Attributes: map[string]string{
						"image": "nginx:latest",
					},
				},
			},
			expected: "nginx:latest",
		},
		{
			name: "without attributes",
			event: Event{
				Type:   "container",
				Action: "start",
				Actor: EventActor{
					ID: "abc123",
				},
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetContainerImage(tt.event)
			if result != tt.expected {
				t.Errorf("GetContainerImage() = %s, want %s", result, tt.expected)
			}
		})
	}
}

func TestEventTimestamp(t *testing.T) {
	// Test with known timestamp
	event := Event{
		Type:   "container",
		Action: "start",
		Time:   1609459200, // 2021-01-01 00:00:00 UTC
	}

	ts := EventTimestamp(event)
	expected := time.Unix(1609459200, 0)

	if !ts.Equal(expected) {
		t.Errorf("EventTimestamp() = %v, want %v", ts, expected)
	}
}

func TestContainerInfoChanged(t *testing.T) {
	baseTime := time.Now()

	base := &ContainerInfo{
		ID:        "abc123",
		Name:      "test-container",
		Address:   "192.168.1.1:8080",
		Healthy:   true,
		Config:    &RouteConfig{Host: "example.com", Path: "/"},
		UpdatedAt: baseTime,
	}

	tests := []struct {
		name     string
		other    *ContainerInfo
		expected bool
	}{
		{
			name:     "identical",
			other:    &ContainerInfo{ID: "abc123", Name: "test-container", Address: "192.168.1.1:8080", Healthy: true, Config: &RouteConfig{Host: "example.com", Path: "/"}},
			expected: false,
		},
		{
			name:     "address changed",
			other:    &ContainerInfo{ID: "abc123", Name: "test-container", Address: "192.168.1.2:8080", Healthy: true, Config: &RouteConfig{Host: "example.com", Path: "/"}},
			expected: true,
		},
		{
			name:     "health changed",
			other:    &ContainerInfo{ID: "abc123", Name: "test-container", Address: "192.168.1.1:8080", Healthy: false, Config: &RouteConfig{Host: "example.com", Path: "/"}},
			expected: true,
		},
		{
			name:     "host changed",
			other:    &ContainerInfo{ID: "abc123", Name: "test-container", Address: "192.168.1.1:8080", Healthy: true, Config: &RouteConfig{Host: "new.example.com", Path: "/"}},
			expected: true,
		},
		{
			name:     "path changed",
			other:    &ContainerInfo{ID: "abc123", Name: "test-container", Address: "192.168.1.1:8080", Healthy: true, Config: &RouteConfig{Host: "example.com", Path: "/api"}},
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

func TestExtractName(t *testing.T) {
	tests := []struct {
		name     string
		names    []string
		expected string
	}{
		{
			name:     "with leading slash",
			names:    []string{"/my-container"},
			expected: "my-container",
		},
		{
			name:     "without slash",
			names:    []string{"my-container"},
			expected: "my-container",
		},
		{
			name:     "empty",
			names:    []string{},
			expected: "",
		},
		{
			name:     "nil",
			names:    nil,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractName(tt.names)
			if result != tt.expected {
				t.Errorf("extractName() = %s, want %s", result, tt.expected)
			}
		})
	}
}

func TestIntToStr(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{0, "0"},
		{1, "1"},
		{80, "80"},
		{443, "443"},
		{8080, "8080"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := intToStr(tt.input)
			if result != tt.expected {
				t.Errorf("intToStr(%d) = %s, want %s", tt.input, result, tt.expected)
			}
		})
	}
}
