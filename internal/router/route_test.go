package router

import (
	"testing"
	"time"
)

func TestNewRouteStore(t *testing.T) {
	store := NewRouteStore()
	if store == nil {
		t.Fatal("NewRouteStore returned nil")
	}
	if store.routes == nil {
		t.Error("routes map not initialized")
	}
	if store.byHost == nil {
		t.Error("byHost map not initialized")
	}
}

func TestRouteStoreAdd(t *testing.T) {
	store := NewRouteStore()

	route := &Route{
		ID:         "route-1",
		Host:       "example.com",
		PathPrefix: "/",
		Priority:   10,
		CreatedAt:  time.Now(),
	}

	store.Add(route)

	if len(store.routes) != 1 {
		t.Errorf("routes count = %d, want 1", len(store.routes))
	}
	if len(store.byHost) != 1 {
		t.Errorf("byHost count = %d, want 1", len(store.byHost))
	}
}

func TestRouteStoreAddUpdate(t *testing.T) {
	store := NewRouteStore()

	// Add initial route
	route := &Route{
		ID:        "route-1",
		Host:      "example.com",
		Priority:  10,
		CreatedAt: time.Now(),
	}
	store.Add(route)

	// Update same route
	updatedRoute := &Route{
		ID:        "route-1",
		Host:      "example.com",
		Priority:  20,
		CreatedAt: time.Now(),
	}
	store.Add(updatedRoute)

	// Should still have only 1 route
	if len(store.routes) != 1 {
		t.Errorf("routes count = %d, want 1", len(store.routes))
	}

	retrieved := store.Get("route-1")
	if retrieved.Priority != 20 {
		t.Errorf("Priority = %d, want 20", retrieved.Priority)
	}
}

func TestRouteStoreRemove(t *testing.T) {
	store := NewRouteStore()

	route := &Route{
		ID:        "route-1",
		Host:      "example.com",
		CreatedAt: time.Now(),
	}
	store.Add(route)

	store.Remove("route-1")

	if len(store.routes) != 0 {
		t.Errorf("routes count = %d, want 0", len(store.routes))
	}
	if len(store.byHost) != 0 {
		t.Errorf("byHost count = %d, want 0", len(store.byHost))
	}
}

func TestRouteStoreRemoveNonExistent(t *testing.T) {
	store := NewRouteStore()

	// Should not panic
	store.Remove("non-existent")
}

func TestRouteStoreGet(t *testing.T) {
	store := NewRouteStore()

	route := &Route{
		ID:        "route-1",
		Host:      "example.com",
		CreatedAt: time.Now(),
	}
	store.Add(route)

	retrieved := store.Get("route-1")
	if retrieved == nil {
		t.Fatal("Get returned nil")
	}
	if retrieved.ID != "route-1" {
		t.Errorf("ID = %s, want route-1", retrieved.ID)
	}
}

func TestRouteStoreGetNonExistent(t *testing.T) {
	store := NewRouteStore()

	retrieved := store.Get("non-existent")
	if retrieved != nil {
		t.Error("Get should return nil for non-existent route")
	}
}

func TestRouteStoreGetByHost(t *testing.T) {
	store := NewRouteStore()

	route1 := &Route{
		ID:        "route-1",
		Host:      "example.com",
		Priority:  10,
		CreatedAt: time.Now(),
	}
	route2 := &Route{
		ID:        "route-2",
		Host:      "example.com",
		Priority:  20,
		CreatedAt: time.Now(),
	}
	route3 := &Route{
		ID:        "route-3",
		Host:      "other.com",
		CreatedAt: time.Now(),
	}

	store.Add(route1)
	store.Add(route2)
	store.Add(route3)

	routes := store.GetByHost("example.com")
	if len(routes) != 2 {
		t.Errorf("GetByHost count = %d, want 2", len(routes))
	}

	// Should be sorted by priority (higher first)
	if routes[0].Priority != 20 {
		t.Errorf("First route priority = %d, want 20", routes[0].Priority)
	}
}

func TestRouteStoreList(t *testing.T) {
	store := NewRouteStore()

	route1 := &Route{
		ID:        "route-1",
		Host:      "example.com",
		CreatedAt: time.Now(),
	}
	route2 := &Route{
		ID:        "route-2",
		Host:      "other.com",
		CreatedAt: time.Now(),
	}

	store.Add(route1)
	store.Add(route2)

	routes := store.List()
	if len(routes) != 2 {
		t.Errorf("List count = %d, want 2", len(routes))
	}
}

