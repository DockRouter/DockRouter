// Package config handles application configuration
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all DockRouter configuration
type Config struct {
	// HTTP/HTTPS listeners
	HTTPPort  int
	HTTPSPort int

	// Admin dashboard
	Admin     bool
	AdminPort int
	AdminBind string
	AdminUser string
	AdminPass string

	// Docker
	DockerSocket string
	PollInterval time.Duration

	// Data
	DataDir string

	// ACME
	ACMEEmail    string
	ACMEProvider string
	ACMEStaging  bool

	// Logging
	LogLevel  string
	LogFormat string
	AccessLog bool

	// Defaults
	DefaultTLS  string
	MaxBodySize string
	TrustedIPs  []string

	// Webhook notifications
	WebhookURLs       []string
	WebhookSecretKey  string
	WebhookEvents     []string
	WebhookRetryCount int

	// Runtime info
	Version   string
	BuildTime string
}

// Load creates a new Config from flags, env vars, and defaults
func Load(version, buildTime string) (*Config, error) {
	cfg := &Config{
		Version:   version,
		BuildTime: buildTime,
	}

	// Load defaults first
	cfg.applyDefaults()

	// Override from environment variables
	cfg.loadFromEnv()

	// Parse command-line flags
	if err := cfg.parseFlags(); err != nil {
		return nil, err
	}

	// Validate
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// parseFlags parses command-line flags
func (c *Config) parseFlags() error {
	fs := NewFlagSet("dockrouter")

	fs.IntVar(&c.HTTPPort, "http-port", c.HTTPPort, "HTTP listener port")
	fs.IntVar(&c.HTTPSPort, "https-port", c.HTTPSPort, "HTTPS listener port")
	fs.BoolVar(&c.Admin, "admin", c.Admin, "Enable admin dashboard")
	fs.IntVar(&c.AdminPort, "admin-port", c.AdminPort, "Admin dashboard port")
	fs.StringVar(&c.AdminBind, "admin-bind", c.AdminBind, "Admin bind address")
	fs.StringVar(&c.AdminUser, "admin-user", c.AdminUser, "Admin username")
	fs.StringVar(&c.AdminPass, "admin-pass", c.AdminPass, "Admin password")
	fs.StringVar(&c.DockerSocket, "docker-socket", c.DockerSocket, "Docker socket path")
	fs.StringVar(&c.DataDir, "data-dir", c.DataDir, "Data directory")
	fs.StringVar(&c.ACMEEmail, "acme-email", c.ACMEEmail, "ACME account email")
	fs.StringVar(&c.ACMEProvider, "acme-provider", c.ACMEProvider, "ACME provider (letsencrypt, zerossl)")
	fs.BoolVar(&c.ACMEStaging, "acme-staging", c.ACMEStaging, "Use ACME staging server")
	fs.StringVar(&c.LogLevel, "log-level", c.LogLevel, "Log level (debug, info, warn, error)")
	fs.StringVar(&c.LogFormat, "log-format", c.LogFormat, "Log format (json, text)")
	fs.BoolVar(&c.AccessLog, "access-log", c.AccessLog, "Enable access logging")
	fs.StringVar(&c.DefaultTLS, "default-tls", c.DefaultTLS, "Default TLS mode (auto, manual, off)")
	fs.StringVar(&c.MaxBodySize, "max-body-size", c.MaxBodySize, "Max request body size")
	fs.DurationVar(&c.PollInterval, "poll-interval", c.PollInterval, "Docker polling interval")
	fs.StringSliceVar(&c.TrustedIPs, "trusted-ips", c.TrustedIPs, "Trusted proxy IPs for X-Forwarded-For")

	fs.BoolVar(new(bool), "help", false, "Show help")
	fs.BoolVar(new(bool), "version", false, "Show version")

	if err := fs.Parse(os.Args[1:]); err != nil {
		return err
	}

	// Handle help and version flags
	if fs.Bool("help") {
		fmt.Println("DockRouter - Zero-dependency Docker-native ingress router")
		fmt.Println()
		fmt.Println("Usage: dockrouter [options]")
		fmt.Println()
		fs.PrintDefaults()
		os.Exit(0)
	}

	if fs.Bool("version") {
		fmt.Printf("DockRouter %s (built %s)\n", c.Version, c.BuildTime)
		os.Exit(0)
	}

	return nil
}

// loadFromEnv loads configuration from environment variables
func (c *Config) loadFromEnv() {
	loadInt(&c.HTTPPort, "DR_HTTP_PORT")
	loadInt(&c.HTTPSPort, "DR_HTTPS_PORT")
	loadBool(&c.Admin, "DR_ADMIN")
	loadInt(&c.AdminPort, "DR_ADMIN_PORT")
	loadString(&c.AdminBind, "DR_ADMIN_BIND")
	loadString(&c.AdminUser, "DR_ADMIN_USER")
	loadString(&c.AdminPass, "DR_ADMIN_PASS")
	loadString(&c.DockerSocket, "DR_DOCKER_SOCKET")
	loadString(&c.DataDir, "DR_DATA_DIR")
	loadString(&c.ACMEEmail, "DR_ACME_EMAIL")
	loadString(&c.ACMEProvider, "DR_ACME_PROVIDER")
	loadBool(&c.ACMEStaging, "DR_ACME_STAGING")
	loadString(&c.LogLevel, "DR_LOG_LEVEL")
	loadString(&c.LogFormat, "DR_LOG_FORMAT")
	loadBool(&c.AccessLog, "DR_ACCESS_LOG")
	loadString(&c.DefaultTLS, "DR_DEFAULT_TLS")
	loadString(&c.MaxBodySize, "DR_MAX_BODY_SIZE")
	loadDuration(&c.PollInterval, "DR_POLL_INTERVAL")
	loadSlice(&c.TrustedIPs, "DR_TRUSTED_IPS")

	// Webhook
	loadSlice(&c.WebhookURLs, "DR_WEBHOOK_URLS")
	loadString(&c.WebhookSecretKey, "DR_WEBHOOK_SECRET_KEY")
	loadSlice(&c.WebhookEvents, "DR_WEBHOOK_EVENTS")
	loadInt(&c.WebhookRetryCount, "DR_WEBHOOK_RETRY_COUNT")
}

// Helper functions for env loading
func loadString(target *string, key string) {
	if v := os.Getenv(key); v != "" {
		*target = v
	}
}

func loadInt(target *int, key string) {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			*target = i
		}
	}
}

func loadBool(target *bool, key string) {
	if v := os.Getenv(key); v != "" {
		*target = strings.ToLower(v) == "true" || v == "1"
	}
}

func loadDuration(target *time.Duration, key string) {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			*target = d
		}
	}
}

func loadSlice(target *[]string, key string) {
	if v := os.Getenv(key); v != "" {
		*target = strings.Split(v, ",")
	}
}

// String returns a safe string representation (masks sensitive values)
func (c *Config) String() string {
	return fmt.Sprintf(
		"Config{HTTPPort: %d, HTTPSPort: %d, Admin: %v, AdminPort: %d, AdminBind: %s, AdminUser: %s, AdminPass: ***, DockerSocket: %s, DataDir: %s, ACMEEmail: %s, LogLevel: %s}",
		c.HTTPPort, c.HTTPSPort, c.Admin, c.AdminPort, c.AdminBind, c.AdminUser, c.DockerSocket, c.DataDir, c.ACMEEmail, c.LogLevel,
	)
}
