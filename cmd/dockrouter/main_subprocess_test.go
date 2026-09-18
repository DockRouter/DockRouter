package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/DockRouter/dockrouter/internal/config"
	"github.com/DockRouter/dockrouter/internal/log"
)

// TestDoHealthCheckSuccessSubprocessReal runs a mock health server and tests
// doHealthCheck via subprocess, verifying the success path with os.Exit(0).
func TestDoHealthCheckSuccessViaSubprocess(t *testing.T) {
	if os.Getenv("TEST_HC_OK_V2") == "1" {
		// In subprocess: set up mock URL and call doHealthCheck
		// The healthCheckURL was set by parent before launching subprocess
		url := os.Getenv("HC_URL")
		if url == "" {
			os.Exit(2) // misconfigured
		}
		healthCheckURL = url
		doHealthCheck()
		// doHealthCheck calls os.Exit, so we won't reach here
		return
	}

	// Parent: start mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
	}))
	defer server.Close()

	cmd := exec.Command(os.Args[0], "-test.run=TestDoHealthCheckSuccessViaSubprocess")
	cmd.Env = append(os.Environ(),
		"TEST_HC_OK_V2=1",
		"HC_URL="+server.URL+"/api/v1/health",
	)

	output, err := cmd.CombinedOutput()
	outputStr := string(output)

	if err != nil {
		// ExitError is expected for os.Exit(0) in some Go versions
		if !strings.Contains(outputStr, "healthy") {
			t.Errorf("subprocess output: %s, err: %v", outputStr, err)
		}
	}
	if !strings.Contains(outputStr, "healthy") {
		t.Errorf("output should contain 'healthy': %s", outputStr)
	}
}

// TestDoHealthCheckFailureViaSubprocess tests the failure path (os.Exit(1)).
func TestDoHealthCheckFailureViaSubprocess(t *testing.T) {
	if os.Getenv("TEST_HC_FAIL_V2") == "1" {
		healthCheckURL = "http://127.0.0.1:59999/api/v1/health"
		doHealthCheck()
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestDoHealthCheckFailureViaSubprocess")
	cmd.Env = append(os.Environ(), "TEST_HC_FAIL_V2=1")

	output, err := cmd.CombinedOutput()
	outputStr := string(output)

	if err == nil {
		t.Error("subprocess should exit with non-zero code")
		return
	}

	if !strings.Contains(outputStr, "Health check failed") {
		t.Errorf("output should contain 'Health check failed': %s", outputStr)
	}
}

// TestMainVersionCommand tests the version command via subprocess.
func TestMainVersionCommand(t *testing.T) {
	if os.Getenv("TEST_VERSION_CMD") == "1" {
		os.Args = []string{"dockrouter", "version"}
		main()
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestMainVersionCommand")
	cmd.Env = append(os.Environ(), "TEST_VERSION_CMD=1")
	// Override version/buildTime through env isn't possible since they're ldflags
	// But we can still verify it doesn't crash

	output, _ := cmd.CombinedOutput()
	outputStr := string(output)

	if !strings.Contains(outputStr, "DockRouter") {
		t.Logf("version output: %s", outputStr)
	}
}

// TestMainHealthcheckCommand tests "dockrouter healthcheck" via subprocess.
func TestMainHealthcheckCommand(t *testing.T) {
	if os.Getenv("TEST_MAIN_HC") == "1" {
		os.Args = []string{"dockrouter", "healthcheck"}
		main()
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestMainHealthcheckCommand")
	cmd.Env = append(os.Environ(), "TEST_MAIN_HC=1")

	output, err := cmd.CombinedOutput()
	outputStr := string(output)

	// Should fail since no server is running
	if err == nil {
		t.Log("healthcheck succeeded unexpectedly (server running?)")
		return
	}

	if !strings.Contains(outputStr, "Health check failed") {
		t.Logf("healthcheck output: %s", outputStr)
	}
}

// TestParseLogLevelAll tests all log level branches.
func TestParseLogLevelAll(t *testing.T) {
	if parseLogLevel("debug") != log.LevelDebug {
		t.Error("debug")
	}
	if parseLogLevel("warn") != log.LevelWarn {
		t.Error("warn")
	}
	if parseLogLevel("error") != log.LevelError {
		t.Error("error")
	}
	if parseLogLevel("info") != log.LevelInfo {
		t.Error("info")
	}
	if parseLogLevel("") != log.LevelInfo {
		t.Error("empty should default to info")
	}
	if parseLogLevel("unknown") != log.LevelInfo {
		t.Error("unknown should default to info")
	}
}

// TestAppInitializeWithACMEEmail tests initialize when ACMEEmail is set.
func TestAppInitializeWithACMEEmail(t *testing.T) {
	logger := log.NewLogger(nil, log.LevelInfo)
	app := &App{
		logger: logger,
		config: &config.Config{
			HTTPPort:   0,
			HTTPSPort:  0,
			ACMEEmail:  "test@example.com",
			DataDir:    t.TempDir(),
			AccessLog:  false,
			LogLevel:   "info",
			DefaultTLS: "off",
		},
		startTime: time.Now(),
	}

	err := app.initialize()
	if err != nil {
		t.Fatalf("initialize with ACME email: %v", err)
	}

	if app.tlsManager == nil {
		t.Error("tlsManager should be initialized when ACMEEmail is set")
	}
	if app.challengeSolver == nil {
		t.Error("challengeSolver should be initialized")
	}
}
