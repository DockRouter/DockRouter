// Package discovery handles Docker container discovery
package discovery

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

// Label prefix for all DockRouter labels
const LabelPrefix = "dr."

// Required labels
const (
	LabelEnable = "dr.enable"
	LabelHost   = "dr.host"
)

// Routing labels
const (
	LabelPort        = "dr.port"
	LabelPath        = "dr.path"
	LabelPriority    = "dr.priority"
	LabelAddress     = "dr.address"
	LabelLoadBalance = "dr.loadbalancer"
	LabelWeight      = "dr.weight"
)

// TLS labels
const (
	LabelTLS        = "dr.tls"
	LabelTLSDomains = "dr.tls.domains"
	LabelTLSCert    = "dr.tls.cert"
	LabelTLSKey     = "dr.tls.key"
)

// Middleware labels
const (
	LabelRateLimit        = "dr.ratelimit"
	LabelRateLimitBy      = "dr.ratelimit.by"
	LabelCORSOrigins      = "dr.cors.origins"
	LabelCORSMethods      = "dr.cors.methods"
	LabelCORSHeaders      = "dr.cors.headers"
	LabelCompress         = "dr.compress"
	LabelRedirectHTTPS    = "dr.redirect.https"
	LabelStripPrefix      = "dr.stripprefix"
	LabelAddPrefix        = "dr.addprefix"
	LabelMaxBody          = "dr.maxbody"
	LabelAuthBasicUsers   = "dr.auth.basic.users"
	LabelIPWhitelist      = "dr.ipwhitelist"
	LabelIPBlacklist      = "dr.ipblacklist"
	LabelRetry            = "dr.retry"
	LabelCircuitBreaker   = "dr.circuitbreaker"
	LabelMiddlewares      = "dr.middlewares"
)

// Health check labels
const (
	LabelHealthPath      = "dr.healthcheck.path"
	LabelHealthInterval  = "dr.healthcheck.interval"
	LabelHealthTimeout   = "dr.healthcheck.timeout"
	LabelHealthThreshold = "dr.healthcheck.threshold"
	LabelHealthRecovery  = "dr.healthcheck.recovery"
)

