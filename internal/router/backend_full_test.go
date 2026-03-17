package router

import (
	"fmt"
	"testing"
	"time"
)

func TestBackendPoolSelectIPHash(t *testing.T) {
	pool := NewBackendPool(IPHash)
	pool.Add(&BackendTarget{Address: "10.0.0.1:8080", Healthy: true})
	pool.Add(&BackendTarget{Address: "10.0.0.2:8080", Healthy: true})
	pool.Add(&BackendTarget{Address: "10.0.0.3:8080", Healthy: true})

	// Test with different client IPs
	tests := []struct {
		clientIP string
	}{
		{"192.168.1.1"},
		{"192.168.1.2"},
		{"10.0.0.100"},
		{"172.16.0.1"},
	}

	for _, tt := range tests {
		t.Run(tt.clientIP, func(t *testing.T) {
			selected := pool.Select(tt.clientIP)
			if selected == nil {
				t.Fatal("Select should not return nil")
			}
			// Same IP should always get same backend
			selected2 := pool.Select(tt.clientIP)
			if selected.Address != selected2.Address {
				t.Errorf("IPHash should be consistent: %s != %s", selected.Address, selected2.Address)
			}
		})
	}
}

func TestBackendPoolSelectIPHashNoIP(t *testing.T) {
	pool := NewBackendPool(IPHash)
	pool.Add(&BackendTarget{Address: "10.0.0.1:8080", Healthy: true})

	// Empty IP should fall back to round-robin
	selected := pool.Select("")
	if selected == nil {
		t.Error("Select should not return nil even with empty IP")
	}
}

func TestBackendPoolSelectLeastConn(t *testing.T) {
	pool := NewBackendPool(LeastConn)
	pool.Add(&BackendTarget{Address: "10.0.0.1:8080", Healthy: true, requests: 10})
	pool.Add(&BackendTarget{Address: "10.0.0.2:8080", Healthy: true, requests: 5})
	pool.Add(&BackendTarget{Address: "10.0.0.3:8080", Healthy: true, requests: 20})

	selected := pool.Select("")
	if selected == nil {
		t.Fatal("Select should not return nil")
	}
	if selected.Address != "10.0.0.2:8080" {
		t.Errorf("LeastConn should select backend with least requests, got %s", selected.Address)
	}
}

func TestBackendPoolRecordRequest(t *testing.T) {
	pool := NewBackendPool(RoundRobin)
	pool.Add(&BackendTarget{Address: "10.0.0.1:8080", Healthy: true})

	// Record requests
	pool.RecordRequest("10.0.0.1:8080")
	pool.RecordRequest("10.0.0.1:8080")
	pool.RecordRequest("10.0.0.1:8080")

	// Verify through LeastConn selection
	target := pool.Targets[0]
	if target.requests != 3 {
		t.Errorf("requests = %d, want 3", target.requests)
	}
}

func TestBackendPoolRecordRequestNonExistent(t *testing.T) {
	pool := NewBackendPool(RoundRobin)
	pool.Add(&BackendTarget{Address: "10.0.0.1:8080", Healthy: true})

	// Record request for non-existent address - should not panic
	pool.RecordRequest("10.0.0.99:8080")
}

func TestBackendPoolRecordFailure(t *testing.T) {
	pool := NewBackendPool(RoundRobin)
	pool.Add(&BackendTarget{Address: "10.0.0.1:8080", Healthy: true})

	// Record failures
	pool.RecordFailure("10.0.0.1:8080")
	pool.RecordFailure("10.0.0.1:8080")

	target := pool.Targets[0]
	if target.failures != 2 {
		t.Errorf("failures = %d, want 2", target.failures)
	}
}

func TestBackendPoolRecordFailureNonExistent(t *testing.T) {
	pool := NewBackendPool(RoundRobin)
	pool.Add(&BackendTarget{Address: "10.0.0.1:8080", Healthy: true})

	// Record failure for non-existent address - should not panic
	pool.RecordFailure("10.0.0.99:8080")
}