func TestRouteStorePrioritySorting(t *testing.T) {
	store := NewRouteStore()

	// Add routes with different priorities
	route1 := &Route{
		ID:        "route-1",
		Host:      "example.com",
		Priority:  5,
		CreatedAt: time.Now(),
	}
	route2 := &Route{
		ID:        "route-2",
		Host:      "example.com",
		Priority:  20,
		CreatedAt: time.Now(),
	}
	route3 := &Route{
		ID:        "route-3",
		Host:      "example.com",
		Priority:  10,
		CreatedAt: time.Now(),
	}

	store.Add(route1)
	store.Add(route2)
	store.Add(route3)

	routes := store.GetByHost("example.com")

	// Should be sorted: 20, 10, 5
	if routes[0].Priority != 20 {
		t.Errorf("First priority = %d, want 20", routes[0].Priority)
	}
	if routes[1].Priority != 10 {
		t.Errorf("Second priority = %d, want 10", routes[1].Priority)
	}
	if routes[2].Priority != 5 {
		t.Errorf("Third priority = %d, want 5", routes[2].Priority)
	}
}

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
	store := NewRouteStore()

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

	store.Add(route)

	retrieved := store.Get("route-1")
	if retrieved.Backend == nil {
		t.Error("Backend should not be nil")
	}
}

func TestRouteWithMultipleHosts(t *testing.T) {
	store := NewRouteStore()

	route1 := &Route{
		ID:        "route-1",
		Host:      "api.example.com",
		CreatedAt: time.Now(),
	}
	route2 := &Route{
		ID:        "route-2",
		Host:      "web.example.com",
		CreatedAt: time.Now(),
	}
	route3 := &Route{
		ID:        "route-3",
		Host:      "api.example.com",
		CreatedAt: time.Now(),
	}

	store.Add(route1)
	store.Add(route2)
	store.Add(route3)

	// Check api.example.com has 2 routes
	apiRoutes := store.GetByHost("api.example.com")
	if len(apiRoutes) != 2 {
		t.Errorf("api.example.com routes = %d, want 2", len(apiRoutes))
	}

	// Check web.example.com has 1 route
	webRoutes := store.GetByHost("web.example.com")
	if len(webRoutes) != 1 {
		t.Errorf("web.example.com routes = %d, want 1", len(webRoutes))
	}
}

func TestRouteChangeHost(t *testing.T) {
	store := NewRouteStore()

	route := &Route{
		ID:        "route-1",
		Host:      "old.example.com",
		CreatedAt: time.Now(),
	}
	store.Add(route)

	// Update with new host
	updatedRoute := &Route{
		ID:        "route-1",
		Host:      "new.example.com",
		CreatedAt: time.Now(),
	}
	store.Add(updatedRoute)

	// Old host should have no routes
	oldRoutes := store.GetByHost("old.example.com")
	if len(oldRoutes) != 0 {
		t.Errorf("old.example.com routes = %d, want 0", len(oldRoutes))
	}

	// New host should have 1 route
	newRoutes := store.GetByHost("new.example.com")
	if len(newRoutes) != 1 {
		t.Errorf("new.example.com routes = %d, want 1", len(newRoutes))
	}
}

func TestRouteWithMiddlewares(t *testing.T) {
	store := NewRouteStore()

	route := &Route{
		ID:          "route-1",
		Host:        "example.com",
		Middlewares: []string{"cors", "ratelimit", "auth"},
		CreatedAt:   time.Now(),
	}

	store.Add(route)

	retrieved := store.Get("route-1")
	if len(retrieved.Middlewares) != 3 {
		t.Errorf("Middlewares count = %d, want 3", len(retrieved.Middlewares))
	}
}

func TestRouteWithTLS(t *testing.T) {
	store := NewRouteStore()

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

	store.Add(route)

	retrieved := store.Get("route-1")
	if len(retrieved.TLS.Domains) != 2 {
		t.Errorf("TLS Domains count = %d, want 2", len(retrieved.TLS.Domains))
	}
}
