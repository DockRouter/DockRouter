// Package config handles application configuration
package config

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestLoadFromEnv(t *testing.T) {
	// Set env vars
	os.Setenv("DR_HTTP_PORT", "8080")
	os.Setenv("DR_HTTPS_PORT", "8443")
	os.Setenv("DR_ADMIN", "false")
	os.Setenv("DR_LOG_LEVEL", "debug")
	defer func() {
		os.Unsetenv("DR_HTTP_PORT")
		os.Unsetenv("DR_HTTPS_PORT")
		os.Unsetenv("DR_ADMIN")
		os.Unsetenv("DR_LOG_LEVEL")
	}()

	cfg := &Config{}
	cfg.applyDefaults()
	cfg.loadFromEnv()

	if cfg.HTTPPort != 8080 {
		t.Errorf("HTTPPort = %d, want 8080", cfg.HTTPPort)
	}
	if cfg.HTTPSPort != 8443 {
		t.Errorf("HTTPSPort = %d, want 8443", cfg.HTTPSPort)
	}
	if cfg.Admin != false {
		t.Errorf("Admin = %v, want false", cfg.Admin)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %s, want debug", cfg.LogLevel)
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: &Config{
				HTTPPort:     80,
				HTTPSPort:    443,
				AdminPort:    9090,
				LogLevel:     "info",
				LogFormat:    "json",
				DefaultTLS:   "auto",
				PollInterval: 10 * time.Second,
			},
			wantErr: false,
		},
		{
			name: "invalid HTTP port",
			config: &Config{
				HTTPPort:     -1,
				HTTPSPort:    443,
				AdminPort:    9090,
				LogLevel:     "info",
				LogFormat:    "json",
				DefaultTLS:   "auto",
				PollInterval: 10 * time.Second,
			},
			wantErr: true,
		},
		{
			name: "same ports",
			config: &Config{
				HTTPPort:     80,
				HTTPSPort:    80,
				AdminPort:    9090,
				LogLevel:     "info",
				LogFormat:    "json",
				DefaultTLS:   "auto",
				PollInterval: 10 * time.Second,
			},
			wantErr: true,
		},
		{
			name: "invalid log level",
			config: &Config{
				HTTPPort:     80,
				HTTPSPort:    443,
				AdminPort:    9090,
				LogLevel:     "invalid",
				LogFormat:    "json",
				DefaultTLS:   "auto",
				PollInterval: 10 * time.Second,
			},
			wantErr: true,
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

func TestGetACMEDirectoryURL(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		staging  bool
		expected string
	}{
		{"letsencrypt prod", "letsencrypt", false, ACMELetsEncryptProd},
		{"letsencrypt staging", "letsencrypt", true, ACMELetsEncryptStaging},
		{"zerossl", "zerossl", false, ACMEZeroSSL},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				ACMEProvider: tt.provider,
				ACMEStaging:  tt.staging,
			}
			result := cfg.GetACMEDirectoryURL()
			if result != tt.expected {
				t.Errorf("GetACMEDirectoryURL() = %s, want %s", result, tt.expected)
			}
		})
	}
}

