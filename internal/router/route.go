// Package router handles HTTP routing
package router

import (
	"net"
	"sync"
	"time"
)

// Route represents a single routing entry
type Route struct {
	ID          string
	Host        string
	PathPrefix  string
	Backend     *BackendPool
	TLS         TLSConfig
	Middlewares []string
	Priority    int
	CreatedAt   time.Time
	Labels      map[string]string

	// Container info
	ContainerID   string
	ContainerName string

	// Additional fields for routing
	Address string // Direct backend address

	// Per-route middleware configuration
	MiddlewareConfig MiddlewareConfig
}

// MiddlewareConfig holds per-route middleware settings
type MiddlewareConfig struct {
	// Rate limiting
	RateLimit RateLimitConfig

	// CORS
	CORS CORSConfig

	// Compression
	Compress bool

	// Path modification
	StripPrefix string
	AddPrefix   string

	// Security
	BasicAuthUsers []BasicAuthUser
	IPWhitelist    []*net.IPNet
	IPBlacklist    []*net.IPNet
	MaxBody        int64

	// Reliability
	Retry          int
	CircuitBreaker CircuitBreakerConfig
}

// RateLimitConfig holds rate limiting configuration
type RateLimitConfig struct {
	Enabled bool
	Count   int
	Window  time.Duration
	ByKey   string
}

// CORSConfig holds CORS configuration
type CORSConfig struct {
	Enabled    bool
	Origins    []string
	Methods    []string
	Headers    []string
}

// BasicAuthUser holds basic auth credentials
type BasicAuthUser struct {
	Username string
	Hash     string
}

// CircuitBreakerConfig holds circuit breaker configuration
type CircuitBreakerConfig struct {
	Enabled  bool
	Failures int
	Window   time.Duration
}

// TLSConfig holds TLS-related configuration for a route
type TLSConfig struct {
	Mode     string   // auto, manual, off
	Domains  []string // SAN domains
	CertFile string
	KeyFile  string
}

// IsEnabled returns true if TLS is enabled
func (t *TLSConfig) IsEnabled() bool {
	return t.Mode != "off"
}

// IsAuto returns true if auto TLS (ACME) is enabled
func (t *TLSConfig) IsAuto() bool {
	return t.Mode == "auto"
}

// RouteStore manages all routes
type RouteStore struct {
	mu     sync.RWMutex
	routes map[string]*Route  // by ID
	byHost map[string][]*Route // by host
}

// NewRouteStore creates a new route store
func NewRouteStore() *RouteStore {
	return &RouteStore{
		routes: make(map[string]*Route),
		byHost: make(map[string][]*Route),
	}
}

// Add adds or updates a route
func (s *RouteStore) Add(route *Route) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Remove old if exists
	if existing, ok := s.routes[route.ID]; ok {
		s.removeFromHostIndex(existing)
	}

	// Add to main map
	s.routes[route.ID] = route

	// Add to host index
	s.byHost[route.Host] = append(s.byHost[route.Host], route)

	// Sort by priority (higher first)
	s.sortHostRoutes(route.Host)
}

// Remove removes a route by ID
func (s *RouteStore) Remove(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if route, ok := s.routes[id]; ok {
		s.removeFromHostIndex(route)
		delete(s.routes, id)
	}
}

// Get retrieves a route by ID
func (s *RouteStore) Get(id string) *Route {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.routes[id]
}

// GetByHost returns all routes for a host
func (s *RouteStore) GetByHost(host string) []*Route {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.byHost[host]
}

// List returns all routes
func (s *RouteStore) List() []*Route {
	s.mu.RLock()
	defer s.mu.RUnlock()

	routes := make([]*Route, 0, len(s.routes))
	for _, r := range s.routes {
		routes = append(routes, r)
	}
	return routes
}

// removeFromHostIndex removes a route from the host index
func (s *RouteStore) removeFromHostIndex(route *Route) {
	routes := s.byHost[route.Host]
	for i, r := range routes {
		if r.ID == route.ID {
			s.byHost[route.Host] = append(routes[:i], routes[i+1:]...)
			break
		}
	}
	if len(s.byHost[route.Host]) == 0 {
		delete(s.byHost, route.Host)
	}
}

// sortHostRoutes sorts routes by priority (descending)
func (s *RouteStore) sortHostRoutes(host string) {
	routes := s.byHost[host]
	for i := 0; i < len(routes)-1; i++ {
		for j := i + 1; j < len(routes); j++ {
			if routes[j].Priority > routes[i].Priority {
				routes[i], routes[j] = routes[j], routes[i]
			}
		}
	}
}
