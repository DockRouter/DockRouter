// Package router handles HTTP routing
package router

import (
	"net"
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
	Enabled bool
	Origins []string
	Methods []string
	Headers []string
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