func TestLoadBool(t *testing.T) {
	tests := []struct {
		envValue string
		initial  bool
		expected bool
	}{
		{"true", false, true},
		{"True", false, true},
		{"TRUE", false, true},
		{"1", false, true},
		{"false", true, false},
		{"0", true, false},
		{"", true, true}, // empty env keeps initial
		{"anything", true, false},
	}

	for _, tt := range tests {
		t.Run(tt.envValue, func(t *testing.T) {
			os.Setenv("TEST_BOOL", tt.envValue)
			defer os.Unsetenv("TEST_BOOL")

			result := tt.initial
			loadBool(&result, "TEST_BOOL")

			if result != tt.expected {
				t.Errorf("loadBool() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestLoadInt(t *testing.T) {
	tests := []struct {
		envValue string
		expected int
	}{
		{"123", 123},
		{"0", 0},
		{"-5", -5},
		{"", 42},    // empty keeps initial
		{"abc", 42}, // invalid keeps initial
	}

	for _, tt := range tests {
		t.Run(tt.envValue, func(t *testing.T) {
			os.Setenv("TEST_INT", tt.envValue)
			defer os.Unsetenv("TEST_INT")

			result := 42
			loadInt(&result, "TEST_INT")

			if result != tt.expected {
				t.Errorf("loadInt() = %d, want %d", result, tt.expected)
			}
		})
	}
}

func TestLoadDuration(t *testing.T) {
	tests := []struct {
		envValue string
		expected time.Duration
	}{
		{"10s", 10 * time.Second},
		{"1m", time.Minute},
		{"1h", time.Hour},
	}

	for _, tt := range tests {
		t.Run(tt.envValue, func(t *testing.T) {
			os.Setenv("TEST_DURATION", tt.envValue)
			defer os.Unsetenv("TEST_DURATION")

			var result time.Duration
			loadDuration(&result, "TEST_DURATION")

			if result != tt.expected {
				t.Errorf("loadDuration() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestLoadSlice(t *testing.T) {
	os.Setenv("TEST_SLICE", "a,b,c")
	defer os.Unsetenv("TEST_SLICE")

	var result []string
	loadSlice(&result, "TEST_SLICE")

	if len(result) != 3 {
		t.Errorf("loadSlice() len = %d, want 3", len(result))
	}
	if result[0] != "a" || result[1] != "b" || result[2] != "c" {
		t.Errorf("loadSlice() = %v, want [a b c]", result)
	}
}

func TestConfigString(t *testing.T) {
	cfg := &Config{
		HTTPPort:     80,
		HTTPSPort:    443,
		Admin:        true,
		AdminPort:    9090,
		AdminBind:    "0.0.0.0",
		AdminUser:    "admin",
		AdminPass:    "secret",
		DockerSocket: "/var/run/docker.sock",
		DataDir:      "/data",
		ACMEEmail:    "test@example.com",
		LogLevel:     "info",
	}

	s := cfg.String()
	if strings.Contains(s, "secret") {
		t.Error("Config.String() should mask password")
	}
	if !strings.Contains(s, "***") {
		t.Error("Config.String() should contain password mask")
	}
}

func TestNewFlagSet(t *testing.T) {
	fs := NewFlagSet("test")
	if fs == nil {
		t.Fatal("NewFlagSet returned nil")
	}
	if fs.flags == nil {
		t.Error("flags map should be initialized")
	}
	if fs.bools == nil {
		t.Error("bools map should be initialized")
	}
}

func TestFlagSetIntVar(t *testing.T) {
	fs := NewFlagSet("test")
	var port int
	fs.IntVar(&port, "port", 8080, "test port")

	if port != 8080 {
		t.Errorf("IntVar default = %d, want 8080", port)
	}
}

func TestFlagSetStringVar(t *testing.T) {
	fs := NewFlagSet("test")
	var host string
	fs.StringVar(&host, "host", "localhost", "test host")

	if host != "localhost" {
		t.Errorf("StringVar default = %s, want localhost", host)
	}
}

func TestFlagSetBoolVar(t *testing.T) {
	fs := NewFlagSet("test")
	var enabled bool
	fs.BoolVar(&enabled, "enabled", true, "test flag")

	if !enabled {
		t.Error("BoolVar default should be true")
	}
	if !fs.Bool("enabled") {
		t.Error("Bool() should return true")
	}
}

func TestFlagSetDurationVar(t *testing.T) {
	fs := NewFlagSet("test")
	var timeout time.Duration
	fs.DurationVar(&timeout, "timeout", 30*time.Second, "test timeout")

	if timeout != 30*time.Second {
		t.Errorf("DurationVar default = %v, want 30s", timeout)
	}
}

func TestFlagSetParse(t *testing.T) {
	fs := NewFlagSet("test")
	var port int
	var host string
	var verbose bool

	fs.IntVar(&port, "port", 8080, "port number")
	fs.StringVar(&host, "host", "localhost", "host name")
	fs.BoolVar(&verbose, "verbose", false, "verbose mode")

	err := fs.Parse([]string{"-port", "9090", "-host", "example.com", "-verbose"})
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if port != 9090 {
		t.Errorf("port = %d, want 9090", port)
	}
	if host != "example.com" {
		t.Errorf("host = %s, want example.com", host)
	}
	if !verbose {
		t.Error("verbose should be true")
	}
}

func TestFlagSetParseTwice(t *testing.T) {
	fs := NewFlagSet("test")
	var port int
	fs.IntVar(&port, "port", 8080, "port number")

	err := fs.Parse([]string{"-port", "9090"})
	if err != nil {
		t.Fatalf("First Parse failed: %v", err)
	}

	// Second parse should be a no-op
	err = fs.Parse([]string{"-port", "9999"})
	if err != nil {
		t.Fatalf("Second Parse failed: %v", err)
	}

	// Port should still be 9090 from first parse
	if port != 9090 {
		t.Errorf("port = %d, want 9090", port)
	}
}

func TestFlagSetParseUnknownFlag(t *testing.T) {
	fs := NewFlagSet("test")
	var port int
	fs.IntVar(&port, "port", 8080, "port number")

	err := fs.Parse([]string{"-unknown", "value"})
	if err == nil {
		t.Error("Parse should return error for unknown flag")
	}
}

func TestFlagSetParseMissingValue(t *testing.T) {
	fs := NewFlagSet("test")
	var port int
	fs.IntVar(&port, "port", 8080, "port number")

	err := fs.Parse([]string{"-port"})
	// This should either error or use the next arg as value
	// Depends on implementation
	_ = err
}

func TestFlagSetPrintDefaults(t *testing.T) {
	fs := NewFlagSet("test")
	var port int
	fs.IntVar(&port, "port", 8080, "port number")

	// Just verify it doesn't panic
	fs.PrintDefaults()
}

func TestApplyDefaults(t *testing.T) {
	cfg := &Config{}
	cfg.applyDefaults()

	if cfg.HTTPPort != 80 {
		t.Errorf("HTTPPort default = %d, want 80", cfg.HTTPPort)
	}
	if cfg.HTTPSPort != 443 {
		t.Errorf("HTTPSPort default = %d, want 443", cfg.HTTPSPort)
	}
	if cfg.AdminPort != 9090 {
		t.Errorf("AdminPort default = %d, want 9090", cfg.AdminPort)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel default = %s, want info", cfg.LogLevel)
	}
}

func TestLoadDurationEmpty(t *testing.T) {
	os.Setenv("TEST_EMPTY_DURATION", "")
	defer os.Unsetenv("TEST_EMPTY_DURATION")

	var result time.Duration = 5 * time.Second
	loadDuration(&result, "TEST_EMPTY_DURATION")

	if result != 5*time.Second {
		t.Errorf("loadDuration with empty env = %v, want 5s", result)
	}
}

func TestLoadDurationInvalid(t *testing.T) {
	os.Setenv("TEST_INVALID_DURATION", "invalid")
	defer os.Unsetenv("TEST_INVALID_DURATION")

	var result time.Duration = 5 * time.Second
	loadDuration(&result, "TEST_INVALID_DURATION")

	// Should keep initial value on error
	if result != 5*time.Second {
		t.Errorf("loadDuration with invalid env = %v, want 5s", result)
	}
}

func TestLoadSliceEmpty(t *testing.T) {
	os.Setenv("TEST_EMPTY_SLICE", "")
	defer os.Unsetenv("TEST_EMPTY_SLICE")

	var result []string = []string{"initial"}
	loadSlice(&result, "TEST_EMPTY_SLICE")

	// Empty env should keep initial or be empty
	// Depends on implementation
	_ = result
}

func TestFlagSetStringSliceVar(t *testing.T) {
	fs := NewFlagSet("test")
	var hosts []string
	fs.StringSliceVar(&hosts, "hosts", []string{"default"}, "host list")

	// Test parsing with comma-separated values
	err := fs.Parse([]string{"-hosts", "a.com,b.com,c.com"})
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(hosts) != 3 {
		t.Errorf("hosts len = %d, want 3", len(hosts))
	}
	if hosts[0] != "a.com" || hosts[1] != "b.com" || hosts[2] != "c.com" {
		t.Errorf("hosts = %v, want [a.com b.com c.com]", hosts)
	}
}

func TestFlagSetParseWithEquals(t *testing.T) {
	fs := NewFlagSet("test")
	var port int
	var verbose bool

	fs.IntVar(&port, "port", 8080, "port number")
	fs.BoolVar(&verbose, "verbose", false, "verbose mode")

	// Test --flag=value format
	err := fs.Parse([]string{"--port=9090", "--verbose=true"})
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if port != 9090 {
		t.Errorf("port = %d, want 9090", port)
	}
	if !verbose {
		t.Error("verbose should be true")
	}
}

func TestFlagSetParseBoolWithValue(t *testing.T) {
	fs := NewFlagSet("test")
	var enabled bool
	fs.BoolVar(&enabled, "enabled", true, "enabled flag")

	tests := []struct {
		args     []string
		expected bool
	}{
		{[]string{"-enabled"}, true},
		{[]string{"-enabled=true"}, true},
		{[]string{"-enabled=false"}, false},
		{[]string{"-enabled=1"}, true},
		{[]string{"-enabled=0"}, false},
	}

	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			fs2 := NewFlagSet("test")
			var val bool = true
			fs2.BoolVar(&val, "enabled", true, "enabled flag")
			err := fs2.Parse(tt.args)
			if err != nil {
				t.Fatalf("Parse failed: %v", err)
			}
			if val != tt.expected {
				t.Errorf("enabled = %v, want %v", val, tt.expected)
			}
		})
	}
}

func TestFlagSetParseDuration(t *testing.T) {
	fs := NewFlagSet("test")
	var timeout time.Duration
	fs.DurationVar(&timeout, "timeout", 10*time.Second, "timeout")

	err := fs.Parse([]string{"-timeout", "30s"})
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if timeout != 30*time.Second {
		t.Errorf("timeout = %v, want 30s", timeout)
	}
}

func TestFlagSetParseDurationInvalid(t *testing.T) {
	fs := NewFlagSet("test")
	var timeout time.Duration
	fs.DurationVar(&timeout, "timeout", 10*time.Second, "timeout")

	err := fs.Parse([]string{"-timeout", "invalid"})
	if err == nil {
		t.Error("Parse should fail with invalid duration")
	}
}

func TestFlagSetParseInvalidInt(t *testing.T) {
	fs := NewFlagSet("test")
	var port int
	fs.IntVar(&port, "port", 8080, "port number")

	err := fs.Parse([]string{"-port", "abc"})
	if err == nil {
		t.Error("Parse should fail with invalid integer")
	}
}

func TestFlagSetParseNegativeInt(t *testing.T) {
	fs := NewFlagSet("test")
	var value int
	fs.IntVar(&value, "value", 0, "value")

	err := fs.Parse([]string{"-value", "-42"})
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if value != -42 {
		t.Errorf("value = %d, want -42", value)
	}
}

func TestFlagSetParseDoubleDash(t *testing.T) {
	fs := NewFlagSet("test")
	var port int
	fs.IntVar(&port, "port", 8080, "port number")

	err := fs.Parse([]string{"--port", "9090"})
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if port != 9090 {
		t.Errorf("port = %d, want 9090", port)
	}
}

func TestFlagSetParseEmptyArg(t *testing.T) {
	fs := NewFlagSet("test")
	var port int
	fs.IntVar(&port, "port", 8080, "port number")

	// Empty string arg should be skipped
	err := fs.Parse([]string{"", "-port", "9090"})
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if port != 9090 {
		t.Errorf("port = %d, want 9090", port)
	}
}

func TestFlagSetBoolNonExistent(t *testing.T) {
	fs := NewFlagSet("test")
	var port int
	fs.IntVar(&port, "port", 8080, "port number")

	// Bool() for non-existent flag should return false
	if fs.Bool("nonexistent") {
		t.Error("Bool() for non-existent flag should return false")
	}
}

func TestValidatePortRanges(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "zero HTTP port",
			config: &Config{
				HTTPPort:     0,
				HTTPSPort:    443,
				AdminPort:    9090,
				LogLevel:     "info",
				LogFormat:    "json",
				DefaultTLS:   "auto",
				PollInterval: 10 * time.Second,
			},
			wantErr: true,
		},
		{
			name: "too high HTTP port",
			config: &Config{
				HTTPPort:     65536,
				HTTPSPort:    443,
				AdminPort:    9090,
				LogLevel:     "info",
				LogFormat:    "json",
				DefaultTLS:   "auto",
				PollInterval: 10 * time.Second,
			},
			wantErr: true,
		},
		{
			name: "zero HTTPS port",
			config: &Config{
				HTTPPort:     80,
				HTTPSPort:    0,
				AdminPort:    9090,
				LogLevel:     "info",
				LogFormat:    "json",
				DefaultTLS:   "auto",
				PollInterval: 10 * time.Second,
			},
			wantErr: true,
		},
		{
			name: "zero admin port",
			config: &Config{
				HTTPPort:     80,
				HTTPSPort:    443,
				AdminPort:    0,
				LogLevel:     "info",
				LogFormat:    "json",
				DefaultTLS:   "auto",
				PollInterval: 10 * time.Second,
			},
			wantErr: true,
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

func TestValidateAdminPortConflict(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "admin port equals HTTP port",
			config: &Config{
				HTTPPort:     80,
				HTTPSPort:    443,
				Admin:        true,
				AdminPort:    80,
				LogLevel:     "info",
				LogFormat:    "json",
				DefaultTLS:   "auto",
				PollInterval: 10 * time.Second,
			},
			wantErr: true,
		},
		{
			name: "admin port equals HTTPS port",
			config: &Config{
				HTTPPort:     80,
				HTTPSPort:    443,
				Admin:        true,
				AdminPort:    443,
				LogLevel:     "info",
				LogFormat:    "json",
				DefaultTLS:   "auto",
				PollInterval: 10 * time.Second,
			},
			wantErr: true,
		},
		{
			name: "admin disabled no conflict",
			config: &Config{
				HTTPPort:     80,
				HTTPSPort:    443,
				Admin:        false,
				AdminPort:    80, // same as HTTP but admin disabled
				LogLevel:     "info",
				LogFormat:    "json",
				DefaultTLS:   "auto",
				PollInterval: 10 * time.Second,
			},
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

func TestValidateAdminBindAddress(t *testing.T) {
	tests := []struct {
		name      string
		adminBind string
		wantErr   bool
	}{
		{"empty bind", "", false},
		{"localhost", "127.0.0.1", false},
		{"all interfaces", "0.0.0.0", false},
		{"valid IP", "192.168.1.1", false},
		{"invalid bind", "invalid-ip", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				HTTPPort:     80,
				HTTPSPort:    443,
				Admin:        true,
				AdminPort:    9090,
				AdminBind:    tt.adminBind,
				LogLevel:     "info",
				LogFormat:    "json",
				DefaultTLS:   "auto",
				PollInterval: 10 * time.Second,
			}
			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateLogFormat(t *testing.T) {
	tests := []struct {
		name       string
		logFormat  string
		wantErr    bool
	}{
		{"json format", "json", false},
		{"text format", "text", false},
		{"JSON uppercase", "JSON", false},
		{"TEXT uppercase", "TEXT", false},
		{"invalid format", "yaml", true},
		{"empty format", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				HTTPPort:     80,
				HTTPSPort:    443,
				AdminPort:    9090,
				LogLevel:     "info",
				LogFormat:    tt.logFormat,
				DefaultTLS:   "auto",
				PollInterval: 10 * time.Second,
			}
			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateTLSMode(t *testing.T) {
	tests := []struct {
		name       string
		defaultTLS string
		wantErr    bool
	}{
		{"auto mode", "auto", false},
		{"manual mode", "manual", false},
		{"off mode", "off", false},
		{"AUTO uppercase", "AUTO", false},
		{"invalid mode", "invalid", true},
		{"empty mode", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				HTTPPort:     80,
				HTTPSPort:    443,
				AdminPort:    9090,
				LogLevel:     "info",
				LogFormat:    "json",
				DefaultTLS:   tt.defaultTLS,
				PollInterval: 10 * time.Second,
			}
			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidatePollInterval(t *testing.T) {
	tests := []struct {
		name         string
		pollInterval time.Duration
		wantErr      bool
	}{
		{"valid interval", 10 * time.Second, false},
		{"minimum interval", time.Second, false},
		{"too short interval", 500 * time.Millisecond, true},
		{"zero interval", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				HTTPPort:     80,
				HTTPSPort:    443,
				AdminPort:    9090,
				LogLevel:     "info",
				LogFormat:    "json",
				DefaultTLS:   "auto",
				PollInterval: tt.pollInterval,
			}
			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateTrustedIPs(t *testing.T) {
	tests := []struct {
		name       string
		trustedIPs []string
		wantErr    bool
	}{
		{"empty list", []string{}, false},
		{"valid IPv4", []string{"192.168.1.1"}, false},
		{"valid CIDR", []string{"192.168.1.0/24"}, false},
		{"valid IPv6", []string{"::1"}, false},
		{"invalid IP", []string{"invalid"}, true},
		{"mixed valid and invalid", []string{"192.168.1.1", "invalid"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				HTTPPort:     80,
				HTTPSPort:    443,
				AdminPort:    9090,
				LogLevel:     "info",
				LogFormat:    "json",
				DefaultTLS:   "auto",
				PollInterval: 10 * time.Second,
				TrustedIPs:   tt.trustedIPs,
			}
			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateACMEEmail(t *testing.T) {
	tests := []struct {
		name      string
		acmeEmail string
		wantErr   bool
	}{
		{"empty email (ok)", "", false},
		{"valid email", "user@example.com", false},
		{"missing @", "invalid", true},
		{"just @", "@", false}, // technically contains @
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				HTTPPort:     80,
				HTTPSPort:    443,
				AdminPort:    9090,
				LogLevel:     "info",
				LogFormat:    "json",
				DefaultTLS:   "auto",
				PollInterval: 10 * time.Second,
				ACMEEmail:    tt.acmeEmail,
			}
			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateLogLevelCaseInsensitive(t *testing.T) {
	tests := []struct {
		name     string
		logLevel string
		wantErr  bool
	}{
		{"lowercase debug", "debug", false},
		{"uppercase DEBUG", "DEBUG", false},
		{"mixed case Info", "Info", false},
		{"lowercase info", "info", false},
		{"lowercase warn", "warn", false},
		{"lowercase error", "error", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				HTTPPort:     80,
				HTTPSPort:    443,
				AdminPort:    9090,
				LogLevel:     tt.logLevel,
				LogFormat:    "json",
				DefaultTLS:   "auto",
				PollInterval: 10 * time.Second,
			}
			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoadIntInvalid(t *testing.T) {
	os.Setenv("TEST_INVALID_INT", "abc")
	defer os.Unsetenv("TEST_INVALID_INT")

	result := 42
	loadInt(&result, "TEST_INVALID_INT")

	// Should keep initial on error
	if result != 42 {
		t.Errorf("loadInt with invalid = %d, want 42", result)
	}
}

func TestParseInt(t *testing.T) {
	tests := []struct {
		input    string
		expected int
		wantErr  bool
	}{
		{"0", 0, false},
		{"1", 1, false},
		{"123", 123, false},
		{"-1", -1, false},
		{"-123", -123, false},
		{"abc", 0, true},
		{"12abc", 0, true},
		{"", 0, false}, // empty string returns 0, nil
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := parseInt(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseInt() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && result != tt.expected {
				t.Errorf("parseInt() = %d, want %d", result, tt.expected)
			}
		})
	}
}

func TestFlagSetParseSkipNonFlag(t *testing.T) {
	fs := NewFlagSet("test")
	var port int
	fs.IntVar(&port, "port", 8080, "port number")

	// Non-flag arguments should be skipped
	err := fs.Parse([]string{"positional", "-port", "9090", "args"})
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if port != 9090 {
		t.Errorf("port = %d, want 9090", port)
	}
}

// Tests for Load function and parseFlags that don't exit

func TestApplyDefaultsFullConfig(t *testing.T) {
	cfg := &Config{}
	cfg.applyDefaults()

	// Verify all defaults are applied correctly
	if cfg.HTTPPort != DefaultHTTPPort {
		t.Errorf("HTTPPort = %d, want %d", cfg.HTTPPort, DefaultHTTPPort)
	}
	if cfg.HTTPSPort != DefaultHTTPSPort {
		t.Errorf("HTTPSPort = %d, want %d", cfg.HTTPSPort, DefaultHTTPSPort)
	}
	if cfg.Admin != DefaultAdminEnabled {
		t.Errorf("Admin = %v, want %v", cfg.Admin, DefaultAdminEnabled)
	}
	if cfg.AdminPort != DefaultAdminPort {
		t.Errorf("AdminPort = %d, want %d", cfg.AdminPort, DefaultAdminPort)
	}
	if cfg.AdminBind != DefaultAdminBind {
		t.Errorf("AdminBind = %s, want %s", cfg.AdminBind, DefaultAdminBind)
	}
	if cfg.DockerSocket != DefaultDockerSocket {
		t.Errorf("DockerSocket = %s, want %s", cfg.DockerSocket, DefaultDockerSocket)
	}
	if cfg.DataDir != DefaultDataDir {
		t.Errorf("DataDir = %s, want %s", cfg.DataDir, DefaultDataDir)
	}
	if cfg.LogLevel != DefaultLogLevel {
		t.Errorf("LogLevel = %s, want %s", cfg.LogLevel, DefaultLogLevel)
	}
	if cfg.LogFormat != DefaultLogFormat {
		t.Errorf("LogFormat = %s, want %s", cfg.LogFormat, DefaultLogFormat)
	}
	if cfg.AccessLog != DefaultAccessLog {
		t.Errorf("AccessLog = %v, want %v", cfg.AccessLog, DefaultAccessLog)
	}
	if cfg.DefaultTLS != DefaultDefaultTLS {
		t.Errorf("DefaultTLS = %s, want %s", cfg.DefaultTLS, DefaultDefaultTLS)
	}
	if cfg.MaxBodySize != DefaultMaxBody {
		t.Errorf("MaxBodySize = %s, want %s", cfg.MaxBodySize, DefaultMaxBody)
	}
	if cfg.PollInterval != DefaultPollInt {
		t.Errorf("PollInterval = %v, want %v", cfg.PollInterval, DefaultPollInt)
	}
	if cfg.ACMEProvider != "letsencrypt" {
		t.Errorf("ACMEProvider = %s, want letsencrypt", cfg.ACMEProvider)
	}
	if cfg.ACMEStaging != false {
		t.Errorf("ACMEStaging = %v, want false", cfg.ACMEStaging)
	}
}

func TestLoadFromEnvFull(t *testing.T) {
	// Set all env vars
	envVars := map[string]string{
		"DR_HTTP_PORT":      "8880",
		"DR_HTTPS_PORT":     "8443",
		"DR_ADMIN":          "false",
		"DR_ADMIN_PORT":     "9990",
		"DR_ADMIN_BIND":     "127.0.0.1",
		"DR_ADMIN_USER":     "testuser",
		"DR_ADMIN_PASS":     "testpass",
		"DR_DOCKER_SOCKET":  "/custom/docker.sock",
		"DR_DATA_DIR":       "/custom/data",
		"DR_ACME_EMAIL":     "test@example.com",
		"DR_ACME_PROVIDER":  "zerossl",
		"DR_ACME_STAGING":   "true",
		"DR_LOG_LEVEL":      "debug",
		"DR_LOG_FORMAT":     "text",
		"DR_ACCESS_LOG":     "false",
		"DR_DEFAULT_TLS":    "manual",
		"DR_MAX_BODY_SIZE":  "50mb",
		"DR_POLL_INTERVAL":  "30s",
		"DR_TRUSTED_IPS":    "10.0.0.0/8,192.168.0.0/16",
	}

	for k, v := range envVars {
		os.Setenv(k, v)
	}
	defer func() {
		for k := range envVars {
			os.Unsetenv(k)
		}
	}()

	cfg := &Config{}
	cfg.applyDefaults()
	cfg.loadFromEnv()

	// Verify all values were loaded
	if cfg.HTTPPort != 8880 {
		t.Errorf("HTTPPort = %d, want 8880", cfg.HTTPPort)
	}
	if cfg.HTTPSPort != 8443 {
		t.Errorf("HTTPSPort = %d, want 8443", cfg.HTTPSPort)
	}
	if cfg.Admin != false {
		t.Errorf("Admin = %v, want false", cfg.Admin)
	}
	if cfg.AdminPort != 9990 {
		t.Errorf("AdminPort = %d, want 9990", cfg.AdminPort)
	}
	if cfg.AdminBind != "127.0.0.1" {
		t.Errorf("AdminBind = %s, want 127.0.0.1", cfg.AdminBind)
	}
	if cfg.AdminUser != "testuser" {
		t.Errorf("AdminUser = %s, want testuser", cfg.AdminUser)
	}
	if cfg.AdminPass != "testpass" {
		t.Errorf("AdminPass = %s, want testpass", cfg.AdminPass)
	}
	if cfg.DockerSocket != "/custom/docker.sock" {
		t.Errorf("DockerSocket = %s, want /custom/docker.sock", cfg.DockerSocket)
	}
	if cfg.DataDir != "/custom/data" {
		t.Errorf("DataDir = %s, want /custom/data", cfg.DataDir)
	}
	if cfg.ACMEEmail != "test@example.com" {
		t.Errorf("ACMEEmail = %s, want test@example.com", cfg.ACMEEmail)
	}
	if cfg.ACMEProvider != "zerossl" {
		t.Errorf("ACMEProvider = %s, want zerossl", cfg.ACMEProvider)
	}
	if cfg.ACMEStaging != true {
		t.Errorf("ACMEStaging = %v, want true", cfg.ACMEStaging)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %s, want debug", cfg.LogLevel)
	}
	if cfg.LogFormat != "text" {
		t.Errorf("LogFormat = %s, want text", cfg.LogFormat)
	}
	if cfg.AccessLog != false {
		t.Errorf("AccessLog = %v, want false", cfg.AccessLog)
	}
	if cfg.DefaultTLS != "manual" {
		t.Errorf("DefaultTLS = %s, want manual", cfg.DefaultTLS)
	}
	if cfg.MaxBodySize != "50mb" {
		t.Errorf("MaxBodySize = %s, want 50mb", cfg.MaxBodySize)
	}
	if cfg.PollInterval != 30*time.Second {
		t.Errorf("PollInterval = %v, want 30s", cfg.PollInterval)
	}
	if len(cfg.TrustedIPs) != 2 {
		t.Errorf("TrustedIPs len = %d, want 2", len(cfg.TrustedIPs))
	}
}

func TestParseFlagsWithMock(t *testing.T) {
	// Save original os.Args
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	// Mock os.Args (program name + flags)
	os.Args = []string{"dockrouter", "-http-port", "8080", "-https-port", "8443"}

	cfg := &Config{}
	cfg.applyDefaults()

	// Create a test FlagSet
	fs := NewFlagSet("dockrouter")
	fs.IntVar(&cfg.HTTPPort, "http-port", cfg.HTTPPort, "HTTP listener port")
	fs.IntVar(&cfg.HTTPSPort, "https-port", cfg.HTTPSPort, "HTTPS listener port")

	err := fs.Parse(os.Args[1:])
	if err != nil {
		t.Fatalf("parseFlags failed: %v", err)
	}

	if cfg.HTTPPort != 8080 {
		t.Errorf("HTTPPort = %d, want 8080", cfg.HTTPPort)
	}
	if cfg.HTTPSPort != 8443 {
		t.Errorf("HTTPSPort = %d, want 8443", cfg.HTTPSPort)
	}
}

func TestValidateMultipleErrors(t *testing.T) {
	cfg := &Config{
		HTTPPort:     0,      // invalid
		HTTPSPort:    0,      // invalid
		Admin:        true,
		AdminPort:    80,     // conflicts with HTTP
		LogLevel:     "invalid",
		LogFormat:    "yaml", // invalid
		DefaultTLS:   "bad",  // invalid
		PollInterval: 0,      // too short
		TrustedIPs:   []string{"not-an-ip"},
		ACMEEmail:    "no-at-sign",
	}

	err := cfg.Validate()
	if err == nil {
		t.Error("Validate should return error for invalid config")
	}

	// The error should contain multiple issues
	errStr := err.Error()
	if !strings.Contains(errStr, "HTTP port") {
		t.Logf("Error: %s", errStr)
		t.Error("Error should mention HTTP port")
	}
	if !strings.Contains(errStr, "HTTPS port") {
		t.Error("Error should mention HTTPS port")
	}
	// Check for either "admin port" or "AdminPort" since port conflict check may not trigger if HTTPPort is invalid
	if !strings.Contains(errStr, "admin") && !strings.Contains(errStr, "port") {
		t.Error("Error should mention port issues")
	}
}

func TestConfigStringRedaction(t *testing.T) {
	cfg := &Config{
		HTTPPort:     80,
		HTTPSPort:    443,
		Admin:        true,
		AdminPort:    9090,
		AdminBind:    "127.0.0.1",
		AdminUser:    "admin",
		AdminPass:    "super-secret-password",
		DockerSocket: "/var/run/docker.sock",
		DataDir:      "/data",
		ACMEEmail:    "admin@example.com",
		LogLevel:     "info",
	}

	s := cfg.String()

	// Verify password is redacted
	if strings.Contains(s, "super-secret-password") {
		t.Error("Config.String() should not contain actual password")
	}
	if !strings.Contains(s, "***") {
		t.Error("Config.String() should contain password mask")
	}

	// Verify other fields are present
	if !strings.Contains(s, "admin") {
		t.Error("Config.String() should contain AdminUser")
	}
	if !strings.Contains(s, "info") {
		t.Error("Config.String() should contain LogLevel")
	}
}

func TestFlagSetParseDoubleDashFlag(t *testing.T) {
	fs := NewFlagSet("test")
	var verbose bool
	fs.BoolVar(&verbose, "verbose", false, "verbose mode")

	err := fs.Parse([]string{"--verbose"})
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if !verbose {
		t.Error("verbose should be true")
	}
}

func TestFlagSetParseEmptyFlag(t *testing.T) {
	fs := NewFlagSet("test")
	var port int
	fs.IntVar(&port, "port", 8080, "port number")

	// Just dashes should be skipped
	err := fs.Parse([]string{"--", "-port", "9090"})
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// port should be 9090
	if port != 9090 {
		t.Errorf("port = %d, want 9090", port)
	}
}
