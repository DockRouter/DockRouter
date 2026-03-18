package router

import (
	"testing"
)

// BenchmarkRadixTreeInsert benchmarks radix tree insertions
func BenchmarkRadixTreeInsert(b *testing.B) {
	routes := []*Route{
		{ID: "1", PathPrefix: "/api/v1/users"},
		{ID: "2", PathPrefix: "/api/v1/users/:id"},
		{ID: "3", PathPrefix: "/api/v1/posts"},
		{ID: "4", PathPrefix: "/api/v1/posts/:id/comments"},
		{ID: "5", PathPrefix: "/health"},
		{ID: "6", PathPrefix: "/metrics"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tree := NewRadixTree()
		for _, route := range routes {
			tree.Insert(route.PathPrefix, route)
		}
	}
}

// BenchmarkRadixTreeMatch benchmarks radix tree lookups
func BenchmarkRadixTreeMatch(b *testing.B) {
	tree := NewRadixTree()
	routes := []*Route{
		{ID: "1", PathPrefix: "/api/v1/users"},
		{ID: "2", PathPrefix: "/api/v1/users/:id"},
		{ID: "3", PathPrefix: "/api/v1/posts"},
		{ID: "4", PathPrefix: "/api/v1/posts/:id/comments"},
		{ID: "5", PathPrefix: "/health"},
		{ID: "6", PathPrefix: "/metrics"},
	}
	for _, route := range routes {
		tree.Insert(route.PathPrefix, route)
	}

	paths := []string{
		"/api/v1/users",
		"/api/v1/users/123",
		"/api/v1/posts/456/comments",
		"/health",
		"/notfound",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tree.Match(paths[i%len(paths)])
	}
}

// BenchmarkTableAdd benchmarks route table additions
func BenchmarkTableAdd(b *testing.B) {
	table := NewTable()
	route := &Route{
		ID:         "test-route",
		Host:       "example.com",
		PathPrefix: "/api",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		route.ID = string(rune(i)) // Unique ID for each iteration
		table.Add(route)
	}
}

// BenchmarkTableMatchWithParams benchmarks parameterized routes
func BenchmarkTableMatchWithParams(b *testing.B) {
	table := NewTable()
	table.Add(&Route{
		ID:         "users",
		Host:       "api.example.com",
		PathPrefix: "/users/:id/posts/:postId",
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		table.Match("api.example.com", "/users/123/posts/456")
	}
}

// BenchmarkBackendPoolRoundRobin benchmarks round-robin selection
func BenchmarkBackendPoolRoundRobin(b *testing.B) {
	pool := NewBackendPool(RoundRobin)
	pool.Add(&BackendTarget{Address: "10.0.0.1:8080", Healthy: true})
	pool.Add(&BackendTarget{Address: "10.0.0.2:8080", Healthy: true})
	pool.Add(&BackendTarget{Address: "10.0.0.3:8080", Healthy: true})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pool.Select("")
	}
}

// BenchmarkBackendPoolIPHash benchmarks IP hash selection
func BenchmarkBackendPoolIPHash(b *testing.B) {
	pool := NewBackendPool(IPHash)
	pool.Add(&BackendTarget{Address: "10.0.0.1:8080", Healthy: true})
	pool.Add(&BackendTarget{Address: "10.0.0.2:8080", Healthy: true})
	pool.Add(&BackendTarget{Address: "10.0.0.3:8080", Healthy: true})

	clientIPs := []string{
		"192.168.1.1",
		"192.168.1.2",
		"192.168.1.3",
		"10.0.0.5",
		"10.0.0.6",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pool.Select(clientIPs[i%len(clientIPs)])
	}
}

// BenchmarkBackendPoolLeastConn benchmarks least connections selection
func BenchmarkBackendPoolLeastConn(b *testing.B) {
	pool := NewBackendPool(LeastConn)
	pool.Add(&BackendTarget{Address: "10.0.0.1:8080", Healthy: true})
	pool.Add(&BackendTarget{Address: "10.0.0.2:8080", Healthy: true})
	pool.Add(&BackendTarget{Address: "10.0.0.3:8080", Healthy: true})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pool.Select("")
	}
}

// BenchmarkBackendPoolWeighted benchmarks weighted round-robin
func BenchmarkBackendPoolWeighted(b *testing.B) {
	pool := NewBackendPool(WeightedRoundRobin)
	pool.Add(&BackendTarget{Address: "10.0.0.1:8080", Healthy: true, Weight: 3})
	pool.Add(&BackendTarget{Address: "10.0.0.2:8080", Healthy: true, Weight: 2})
	pool.Add(&BackendTarget{Address: "10.0.0.3:8080", Healthy: true, Weight: 1})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pool.Select("")
	}
}

// BenchmarkBackendPoolRandom benchmarks random selection
func BenchmarkBackendPoolRandom(b *testing.B) {
	pool := NewBackendPool(Random)
	pool.Add(&BackendTarget{Address: "10.0.0.1:8080", Healthy: true})
	pool.Add(&BackendTarget{Address: "10.0.0.2:8080", Healthy: true})
	pool.Add(&BackendTarget{Address: "10.0.0.3:8080", Healthy: true})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pool.Select("")
	}
}
