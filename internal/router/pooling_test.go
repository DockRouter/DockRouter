package router

import (
	"testing"
)

func target(addr, containerID string, healthy bool) *BackendTarget {
	return &BackendTarget{Address: addr, ContainerID: containerID, Weight: 1, Healthy: healthy}
}

func TestRecordFailureEjectsOnlyAfterThreshold(t *testing.T) {
	pool := NewBackendPool(RoundRobin)
	pool.Add(target("10.0.0.1:80", "a", true))
	pool.Add(target("10.0.0.2:80", "b", true))

	// Below the threshold the backend stays in rotation: a single blip must not
	// remove capacity.
	for i := 0; i < PassiveEjectThreshold-1; i++ {
		pool.RecordFailure("10.0.0.1:80")
	}
	if pool.HealthyCount() != 2 {
		t.Fatalf("HealthyCount = %d after %d failures, want 2",
			pool.HealthyCount(), PassiveEjectThreshold-1)
	}

	// Reaching the threshold ejects it.
	pool.RecordFailure("10.0.0.1:80")
	if pool.HealthyCount() != 1 {
		t.Errorf("HealthyCount = %d after reaching the threshold, want 1", pool.HealthyCount())
	}
}

func TestRecordSuccessResetsFailureStreak(t *testing.T) {
	pool := NewBackendPool(RoundRobin)
	pool.Add(target("10.0.0.1:80", "a", true))
	pool.Add(target("10.0.0.2:80", "b", true))

	for i := 0; i < PassiveEjectThreshold-1; i++ {
		pool.RecordFailure("10.0.0.1:80")
	}
	pool.RecordSuccess("10.0.0.1:80")

	// The streak restarted, so one more failure must not eject.
	pool.RecordFailure("10.0.0.1:80")
	if pool.HealthyCount() != 2 {
		t.Errorf("HealthyCount = %d, want 2: a success should reset the failure streak",
			pool.HealthyCount())
	}
}

func TestRecordFailureNeverEjectsLastHealthyBackend(t *testing.T) {
	pool := NewBackendPool(RoundRobin)
	pool.Add(target("10.0.0.1:80", "a", true))

	for i := 0; i < PassiveEjectThreshold*3; i++ {
		pool.RecordFailure("10.0.0.1:80")
	}

	if pool.HealthyCount() != 1 {
		t.Error("the only backend was ejected; a partial outage must not become a total one")
	}
}

func TestMarkHealthyClearsFailureStreak(t *testing.T) {
	pool := NewBackendPool(RoundRobin)
	pool.Add(target("10.0.0.1:80", "a", true))
	pool.Add(target("10.0.0.2:80", "b", true))

	for i := 0; i < PassiveEjectThreshold; i++ {
		pool.RecordFailure("10.0.0.1:80")
	}
	if pool.HealthyCount() != 1 {
		t.Fatal("precondition: backend should be ejected")
	}

	// This is what the health checker does on recovery.
	pool.MarkHealthy("10.0.0.1:80")
	if pool.HealthyCount() != 2 {
		t.Fatal("MarkHealthy should return the backend to rotation")
	}

	// And the streak must be cleared, otherwise it is ejected again immediately.
	pool.RecordFailure("10.0.0.1:80")
	if pool.HealthyCount() != 2 {
		t.Error("MarkHealthy did not reset the consecutive failure count")
	}
}

func TestTargetStats(t *testing.T) {
	pool := NewBackendPool(RoundRobin)
	pool.Add(target("10.0.0.1:80", "a", true))
	pool.Add(target("10.0.0.2:80", "b", true))

	pool.RecordRequest("10.0.0.1:80")
	pool.RecordRequest("10.0.0.1:80")
	pool.RecordFailure("10.0.0.1:80")
	pool.CompleteRequest("10.0.0.1:80")

	var found bool
	for _, tgt := range pool.Snapshot() {
		if tgt.Address != "10.0.0.1:80" {
			continue
		}
		found = true
		requests, failures, active := tgt.Stats()
		if requests != 2 {
			t.Errorf("requests = %d, want 2", requests)
		}
		if failures != 1 {
			t.Errorf("failures = %d, want 1", failures)
		}
		if active != 1 {
			t.Errorf("active = %d, want 1", active)
		}
	}
	if !found {
		t.Fatal("Snapshot did not include the target")
	}
}

func TestRouteKeyIdentity(t *testing.T) {
	tests := []struct {
		name       string
		hostA, pkA string
		hostB, pkB string
		wantSame   bool
	}{
		{"same host and path", "a.example.com", "/", "a.example.com", "/", true},
		{"empty path normalises to root", "a.example.com", "", "a.example.com", "/", true},
		{"host case is ignored", "A.example.com", "/", "a.example.com", "/", true},
		{"trailing slash ignored", "a.example.com", "/api/", "a.example.com", "/api", true},
		{"different path", "a.example.com", "/api", "a.example.com", "/web", false},
		{"different host", "a.example.com", "/", "b.example.com", "/", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RouteKey(tt.hostA, tt.pkA) == RouteKey(tt.hostB, tt.pkB)
			if got != tt.wantSame {
				t.Errorf("RouteKey(%q,%q)==RouteKey(%q,%q) = %v, want %v",
					tt.hostA, tt.pkA, tt.hostB, tt.pkB, got, tt.wantSame)
			}
		})
	}
}

