package discovery

import (
	"context"
	"testing"
	"time"
)

// --- parseBool/parseBoolDefault/parseInt/parseDuration edge cases ---

func TestParseBoolVariants(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"true", true},
		{"TRUE", true},
		{"True", true},
		{"1", true},
		{"false", false},
		{"0", false},
		{"", false},
		{"yes", false},
	}
	for _, tt := range tests {
		result := parseBool(tt.input)
		if result != tt.expected {
			t.Errorf("parseBool(%q) = %v, want %v", tt.input, result, tt.expected)
		}
	}
}

func TestParseBoolDefaultEdges(t *testing.T) {
	if !parseBoolDefault("", true) {
		t.Error("parseBoolDefault('', true) should return true")
	}
	if parseBoolDefault("", false) {
		t.Error("parseBoolDefault('', false) should return false")
	}
	if !parseBoolDefault("true", false) {
		t.Error("parseBoolDefault('true', false) should return true")
	}
	if parseBoolDefault("false", true) {
		t.Error("parseBoolDefault('false', true) should return false")
	}
}

func TestParseIntEdges(t *testing.T) {
	tests := []struct {
		input    string
		def      int
		expected int
	}{
		{"", 42, 42},
		{"notanumber", 99, 99},
		{"0", 10, 0},
		{"-1", 10, -1},
		{"99999", 0, 99999},
	}
	for _, tt := range tests {
		result := parseInt(tt.input, tt.def)
		if result != tt.expected {
			t.Errorf("parseInt(%q, %d) = %d, want %d", tt.input, tt.def, result, tt.expected)
		}
	}
}

func TestParseDurationEdges(t *testing.T) {
	def := 5 * time.Second
	if parseDuration("", def) != def {
		t.Error("empty string should return default")
	}
	if parseDuration("notaduration", def) != def {
		t.Error("invalid string should return default")
	}
	if parseDuration("1s", def) != time.Second {
		t.Error("1s should parse to 1 second")
	}
	if parseDuration("500ms", def) != 500*time.Millisecond {
		t.Error("500ms should parse correctly")
	}
}

// --- parseSize edge cases ---

