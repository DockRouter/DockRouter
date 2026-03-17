// Package config handles application configuration
package config

import "time"

// Default configuration values per SPECIFICATION.md §5
const (
	DefaultHTTPPort     = 80
	DefaultHTTPSPort    = 443
	DefaultAdminPort    = 9090
	DefaultAdminBind    = "127.0.0.1"
	DefaultAdminEnabled = true

	DefaultDockerSocket = "/var/run/docker.sock"
	DefaultDataDir      = "/data"

	DefaultLogLevel  = "info"
	DefaultLogFormat = "json"
	DefaultAccessLog = true

	DefaultDefaultTLS = "auto"
	DefaultMaxBody    = "10mb"
	DefaultPollInt    = 10 * time.Second
)

// ACME provider URLs
const (
	ACMELetsEncryptProd    = "https://acme-v02.api.letsencrypt.org/directory"
	ACMELetsEncryptStaging = "https://acme-staging-v02.api.letsencrypt.org/directory"
	ACMEZeroSSL            = "https://acme.zerossl.com/v2/DV90"
)

// applyDefaults sets default values for all config fields
func (c *Config) applyDefaults() {
	c.HTTPPort = DefaultHTTPPort
	c.HTTPSPort = DefaultHTTPSPort
	c.Admin = DefaultAdminEnabled
	c.AdminPort = DefaultAdminPort
	c.AdminBind = DefaultAdminBind

	c.DockerSocket = DefaultDockerSocket
	c.DataDir = DefaultDataDir
	c.PollInterval = DefaultPollInt

	c.LogLevel = DefaultLogLevel
	c.LogFormat = DefaultLogFormat
	c.AccessLog = DefaultAccessLog

	c.DefaultTLS = DefaultDefaultTLS
	c.MaxBodySize = DefaultMaxBody

	c.ACMEProvider = "letsencrypt"
	c.ACMEStaging = false

	c.WebhookRetryCount = 3
}

// GetACMEDirectoryURL returns the ACME directory URL based on provider and staging settings
func (c *Config) GetACMEDirectoryURL() string {
	if c.ACMEStaging {
		return ACMELetsEncryptStaging
	}

	switch c.ACMEProvider {
	case "zerossl":
		return ACMEZeroSSL
	default:
		return ACMELetsEncryptProd
	}
}