func TestBackendPoolHealthyCount(t *testing.T) {
	pool := NewBackendPool(RoundRobin)
	pool.Add(&BackendTarget{Address: "10.0.0.1:8080", Healthy: true})
	pool.Add(&BackendTarget{Address: "10.0.0.2:8080", Healthy: true})
	pool.Add(&BackendTarget{Address: "10.0.0.3:8080", Healthy: false})

	count := pool.HealthyCount()
	if count != 2 {
		t.Errorf("HealthyCount = %d, want 2", count)
	}
}

func TestBackendPoolIsEmpty(t *testing.T) {
	pool := NewBackendPool(RoundRobin)

	if !pool.IsEmpty() {
		t.Error("Empty pool should return true")
	}

	pool.Add(&BackendTarget{Address: "10.0.0.1:8080", Healthy: true})

	if pool.IsEmpty() {
		t.Error("Non-empty pool should return false")
	}
}

func TestBackendPoolAllUnhealthyEmpty(t *testing.T) {
	pool := NewBackendPool(RoundRobin)

	if pool.AllUnhealthy() {
		t.Error("Empty pool should return false for AllUnhealthy")
	}
}

func TestBackendPoolAddUpdate(t *testing.T) {
	pool := NewBackendPool(RoundRobin)

	// Add initial
	pool.Add(&BackendTarget{
		Address:     "10.0.0.1:8080",
		ContainerID: "container-1",
		Weight:      1,
		Healthy:     true,
	})

	// Update same address
	pool.Add(&BackendTarget{
		Address:     "10.0.0.1:8080",
		ContainerID: "container-1-updated",
		Weight:      2,
		Healthy:     false,
	})

	if len(pool.Targets) != 1 {
		t.Errorf("Expected 1 target, got %d", len(pool.Targets))
	}

	if pool.Targets[0].ContainerID != "container-1-updated" {
		t.Error("ContainerID should be updated")
	}
	if pool.Targets[0].Weight != 2 {
		t.Error("Weight should be updated")
	}
}

func TestBackendPoolSelectNoHealthy(t *testing.T) {
	pool := NewBackendPool(RoundRobin)
	pool.Add(&BackendTarget{Address: "10.0.0.1:8080", Healthy: false})
	pool.Add(&BackendTarget{Address: "10.0.0.2:8080", Healthy: false})

	selected := pool.Select("")
	if selected != nil {
		t.Error("Select should return nil when no healthy backends")
	}
}

func TestBackendTargetStats(t *testing.T) {
	target := &BackendTarget{
		Address:     "10.0.0.1:8080",
		ContainerID: "container-1",
		Healthy:     true,
		LastCheck:   time.Now(),
	}

	if target.Address != "10.0.0.1:8080" {
		t.Error("Address should be set")
	}
	if target.ContainerID != "container-1" {
		t.Error("ContainerID should be set")
	}
}

func TestLoadBalanceStrategyValues(t *testing.T) {
	tests := []struct {
		strategy LoadBalanceStrategy
		name     string
	}{
		{RoundRobin, "RoundRobin"},
		{Random, "Random"},
		{IPHash, "IPHash"},
		{LeastConn, "LeastConn"},
		{WeightedRoundRobin, "WeightedRoundRobin"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := NewBackendPool(tt.strategy)
			if pool.Strategy != tt.strategy {
				t.Errorf("Strategy = %d, want %d", pool.Strategy, tt.strategy)
			}
		})
	}
}

func TestBackendPoolSelectRandom(t *testing.T) {
	pool := NewBackendPool(Random)
	pool.Add(&BackendTarget{Address: "10.0.0.1:8080", Healthy: true})
	pool.Add(&BackendTarget{Address: "10.0.0.2:8080", Healthy: true})

	// Random falls through to default (round-robin) in current implementation
	selected := pool.Select("")
	if selected == nil {
		t.Error("Select should not return nil")
	}
}

func TestBackendPoolSelectWeightedRoundRobin(t *testing.T) {
	pool := NewBackendPool(WeightedRoundRobin)
	pool.Add(&BackendTarget{Address: "10.0.0.1:8080", Weight: 3, Healthy: true})
	pool.Add(&BackendTarget{Address: "10.0.0.2:8080", Weight: 1, Healthy: true})

	// Run many selections and count distribution
	counts := make(map[string]int)
	for i := 0; i < 100; i++ {
		selected := pool.Select("")
		if selected == nil {
			t.Fatal("Select should not return nil")
		}
		counts[selected.Address]++
	}

	// With weights 3:1, first backend should get ~75% of requests
	// Allow some variance: 60-90% range
	ratio := float64(counts["10.0.0.1:8080"]) / 100.0
	if ratio < 0.60 || ratio > 0.90 {
		t.Errorf("Weighted distribution off: backend1 got %.2f%% (expected ~75%%)", ratio*100)
	}
}

