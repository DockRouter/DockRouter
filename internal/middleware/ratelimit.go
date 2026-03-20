// Package middleware provides HTTP middleware components
package middleware

import (
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// RateLimiter implements token bucket rate limiting
type RateLimiter struct {
	mu         sync.RWMutex
	buckets    map[string]*tokenBucket
	rate       int     // requests per window
	window     int     // window in seconds
	maxSize    int     // max bucket size (burst)
	refillRate float64 // tokens per second
	done       chan struct{}
}

type tokenBucket struct {
	tokens     float64
	lastRefill time.Time
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(rate, window, maxSize int) *RateLimiter {
	if window <= 0 {
		window = 60
	}
	if maxSize <= 0 {
		maxSize = rate
	}
	rl := &RateLimiter{
		buckets:    make(map[string]*tokenBucket),
		rate:       rate,
		window:     window,
		maxSize:    maxSize,
		refillRate: float64(rate) / float64(window),
		done:       make(chan struct{}),
	}

	// Start cleanup goroutine to remove old buckets
	go rl.cleanup()

	return rl
}

// Close stops the cleanup goroutine
func (rl *RateLimiter) Close() {
	close(rl.done)
}

// Middleware returns a rate limiting middleware
func (rl *RateLimiter) Middleware() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract IP without port for per-IP rate limiting
			key := r.RemoteAddr
			if host, _, err := net.SplitHostPort(key); err == nil {
				key = host
			}

			allowed, remaining := rl.allow(key)
			if !allowed {
				w.Header().Set("X-RateLimit-Limit", intToStr(rl.rate))
				w.Header().Set("X-RateLimit-Remaining", "0")
				w.Header().Set("Retry-After", intToStr(rl.window))
				http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
				return
			}

			w.Header().Set("X-RateLimit-Limit", intToStr(rl.rate))
			w.Header().Set("X-RateLimit-Remaining", intToStr(int(remaining)))
			next.ServeHTTP(w, r)
		})
	}
}

func (rl *RateLimiter) allow(key string) (bool, float64) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()

	bucket, exists := rl.buckets[key]
	if !exists {
		bucket = &tokenBucket{
			tokens:     float64(rl.maxSize - 1),
			lastRefill: now,
		}
		rl.buckets[key] = bucket
		return true, bucket.tokens
	}

	// Refill tokens based on time elapsed
	elapsed := now.Sub(bucket.lastRefill).Seconds()
	tokensToAdd := elapsed * rl.refillRate
	bucket.tokens += tokensToAdd
	if bucket.tokens > float64(rl.maxSize) {
		bucket.tokens = float64(rl.maxSize)
	}
	bucket.lastRefill = now

	// Check if we have tokens
	if bucket.tokens >= 1 {
		bucket.tokens--
		return true, bucket.tokens
	}

	return false, 0
}

// cleanup removes old buckets periodically
func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-rl.done:
			return
		case <-ticker.C:
			rl.mu.Lock()
			threshold := time.Now().Add(-5 * time.Minute)
			for key, bucket := range rl.buckets {
				if bucket.lastRefill.Before(threshold) {
					delete(rl.buckets, key)
				}
			}
			rl.mu.Unlock()
		}
	}
}

func intToStr(n int) string {
	return strconv.Itoa(n)
}