func TestUpsertMergesReplicasIntoOneRoute(t *testing.T) {
	table := NewTable()

	mkRoute := func(strategy LoadBalanceStrategy) *Route {
		return &Route{
			Host: "app.example.com", PathPrefix: "/",
			Backend: NewBackendPool(strategy),
		}
	}

	table.Upsert(mkRoute(RoundRobin), target("10.0.0.1:80", "c1", true))
	table.Upsert(mkRoute(RoundRobin), target("10.0.0.2:80", "c2", true))
	table.Upsert(mkRoute(RoundRobin), target("10.0.0.3:80", "c3", true))

	if table.Count() != 1 {
		t.Errorf("Count = %d, want 1: replicas of one service are a single route", table.Count())
	}

	route := table.Match("app.example.com", "/")
	if route == nil {
		t.Fatal("route not found")
	}
	if got := len(route.Backend.Snapshot()); got != 3 {
		t.Errorf("pool holds %d backends, want 3", got)
	}

	// A later container may change the strategy for the whole route.
	table.Upsert(mkRoute(LeastConn), target("10.0.0.4:80", "c4", true))
	if route = table.Match("app.example.com", "/"); route.Backend.Strategy != LeastConn {
		t.Errorf("Strategy = %v, want LeastConn", route.Backend.Strategy)
	}
}

func TestUpsertPreservesHealthAcrossUpdates(t *testing.T) {
	table := NewTable()
	mk := func() *Route {
		return &Route{Host: "app.example.com", PathPrefix: "/", Backend: NewBackendPool(RoundRobin)}
	}

	table.Upsert(mk(), target("10.0.0.1:80", "c1", true))
	table.Upsert(mk(), target("10.0.0.2:80", "c2", true))

	table.SetBackendHealth("10.0.0.1:80", false)

	// A config refresh for an unrelated replica must not silently revive it.
	table.Upsert(mk(), target("10.0.0.2:80", "c2", true))

	route := table.Match("app.example.com", "/")
	if route.Backend.HealthyCount() != 1 {
		t.Errorf("HealthyCount = %d, want 1: health state should survive an update",
			route.Backend.HealthyCount())
	}
}

func TestSetBackendHealthAndAddresses(t *testing.T) {
	table := NewTable()
	mk := func(host string) *Route {
		return &Route{Host: host, PathPrefix: "/", Backend: NewBackendPool(RoundRobin)}
	}

	table.Upsert(mk("a.example.com"), target("10.0.0.1:80", "c1", true))
	table.Upsert(mk("b.example.com"), target("10.0.0.1:80", "c1", true))
	table.Upsert(mk("b.example.com"), target("10.0.0.2:80", "c2", true))

	addrs := table.BackendAddresses()
	if len(addrs) != 2 {
		t.Errorf("BackendAddresses = %v, want 2 distinct addresses", addrs)
	}

	// A shared backend goes out of rotation everywhere it is used.
	table.SetBackendHealth("10.0.0.1:80", false)
	if got := table.Match("a.example.com", "/").Backend.HealthyCount(); got != 0 {
		t.Errorf("a.example.com healthy = %d, want 0", got)
	}
	if got := table.Match("b.example.com", "/").Backend.HealthyCount(); got != 1 {
		t.Errorf("b.example.com healthy = %d, want 1", got)
	}

	table.SetBackendHealth("10.0.0.1:80", true)
	if got := table.Match("a.example.com", "/").Backend.HealthyCount(); got != 1 {
		t.Errorf("a.example.com healthy after recovery = %d, want 1", got)
	}
}

func TestRemoveByContainerDropsRouteOnlyWhenPoolEmpties(t *testing.T) {
	table := NewTable()
	mk := func() *Route {
		return &Route{Host: "app.example.com", PathPrefix: "/", Backend: NewBackendPool(RoundRobin)}
	}

	table.Upsert(mk(), target("10.0.0.1:80", "c1", true))
	table.Upsert(mk(), target("10.0.0.2:80", "c2", true))

	table.RemoveByContainer("c1")
	route := table.Match("app.example.com", "/")
	if route == nil {
		t.Fatal("route disappeared while a replica was still running")
	}
	if got := len(route.Backend.Snapshot()); got != 1 {
		t.Errorf("pool holds %d backends, want 1", got)
	}

	table.RemoveByContainer("c2")
	if table.Match("app.example.com", "/") != nil {
		t.Error("route should be removed once its last backend is gone")
	}
	if table.Count() != 0 {
		t.Errorf("Count = %d, want 0", table.Count())
	}
}

func TestUpsertKeepsDistinctPathsSeparate(t *testing.T) {
	table := NewTable()
	mk := func(path string) *Route {
		return &Route{Host: "app.example.com", PathPrefix: path, Backend: NewBackendPool(RoundRobin)}
	}

	table.Upsert(mk("/"), target("10.0.0.1:80", "root", true))
	table.Upsert(mk("/api"), target("10.0.0.2:80", "api", true))

	if table.Count() != 2 {
		t.Fatalf("Count = %d, want 2 distinct routes", table.Count())
	}

	if got := table.Match("app.example.com", "/api/users"); got == nil ||
		got.Backend.Snapshot()[0].Address != "10.0.0.2:80" {
		t.Error("/api/users should match the /api route")
	}

	// Removing the /api container must leave the root route intact.
	table.RemoveByContainer("api")
	if table.Match("app.example.com", "/") == nil {
		t.Error("removing the /api backend also removed the root route")
	}
}