func TestBackendPoolSelectWeightedRoundRobinEqualWeights(t *testing.T) {
	pool := NewBackendPool(WeightedRoundRobin)
	pool.Add(&BackendTarget{Address: "10.0.0.1:8080", Weight: 1, Healthy: true})
	pool.Add(&BackendTarget{Address: "10.0.0.2:8080", Weight: 1, Healthy: true})

	// With equal weights, distribution should be ~50/50
	counts := make(map[string]int)
	for i := 0; i < 100; i++ {
		selected := pool.Select("")
		counts[selected.Address]++
	}

	// Each should get ~50%, allow 40-60% range
	for addr, count := range counts {
		ratio := float64(count) / 100.0
		if ratio < 0.40 || ratio > 0.60 {
			t.Errorf("Equal weight distribution off: %s got %.2f%% (expected ~50%%)", addr, ratio*100)
		}
	}
}

func TestParseLoadBalanceStrategy(t *testing.T) {
	tests := []struct {
		input    string
		expected LoadBalanceStrategy
	}{
		{"roundrobin", RoundRobin},
		{"iphash", IPHash},
		{"leastconn", LeastConn},
		{"weighted", WeightedRoundRobin},
		{"random", Random},
		{"unknown", RoundRobin},
		{"", RoundRobin},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := ParseLoadBalanceStrategy(tt.input)
			if result != tt.expected {
				t.Errorf("ParseLoadBalanceStrategy(%q) = %d, want %d", tt.input, result, tt.expected)
			}
		})
	}
}

// Table tests

func TestTableNew(t *testing.T) {
	table := NewTable()
	if table == nil {
		t.Fatal("NewTable returned nil")
	}
	if table.exact == nil {
		t.Error("exact map should be initialized")
	}
	if table.wildcard == nil {
		t.Error("wildcard map should be initialized")
	}
	if table.routes == nil {
		t.Error("routes map should be initialized")
	}
}

func TestTableRemoveByContainer(t *testing.T) {
	table := NewTable()

	route1 := &Route{
		ID:          "route-1",
		Host:        "example.com",
		PathPrefix:  "/",
		ContainerID: "container-1",
	}
	route2 := &Route{
		ID:          "route-2",
		Host:        "test.com",
		PathPrefix:  "/",
		ContainerID: "container-2",
	}
	route3 := &Route{
		ID:          "route-3",
		Host:        "api.example.com",
		PathPrefix:  "/v1",
		ContainerID: "container-1",
	}

	table.Add(route1)
	table.Add(route2)
	table.Add(route3)

	if table.Count() != 3 {
		t.Fatalf("Expected 3 routes, got %d", table.Count())
	}

	// Remove all routes for container-1
	table.RemoveByContainer("container-1")

	if table.Count() != 1 {
		t.Errorf("Expected 1 route after remove, got %d", table.Count())
	}
	if table.Get("route-2") == nil {
		t.Error("route-2 should still exist")
	}
}

func TestTableListByHost(t *testing.T) {
	table := NewTable()

	table.Add(&Route{ID: "route-1", Host: "example.com", PathPrefix: "/"})
	table.Add(&Route{ID: "route-2", Host: "example.com", PathPrefix: "/api"})
	table.Add(&Route{ID: "route-3", Host: "test.com", PathPrefix: "/"})

	routes := table.ListByHost("example.com")
	if len(routes) != 2 {
		t.Errorf("ListByHost(example.com) = %d routes, want 2", len(routes))
	}

	routes = table.ListByHost("test.com")
	if len(routes) != 1 {
		t.Errorf("ListByHost(test.com) = %d routes, want 1", len(routes))
	}

	routes = table.ListByHost("nonexistent.com")
	if len(routes) != 0 {
		t.Errorf("ListByHost(nonexistent.com) = %d routes, want 0", len(routes))
	}
}

