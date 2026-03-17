// Package middleware provides HTTP middleware components
package middleware

import (
	"net/http"
	"sync"
	"time"
)

// CircuitBreaker implements the circuit breaker pattern
type CircuitBreaker struct {
	mu          sync.RWMutex
	failures    int
	successes   int
	threshold   int
	successMin  int // successes needed to close from half-open
	window      time.Duration
	state       State
	lastFailure time.Time
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
		threshold:  threshold,
		successMin: 3, // Need 3 successes to close from half-open
		window:     window,
		state:      StateClosed,
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

			// Wrap response writer to track status
			wrapped := &responseWriterTracker{ResponseWriter: w}
			next.ServeHTTP(wrapped, r)

			// Record result
			if wrapped.status >= 500 {
				cb.recordFailure()
			} else {
				cb.recordSuccess()
			}
		})
	}
}

// responseWriterTracker tracks response status
type responseWriterTracker struct {
	http.ResponseWriter
	status int
}

func (r *responseWriterTracker) WriteHeader(statusCode int) {
	r.status = statusCode
	r.ResponseWriter.WriteHeader(statusCode)
}

func (cb *CircuitBreaker) allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateClosed:
		return true
	case StateOpen:
		// Check if we should transition to half-open
		if time.Since(cb.lastFailure) > cb.window {
			cb.state = StateHalfOpen
			cb.successes = 0
			return true
		}
		return false
	case StateHalfOpen:
		return true
	}
	return false
}

func (cb *CircuitBreaker) recordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures++
	cb.lastFailure = time.Now()

	if cb.state == StateHalfOpen {
		// Failure in half-open -> back to open
		cb.state = StateOpen
		return
	}

	if cb.failures >= cb.threshold {
		cb.state = StateOpen
	}
}

func (cb *CircuitBreaker) recordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state == StateClosed {
		// Reset failure count on success
		if cb.failures > 0 {
			cb.failures = 0
		}
		return
	}

	if cb.state == StateHalfOpen {
		cb.successes++
		if cb.successes >= cb.successMin {
			cb.state = StateClosed
			cb.failures = 0
			cb.successes = 0
		}
	}
}

// State returns current circuit breaker state
func (cb *CircuitBreaker) State() State {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}
