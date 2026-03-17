// Package discovery handles Docker container discovery
package discovery

import (
	"testing"
	"time"
)

func TestParseLabels(t *testing.T) {
	tests := []struct {
		name    string
		labels  map[string]string
		enabled bool
		host    string
	}{
		{
			name:    "disabled",
			labels:  map[string]string{"dr.enable": "false"},
			enabled: false,
		},
		{
			name:    "enabled with host",
			labels:  map[string]string{"dr.enable": "true", "dr.host": "example.com"},
			enabled: true,
			host:    "example.com",
		},
		{
			name:    "nil labels",
			labels:  nil,
			enabled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := ParseLabels(tt.labels)
			if config == nil {
				if tt.enabled {
					t.Error("Expected config, got nil")
				}
				return
			}
			if config.Enabled != tt.enabled {
				t.Errorf("Enabled = %v, want %v", config.Enabled, tt.enabled)
			}
			if config.Host != tt.host {
				t.Errorf("Host = %s, want %s", config.Host, tt.host)
			}
		})
	}
}

func TestParseLabelsRouting(t *testing.T) {
	labels := map[string]string{
		"dr.enable":       "true",
		"dr.host":         "api.example.com",
		"dr.port":         "3000",
		"dr.path":         "/v1",
		"dr.priority":     "10",
		"dr.loadbalancer": "iphash",
		"dr.weight":       "5",
	}

	config := ParseLabels(labels)

	if !config.Enabled {
		t.Error("Should be enabled")
	}
	if config.Host != "api.example.com" {
		t.Errorf("Host = %s, want api.example.com", config.Host)
	}
	if config.Port != 3000 {
		t.Errorf("Port = %d, want 3000", config.Port)
	}
	if config.Path != "/v1" {
		t.Errorf("Path = %s, want /v1", config.Path)
	}
	if config.Priority != 10 {
		t.Errorf("Priority = %d, want 10", config.Priority)
	}
	if config.LoadBalance != "iphash" {
		t.Errorf("LoadBalance = %s, want iphash", config.LoadBalance)
	}
	if config.Weight != 5 {
		t.Errorf("Weight = %d, want 5", config.Weight)
	}
}

func TestParseLabelsTLS(t *testing.T) {
	labels := map[string]string{
		"dr.enable":      "true",
		"dr.host":        "example.com",
		"dr.tls":         "auto",
		"dr.tls.domains": "www.example.com,example.com",
	}

	config := ParseLabels(labels)

	if config.TLS != "auto" {
		t.Errorf("TLS = %s, want auto", config.TLS)
	}
	if len(config.TLSDomains) != 2 {
		t.Errorf("TLSDomains count = %d, want 2", len(config.TLSDomains))
	}
}

func TestParseLabelsRateLimit(t *testing.T) {
	labels := map[string]string{
		"dr.enable":       "true",
		"dr.host":         "example.com",
		"dr.ratelimit":    "100/m",
		"dr.ratelimit.by": "X-API-Key",
	}

	config := ParseLabels(labels)

	if !config.RateLimit.Enabled {
		t.Error("Rate limit should be enabled")
	}
	if config.RateLimit.Count != 100 {
		t.Errorf("Rate limit count = %d, want 100", config.RateLimit.Count)
	}
	if config.RateLimit.ByKey != "X-API-Key" {
		t.Errorf("Rate limit key = %s, want X-API-Key", config.RateLimit.ByKey)
	}
}

func TestParseLabelsCORS(t *testing.T) {
	labels := map[string]string{
		"dr.enable":       "true",
		"dr.host":         "example.com",
		"dr.cors.origins": "https://app.com,https://web.com",
		"dr.cors.methods": "GET,POST",
	}

	config := ParseLabels(labels)

	if !config.CORS.Enabled {
		t.Error("CORS should be enabled")
	}
	if len(config.CORS.Origins) != 2 {
		t.Errorf("CORS origins count = %d, want 2", len(config.CORS.Origins))
	}
}

