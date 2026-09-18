package router

import (
	"net"
	"testing"
)

func TestRouteClone(t *testing.T) {
	original := &Route{
		ID:          "test",
		Host:        "example.com",
		PathPrefix:  "/",
		Middlewares: []string{"m1", "m2"},
		Labels:      map[string]string{"key": "value"},
		TLS:         TLSConfig{Domains: []string{"example.com"}},
		MiddlewareConfig: MiddlewareConfig{
			CORS: CORSConfig{Origins: []string{"https://example.com"}},
		},
	}

	cloned := original.Clone()

	// Modify clone
	cloned.Middlewares[0] = "changed"
	cloned.Labels["key"] = "changed"
	cloned.TLS.Domains[0] = "changed.com"
	cloned.MiddlewareConfig.CORS.Origins[0] = "changed"

	// Verify original unchanged
	if original.Middlewares[0] != "m1" {
		t.Error("Clone did not deep copy Middlewares")
	}
	if original.Labels["key"] != "value" {
		t.Error("Clone did not deep copy Labels")
	}
	if original.TLS.Domains[0] != "example.com" {
		t.Error("Clone did not deep copy TLS.Domains")
	}
	if original.MiddlewareConfig.CORS.Origins[0] != "https://example.com" {
		t.Error("Clone did not deep copy CORS.Origins")
	}
}

func TestRouteCloneNilFields(t *testing.T) {
	original := &Route{
		ID:   "nil-test",
		Host: "example.com",
		// All slice/map fields are nil
	}

	cloned := original.Clone()

	if cloned == nil {
		t.Fatal("Clone returned nil")
	}
	if cloned.ID != "nil-test" {
		t.Errorf("ID = %s, want nil-test", cloned.ID)
	}
}

func TestRouteCloneWithAllFields(t *testing.T) {
	_, ipNet, _ := net.ParseCIDR("10.0.0.0/8")
	_, ipNet2, _ := net.ParseCIDR("192.168.0.0/16")

	original := &Route{
		ID:          "full-test",
		Host:        "full.example.com",
		PathPrefix:  "/api",
		Priority:    10,
		Address:     "backend:8080",
		ContainerID: "abc123",
		Middlewares: []string{"auth", "cors", "compress"},
		Labels:      map[string]string{"env": "prod", "team": "backend"},
		TLS: TLSConfig{
			Mode:     "auto",
			Domains:  []string{"full.example.com", "www.full.example.com"},
			CertFile: "/certs/full.crt",
			KeyFile:  "/certs/full.key",
		},
		MiddlewareConfig: MiddlewareConfig{
			Compress:    true,
			StripPrefix: "/api",
			AddPrefix:   "/v2",
			MaxBody:     1024,
			Retry:       3,
			BasicAuthUsers: []BasicAuthUser{
				{Username: "admin", Hash: "hash1"},
				{Username: "user", Hash: "hash2"},
			},
			IPWhitelist: []*net.IPNet{ipNet},
			IPBlacklist: []*net.IPNet{ipNet2},
			CORS: CORSConfig{
				Enabled: true,
				Origins: []string{"https://example.com"},
				Methods: []string{"GET", "POST"},
				Headers: []string{"Content-Type"},
			},
			CircuitBreaker: CircuitBreakerConfig{
				Enabled:  true,
				Failures: 5,
			},
		},
	}

	cloned := original.Clone()

	// Verify scalar fields copied
	if cloned.ID != original.ID {
		t.Errorf("ID mismatch")
	}
	if cloned.Host != original.Host {
		t.Errorf("Host mismatch")
	}
	if cloned.Priority != original.Priority {
		t.Errorf("Priority mismatch")
	}

	// Modify cloned slice/map fields
	cloned.Middlewares[0] = "modified"
	cloned.Labels["env"] = "staging"
	cloned.TLS.Domains[0] = "modified.com"
	cloned.MiddlewareConfig.BasicAuthUsers[0].Username = "modified"
	cloned.MiddlewareConfig.CORS.Origins[0] = "modified"
	cloned.MiddlewareConfig.CORS.Methods[0] = "PUT"
	cloned.MiddlewareConfig.CORS.Headers[0] = "X-Modified"

	// Verify originals unchanged
	if original.Middlewares[0] != "auth" {
		t.Error("Clone did not deep copy Middlewares")
	}
	if original.Labels["env"] != "prod" {
		t.Error("Clone did not deep copy Labels")
	}
	if original.TLS.Domains[0] != "full.example.com" {
		t.Error("Clone did not deep copy TLS.Domains")
	}
	if original.MiddlewareConfig.BasicAuthUsers[0].Username != "admin" {
		t.Error("Clone did not deep copy BasicAuthUsers")
	}
	if original.MiddlewareConfig.CORS.Origins[0] != "https://example.com" {
		t.Error("Clone did not deep copy CORS.Origins")
	}
	if original.MiddlewareConfig.CORS.Methods[0] != "GET" {
		t.Error("Clone did not deep copy CORS.Methods")
	}
	if original.MiddlewareConfig.CORS.Headers[0] != "Content-Type" {
		t.Error("Clone did not deep copy CORS.Headers")
	}

	// Verify IPWhitelist/IPBlacklist pointers are different (slice level isolation)
	if &original.MiddlewareConfig.IPWhitelist[0] == &cloned.MiddlewareConfig.IPWhitelist[0] {
		t.Error("Clone did not deep copy IPWhitelist slice entries")
	}
	if &original.MiddlewareConfig.IPBlacklist[0] == &cloned.MiddlewareConfig.IPBlacklist[0] {
		t.Error("Clone did not deep copy IPBlacklist slice entries")
	}
}
