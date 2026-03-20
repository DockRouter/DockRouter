// Package router handles HTTP routing
package router

import (
	"net/http"
	"sync"
	"time"

	"github.com/DockRouter/dockrouter/internal/middleware"
)

// RouteMiddlewareBuilder builds per-route middleware chains
type RouteMiddlewareBuilder struct {
	rateLimiters    sync.Map // routeID -> *middleware.RateLimiter
	circuitBreakers sync.Map // routeID -> *middleware.CircuitBreaker
}

// NewRouteMiddlewareBuilder creates a new middleware builder
func NewRouteMiddlewareBuilder() *RouteMiddlewareBuilder {
	return &RouteMiddlewareBuilder{}
}

// BuildChain builds a middleware chain for a route
func (b *RouteMiddlewareBuilder) BuildChain(route *Route, next http.Handler) http.Handler {
	chain := next

	// Apply rate limiting
	if route.MiddlewareConfig.RateLimit.Enabled {
		rl := b.getOrCreateRateLimiter(route.ID, route.MiddlewareConfig.RateLimit)
		chain = rl.Middleware()(chain)
	}

	// Apply CORS
	if route.MiddlewareConfig.CORS.Enabled {
		corsConfig := middleware.CORSConfig{
			Origins: route.MiddlewareConfig.CORS.Origins,
			Methods: route.MiddlewareConfig.CORS.Methods,
			Headers: route.MiddlewareConfig.CORS.Headers,
		}
		chain = middleware.CORS(corsConfig)(chain)
	}

	// Apply compression
	if route.MiddlewareConfig.Compress {
		chain = middleware.Compress(chain)
	}

	// Apply path modifications (StripPrefix before AddPrefix)
	if route.MiddlewareConfig.StripPrefix != "" {
		chain = middleware.StripPrefix(route.MiddlewareConfig.StripPrefix)(chain)
	}
	if route.MiddlewareConfig.AddPrefix != "" {
		chain = middleware.AddPrefix(route.MiddlewareConfig.AddPrefix)(chain)
	}

	// Apply basic auth
	if len(route.MiddlewareConfig.BasicAuthUsers) > 0 {
		users := make(map[string]string)
		for _, u := range route.MiddlewareConfig.BasicAuthUsers {
			users[u.Username] = u.Hash
		}
		chain = middleware.BasicAuth(users)(chain)
	}

	// Apply IP filtering
	if len(route.MiddlewareConfig.IPWhitelist) > 0 || len(route.MiddlewareConfig.IPBlacklist) > 0 {
		filter := middleware.NewIPFilter()
		for _, network := range route.MiddlewareConfig.IPWhitelist {
			filter.AddWhitelist(network.String())
		}
		for _, network := range route.MiddlewareConfig.IPBlacklist {
			filter.AddBlacklist(network.String())
		}
		chain = filter.Middleware()(chain)
	}

	// Apply max body limit
	if route.MiddlewareConfig.MaxBody > 0 {
		chain = middleware.MaxBody(route.MiddlewareConfig.MaxBody)(chain)
	}

	// Apply circuit breaker
	if route.MiddlewareConfig.CircuitBreaker.Enabled {
		cb := b.getOrCreateCircuitBreaker(route.ID, route.MiddlewareConfig.CircuitBreaker)
		chain = cb.Middleware()(chain)
	}

	return chain
}

// getOrCreateRateLimiter gets or creates a rate limiter for a route
func (b *RouteMiddlewareBuilder) getOrCreateRateLimiter(routeID string, config RateLimitConfig) *middleware.RateLimiter {
	if rl, ok := b.rateLimiters.Load(routeID); ok {
		return rl.(*middleware.RateLimiter)
	}

	window := config.Window
	if window == 0 {
		window = time.Minute
	}

	maxSize := config.Count
	if maxSize == 0 {
		maxSize = 100 // Default burst size
	}

	rl := middleware.NewRateLimiter(config.Count, int(window.Seconds()), maxSize)
	actual, loaded := b.rateLimiters.LoadOrStore(routeID, rl)
	if loaded {
		// Another goroutine created it first, close ours and use theirs
		rl.Close()
		return actual.(*middleware.RateLimiter)
	}
	return rl
}

// RemoveRateLimiter removes the rate limiter for a route (cleanup on route removal)
func (b *RouteMiddlewareBuilder) RemoveRateLimiter(routeID string) {
	if rl, ok := b.rateLimiters.LoadAndDelete(routeID); ok {
		rl.(*middleware.RateLimiter).Close()
	}
}

// getOrCreateCircuitBreaker gets or creates a circuit breaker for a route
func (b *RouteMiddlewareBuilder) getOrCreateCircuitBreaker(routeID string, config CircuitBreakerConfig) *middleware.CircuitBreaker {
	if cb, ok := b.circuitBreakers.Load(routeID); ok {
		return cb.(*middleware.CircuitBreaker)
	}

	threshold := config.Failures
	if threshold == 0 {
		threshold = 5 // Default threshold
	}

	window := config.Window
	if window == 0 {
		window = time.Minute // Default window
	}

	cb := middleware.NewCircuitBreaker(threshold, window)
	actual, loaded := b.circuitBreakers.LoadOrStore(routeID, cb)
	if loaded {
		return actual.(*middleware.CircuitBreaker)
	}
	return cb
}

// RemoveCircuitBreaker removes the circuit breaker for a route (cleanup on route removal)
func (b *RouteMiddlewareBuilder) RemoveCircuitBreaker(routeID string) {
	b.circuitBreakers.Delete(routeID)
}