func TestParseSize(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		{"10mb", 10 * 1024 * 1024},
		{"1gb", 1 * 1024 * 1024 * 1024},
		{"500kb", 500 * 1024},
		{"1024b", 1024},
		{"1024", 1024},
		{"", 0},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := parseSize(tt.input)
			if result != tt.expected {
				t.Errorf("parseSize(%q) = %d, want %d", tt.input, result, tt.expected)
			}
		})
	}
}

func TestIsEnabled(t *testing.T) {
	if !IsEnabled(map[string]string{"dr.enable": "true"}) {
		t.Error("Should be enabled")
	}
	if IsEnabled(map[string]string{"dr.enable": "false"}) {
		t.Error("Should not be enabled")
	}
	if IsEnabled(map[string]string{}) {
		t.Error("Empty labels should not be enabled")
	}
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  *RouteConfig
		wantErr bool
	}{
		{
			name:    "valid",
			config:  &RouteConfig{Enabled: true, Host: "example.com", TLS: "auto"},
			wantErr: false,
		},
		{
			name:    "missing host",
			config:  &RouteConfig{Enabled: true, Host: ""},
			wantErr: true,
		},
		{
			name:    "disabled",
			config:  &RouteConfig{Enabled: false},
			wantErr: false,
		},
		{
			name:    "invalid TLS mode",
			config:  &RouteConfig{Enabled: true, Host: "example.com", TLS: "invalid"},
			wantErr: true,
		},
		{
			name:    "host with port",
			config:  &RouteConfig{Enabled: true, Host: "example.com:8080", TLS: "auto"},
			wantErr: true,
		},
		{
			name:    "path without leading slash",
			config:  &RouteConfig{Enabled: true, Host: "example.com", Path: "api/v1", TLS: "auto"},
			wantErr: true,
		},
		{
			name:    "path with leading slash valid",
			config:  &RouteConfig{Enabled: true, Host: "example.com", Path: "/api/v1", TLS: "auto"},
			wantErr: false,
		},
		{
			name:    "empty path valid",
			config:  &RouteConfig{Enabled: true, Host: "example.com", Path: "", TLS: "auto"},
			wantErr: false,
		},
		{
			name:    "manual TLS with cert and key",
			config:  &RouteConfig{Enabled: true, Host: "example.com", TLS: "manual", TLSCertFile: "/certs/cert.pem", TLSKeyFile: "/certs/key.pem"},
			wantErr: false,
		},
		{
			name:    "manual TLS missing cert",
			config:  &RouteConfig{Enabled: true, Host: "example.com", TLS: "manual", TLSCertFile: "", TLSKeyFile: "/certs/key.pem"},
			wantErr: true,
		},
		{
			name:    "manual TLS missing key",
			config:  &RouteConfig{Enabled: true, Host: "example.com", TLS: "manual", TLSCertFile: "/certs/cert.pem", TLSKeyFile: ""},
			wantErr: true,
		},
		{
			name:    "manual TLS missing both",
			config:  &RouteConfig{Enabled: true, Host: "example.com", TLS: "manual", TLSCertFile: "", TLSKeyFile: ""},
			wantErr: true,
		},
		{
			name:    "TLS off valid",
			config:  &RouteConfig{Enabled: true, Host: "example.com", TLS: "off"},
			wantErr: false,
		},
		{
			name:    "TLS case insensitive auto",
			config:  &RouteConfig{Enabled: true, Host: "example.com", TLS: "AUTO"},
			wantErr: false,
		},
		{
			name:    "TLS case insensitive manual",
			config:  &RouteConfig{Enabled: true, Host: "example.com", TLS: "MANUAL", TLSCertFile: "/certs/cert.pem", TLSKeyFile: "/certs/key.pem"},
			wantErr: false,
		},
		{
			name:    "TLS case insensitive off",
			config:  &RouteConfig{Enabled: true, Host: "example.com", TLS: "OFF"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestParseLabelsBasicAuth(t *testing.T) {
	tests := []struct {
		name          string
		labels        map[string]string
		expectedUsers int
	}{
		{
			name:          "no auth labels",
			labels:        map[string]string{"dr.enable": "true", "dr.host": "example.com"},
			expectedUsers: 0,
		},
		{
			name: "single user",
			labels: map[string]string{
				"dr.enable":           "true",
				"dr.host":             "example.com",
				"dr.auth.basic.users": "admin:$2a$10$hash",
			},
			expectedUsers: 1,
		},
		{
			name: "multiple users",
			labels: map[string]string{
				"dr.enable":           "true",
				"dr.host":             "example.com",
				"dr.auth.basic.users": "admin:$2a$10$hash1,user:$2a$10$hash2",
			},
			expectedUsers: 2,
		},
		{
			name: "malformed user (no colon)",
			labels: map[string]string{
				"dr.enable":           "true",
				"dr.host":             "example.com",
				"dr.auth.basic.users": "admininvalid",
			},
			expectedUsers: 0,
		},
		{
			name: "empty users label",
			labels: map[string]string{
				"dr.enable":           "true",
				"dr.host":             "example.com",
				"dr.auth.basic.users": "",
			},
			expectedUsers: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := ParseLabels(tt.labels)
			if config == nil {
				t.Fatal("ParseLabels returned nil")
			}
			if len(config.BasicAuthUsers) != tt.expectedUsers {
				t.Errorf("BasicAuthUsers count = %d, want %d", len(config.BasicAuthUsers), tt.expectedUsers)
			}
		})
	}
}

func TestParseLabelsIPFilters(t *testing.T) {
	tests := []struct {
		name              string
		labels            map[string]string
		expectedWhitelist int
		expectedBlacklist int
	}{
		{
			name:              "no IP labels",
			labels:            map[string]string{"dr.enable": "true", "dr.host": "example.com"},
			expectedWhitelist: 0,
			expectedBlacklist: 0,
		},
		{
			name: "CIDR whitelist",
			labels: map[string]string{
				"dr.enable":      "true",
				"dr.host":        "example.com",
				"dr.ipwhitelist": "192.168.1.0/24",
			},
			expectedWhitelist: 1,
			expectedBlacklist: 0,
		},
		{
			name: "single IP whitelist",
			labels: map[string]string{
				"dr.enable":      "true",
				"dr.host":        "example.com",
				"dr.ipwhitelist": "192.168.1.100",
			},
			expectedWhitelist: 1,
			expectedBlacklist: 0,
		},
		{
			name: "multiple CIDRs",
			labels: map[string]string{
				"dr.enable":      "true",
				"dr.host":        "example.com",
				"dr.ipwhitelist": "192.168.1.0/24,10.0.0.0/8",
			},
			expectedWhitelist: 2,
			expectedBlacklist: 0,
		},
		{
			name: "blacklist only",
			labels: map[string]string{
				"dr.enable":      "true",
				"dr.host":        "example.com",
				"dr.ipblacklist": "10.0.0.1",
			},
			expectedWhitelist: 0,
			expectedBlacklist: 1,
		},
		{
			name: "both whitelist and blacklist",
			labels: map[string]string{
				"dr.enable":      "true",
				"dr.host":        "example.com",
				"dr.ipwhitelist": "192.168.0.0/16",
				"dr.ipblacklist": "10.0.0.0/8",
			},
			expectedWhitelist: 1,
			expectedBlacklist: 1,
		},
		{
			name: "invalid CIDR (ignored)",
			labels: map[string]string{
				"dr.enable":      "true",
				"dr.host":        "example.com",
				"dr.ipwhitelist": "invalid-cidr",
			},
			expectedWhitelist: 0,
			expectedBlacklist: 0,
		},
		{
			name: "IPv6 single address",
			labels: map[string]string{
				"dr.enable":      "true",
				"dr.host":        "example.com",
				"dr.ipwhitelist": "::1",
			},
			expectedWhitelist: 1,
			expectedBlacklist: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := ParseLabels(tt.labels)
			if config == nil {
				t.Fatal("ParseLabels returned nil")
			}
			if len(config.IPWhitelist) != tt.expectedWhitelist {
				t.Errorf("IPWhitelist count = %d, want %d", len(config.IPWhitelist), tt.expectedWhitelist)
			}
			if len(config.IPBlacklist) != tt.expectedBlacklist {
				t.Errorf("IPBlacklist count = %d, want %d", len(config.IPBlacklist), tt.expectedBlacklist)
			}
		})
	}
}

func TestParseLabelsCircuitBreaker(t *testing.T) {
	tests := []struct {
		name          string
		labels        map[string]string
		expectEnabled bool
		expectFails   int
	}{
		{
			name:          "no circuit breaker",
			labels:        map[string]string{"dr.enable": "true", "dr.host": "example.com"},
			expectEnabled: false,
		},
		{
			name: "circuit breaker with defaults",
			labels: map[string]string{
				"dr.enable":         "true",
				"dr.host":           "example.com",
				"dr.circuitbreaker": "5/30s",
			},
			expectEnabled: true,
			expectFails:   5,
		},
		{
			name: "circuit breaker empty",
			labels: map[string]string{
				"dr.enable":         "true",
				"dr.host":           "example.com",
				"dr.circuitbreaker": "",
			},
			expectEnabled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := ParseLabels(tt.labels)
			if config == nil {
				t.Fatal("ParseLabels returned nil")
			}
			if config.CircuitBreaker.Enabled != tt.expectEnabled {
				t.Errorf("CircuitBreaker.Enabled = %v, want %v", config.CircuitBreaker.Enabled, tt.expectEnabled)
			}
			if tt.expectEnabled && config.CircuitBreaker.Failures != tt.expectFails {
				t.Errorf("CircuitBreaker.Failures = %d, want %d", config.CircuitBreaker.Failures, tt.expectFails)
			}
		})
	}
}

