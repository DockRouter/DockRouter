// Package middleware provides HTTP middleware components
package middleware

import (
	"net/http"
	"sync"
	"time"
)

// CircuitBreaker implements the circuit breaker pattern
type CircuitBreaker struct {
	mu              sync.RWMutex
	failures        int
	threshold       int
	window          time.Duration
	state           State
	lastFailure     time.Time
}

type State int

const (
	StateClosed State = iota
	StateOpen
	StateHalfOpen
)

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(threshold int, window time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		threshold: threshold,
		window:    window,
		state:     StateClosed,
	}
}

// Middleware returns circuit breaker middleware
func (cb *CircuitBreaker) Middleware() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !cb.allow() {
				http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
				return
			}
			// TODO: Track success/failure
			next.ServeHTTP(w, r)
		})
	}
}

func (cb *CircuitBreaker) allow() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	switch cb.state {
	case StateClosed:
		return true
	case StateOpen:
		return time.Since(cb.lastFailure) > cb.window
	case StateHalfOpen:
		return true
	}
	return false
}