func TestTableHosts(t *testing.T) {
	table := NewTable()

	table.Add(&Route{ID: "route-1", Host: "example.com", PathPrefix: "/"})
	table.Add(&Route{ID: "route-2", Host: "test.com", PathPrefix: "/"})
	table.Add(&Route{ID: "route-3", Host: "*.wildcard.com", PathPrefix: "/"})

	hosts := table.Hosts()
	if len(hosts) != 3 {
		t.Errorf("Hosts() = %d hosts, want 3", len(hosts))
	}

	// Verify hosts contain expected values
	hostMap := make(map[string]bool)
	for _, h := range hosts {
		hostMap[h] = true
	}
	if !hostMap["example.com"] || !hostMap["test.com"] || !hostMap["*.wildcard.com"] {
		t.Error("Missing expected hosts")
	}
}

func TestTableWildcardMatch(t *testing.T) {
	table := NewTable()

	// Add wildcard route
	table.Add(&Route{
		ID:         "wildcard-1",
		Host:       "*.example.com",
		PathPrefix: "/",
	})

	// Test exact match returns nil (no exact route)
	route := table.Match("sub.example.com", "/")
	if route == nil {
		t.Fatal("Should match wildcard pattern")
	}
	if route.ID != "wildcard-1" {
		t.Errorf("Route ID = %s, want wildcard-1", route.ID)
	}
}

func TestTableWildcardMatchExactHost(t *testing.T) {
	table := NewTable()

	// Add both exact and wildcard
	table.Add(&Route{ID: "exact-1", Host: "api.example.com", PathPrefix: "/"})
	table.Add(&Route{ID: "wild-1", Host: "*.example.com", PathPrefix: "/"})

	// Exact should take precedence
	route := table.Match("api.example.com", "/")
	if route == nil {
		t.Fatal("Should match route")
	}
	if route.ID != "exact-1" {
		t.Errorf("Should prefer exact match, got %s", route.ID)
	}
}

func TestTableMatchNoMatch(t *testing.T) {
	table := NewTable()

	table.Add(&Route{ID: "route-1", Host: "example.com", PathPrefix: "/api"})

	// Request for different host
	route := table.Match("other.com", "/api")
	if route != nil {
		t.Error("Should not match different host")
	}

	// Request for different path
	route = table.Match("example.com", "/other")
	if route != nil {
		t.Error("Should not match different path")
	}
}

func TestWildcardMatchFunction(t *testing.T) {
	tests := []struct {
		pattern  string
		host     string
		expected bool
	}{
		{"*.example.com", "sub.example.com", true},
		{"*.example.com", "api.example.com", true},
		{"*.example.com", "example.com", true},
		{"*.example.com", "other.com", false},
		{"*.example.com", "sub.other.com", false},
		{"example.com", "example.com", false}, // not a wildcard pattern
		{"*.local", "test.local", true},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.host, func(t *testing.T) {
			result := wildcardMatch(tt.pattern, tt.host)
			if result != tt.expected {
				t.Errorf("wildcardMatch(%q, %q) = %v, want %v", tt.pattern, tt.host, result, tt.expected)
			}
		})
	}
}

func TestTableUpdateRoute(t *testing.T) {
	table := NewTable()

	// Add initial route
	route := &Route{
		ID:         "route-1",
		Host:       "example.com",
		PathPrefix: "/",
		Priority:   10,
	}
	table.Add(route)

	// Update same route
	updated := &Route{
		ID:         "route-1",
		Host:       "example.com",
		PathPrefix: "/api",
		Priority:   20,
	}
	table.Add(updated)

	if table.Count() != 1 {
		t.Errorf("Count = %d, want 1", table.Count())
	}

	got := table.Get("route-1")
	if got.Priority != 20 {
		t.Errorf("Priority = %d, want 20", got.Priority)
	}
	if got.PathPrefix != "/api" {
		t.Errorf("PathPrefix = %s, want /api", got.PathPrefix)
	}
}

func TestTableRemoveNonExistent(t *testing.T) {
	table := NewTable()

	// Should not panic
	table.Remove("non-existent")
}