func TestParseLabelsRetry(t *testing.T) {
	labels := map[string]string{
		"dr.enable": "true",
		"dr.host":   "example.com",
		"dr.retry":  "3",
	}

	config := ParseLabels(labels)
	if config == nil {
		t.Fatal("ParseLabels returned nil")
	}
	if config.Retry != 3 {
		t.Errorf("Retry = %d, want 3", config.Retry)
	}
}

func TestParseLabelsCompress(t *testing.T) {
	labels := map[string]string{
		"dr.enable":   "true",
		"dr.host":     "example.com",
		"dr.compress": "true",
	}

	config := ParseLabels(labels)
	if config == nil {
		t.Fatal("ParseLabels returned nil")
	}
	if !config.Compress {
		t.Error("Compress should be true")
	}
}

func TestParseLabelsHealthCheck(t *testing.T) {
	labels := map[string]string{
		"dr.enable":               "true",
		"dr.host":                 "example.com",
		"dr.healthcheck.path":     "/health",
		"dr.healthcheck.interval": "30s",
	}

	config := ParseLabels(labels)
	if config == nil {
		t.Fatal("ParseLabels returned nil")
	}
	if config.HealthCheck.Path != "/health" {
		t.Errorf("HealthCheck.Path = %s, want /health", config.HealthCheck.Path)
	}
}