// RouteConfig represents parsed labels for a route
type RouteConfig struct {
	// Required
	Enabled bool
	Host    string

	// Routing
	Port        int
	Path        string
	Priority    int
	Address     string
	LoadBalance string
	Weight      int

	// TLS
	TLS         string
	TLSDomains  []string
	TLSCertFile string
	TLSKeyFile  string

	// Middlewares
	RateLimit      RateLimitConfig
	CORS           CORSConfig
	Compress       bool
	RedirectHTTPS  bool
	StripPrefix    string
	AddPrefix      string
	MaxBody        int64
	BasicAuthUsers []BasicAuthUser
	IPWhitelist    []*net.IPNet
	IPBlacklist    []*net.IPNet
	Retry          int
	CircuitBreaker CircuitBreakerConfig
	Middlewares    []string

	// Health
	HealthCheck HealthCheckLabelConfig

	// Raw labels for reference
	RawLabels map[string]string
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
	Credentials bool
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

// HealthCheckLabelConfig holds health check configuration from labels
type HealthCheckLabelConfig struct {
	Path      string
	Interval  time.Duration
	Timeout   time.Duration
	Threshold int
	Recovery  int
}

// ParseLabels extracts RouteConfig from container labels
func ParseLabels(labels map[string]string) *RouteConfig {
	if labels == nil {
		return nil
	}

	config := &RouteConfig{
		RawLabels: labels,
	}

	// Check if enabled
	config.Enabled = parseBool(labels[LabelEnable])
	if !config.Enabled {
		return config
	}

	// Required: Host
	config.Host = labels[LabelHost]

	// Routing
	config.Port = parseInt(labels[LabelPort], 0)
	config.Path = labels[LabelPath]
	config.Priority = parseInt(labels[LabelPriority], 0)
	config.Address = labels[LabelAddress]
	config.LoadBalance = labels[LabelLoadBalance]
	if config.LoadBalance == "" {
		config.LoadBalance = "roundrobin"
	}
	config.Weight = parseInt(labels[LabelWeight], 1)

	// TLS
	config.TLS = labels[LabelTLS]
	if config.TLS == "" {
		config.TLS = "auto" // default
	}
	if domains := labels[LabelTLSDomains]; domains != "" {
		config.TLSDomains = strings.Split(domains, ",")
	}
	config.TLSCertFile = labels[LabelTLSCert]
	config.TLSKeyFile = labels[LabelTLSKey]

	// Parse middlewares
	config.parseRateLimit(labels)
	config.parseCORS(labels)
	config.parseBasicAuth(labels)
	config.parseIPFilters(labels)
	config.parseCircuitBreaker(labels)

	config.Compress = parseBool(labels[LabelCompress])
	config.RedirectHTTPS = parseBoolDefault(labels[LabelRedirectHTTPS], true) // default true when TLS is on
	config.StripPrefix = labels[LabelStripPrefix]
	config.AddPrefix = labels[LabelAddPrefix]
	config.MaxBody = parseSize(labels[LabelMaxBody])
	config.Retry = parseInt(labels[LabelRetry], 0)

	// Explicit middleware list
	if mw := labels[LabelMiddlewares]; mw != "" {
		config.Middlewares = strings.Split(mw, ",")
		for i, m := range config.Middlewares {
			config.Middlewares[i] = strings.TrimSpace(m)
		}
	}

	// Health check
	config.parseHealthCheck(labels)

	return config
}

func (c *RouteConfig) parseRateLimit(labels map[string]string) {
	rl := labels[LabelRateLimit]
	if rl == "" {
		return
	}

	c.RateLimit.Enabled = true
	c.RateLimit.ByKey = labels[LabelRateLimitBy]
	if c.RateLimit.ByKey == "" {
		c.RateLimit.ByKey = "client_ip"
	}

	// Parse rate limit: {count}/{window}
	// Examples: 100/m, 10/s, 5000/h
	parts := strings.Split(rl, "/")
	if len(parts) == 2 {
		c.RateLimit.Count = parseInt(parts[0], 100)
		c.RateLimit.Window = parseWindow(parts[1])
	}
}

func (c *RouteConfig) parseCORS(labels map[string]string) {
	origins := labels[LabelCORSOrigins]
	if origins == "" {
		return
	}

	c.CORS.Enabled = true
	c.CORS.Origins = strings.Split(origins, ",")
	for i, o := range c.CORS.Origins {
		c.CORS.Origins[i] = strings.TrimSpace(o)
	}

	if methods := labels[LabelCORSMethods]; methods != "" {
		c.CORS.Methods = strings.Split(methods, ",")
	} else {
		c.CORS.Methods = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}
	}

	if headers := labels[LabelCORSHeaders]; headers != "" {
		c.CORS.Headers = strings.Split(headers, ",")
	}
}

func (c *RouteConfig) parseBasicAuth(labels map[string]string) {
	users := labels[LabelAuthBasicUsers]
	if users == "" {
		return
	}

	for _, user := range strings.Split(users, ",") {
		parts := strings.SplitN(strings.TrimSpace(user), ":", 2)
		if len(parts) == 2 {
			c.BasicAuthUsers = append(c.BasicAuthUsers, BasicAuthUser{
				Username: parts[0],
				Hash:     parts[1],
			})
		}
	}
}

func (c *RouteConfig) parseIPFilters(labels map[string]string) {
	// Whitelist
	if whitelist := labels[LabelIPWhitelist]; whitelist != "" {
		for _, cidr := range strings.Split(whitelist, ",") {
			cidr = strings.TrimSpace(cidr)
			if _, network, err := net.ParseCIDR(cidr); err == nil {
				c.IPWhitelist = append(c.IPWhitelist, network)
			} else if ip := net.ParseIP(cidr); ip != nil {
				// Single IP, convert to /32 or /128
				bits := 32
				if ip.To4() == nil {
					bits = 128
				}
				_, network, _ := net.ParseCIDR(fmt.Sprintf("%s/%d", cidr, bits))
				if network != nil {
					c.IPWhitelist = append(c.IPWhitelist, network)
				}
			}
		}
	}

	// Blacklist
	if blacklist := labels[LabelIPBlacklist]; blacklist != "" {
		for _, cidr := range strings.Split(blacklist, ",") {
			cidr = strings.TrimSpace(cidr)
			if _, network, err := net.ParseCIDR(cidr); err == nil {
				c.IPBlacklist = append(c.IPBlacklist, network)
			} else if ip := net.ParseIP(cidr); ip != nil {
				bits := 32
				if ip.To4() == nil {
					bits = 128
				}
				_, network, _ := net.ParseCIDR(fmt.Sprintf("%s/%d", cidr, bits))
				if network != nil {
					c.IPBlacklist = append(c.IPBlacklist, network)
				}
			}
		}
	}
}

