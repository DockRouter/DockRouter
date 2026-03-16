// Package router handles HTTP routing
package router

import (
	"testing"
)

func TestRadixTreeInsert(t *testing.T) {
	tree := NewRadixTree()

	tree.Insert("/", &Route{ID: "root"})
	tree.Insert("/api", &Route{ID: "api"})
	tree.Insert("/api/v1", &Route{ID: "api-v1"})
	tree.Insert("/api/v2", &Route{ID: "api-v2"})

	routes := tree.List()
	if len(routes) != 4 {
		t.Errorf("Expected 4 routes, got %d", len(routes))
	}
}

func TestRadixTreeMatch(t *testing.T) {
	tree := NewRadixTree()

	tree.Insert("/", &Route{ID: "root", PathPrefix: "/"})
	tree.Insert("/api", &Route{ID: "api", PathPrefix: "/api"})
	tree.Insert("/api/v1", &Route{ID: "api-v1", PathPrefix: "/api/v1"})
	tree.Insert("/api/v2", &Route{ID: "api-v2", PathPrefix: "/api/v2"})

	tests := []struct {
		path   string
		wantID string
	}{
		{"/", "root"},
		{"/api", "api"},
		{"/api/v1", "api-v1"},
		{"/api/v1/users", "api-v1"},
		{"/api/v2", "api-v2"},
		{"/api/v2/orders", "api-v2"},
		{"/api/v3", "api"}, // falls back to /api
		{"/other", "root"}, // falls back to /
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			route := tree.Match(tt.path)
			if route == nil {
				t.Errorf("No match for %s", tt.path)
				return
			}
			if route.ID != tt.wantID {
				t.Errorf("Match(%s) = %s, want %s", tt.path, route.ID, tt.wantID)
			}
		})
	}
}

func TestRadixTreeDelete(t *testing.T) {
	tree := NewRadixTree()

	tree.Insert("/api", &Route{ID: "api"})
	tree.Insert("/api/v1", &Route{ID: "api-v1"})

	tree.Delete("/api/v1")

	if tree.Match("/api/v1") != nil && tree.Match("/api/v1").ID == "api-v1" {
		t.Error("Route should be deleted")
	}

	if tree.Match("/api") == nil {
		t.Error("/api should still exist")
	}
}

func TestRadixTreeNormalizePath(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"/api", "/api"},
		{"api", "/api"},
		{"/api/", "/api"},
		{"/", "/"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizePath(tt.input)
			if result != tt.expected {
				t.Errorf("normalizePath(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestCommonPrefix(t *testing.T) {
	tests := []struct {
		a, b     string
		expected string
	}{
		{"/api/v1", "/api/v2", "/api/v"},
		{"/api", "/api", "/api"},
		{"/api", "/other", "/"},
		{"", "/api", ""},
	}

	for _, tt := range tests {
		t.Run(tt.a+"_"+tt.b, func(t *testing.T) {
			result := commonPrefix(tt.a, tt.b)
			if result != tt.expected {
				t.Errorf("commonPrefix(%q, %q) = %q, want %q", tt.a, tt.b, result, tt.expected)
			}
		})
	}
}

func TestRadixTreeRemoveByContainerID(t *testing.T) {
	tree := NewRadixTree()

	// Add multiple routes with different container IDs
	tree.Insert("/api", &Route{ID: "api", ContainerID: "container-1"})
	tree.Insert("/api/v1", &Route{ID: "api-v1", ContainerID: "container-1"})
	tree.Insert("/api/v2", &Route{ID: "api-v2", ContainerID: "container-2"})
	tree.Insert("/web", &Route{ID: "web", ContainerID: "container-3"})

	// Verify initial state
	routes := tree.List()
	if len(routes) != 4 {
		t.Errorf("Expected 4 routes, got %d", len(routes))
	}

	// Remove routes for container-1
	tree.RemoveByContainerID("container-1")

	// Verify container-1 routes are removed
	if tree.Match("/api") != nil {
		t.Error("/api route should be removed")
	}
	if tree.Match("/api/v1") != nil {
		t.Error("/api/v1 route should be removed")
	}

	// Verify other routes still exist
	v2Route := tree.Match("/api/v2")
	if v2Route == nil || v2Route.ID != "api-v2" {
		t.Error("/api/v2 route should still exist")
	}
	webRoute := tree.Match("/web")
	if webRoute == nil || webRoute.ID != "web" {
		t.Error("/web route should still exist")
	}
}

func TestRadixTreeRemoveByContainerIDNonExistent(t *testing.T) {
	tree := NewRadixTree()

	// Add a route
	tree.Insert("/api", &Route{ID: "api", ContainerID: "container-1"})

	// Remove non-existent container - should not panic
	tree.RemoveByContainerID("non-existent")

	// Route should still exist
	route := tree.Match("/api")
	if route == nil || route.ID != "api" {
		t.Error("/api route should still exist")
	}
}

func TestRadixTreeRemoveByContainerIDAll(t *testing.T) {
	tree := NewRadixTree()

	// Add routes all with same container ID
	tree.Insert("/api", &Route{ID: "api", ContainerID: "container-1"})
	tree.Insert("/web", &Route{ID: "web", ContainerID: "container-1"})
	tree.Insert("/admin", &Route{ID: "admin", ContainerID: "container-1"})

	// Remove all routes for container-1
	tree.RemoveByContainerID("container-1")

	// Verify all routes are removed
	routes := tree.List()
	if len(routes) != 0 {
		t.Errorf("Expected 0 routes, got %d", len(routes))
	}
}

func TestRadixTreeInsertOverwrite(t *testing.T) {
	tree := NewRadixTree()

	// Insert initial route
	tree.Insert("/api", &Route{ID: "api-v1", ContainerID: "container-1"})

	// Overwrite with new route
	tree.Insert("/api", &Route{ID: "api-v2", ContainerID: "container-2"})

	// Verify it was overwritten
	route := tree.Match("/api")
	if route == nil {
		t.Fatal("Route should exist")
	}
	if route.ID != "api-v2" {
		t.Errorf("Route ID = %s, want api-v2", route.ID)
	}
}

func TestRadixTreeMatchLongestPrefix(t *testing.T) {
	tree := NewRadixTree()

	// Add routes with different prefix depths
	tree.Insert("/", &Route{ID: "root", PathPrefix: "/"})
	tree.Insert("/api", &Route{ID: "api", PathPrefix: "/api"})
	tree.Insert("/api/v1", &Route{ID: "api-v1", PathPrefix: "/api/v1"})
	tree.Insert("/api/v1/users", &Route{ID: "users", PathPrefix: "/api/v1/users"})

	tests := []struct {
		path   string
		wantID string
	}{
		{"/", "root"},
		{"/api", "api"},
		{"/api/v1", "api-v1"},
		{"/api/v1/users", "users"},
		{"/api/v1/users/123", "users"},
		{"/api/v2", "api"},
		{"/api/v2/orders", "api"},
		{"/other", "root"},
		{"/other/path", "root"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			route := tree.Match(tt.path)
			if route == nil {
				t.Fatalf("No match for %s", tt.path)
			}
			if route.ID != tt.wantID {
				t.Errorf("Match(%s) = %s, want %s", tt.path, route.ID, tt.wantID)
			}
		})
	}
}