func TestParseLabelsRedirectHTTPS(t *testing.T) {
	labels := map[string]string{
		"dr.enable":         "true",
		"dr.host":           "example.com",
		"dr.redirect.https": "true",
	}

	config := ParseLabels(labels)
	if config == nil {
		t.Fatal("ParseLabels returned nil")
	}
	if !config.RedirectHTTPS {
		t.Error("RedirectHTTPS should be true")
	}
}

func TestParseLabelsStripPrefix(t *testing.T) {
	labels := map[string]string{
		"dr.enable":      "true",
		"dr.host":        "example.com",
		"dr.stripprefix": "/api/v1",
	}

	config := ParseLabels(labels)
	if config == nil {
		t.Fatal("ParseLabels returned nil")
	}
	if config.StripPrefix != "/api/v1" {
		t.Errorf("StripPrefix = %s, want /api/v1", config.StripPrefix)
	}
}

func TestParseLabelsAddPrefix(t *testing.T) {
	labels := map[string]string{
		"dr.enable":    "true",
		"dr.host":      "example.com",
		"dr.addprefix": "/api",
	}

	config := ParseLabels(labels)
	if config == nil {
		t.Fatal("ParseLabels returned nil")
	}
	if config.AddPrefix != "/api" {
		t.Errorf("AddPrefix = %s, want /api", config.AddPrefix)
	}
}

