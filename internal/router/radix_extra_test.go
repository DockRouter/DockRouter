package router

import (
	"testing"
)

func TestRadixTreeDeleteWithCompaction(t *testing.T) {
	tree := NewRadixTree()

	// Build a tree structure that will need compaction after delete
	tree.Insert("/api/v1/users", &Route{ID: "users", ContainerID: "c1"})
	tree.Insert("/api/v1/orders", &Route{ID: "orders", ContainerID: "c2"})

	// Delete one route - should trigger compaction
	tree.Delete("/api/v1/orders")

	// Verify users route still works
	route := tree.Match("/api/v1/users")
	if route == nil || route.ID != "users" {
		t.Error("/api/v1/users should still match")
	}

	// Verify orders route is gone
	route = tree.Match("/api/v1/orders")
	if route != nil && route.ID == "orders" {
		t.Error("/api/v1/orders should be deleted")
	}
}

func TestRadixTreeDeleteNodeWithChildren(t *testing.T) {
	tree := NewRadixTree()

	// Build tree where parent has children
	tree.Insert("/api", &Route{ID: "api", ContainerID: "c1"})
	tree.Insert("/api/v1", &Route{ID: "api-v1", ContainerID: "c2"})

	// Delete parent - should just clear route, not remove node
	tree.Delete("/api")

	// Parent route should be gone (might return nil or fall back to root)
	if route := tree.Match("/api"); route != nil && route.ID == "api" {
		t.Error("/api route should be cleared after delete")
	}

	// Child should still exist
	route := tree.Match("/api/v1")
	if route == nil || route.ID != "api-v1" {
		t.Error("/api/v1 should still exist")
	}
}

func TestRadixTreeDeleteNonExistent(t *testing.T) {
	tree := NewRadixTree()

	tree.Insert("/api", &Route{ID: "api"})

	// Delete non-existent path - should not panic
	tree.Delete("/nonexistent")
	tree.Delete("/api/v1/users")

	// Original route should still exist
	route := tree.Match("/api")
	if route == nil || route.ID != "api" {
		t.Error("/api should still exist")
	}
}

func TestRadixTreeDeleteAllThenInsert(t *testing.T) {
	tree := NewRadixTree()

	// Insert and delete
	tree.Insert("/api", &Route{ID: "api"})
	tree.Delete("/api")

	// Verify empty
	routes := tree.List()
	if len(routes) != 0 {
		t.Errorf("Expected 0 routes, got %d", len(routes))
	}

	// Insert again
	tree.Insert("/api", &Route{ID: "api-new"})
	route := tree.Match("/api")
	if route == nil || route.ID != "api-new" {
		t.Error("New route should be inserted")
	}
}

func TestRadixTreeComplexTreeStructure(t *testing.T) {
	tree := NewRadixTree()

	// Create a complex tree with various paths
	paths := []string{
		"/",
		"/api",
		"/api/v1",
		"/api/v1/users",
		"/api/v1/orders",
		"/api/v2",
		"/api/v2/users",
		"/web",
		"/web/static",
		"/admin",
	}

	for i, path := range paths {
		tree.Insert(path, &Route{ID: "route-" + string(rune('0'+i)), PathPrefix: path})
	}

	// Verify all paths match
	for i, path := range paths {
		route := tree.Match(path)
		if route == nil {
				t.Fatalf("Path %s should match", path)
			}
			expectedID := "route-" + string(rune('0'+i))
			if route.ID != expectedID {
				t.Errorf("Match(%s) = %s, want %s", path, route.ID, expectedID)
		 }
		}

	// Test partial paths
	route := tree.Match("/api/v1/users/123")
	if route == nil || route.ID != "route-3" {
		t.Errorf("Partial path /api/v1/users/123 should match /api/v1/users")
	}
}

func TestRadixTreeDeleteRoot(t *testing.T) {
	tree := NewRadixTree()

	tree.Insert("/", &Route{ID: "root", PathPrefix: "/"})

	// Delete root
	tree.Delete("/")

	// Root should be cleared
	routes := tree.List()
	if len(routes) != 0 {
		t.Errorf("Expected 0 routes after deleting root, got %d", len(routes))
	}
}

func TestRadixTreeEmptyTree(t *testing.T) {
	tree := NewRadixTree()

	// Operations on empty tree should not panic
	route := tree.Match("/api")
	// Empty tree may return nil or have no match
	_ = route

	routes := tree.List()
	if len(routes) != 0 {
		t.Errorf("Empty tree should have 0 routes, got %d", len(routes))
	}

	tree.Delete("/nonexistent") // Should not panic
}

func TestRadixTreeConcurrentAccess(t *testing.T) {
	tree := NewRadixTree()
	done := make(chan bool)

	// Concurrent inserts
	for i := 0; i < 10; i++ {
		go func(idx int) {
			path := "/api/v" + string(rune('0'+idx))
			tree.Insert(path, &Route{ID: "route-" + string(rune('0'+idx))})
			done <- true
		}(i)
	}

	// Concurrent reads
	for i := 0; i < 10; i++ {
		go func(idx int) {
			path := "/api/v" + string(rune('0'+idx))
			tree.Match(path)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 20; i++ {
		<-done
	}
}

func TestCommonPrefixAllCases(t *testing.T) {
	tests := []struct {
		a, b     string
		expected string
	}{
		{"/api/v1", "/api/v2", "/api/v"},
		{"/api", "/api", "/api"},
		{"/api", "/other", "/"},
		{"", "/api", ""},
		{"/api", "", ""},
		{"", "", ""},
		{"/abc", "/def", "/"},
		{"/long/path/here", "/long/path/there", "/long/path/"},
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
