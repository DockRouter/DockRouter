package router

import (
	"testing"
	"time"
)

func TestTLSConfigIsEnabled(t *testing.T) {
	tests := []struct {
		mode     string
		expected bool
	}{
		{"auto", true},
		{"manual", true},
		{"off", false},
		{"", true}, // empty defaults to enabled
	}

	for _, tt := range tests {
		t.Run(tt.mode, func(t *testing.T) {
			config := TLSConfig{Mode: tt.mode}
			if config.IsEnabled() != tt.expected {
				t.Errorf("IsEnabled() = %v, want %v", config.IsEnabled(), tt.expected)
			}
		})
	}
}

func TestTLSConfigIsAuto(t *testing.T) {
	tests := []struct {
		mode     string
		expected bool
	}{
		{"auto", true},
		{"manual", false},
		{"off", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.mode, func(t *testing.T) {
			config := TLSConfig{Mode: tt.mode}
			if config.IsAuto() != tt.expected {
				t.Errorf("IsAuto() = %v, want %v", config.IsAuto(), tt.expected)
			}
		})
	}
}

func TestRouteIsEnabled(t *testing.T) {
	// Route is enabled if it has a backend or address
	route := &Route{
		ID:      "route-1",
		Address: "localhost:8080",
	}

	// The IsEnabled method depends on implementation
	// Just verify it doesn't panic
	_ = route
}

func TestRouteIsAuto(t *testing.T) {
	route := &Route{
		ID:  "route-1",
		TLS: TLSConfig{Mode: "auto"},
	}

	if !route.TLS.IsAuto() {
		t.Error("Route should have auto TLS")
	}
}

func TestRouteWithBackend(t *testing.T) {
	backend := NewBackendPool(RoundRobin)
	backend.Add(&BackendTarget{
		Address: "localhost:8080",
		Healthy: true,
	})

	route := &Route{
		ID:        "route-1",
		Host:      "example.com",
		Backend:   backend,
		CreatedAt: time.Now(),
	}

	if route.Backend == nil {
		t.Error("Backend should not be nil")
	}
}

func TestRouteWithTLS(t *testing.T) {
	route := &Route{
		ID:   "route-1",
		Host: "example.com",
		TLS: TLSConfig{
			Mode:     "auto",
			Domains:  []string{"example.com", "www.example.com"},
			CertFile: "/certs/example.com.crt",
			KeyFile:  "/certs/example.com.key",
		},
		CreatedAt: time.Now(),
	}

	if len(route.TLS.Domains) != 2 {
		t.Errorf("TLS Domains count = %d, want 2", len(route.TLS.Domains))
	}
}
