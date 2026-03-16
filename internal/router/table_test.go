// Package router handles HTTP routing
package router

import (
	"testing"
)

func TestRouteTableAdd(t *testing.T) {
	table := NewTable()

	route := &Route{
		ID:         "test-1",
		Host:       "example.com",
		PathPrefix: "/",
	}

	table.Add(route)

	if table.Count() != 1 {
		t.Errorf("Expected 1 route, got %d", table.Count())
	}

	got := table.Get("test-1")
	if got == nil {
		t.Error("Route not found")
	}
	if got.Host != "example.com" {
		t.Errorf("Host = %s, want example.com", got.Host)
	}
}

func TestRouteTableRemove(t *testing.T) {
	table := NewTable()

	route := &Route{
		ID:         "test-1",
		Host:       "example.com",
		PathPrefix: "/",
	}

	table.Add(route)
	table.Remove("test-1")

	if table.Count() != 0 {
		t.Errorf("Expected 0 routes, got %d", table.Count())
	}
}

func TestRouteTableMatch(t *testing.T) {
	table := NewTable()

	// Add routes
	table.Add(&Route{ID: "1", Host: "api.example.com", PathPrefix: "/"})
	table.Add(&Route{ID: "2", Host: "api.example.com", PathPrefix: "/v2"})
	table.Add(&Route{ID: "3", Host: "*.example.com", PathPrefix: "/"})

	tests := []struct {
		host     string
		path     string
		wantID   string
		wantNone bool
	}{
		{"api.example.com", "/", "1", false},
		{"api.example.com", "/v2/users", "2", false},
		{"app.example.com", "/", "3", false},
		{"other.com", "/", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.host+tt.path, func(t *testing.T) {
			route := table.Match(tt.host, tt.path)
			if tt.wantNone {
				if route != nil {
					t.Errorf("Expected no match, got %s", route.ID)
				}
				return
			}
			if route == nil {
				t.Error("Expected match, got nil")
				return
			}
			if route.ID != tt.wantID {
				t.Errorf("Match() = %s, want %s", route.ID, tt.wantID)
			}
		})
	}
}

func TestRouteTableList(t *testing.T) {
	table := NewTable()

	table.Add(&Route{ID: "1", Host: "a.com", PathPrefix: "/"})
	table.Add(&Route{ID: "2", Host: "b.com", PathPrefix: "/"})
	table.Add(&Route{ID: "3", Host: "c.com", PathPrefix: "/"})

	routes := table.List()
	if len(routes) != 3 {
		t.Errorf("Expected 3 routes, got %d", len(routes))
	}
}

func TestNormalizeHost(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Example.COM", "example.com"},
		{"api.example.com:8080", "api.example.com"},
		{"example.com:443", "example.com"},
		{"example.com", "example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeHost(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeHost(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
