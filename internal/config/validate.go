// Package config handles application configuration
package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"time"
)

// Validate checks the configuration for errors
func (c *Config) Validate() error {
	var errs []error

	// Validate ports
	if c.HTTPPort < 1 || c.HTTPPort > 65535 {
		errs = append(errs, fmt.Errorf("invalid HTTP port: %d", c.HTTPPort))
	}
	if c.HTTPSPort < 1 || c.HTTPSPort > 65535 {
		errs = append(errs, fmt.Errorf("invalid HTTPS port: %d", c.HTTPSPort))
	}
	if c.AdminPort < 1 || c.AdminPort > 65535 {
		errs = append(errs, fmt.Errorf("invalid admin port: %d", c.AdminPort))
	}

	// Check for port conflicts
	if c.HTTPPort == c.HTTPSPort {
		errs = append(errs, errors.New("HTTP and HTTPS ports cannot be the same"))
	}
	if c.Admin && (c.AdminPort == c.HTTPPort || c.AdminPort == c.HTTPSPort) {
		errs = append(errs, errors.New("admin port conflicts with HTTP/HTTPS port"))
	}

	// Validate admin bind address
	if c.Admin {
		if c.AdminBind != "" && c.AdminBind != "0.0.0.0" && net.ParseIP(c.AdminBind) == nil {
			errs = append(errs, fmt.Errorf("invalid admin bind address: %s", c.AdminBind))
		}
	}

	// Validate log level
	validLevels := map[string]bool{
		"debug": true,
		"info":  true,
		"warn":  true,
		"error": true,
	}
	if !validLevels[strings.ToLower(c.LogLevel)] {
		errs = append(errs, fmt.Errorf("invalid log level: %s (valid: debug, info, warn, error)", c.LogLevel))
	}

	// Validate log format
	validFormats := map[string]bool{
		"json": true,
		"text": true,
	}
	if !validFormats[strings.ToLower(c.LogFormat)] {
		errs = append(errs, fmt.Errorf("invalid log format: %s (valid: json, text)", c.LogFormat))
	}

	// Validate TLS mode
	validTLS := map[string]bool{
		"auto":   true,
		"manual": true,
		"off":    true,
	}
	if !validTLS[strings.ToLower(c.DefaultTLS)] {
		errs = append(errs, fmt.Errorf("invalid TLS mode: %s (valid: auto, manual, off)", c.DefaultTLS))
	}

	// Validate poll interval
	if c.PollInterval < time.Second {
		errs = append(errs, fmt.Errorf("poll interval too short: %v (minimum 1s)", c.PollInterval))
	}

	// Validate trusted IPs
	for _, ip := range c.TrustedIPs {
		if _, _, err := net.ParseCIDR(ip); err != nil {
			// Try parsing as plain IP
			if net.ParseIP(ip) == nil {
				errs = append(errs, fmt.Errorf("invalid trusted IP: %s", ip))
			}
		}
	}

	// Validate ACME email format if provided
	if c.ACMEEmail != "" && !strings.Contains(c.ACMEEmail, "@") {
		errs = append(errs, fmt.Errorf("invalid ACME email: %s", c.ACMEEmail))
	}

	// Warn about security issues
	if c.Admin && c.AdminBind == "0.0.0.0" && c.AdminUser == "" {
		fmt.Fprintf(os.Stderr, "WARNING: Admin dashboard exposed on 0.0.0.0 without authentication\n")
	}

	if len(errs) > 0 {
		return fmt.Errorf("configuration errors: %v", errs)
	}

	return nil
}