func TestTableList(t *testing.T) {
	table := NewTable()

	if len(table.List()) != 0 {
		t.Error("Empty table should return empty list")
	}

	table.Add(&Route{ID: "route-1", Host: "example.com"})
	table.Add(&Route{ID: "route-2", Host: "test.com"})

	list := table.List()
	if len(list) != 2 {
		t.Errorf("List() = %d routes, want 2", len(list))
	}
}

func TestTableCount(t *testing.T) {
	table := NewTable()

	if table.Count() != 0 {
		t.Error("Empty table should have 0 count")
	}

	table.Add(&Route{ID: "route-1", Host: "example.com"})
	if table.Count() != 1 {
		t.Errorf("Count = %d, want 1", table.Count())
	}

	table.Remove("route-1")
	if table.Count() != 0 {
		t.Errorf("Count after remove = %d, want 0", table.Count())
	}
}

func TestTableRemoveWildcardRoute(t *testing.T) {
	table := NewTable()

	// Add a wildcard route
	table.Add(&Route{
		ID:         "wildcard-remove-test",
		Host:       "*.removetest.com",
		PathPrefix: "/api",
	})

	if table.Count() != 1 {
		t.Errorf("Count = %d, want 1", table.Count())
	}

	// Verify it matches
	route := table.Match("sub.removetest.com", "/api")
	if route == nil {
		t.Fatal("Route should match wildcard pattern")
	}

	// Remove the wildcard route
	table.Remove("wildcard-remove-test")

	if table.Count() != 0 {
		t.Errorf("Count after remove = %d, want 0", table.Count())
	}

	// Verify it no longer matches
	route = table.Match("sub.removetest.com", "/api")
	if route != nil {
		t.Error("Route should no longer match after removal")
	}
}

func TestTableRemoveExactRouteWithWildcardPresent(t *testing.T) {
	table := NewTable()

	// Add both exact and wildcard routes
	table.Add(&Route{ID: "exact-1", Host: "api.example.com", PathPrefix: "/"})
	table.Add(&Route{ID: "wild-1", Host: "*.example.com", PathPrefix: "/"})

	if table.Count() != 2 {
		t.Errorf("Count = %d, want 2", table.Count())
	}

	// Remove exact route - should only remove from exact tree
	table.Remove("exact-1")

	if table.Count() != 1 {
		t.Errorf("Count after remove = %d, want 1", table.Count())
	}

	// Wildcard should still match
	route := table.Match("api.example.com", "/")
	if route == nil {
		t.Fatal("Wildcard should still match")
	}
	if route.ID != "wild-1" {
		t.Errorf("Route ID = %s, want wild-1", route.ID)
	}
}

func TestTableRemoveByContainerWithWildcard(t *testing.T) {
	table := NewTable()

	// Add a wildcard route with container ID
	table.Add(&Route{
		ID:          "wild-container-test",
		Host:        "*.container.com",
		PathPrefix:  "/",
		ContainerID: "container-123",
	})

	if table.Count() != 1 {
		t.Errorf("Count = %d, want 1", table.Count())
	}

	// Remove by container ID
	table.RemoveByContainer("container-123")

	if table.Count() != 0 {
		t.Errorf("Count = %d, want 0", table.Count())
	}
}

func TestTableListByHostWithWildcard(t *testing.T) {
	table := NewTable()

	// Add a wildcard route
	table.Add(&Route{
		ID:         "wildcard-route",
		Host:       "*.example.com",
		PathPrefix: "/",
	})

	// Add an exact route
	table.Add(&Route{
		ID:         "exact-route",
		Host:       "api.example.com",
		PathPrefix: "/",
	})

	// Test ListByHost with a host that matches the wildcard
	routes := table.ListByHost("sub.example.com")
	if len(routes) != 1 {
		t.Errorf("ListByHost(sub.example.com) = %d routes, want 1 (wildcard match)", len(routes))
	} else if routes[0].ID != "wildcard-route" {
		t.Errorf("Route ID = %s, want wildcard-route", routes[0].ID)
	}

	// Test ListByHost with exact match host
	routes = table.ListByHost("api.example.com")
	if len(routes) != 2 {
		t.Errorf("ListByHost(api.example.com) = %d routes, want 2 (exact + wildcard)", len(routes))
	}

	// Test ListByHost with non-matching host
	routes = table.ListByHost("other.com")
	if len(routes) != 0 {
		t.Errorf("ListByHost(other.com) = %d routes, want 0", len(routes))
	}
}