func TestParseLabelsWeight(t *testing.T) {
	labels := map[string]string{
		"dr.enable": "true",
		"dr.host":   "example.com",
		"dr.weight": "5",
	}

	config := ParseLabels(labels)
	if config == nil {
		t.Fatal("ParseLabels returned nil")
	}
	if config.Weight != 5 {
		t.Errorf("Weight = %d, want 5", config.Weight)
	}
}

func TestParseLabelsPriority(t *testing.T) {
	labels := map[string]string{
		"dr.enable":   "true",
		"dr.host":     "example.com",
		"dr.priority": "100",
	}

	config := ParseLabels(labels)
	if config == nil {
		t.Fatal("ParseLabels returned nil")
	}
	if config.Priority != 100 {
		t.Errorf("Priority = %d, want 100", config.Priority)
	}
}

func TestParseLabelsPort(t *testing.T) {
	labels := map[string]string{
		"dr.enable": "true",
		"dr.host":   "example.com",
		"dr.port":   "8080",
	}

	config := ParseLabels(labels)
	if config == nil {
		t.Fatal("ParseLabels returned nil")
	}
	if config.Port != 8080 {
		t.Errorf("Port = %d, want 8080", config.Port)
	}
}

func TestGetHost(t *testing.T) {
	labels := map[string]string{"dr.host": "test.example.com"}
	if GetHost(labels) != "test.example.com" {
		t.Errorf("GetHost() = %s, want test.example.com", GetHost(labels))
	}

	// Test with empty labels
	if GetHost(map[string]string{}) != "" {
		t.Error("GetHost should return empty string for empty labels")
	}
}

func TestParseLabelsLoadBalance(t *testing.T) {
	labels := map[string]string{
		"dr.enable":       "true",
		"dr.host":         "example.com",
		"dr.loadbalancer": "roundrobin",
	}

	config := ParseLabels(labels)
	if config == nil {
		t.Fatal("ParseLabels returned nil")
	}
	if config.LoadBalance != "roundrobin" {
		t.Errorf("LoadBalance = %s, want roundrobin", config.LoadBalance)
	}
}

func TestParseLabelsAddress(t *testing.T) {
	labels := map[string]string{
		"dr.enable":  "true",
		"dr.host":    "example.com",
		"dr.address": "192.168.1.100:8080",
	}

	config := ParseLabels(labels)
	if config == nil {
		t.Fatal("ParseLabels returned nil")
	}
	if config.Address != "192.168.1.100:8080" {
		t.Errorf("Address = %s, want 192.168.1.100:8080", config.Address)
	}
}

func TestParseLabelsPath(t *testing.T) {
	labels := map[string]string{
		"dr.enable": "true",
		"dr.host":   "example.com",
		"dr.path":   "/api/v1",
	}

	config := ParseLabels(labels)
	if config == nil {
		t.Fatal("ParseLabels returned nil")
	}
	if config.Path != "/api/v1" {
		t.Errorf("Path = %s, want /api/v1", config.Path)
	}
}

func TestParseLabelsEmpty(t *testing.T) {
	config := ParseLabels(nil)
	if config != nil {
		t.Error("ParseLabels(nil) should return nil")
	}

	// Empty labels returns config with Enabled=false
	config = ParseLabels(map[string]string{})
	if config == nil {
		t.Fatal("ParseLabels(empty) should return config")
	}
	if config.Enabled {
		t.Error("ParseLabels(empty).Enabled should be false")
	}
}

func TestParseLabelsDisabled(t *testing.T) {
	labels := map[string]string{
		"dr.enable": "false",
		"dr.host":   "example.com",
	}

	config := ParseLabels(labels)
	if config == nil {
		t.Fatal("ParseLabels should return config even when disabled")
	}
	if config.Enabled {
		t.Error("config.Enabled should be false")
	}
}