func (c *RouteConfig) parseCircuitBreaker(labels map[string]string) {
	cb := labels[LabelCircuitBreaker]
	if cb == "" {
		return
	}

	c.CircuitBreaker.Enabled = true

	// Parse: {failures}/{window}
	// Example: 5/30s
	parts := strings.Split(cb, "/")
	if len(parts) == 2 {
		c.CircuitBreaker.Failures = parseInt(parts[0], 5)
		c.CircuitBreaker.Window = parseDuration(parts[1], 30*time.Second)
	}
}

func (c *RouteConfig) parseHealthCheck(labels map[string]string) {
	c.HealthCheck.Path = labels[LabelHealthPath]
	if c.HealthCheck.Path == "" {
		c.HealthCheck.Path = "/"
	}

	c.HealthCheck.Interval = parseDuration(labels[LabelHealthInterval], 10*time.Second)
	c.HealthCheck.Timeout = parseDuration(labels[LabelHealthTimeout], 5*time.Second)
	c.HealthCheck.Threshold = parseInt(labels[LabelHealthThreshold], 3)
	c.HealthCheck.Recovery = parseInt(labels[LabelHealthRecovery], 2)
}

// Helper functions

func parseBool(v string) bool {
	return strings.ToLower(v) == "true" || v == "1"
}

func parseBoolDefault(v string, def bool) bool {
	if v == "" {
		return def
	}
	return parseBool(v)
}

func parseInt(v string, def int) int {
	if v == "" {
		return def
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return i
}

func parseDuration(v string, def time.Duration) time.Duration {
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}

func parseWindow(v string) time.Duration {
	switch strings.ToLower(v) {
	case "s", "sec", "second", "seconds":
		return time.Second
	case "m", "min", "minute", "minutes":
		return time.Minute
	case "h", "hour", "hours":
		return time.Hour
	default:
		// Try parsing as duration
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
		return time.Minute
	}
}

func parseSize(v string) int64 {
	if v == "" {
		return 0
	}

	// Parse sizes like 10mb, 1gb, 500kb
	v = strings.ToLower(strings.TrimSpace(v))

	var mult int64 = 1
	switch {
	case strings.HasSuffix(v, "gb"):
		mult = 1024 * 1024 * 1024
		v = strings.TrimSuffix(v, "gb")
	case strings.HasSuffix(v, "mb"):
		mult = 1024 * 1024
		v = strings.TrimSuffix(v, "mb")
	case strings.HasSuffix(v, "kb"):
		mult = 1024
		v = strings.TrimSuffix(v, "kb")
	case strings.HasSuffix(v, "b"):
		v = strings.TrimSuffix(v, "b")
	}

	num, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
	if err != nil {
		return 0
	}

	return num * mult
}

// IsEnabled checks if a container has dr.enable=true
func IsEnabled(labels map[string]string) bool {
	return parseBool(labels[LabelEnable])
}

// GetHost returns the configured host for a container
func GetHost(labels map[string]string) string {
	return labels[LabelHost]
}

// Validate validates the route configuration
func (c *RouteConfig) Validate() error {
	if !c.Enabled {
		return nil
	}

	if c.Host == "" {
		return fmt.Errorf("dr.host is required when dr.enable=true")
	}

	// Validate host format
	if strings.Contains(c.Host, ":") {
		return fmt.Errorf("dr.host should not include port: %s", c.Host)
	}

	// Validate path
	if c.Path != "" && !strings.HasPrefix(c.Path, "/") {
		return fmt.Errorf("dr.path must start with /: %s", c.Path)
	}

	// Validate TLS mode
	validTLS := map[string]bool{"auto": true, "manual": true, "off": true}
	if !validTLS[strings.ToLower(c.TLS)] {
		return fmt.Errorf("invalid dr.tls mode: %s", c.TLS)
	}

	// Validate manual TLS requires cert and key
	if c.TLS == "manual" {
		if c.TLSCertFile == "" || c.TLSKeyFile == "" {
			return fmt.Errorf("dr.tls.cert and dr.tls.key are required when dr.tls=manual")
		}
	}

	return nil
}
