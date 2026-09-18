package discovery

import (
	"net"
	"testing"
	"time"
)

func TestDeepCopyNilConfig(t *testing.T) {
	ci := &ContainerInfo{
		ID:      "abc123",
		Name:    "test",
		Image:   "nginx:latest",
		Address: "192.168.1.1",
		Port:    8080,
		Healthy: true,
	}

	cp := ci.deepCopy()
	if cp.ID != ci.ID {
		t.Errorf("ID = %s, want %s", cp.ID, ci.ID)
	}
	if cp.Config != nil {
		t.Error("Config should be nil")
	}
}

func TestDeepCopyWithLabels(t *testing.T) {
	ci := &ContainerInfo{
		ID:   "abc123",
		Name: "test",
		Labels: map[string]string{
			"dr.enable": "true",
			"dr.host":   "example.com",
		},
	}

	cp := ci.deepCopy()

	// Modify original - should not affect copy
	ci.Labels["dr.host"] = "modified.com"

	if cp.Labels["dr.host"] != "example.com" {
		t.Errorf("deepCopy Labels not isolated, got %s", cp.Labels["dr.host"])
	}
}

func TestDeepCopyWithFullConfig(t *testing.T) {
	_, cidr, _ := net.ParseCIDR("10.0.0.0/8")
	_, cidr2, _ := net.ParseCIDR("192.168.0.0/16")

	ci := &ContainerInfo{
		ID:   "full-test",
		Name: "full-container",
		Labels: map[string]string{
			"dr.enable": "true",
		},
		Config: &RouteConfig{
			Host:       "example.com",
			Port:       8080,
			TLSDomains: []string{"example.com", "www.example.com"},
			CORS: CORSConfig{
				Enabled: true,
				Origins: []string{"https://example.com"},
				Methods: []string{"GET", "POST"},
				Headers: []string{"Content-Type"},
			},
			BasicAuthUsers: []BasicAuthUser{
				{Username: "admin", Hash: "$2a$10$abc"},
			},
			IPWhitelist: []*net.IPNet{cidr},
			IPBlacklist: []*net.IPNet{cidr2},
			Middlewares: []string{"ratelimit", "compress"},
			RawLabels: map[string]string{
				"dr.enable": "true",
				"dr.host":   "example.com",
			},
		},
		Healthy:   true,
		UpdatedAt: time.Now(),
	}

	cp := ci.deepCopy()

	// Verify basic fields
	if cp.ID != ci.ID {
		t.Errorf("ID mismatch")
	}
	if cp.Name != ci.Name {
		t.Errorf("Name mismatch")
	}

	// Verify Config is deeply copied
	if cp.Config == ci.Config {
		t.Error("Config should be a different pointer")
	}

	// Modify original slices - should not affect copy
	ci.Config.TLSDomains[0] = "modified.com"
	if cp.Config.TLSDomains[0] != "example.com" {
		t.Errorf("TLSDomains not deeply copied")
	}

	ci.Config.CORS.Origins[0] = "modified"
	if cp.Config.CORS.Origins[0] != "https://example.com" {
		t.Errorf("CORS.Origins not deeply copied")
	}

	ci.Config.CORS.Methods[0] = "PUT"
	if cp.Config.CORS.Methods[0] != "GET" {
		t.Errorf("CORS.Methods not deeply copied")
	}

	ci.Config.CORS.Headers[0] = "Authorization"
	if cp.Config.CORS.Headers[0] != "Content-Type" {
		t.Errorf("CORS.Headers not deeply copied")
	}

	ci.Config.Middlewares[0] = "modified"
	if cp.Config.Middlewares[0] != "ratelimit" {
		t.Errorf("Middlewares not deeply copied")
	}

	ci.Config.RawLabels["dr.host"] = "modified"
	if cp.Config.RawLabels["dr.host"] != "example.com" {
		t.Errorf("RawLabels not deeply copied")
	}

	// Verify IPWhitelist/IPBlacklist are separate pointers
	if cp.Config.IPWhitelist[0] == ci.Config.IPWhitelist[0] {
		t.Error("IPWhitelist entries should be different pointers")
	}
	if cp.Config.IPBlacklist[0] == ci.Config.IPBlacklist[0] {
		t.Error("IPBlacklist entries should be different pointers")
	}
}

func TestDeepCopyNilSlices(t *testing.T) {
	ci := &ContainerInfo{
		ID: "nil-test",
		Config: &RouteConfig{
			Host: "example.com",
		},
	}

	cp := ci.deepCopy()
	if cp.Config == nil {
		t.Fatal("Config should not be nil")
	}
	if cp.Config.TLSDomains != nil {
		t.Error("TLSDomains should be nil")
	}
	if cp.Config.Middlewares != nil {
		t.Error("Middlewares should be nil")
	}
}

func TestDeepCopyEmptyLabels(t *testing.T) {
	ci := &ContainerInfo{
		ID:     "empty-labels",
		Labels: map[string]string{},
	}

	cp := ci.deepCopy()
	if cp.Labels == nil {
		t.Error("Labels should be empty map, not nil")
	}
	if len(cp.Labels) != 0 {
		t.Errorf("Labels should be empty, got %d entries", len(cp.Labels))
	}
}

func TestDeepCopyPreservesBoolAndTime(t *testing.T) {
	now := time.Now()
	ci := &ContainerInfo{
		ID:        "bool-test",
		Healthy:   true,
		UpdatedAt: now,
	}

	cp := ci.deepCopy()
	if cp.Healthy != true {
		t.Error("Healthy should be true")
	}
	if !cp.UpdatedAt.Equal(now) {
		t.Errorf("UpdatedAt mismatch: %v vs %v", cp.UpdatedAt, now)
	}
}