func TestParseLabelsMaxBody(t *testing.T) {
	labels := map[string]string{
		"dr.enable":  "true",
		"dr.host":    "example.com",
		"dr.maxbody": "10mb",
	}

	config := ParseLabels(labels)
	if config == nil {
		t.Fatal("ParseLabels returned nil")
	}
	if config.MaxBody <= 0 {
		t.Error("MaxBody should be set from 10mb")
	}
}

func TestParseLabelsRetryCount(t *testing.T) {
	labels := map[string]string{
		"dr.enable": "true",
		"dr.host":   "example.com",
		"dr.retry":  "5",
	}

	config := ParseLabels(labels)
	if config == nil {
		t.Fatal("ParseLabels returned nil")
	}
	if config.Retry != 5 {
		t.Errorf("Retry = %d, want 5", config.Retry)
	}
}

func TestParseLabelsCircuitBreakerConfig(t *testing.T) {
	labels := map[string]string{
		"dr.enable":         "true",
		"dr.host":           "example.com",
		"dr.circuitbreaker": "10/60s",
	}

	config := ParseLabels(labels)
	if config == nil {
		t.Fatal("ParseLabels returned nil")
	}
	if !config.CircuitBreaker.Enabled {
		t.Error("CircuitBreaker.Enabled should be true")
	}
	if config.CircuitBreaker.Failures != 10 {
		t.Errorf("CircuitBreaker.Failures = %d, want 10", config.CircuitBreaker.Failures)
	}
}

func TestParseLabelsIPBlacklist(t *testing.T) {
	labels := map[string]string{
		"dr.enable":      "true",
		"dr.host":        "example.com",
		"dr.ipblacklist": "10.0.0.1,192.168.1.0/24",
	}

	config := ParseLabels(labels)
	if config == nil {
		t.Fatal("ParseLabels returned nil")
	}
	if len(config.IPBlacklist) != 2 {
		t.Errorf("IPBlacklist count = %d, want 2", len(config.IPBlacklist))
	}
}

func TestParseLabelsTLSCert(t *testing.T) {
	labels := map[string]string{
		"dr.enable":   "true",
		"dr.host":     "example.com",
		"dr.tls":      "manual",
		"dr.tls.cert": "/certs/example.com.crt",
		"dr.tls.key":  "/certs/example.com.key",
	}

	config := ParseLabels(labels)
	if config == nil {
		t.Fatal("ParseLabels returned nil")
	}
	if config.TLSCertFile != "/certs/example.com.crt" {
		t.Errorf("TLSCertFile = %s", config.TLSCertFile)
	}
	if config.TLSKeyFile != "/certs/example.com.key" {
		t.Errorf("TLSKeyFile = %s", config.TLSKeyFile)
	}
}

