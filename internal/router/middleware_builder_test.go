package router

import (
	"testing"
	"time"

	"github.com/DockRouter/dockrouter/internal/middleware"
)

// TestGetOrCreateRateLimiterDefault tests rate limiter creation with zero values.
func TestGetOrCreateRateLimiterDefault(t *testing.T) {
	b := NewRouteMiddlewareBuilder()

	rl := b.getOrCreateRateLimiter("route1", RateLimitConfig{
		Enabled: true,
		Count:   0, // zero — should default to 100
		Window:  0, // zero — should default to 1 minute
	})

	if rl == nil {
		t.Fatal("rate limiter should not be nil")
	}

	// Should be stored
	rl2 := b.getOrCreateRateLimiter("route1", RateLimitConfig{
		Enabled: true,
		Count:   100,
		Window:  time.Minute,
	})
	if rl != rl2 {
		t.Error("should return same instance for same route ID")
	}

	rl.Close()
}

// TestGetOrCreateRateLimiterConcurrency tests concurrent creation.
func TestGetOrCreateRateLimiterConcurrency(t *testing.T) {
	b := NewRouteMiddlewareBuilder()

	done := make(chan *middleware.RateLimiter, 2)

	for i := 0; i < 2; i++ {
		go func() {
			rl := b.getOrCreateRateLimiter("concurrent-route", RateLimitConfig{
				Enabled: true,
				Count:   50,
				Window:  time.Minute,
			})
			done <- rl
		}()
	}

	rl1 := <-done
	rl2 := <-done

	// One of them should be closed (the loser of LoadOrStore)
	// Both should be non-nil
	if rl1 == nil || rl2 == nil {
		t.Error("rate limiters should not be nil")
	}
}

// TestGetOrCreateCircuitBreakerDefault tests circuit breaker with zero values.
func TestGetOrCreateCircuitBreakerDefault(t *testing.T) {
	b := NewRouteMiddlewareBuilder()

	cb := b.getOrCreateCircuitBreaker("route1", CircuitBreakerConfig{
		Enabled:  true,
		Failures: 0, // zero — should default to 5
		Window:   0, // zero — should default to 1 minute
	})

	if cb == nil {
		t.Fatal("circuit breaker should not be nil")
	}

	// Should be stored
	cb2 := b.getOrCreateCircuitBreaker("route1", CircuitBreakerConfig{
		Enabled:  true,
		Failures: 5,
		Window:   time.Minute,
	})
	if cb != cb2 {
		t.Error("should return same instance for same route ID")
	}
}

// TestRemoveRateLimiter tests rate limiter removal and cleanup.
func TestRemoveRateLimiter(t *testing.T) {
	b := NewRouteMiddlewareBuilder()

	rl := b.getOrCreateRateLimiter("route-to-remove", RateLimitConfig{
		Enabled: true,
		Count:   100,
		Window:  time.Minute,
	})
	if rl == nil {
		t.Fatal("rate limiter should be created")
	}

	b.RemoveRateLimiter("route-to-remove")

	// Should create a new one after removal
	rl2 := b.getOrCreateRateLimiter("route-to-remove", RateLimitConfig{
		Enabled: true,
		Count:   100,
		Window:  time.Minute,
	})
	if rl == rl2 {
		t.Error("should create new instance after removal")
	}
	rl2.Close()
}