func TestParseSizeEdgeCases(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		{"abc123mb", 0},
		{"0mb", 0},
		{"0", 0},
		{" 10mb ", 10 * 1024 * 1024},
		{"10MB", 10 * 1024 * 1024},
		{"5GB", 5 * 1024 * 1024 * 1024},
		{"100KB", 100 * 1024},
		{"999B", 999},
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

// --- parseRateLimit malformed input ---

func TestParseRateLimitMalformed(t *testing.T) {
	tests := []struct {
		name   string
		labels map[string]string
	}{
		{
			"no slash",
			map[string]string{"dr.enable": "true", "dr.host": "example.com", "dr.ratelimit": "100"},
		},
		{
			"extra slashes",
			map[string]string{"dr.enable": "true", "dr.host": "example.com", "dr.ratelimit": "100/m/extra"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := ParseLabels(tt.labels)
			if config == nil {
				t.Fatal("nil")
			}
			if !config.RateLimit.Enabled {
				t.Error("RateLimit should be enabled")
			}
		})
	}
}

// --- Health check defaults ---

func TestParseLabelsHealthCheckAllDefaults(t *testing.T) {
	config := ParseLabels(map[string]string{"dr.enable": "true", "dr.host": "x.com"})
	if config.HealthCheck.Path != "/" {
		t.Errorf("Default path = %s", config.HealthCheck.Path)
	}
	if config.HealthCheck.Interval != 10*time.Second {
		t.Errorf("Default interval = %v", config.HealthCheck.Interval)
	}
	if config.HealthCheck.Timeout != 5*time.Second {
		t.Errorf("Default timeout = %v", config.HealthCheck.Timeout)
	}
	if config.HealthCheck.Threshold != 3 {
		t.Errorf("Default threshold = %d", config.HealthCheck.Threshold)
	}
	if config.HealthCheck.Recovery != 2 {
		t.Errorf("Default recovery = %d", config.HealthCheck.Recovery)
	}
}

func TestParseLabelsAllHealthCheckFieldsSet(t *testing.T) {
	labels := map[string]string{
		"dr.enable":                "true",
		"dr.host":                  "x.com",
		"dr.healthcheck.path":      "/healthz",
		"dr.healthcheck.interval":  "30s",
		"dr.healthcheck.timeout":   "10s",
		"dr.healthcheck.threshold": "5",
		"dr.healthcheck.recovery":  "3",
	}
	config := ParseLabels(labels)
	if config.HealthCheck.Path != "/healthz" {
		t.Errorf("Path = %s", config.HealthCheck.Path)
	}
	if config.HealthCheck.Interval != 30*time.Second {
		t.Errorf("Interval = %v", config.HealthCheck.Interval)
	}
	if config.HealthCheck.Timeout != 10*time.Second {
		t.Errorf("Timeout = %v", config.HealthCheck.Timeout)
	}
	if config.HealthCheck.Threshold != 5 {
		t.Errorf("Threshold = %d", config.HealthCheck.Threshold)
	}
	if config.HealthCheck.Recovery != 3 {
		t.Errorf("Recovery = %d", config.HealthCheck.Recovery)
	}
}

// --- Default values ---

func TestParseLabelsDefaultLoadBalanceValue(t *testing.T) {
	config := ParseLabels(map[string]string{"dr.enable": "true", "dr.host": "x.com"})
	if config.LoadBalance != "roundrobin" {
		t.Errorf("Default LoadBalance = %s", config.LoadBalance)
	}
}

func TestParseLabelsDefaultTLSValue(t *testing.T) {
	config := ParseLabels(map[string]string{"dr.enable": "true", "dr.host": "x.com"})
	if config.TLS != "auto" {
		t.Errorf("Default TLS = %s", config.TLS)
	}
}

func TestParseLabelsDefaultWeightValue(t *testing.T) {
	config := ParseLabels(map[string]string{"dr.enable": "true", "dr.host": "x.com"})
	if config.Weight != 1 {
		t.Errorf("Default Weight = %d", config.Weight)
	}
}

// --- parseIPNetworks ---

func TestParseIPNetworksEdges(t *testing.T) {
	// IPv6 single
	if n := parseIPNetworks("::1"); len(n) != 1 {
		t.Errorf("IPv6 single: got %d", len(n))
	}
	// Mixed valid/invalid
	if n := parseIPNetworks("192.168.1.0/24,garbage,10.0.0.1"); len(n) != 2 {
		t.Errorf("Mixed: got %d", len(n))
	}
	// All invalid
	if n := parseIPNetworks("not-an-ip,also-bad"); len(n) != 0 {
		t.Errorf("All invalid: got %d", len(n))
	}
	// Whitespace
	if n := parseIPNetworks(" 192.168.1.0/24 , 10.0.0.0/8 "); len(n) != 2 {
		t.Errorf("Whitespace: got %d", len(n))
	}
}

// --- parseCircuitBreaker malformed ---

func TestParseCircuitBreakerMalformedInput(t *testing.T) {
	config := ParseLabels(map[string]string{
		"dr.enable":         "true",
		"dr.host":           "x.com",
		"dr.circuitbreaker": "noslash",
	})
	if !config.CircuitBreaker.Enabled {
		t.Error("should be enabled")
	}
}

// --- Full config test ---

func TestParseLabelsEveryField(t *testing.T) {
	labels := map[string]string{
		"dr.enable":                "true",
		"dr.host":                  "full.example.com",
		"dr.port":                  "3000",
		"dr.path":                  "/api",
		"dr.priority":              "10",
		"dr.address":               "10.0.0.5:3000",
		"dr.loadbalancer":          "weighted",
		"dr.weight":                "5",
		"dr.tls":                   "manual",
		"dr.tls.domains":           "a.com,b.com",
		"dr.tls.cert":              "/certs/cert.pem",
		"dr.tls.key":               "/certs/key.pem",
		"dr.ratelimit":             "100/m",
		"dr.ratelimit.by":          "X-API-Key",
		"dr.cors.origins":          "https://app.com",
		"dr.cors.methods":          "GET,POST",
		"dr.cors.headers":          "Authorization,X-Custom",
		"dr.compress":              "true",
		"dr.redirect.https":        "false",
		"dr.stripprefix":           "/api",
		"dr.addprefix":             "/v2",
		"dr.maxbody":               "5mb",
		"dr.auth.basic.users":      "admin:hash1,user:hash2",
		"dr.ipwhitelist":           "192.168.0.0/16",
		"dr.ipblacklist":           "10.0.0.1",
		"dr.retry":                 "3",
		"dr.circuitbreaker":        "5/30s",
		"dr.middlewares":            "ratelimit, compress, cors",
		"dr.healthcheck.path":      "/health",
		"dr.healthcheck.interval":  "15s",
		"dr.healthcheck.timeout":   "3s",
		"dr.healthcheck.threshold": "5",
		"dr.healthcheck.recovery":  "2",
	}

	c := ParseLabels(labels)
	if c.Host != "full.example.com" || c.Port != 3000 || c.Path != "/api" || c.Priority != 10 {
		t.Error("basic fields wrong")
	}
	if c.Address != "10.0.0.5:3000" || c.LoadBalance != "weighted" || c.Weight != 5 {
		t.Error("routing fields wrong")
	}
	if c.TLS != "manual" || len(c.TLSDomains) != 2 || c.TLSCertFile != "/certs/cert.pem" {
		t.Error("tls fields wrong")
	}
	if !c.RateLimit.Enabled || c.RateLimit.Count != 100 || c.RateLimit.ByKey != "X-API-Key" {
		t.Error("ratelimit wrong")
	}
	if !c.CORS.Enabled || len(c.CORS.Origins) != 1 || len(c.CORS.Methods) != 2 || len(c.CORS.Headers) != 2 {
		t.Error("cors wrong")
	}
	if !c.Compress || c.RedirectHTTPS || c.StripPrefix != "/api" || c.AddPrefix != "/v2" {
		t.Error("middleware fields wrong")
	}
	if c.MaxBody != 5*1024*1024 || len(c.BasicAuthUsers) != 2 {
		t.Error("body/auth wrong")
	}
	if len(c.IPWhitelist) != 1 || len(c.IPBlacklist) != 1 || c.Retry != 3 {
		t.Error("ip/retry wrong")
	}
	if !c.CircuitBreaker.Enabled || c.CircuitBreaker.Failures != 5 {
		t.Error("circuit breaker wrong")
	}
	if len(c.Middlewares) != 3 || c.Middlewares[1] != "compress" {
		t.Error("middlewares wrong")
	}
}

// --- New Docker client edge case tests ---

func TestNewDockerClientCustomSocket(t *testing.T) {
	client, err := NewDockerClient("/custom/docker.sock")
	if err != nil {
		t.Fatal(err)
	}
	if client.socketPath != "/custom/docker.sock" {
		t.Errorf("socketPath = %s", client.socketPath)
	}
	if client.timeout != 30*time.Second {
		t.Errorf("default timeout = %v", client.timeout)
	}
}

func TestListAllContainersError(t *testing.T) {
	client, _ := NewDockerClient("/nonexistent/docker.sock")
	_, err := client.ListAllContainers(context.Background())
	if err == nil {
		t.Error("should fail")
	}
}

// --- onContainerStop nonexistent ---

func TestOnContainerStopNonexistent(t *testing.T) {
	logger := &mockLogger{}
	sink := newMockRouteSink()
	engine := &Engine{
		routes:     sink,
		logger:     logger,
		containers: make(map[string]*ContainerInfo),
	}

	engine.onContainerStop("nonexistent-id-0123456789012345678901234567890123456789")
	if len(sink.removed) != 0 {
		t.Error("should not remove nonexistent")
	}
}

// --- GetContainers empty ---

func TestEngineGetContainersEmptyMap(t *testing.T) {
	engine := &Engine{
		containers: make(map[string]*ContainerInfo),
	}
	result := engine.GetContainers()
	if result == nil {
		t.Error("should return non-nil empty slice")
	}
	if len(result) != 0 {
		t.Errorf("should be empty, got %d", len(result))
	}
}

// --- BuildContainerInfo: unhealthy (not running) ---

func TestBuildContainerInfoNotRunning(t *testing.T) {
	engine := &Engine{
		routes:     newMockRouteSink(),
		logger:     &mockLogger{},
		containers: make(map[string]*ContainerInfo),
	}

	detail := &ContainerDetail{
		ID:   "abc123",
		Name: "/test",
		State: ContainerState{
			Running: false,
			Healthy: false,
		},
		Network: ContainerNetwork{IPAddress: "172.17.0.5"},
	}

	info := engine.buildContainerInfo(
		Container{ID: "abc123", Names: []string{"/test"}},
		detail,
		&RouteConfig{Enabled: true, Host: "x.com"},
	)

	if info.Healthy {
		t.Error("should not be healthy when not running")
	}
}

// --- BuildContainerInfo: healthy=true ---

func TestBuildContainerInfoHealthyTrue(t *testing.T) {
	engine := &Engine{
		routes:     newMockRouteSink(),
		logger:     &mockLogger{},
		containers: make(map[string]*ContainerInfo),
	}

	detail := &ContainerDetail{
		ID:   "abc123",
		Name: "/test",
		State: ContainerState{
			Running: true,
			Healthy: true,
		},
		Network: ContainerNetwork{IPAddress: "172.17.0.5"},
	}

	info := engine.buildContainerInfo(
		Container{ID: "abc123", Names: []string{"/test"}},
		detail,
		&RouteConfig{Enabled: true, Host: "x.com"},
	)

	if !info.Healthy {
		t.Error("should be healthy")
	}
}

// --- BuildContainerInfo: explicit port ---

func TestBuildContainerInfoExplicitPort(t *testing.T) {
	engine := &Engine{
		routes:     newMockRouteSink(),
		logger:     &mockLogger{},
		containers: make(map[string]*ContainerInfo),
	}

	detail := &ContainerDetail{
		ID:      "abc123",
		Name:    "/test",
		State:   ContainerState{Running: true},
		Network: ContainerNetwork{IPAddress: "172.17.0.5"},
	}

	info := engine.buildContainerInfo(
		Container{ID: "abc123", Names: []string{"/test"}},
		detail,
		&RouteConfig{Enabled: true, Host: "x.com", Port: 9090},
	)

	if info.Address != "172.17.0.5:9090" {
		t.Errorf("Address = %s, want 172.17.0.5:9090", info.Address)
	}
}

// --- GetContainerName/GetContainerImage with attributes ---

func TestGetContainerNameWithAttributes(t *testing.T) {
	event := Event{
		Actor: EventActor{
			Attributes: map[string]string{"name": "my-app"},
		},
	}
	if name := GetContainerName(event); name != "my-app" {
		t.Errorf("got %q", name)
	}
}

func TestGetContainerImageAttr(t *testing.T) {
	event := Event{
		Actor: EventActor{
			Attributes: map[string]string{"image": "nginx:latest"},
		},
	}
	if img := GetContainerImage(event); img != "nginx:latest" {
		t.Errorf("got %q", img)
	}
}

func TestGetContainerImageNilAttr(t *testing.T) {
	event := Event{Actor: EventActor{Attributes: nil}}
	if img := GetContainerImage(event); img != "" {
		t.Errorf("got %q", img)
	}
}