func TestParseWindow(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
	}{
		// Named windows
		{"s", time.Second},
		{"sec", time.Second},
		{"second", time.Second},
		{"seconds", time.Second},
		{"S", time.Second},
		{"SEC", time.Second},
		{"m", time.Minute},
		{"min", time.Minute},
		{"minute", time.Minute},
		{"minutes", time.Minute},
		{"M", time.Minute},
		{"MIN", time.Minute},
		{"h", time.Hour},
		{"hour", time.Hour},
		{"hours", time.Hour},
		{"H", time.Hour},
		// Duration parsing
		{"30s", 30 * time.Second},
		{"5m", 5 * time.Minute},
		{"1h", time.Hour},
		{"100ms", 100 * time.Millisecond},
		// Invalid returns default (minute)
		{"invalid", time.Minute},
		{"", time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := parseWindow(tt.input)
			if result != tt.expected {
				t.Errorf("parseWindow(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestParseLabelsRateLimitWindows(t *testing.T) {
	tests := []struct {
		name           string
		ratelimitLabel string
		expectedCount  int
		expectedWindow time.Duration
	}{
		{
			name:           "requests per second",
			ratelimitLabel: "10/s",
			expectedCount:  10,
			expectedWindow: time.Second,
		},
		{
			name:           "requests per minute",
			ratelimitLabel: "100/m",
			expectedCount:  100,
			expectedWindow: time.Minute,
		},
		{
			name:           "requests per hour",
			ratelimitLabel: "1000/h",
			expectedCount:  1000,
			expectedWindow: time.Hour,
		},
		{
			name:           "requests with seconds window",
			ratelimitLabel: "50/sec",
			expectedCount:  50,
			expectedWindow: time.Second,
		},
		{
			name:           "requests with minutes window",
			ratelimitLabel: "200/minutes",
			expectedCount:  200,
			expectedWindow: time.Minute,
		},
		{
			name:           "requests with duration window",
			ratelimitLabel: "100/30s",
			expectedCount:  100,
			expectedWindow: 30 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			labels := map[string]string{
				"dr.enable":    "true",
				"dr.host":      "example.com",
				"dr.ratelimit": tt.ratelimitLabel,
			}

			config := ParseLabels(labels)
			if config == nil {
				t.Fatal("ParseLabels returned nil")
			}
			if config.RateLimit.Count != tt.expectedCount {
				t.Errorf("RateLimit.Count = %d, want %d", config.RateLimit.Count, tt.expectedCount)
			}
			if config.RateLimit.Window != tt.expectedWindow {
				t.Errorf("RateLimit.Window = %v, want %v", config.RateLimit.Window, tt.expectedWindow)
			}
		})
	}
}

func TestParseLabelsCORSWithHeaders(t *testing.T) {
	labels := map[string]string{
		"dr.enable":       "true",
		"dr.host":         "example.com",
		"dr.cors.origins": "https://app.com",
		"dr.cors.headers": "X-Custom-Header,Authorization",
	}

	config := ParseLabels(labels)
	if config == nil {
		t.Fatal("ParseLabels returned nil")
	}
	if !config.CORS.Enabled {
		t.Error("CORS should be enabled")
	}
	if len(config.CORS.Headers) != 2 {
		t.Errorf("CORS headers count = %d, want 2", len(config.CORS.Headers))
	}
}

func TestParseLabelsCORSDefaultMethods(t *testing.T) {
	labels := map[string]string{
		"dr.enable":       "true",
		"dr.host":         "example.com",
		"dr.cors.origins": "https://app.com",
		// No dr.cors.methods - should use defaults
	}

	config := ParseLabels(labels)
	if config == nil {
		t.Fatal("ParseLabels returned nil")
	}
	if len(config.CORS.Methods) != 5 {
		t.Errorf("CORS methods count = %d, want 5 (default)", len(config.CORS.Methods))
	}
}

func TestParseSizeInvalid(t *testing.T) {
	// Test invalid size format
	result := parseSize("invalid")
	if result != 0 {
		t.Errorf("parseSize('invalid') = %d, want 0", result)
	}

	// Test empty string
	result = parseSize("")
	if result != 0 {
		t.Errorf("parseSize('') = %d, want 0", result)
	}
}

func TestParseIntInvalid(t *testing.T) {
	// parseInt is not exported, test via ParseLabels
	labels := map[string]string{
		"dr.enable": "true",
		"dr.host":   "example.com",
		"dr.port":   "invalid",
	}

	config := ParseLabels(labels)
	if config == nil {
		t.Fatal("ParseLabels returned nil")
	}
	// Should use default (0) when parsing fails
	if config.Port != 0 {
		t.Errorf("Port = %d, want 0 (default for invalid)", config.Port)
	}
}

func TestParseDurationInvalid(t *testing.T) {
	// parseDuration is used in circuit breaker - test via labels
	labels := map[string]string{
		"dr.enable":         "true",
		"dr.host":           "example.com",
		"dr.circuitbreaker": "5/invalid",
	}

	config := ParseLabels(labels)
	if config == nil {
		t.Fatal("ParseLabels returned nil")
	}
	// Should use default duration when parsing fails
	if config.CircuitBreaker.Window != 30*time.Second {
		t.Errorf("CircuitBreaker.Window = %v, want 30s (default)", config.CircuitBreaker.Window)
	}
}

func TestParseLabelsIPBlacklistIPv6(t *testing.T) {
	labels := map[string]string{
		"dr.enable":      "true",
		"dr.host":        "example.com",
		"dr.ipblacklist": "::1,2001:db8::/32",
	}

	config := ParseLabels(labels)
	if config == nil {
		t.Fatal("ParseLabels returned nil")
	}
	if len(config.IPBlacklist) != 2 {
		t.Errorf("IPBlacklist count = %d, want 2", len(config.IPBlacklist))
	}
}

func TestParseLabelsMiddlewares(t *testing.T) {
	labels := map[string]string{
		"dr.enable":      "true",
		"dr.host":        "example.com",
		"dr.middlewares": "ratelimit,compress,auth",
	}

	config := ParseLabels(labels)
	if config == nil {
		t.Fatal("ParseLabels returned nil")
	}
	if len(config.Middlewares) != 3 {
		t.Errorf("Middlewares count = %d, want 3", len(config.Middlewares))
	}
	if config.Middlewares[0] != "ratelimit" {
		t.Errorf("Middlewares[0] = %s, want ratelimit", config.Middlewares[0])
	}
}

func TestParseLabelsRedirectHTTPSDefault(t *testing.T) {
	labels := map[string]string{
		"dr.enable":         "true",
		"dr.host":           "example.com",
		"dr.redirect.https": "false", // Explicitly disable
	}

	config := ParseLabels(labels)
	if config == nil {
		t.Fatal("ParseLabels returned nil")
	}
	if config.RedirectHTTPS {
		t.Error("RedirectHTTPS should be false when explicitly set to false")
	}
}

func TestParseLabelsRateLimitWithDefaultKey(t *testing.T) {
	labels := map[string]string{
		"dr.enable":    "true",
		"dr.host":      "example.com",
		"dr.ratelimit": "100/m",
		// No dr.ratelimit.by - should default to client_ip
	}

	config := ParseLabels(labels)
	if config == nil {
		t.Fatal("ParseLabels returned nil")
	}
	if config.RateLimit.ByKey != "client_ip" {
		t.Errorf("RateLimit.ByKey = %s, want client_ip (default)", config.RateLimit.ByKey)
	}
}

func TestParseLabelsCircuitBreakerWindows(t *testing.T) {
	tests := []struct {
		name             string
		circuitLabel     string
		expectedFailures int
		expectedWindow   time.Duration
	}{
		{
			name:             "failures with seconds",
			circuitLabel:     "5/1s",
			expectedFailures: 5,
			expectedWindow:   time.Second,
		},
		{
			name:             "failures with minutes",
			circuitLabel:     "10/1m",
			expectedFailures: 10,
			expectedWindow:   time.Minute,
		},
		{
			name:             "failures with duration",
			circuitLabel:     "15/30s",
			expectedFailures: 15,
			expectedWindow:   30 * time.Second,
		},
		{
			name:             "failures with custom duration",
			circuitLabel:     "20/5m",
			expectedFailures: 20,
			expectedWindow:   5 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			labels := map[string]string{
				"dr.enable":         "true",
				"dr.host":           "example.com",
				"dr.circuitbreaker": tt.circuitLabel,
			}

			config := ParseLabels(labels)
			if config == nil {
				t.Fatal("ParseLabels returned nil")
			}
			if !config.CircuitBreaker.Enabled {
				t.Error("CircuitBreaker should be enabled")
			}
			if config.CircuitBreaker.Failures != tt.expectedFailures {
				t.Errorf("CircuitBreaker.Failures = %d, want %d", config.CircuitBreaker.Failures, tt.expectedFailures)
			}
			if config.CircuitBreaker.Window != tt.expectedWindow {
				t.Errorf("CircuitBreaker.Window = %v, want %v", config.CircuitBreaker.Window, tt.expectedWindow)
			}
		})
	}
}
