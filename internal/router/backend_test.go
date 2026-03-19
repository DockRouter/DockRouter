// Package router handles HTTP routing
package router

import (
	"testing"
)

func TestBackendPoolAdd(t *testing.T) {
	pool := NewBackendPool(RoundRobin)

	pool.Add(&BackendTarget{Address: "10.0.0.1:8080", Healthy: true})
	pool.Add(&BackendTarget{Address: "10.0.0.2:8080", Healthy: true})

	if len(pool.Targets) != 2 {
		t.Errorf("Expected 2 targets, got %d", len(pool.Targets))
	}
}

func TestBackendPoolSelect(t *testing.T) {
	pool := NewBackendPool(RoundRobin)

	// Empty pool
	if pool.Select("") != nil {
		t.Error("Expected nil for empty pool")
	}

	// Add targets
	pool.Add(&BackendTarget{Address: "10.0.0.1:8080", Healthy: true})
	pool.Add(&BackendTarget{Address: "10.0.0.2:8080", Healthy: false})
	pool.Add(&BackendTarget{Address: "10.0.0.3:8080", Healthy: true})

	// Should only select healthy targets
	selected := pool.Select("")
	if selected == nil {
		t.Fatal("Expected target, got nil")
	}
	if !selected.Healthy {
		t.Error("Selected unhealthy target")
	}

	// Round-robin should cycle
	first := pool.Select("")
	second := pool.Select("")
	if first.Address == second.Address {
		t.Error("Round-robin not cycling")
	}
}

func TestBackendPoolMarkHealthy(t *testing.T) {
	pool := NewBackendPool(RoundRobin)
	pool.Add(&BackendTarget{Address: "10.0.0.1:8080", Healthy: false})

	pool.MarkHealthy("10.0.0.1:8080")

	if !pool.Targets[0].Healthy {
		t.Error("Failed to mark healthy")
	}
}

func TestBackendPoolMarkUnhealthy(t *testing.T) {
	pool := NewBackendPool(RoundRobin)
	pool.Add(&BackendTarget{Address: "10.0.0.1:8080", Healthy: true})

	pool.MarkUnhealthy("10.0.0.1:8080")

	if pool.Targets[0].Healthy {
		t.Error("Failed to mark unhealthy")
	}
}

func TestBackendPoolAllUnhealthy(t *testing.T) {
	pool := NewBackendPool(RoundRobin)

	// Empty pool
	if pool.AllUnhealthy() {
		t.Error("Empty pool should not be all unhealthy")
	}

	// All healthy
	pool.Add(&BackendTarget{Address: "10.0.0.1:8080", Healthy: true})
	if pool.AllUnhealthy() {
		t.Error("Should not be all unhealthy")
	}

	// All unhealthy
	pool.MarkUnhealthy("10.0.0.1:8080")
	if !pool.AllUnhealthy() {
		t.Error("Should be all unhealthy")
	}
}

func TestBackendPoolRemove(t *testing.T) {
	pool := NewBackendPool(RoundRobin)
	pool.Add(&BackendTarget{Address: "10.0.0.1:8080", ContainerID: "container-1"})
	pool.Add(&BackendTarget{Address: "10.0.0.2:8080", ContainerID: "container-2"})

	pool.Remove("container-1")

	if len(pool.Targets) != 1 {
		t.Errorf("Expected 1 target, got %d", len(pool.Targets))
	}
	if pool.Targets[0].ContainerID != "container-2" {
		t.Error("Wrong target removed")
	}
}

// --- Weighted Round Robin Edge Case Tests ---

func TestBackendPoolWeightedRoundRobinZeroWeight(t *testing.T) {
	pool := NewBackendPool(WeightedRoundRobin)

	// Add targets with zero weight (should default to 1)
	pool.Add(&BackendTarget{Address: "10.0.0.1:8080", Healthy: true, Weight: 0})
	pool.Add(&BackendTarget{Address: "10.0.0.2:8080", Healthy: true, Weight: 0})

	// Both should be selected roughly equally (default weight = 1)
	counts := make(map[string]int)
	for i := 0; i < 100; i++ {
		selected := pool.Select("")
		if selected == nil {
			t.Fatal("Expected target, got nil")
		}
		counts[selected.Address]++
	}

	// Both should get roughly 50% each
	if counts["10.0.0.1:8080"] < 30 || counts["10.0.0.1:8080"] > 70 {
		t.Errorf("Expected ~50%% for each target, got %d%% and %d%%", counts["10.0.0.1:8080"], counts["10.0.0.2:8080"])
	}
}

func TestBackendPoolWeightedRoundRobinNegativeWeight(t *testing.T) {
	pool := NewBackendPool(WeightedRoundRobin)

	// Add target with negative weight (should default to 1)
	pool.Add(&BackendTarget{Address: "10.0.0.1:8080", Healthy: true, Weight: -5})

	selected := pool.Select("")
	if selected == nil {
		t.Fatal("Expected target, got nil")
	}
	if selected.Address != "10.0.0.1:8080" {
		t.Errorf("Expected 10.0.0.1:8080, got %s", selected.Address)
	}
}

func TestBackendPoolWeightedRoundRobinSingleTarget(t *testing.T) {
	pool := NewBackendPool(WeightedRoundRobin)

	// Single target should always be selected
	pool.Add(&BackendTarget{Address: "10.0.0.1:8080", Healthy: true, Weight: 5})

	for i := 0; i < 10; i++ {
		selected := pool.Select("")
		if selected == nil {
			t.Fatal("Expected target, got nil")
		}
		if selected.Address != "10.0.0.1:8080" {
			t.Errorf("Expected 10.0.0.1:8080, got %s", selected.Address)
		}
	}
}