func TestTableAddWithEmptyPath(t *testing.T) {
	table := NewTable()

	// Add a route with empty PathPrefix - should be normalized to "/"
	table.Add(&Route{
		ID:         "empty-path-route",
		Host:       "example.com",
		PathPrefix: "",
	})

	// Verify route can be matched at "/"
	route := table.Match("example.com", "/")
	if route == nil {
		t.Fatal("Should match route with empty PathPrefix at /")
	}
	if route.ID != "empty-path-route" {
		t.Errorf("Route ID = %s, want empty-path-route", route.ID)
	}
}

func TestTableRemoveWithEmptyPath(t *testing.T) {
	table := NewTable()

	// Add a route with empty PathPrefix
	table.Add(&Route{
		ID:         "remove-empty-path",
		Host:       "example.com",
		PathPrefix: "",
	})

	if table.Count() != 1 {
		t.Fatalf("Count = %d, want 1", table.Count())
	}

	// Remove the route - empty path should be normalized to "/"
	table.Remove("remove-empty-path")

	if table.Count() != 0 {
		t.Errorf("Count = %d, want 0 after remove", table.Count())
	}
}

func TestTableRemoveWildcardWithEmptyPath(t *testing.T) {
	table := NewTable()

	// Add a wildcard route with empty PathPrefix
	table.Add(&Route{
		ID:         "wildcard-empty-path",
		Host:       "*.example.com",
		PathPrefix: "",
	})

	if table.Count() != 1 {
		t.Fatalf("Count = %d, want 1", table.Count())
	}

	// Remove the route
	table.Remove("wildcard-empty-path")

	if table.Count() != 0 {
		t.Errorf("Count = %d, want 0 after remove", table.Count())
	}
}

// Benchmarks

func BenchmarkBackendPoolSelectRoundRobin(b *testing.B) {
	pool := NewBackendPool(RoundRobin)
	for i := 0; i < 10; i++ {
		pool.Add(&BackendTarget{
			Address: fmt.Sprintf("10.0.0.%d:8080", i+1),
			Healthy: true,
			Weight:  1,
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = pool.Select("192.168.1.1:12345")
	}
}

func BenchmarkBackendPoolSelectIPHash(b *testing.B) {
	pool := NewBackendPool(IPHash)
	for i := 0; i < 10; i++ {
		pool.Add(&BackendTarget{
			Address: fmt.Sprintf("10.0.0.%d:8080", i+1),
			Healthy: true,
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = pool.Select(fmt.Sprintf("192.168.1.%d:12345", i%256))
	}
}

func BenchmarkBackendPoolSelectLeastConn(b *testing.B) {
	pool := NewBackendPool(LeastConn)
	for i := 0; i < 10; i++ {
		pool.Add(&BackendTarget{
			Address: fmt.Sprintf("10.0.0.%d:8080", i+1),
			Healthy: true,
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = pool.Select("192.168.1.1:12345")
	}
}

func BenchmarkBackendPoolSelectWeighted(b *testing.B) {
	pool := NewBackendPool(WeightedRoundRobin)
	pool.Add(&BackendTarget{Address: "10.0.0.1:8080", Healthy: true, Weight: 5})
	pool.Add(&BackendTarget{Address: "10.0.0.2:8080", Healthy: true, Weight: 3})
	pool.Add(&BackendTarget{Address: "10.0.0.3:8080", Healthy: true, Weight: 2})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = pool.Select("192.168.1.1:12345")
	}
}

func BenchmarkParseLoadBalanceStrategy(b *testing.B) {
	strategies := []string{"roundrobin", "iphash", "leastconn", "weighted", "unknown"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ParseLoadBalanceStrategy(strategies[i%len(strategies)])
	}
}

func BenchmarkTableMatch(b *testing.B) {
	table := NewTable()

	// Add routes
	for i := 0; i < 100; i++ {
		table.Add(&Route{
			ID:         fmt.Sprintf("route-%d", i),
			Host:       fmt.Sprintf("app%d.example.com", i),
			PathPrefix: "/",
			Backend:    NewBackendPool(RoundRobin),
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = table.Match(fmt.Sprintf("app%d.example.com", i%100), "/")
	}
}
