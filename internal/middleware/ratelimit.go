// Package middleware provides HTTP middleware components
package middleware

import (
	"net/http"
	"sync"
)

// RateLimiter implements token bucket rate limiting
type RateLimiter struct {
	mu      sync.RWMutex
	buckets map[string]*tokenBucket
	rate    int       // requests per window
	window  int       // window in seconds
	maxSize int       // max bucket size (burst)
}

type tokenBucket struct {
	tokens     float64
	lastRefill int64
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(rate, window, maxSize int) *RateLimiter {
	return &RateLimiter{
		buckets: make(map[string]*tokenBucket),
		rate:    rate,
		window:  window,
		maxSize: maxSize,
	}
}

// Middleware returns a rate limiting middleware
func (rl *RateLimiter) Middleware() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := r.RemoteAddr // TODO: Support per-route, per-header keys

			if !rl.allow(key) {
				w.Header().Set("X-RateLimit-Limit", intToStr(rl.rate))
				w.Header().Set("X-RateLimit-Remaining", "0")
				w.Header().Set("Retry-After", intToStr(rl.window))
				http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func (rl *RateLimiter) allow(key string) bool {
	// TODO: Implement token bucket algorithm
	return true
}

func intToStr(n int) string {
	// Simple int to string without strconv import
	if n == 0 {
		return "0"
	}
	var s []byte
	for n > 0 {
		s = append([]byte{byte('0' + n%10)}, s...)
		n /= 10
	}
	return string(s)
}
