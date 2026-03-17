package health

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestNewChecker(t *testing.T) {
	checker := NewChecker(10*time.Second, 5*time.Second)
	if checker == nil {
		t.Fatal("NewChecker returned nil")
	}
	if checker.interval != 10*time.Second {
		t.Errorf("Expected interval 10s, got %v", checker.interval)
	}
	if checker.timeout != 5*time.Second {
		t.Errorf("Expected timeout 5s, got %v", checker.timeout)
	}
}

func TestRegisterUnregister(t *testing.T) {
	checker := NewChecker(10*time.Second, 5*time.Second)

	check := HealthCheck{
		Target:    "localhost:8080",
		Path:      "/health",
		Interval:  10 * time.Second,
		Timeout:   5 * time.Second,
		Threshold: 3,
	}

	// Register
	checker.Register("localhost:8080", check)

	// Verify registered
	state := checker.GetState("localhost:8080")
	if state != StateUnknown {
		t.Errorf("Expected StateUnknown for new check, got %v", state)
	}

	// Unregister
	checker.Unregister("localhost:8080")

	// Verify unregistered
	state = checker.GetState("localhost:8080")
	if state != StateUnknown {
		t.Errorf("Expected StateUnknown after unregister, got %v", state)
	}
}

func TestGetStateNonExistent(t *testing.T) {
	checker := NewChecker(10*time.Second, 5*time.Second)
	state := checker.GetState("nonexistent:8080")
	if state != StateUnknown {
		t.Errorf("Expected StateUnknown for non-existent target, got %v", state)
	}
}

func TestHealthStateString(t *testing.T) {
	tests := []struct {
		state    HealthState
		expected string
	}{
		{StateUnknown, "unknown"},
		{StateHealthy, "healthy"},
		{StateDegraded, "degraded"},
		{StateUnhealthy, "unhealthy"},
		{StateRecovering, "recovering"},
		{HealthState(99), "unknown"},
	}

	for _, tt := range tests {
		result := tt.state.String()
		if result != tt.expected {
			t.Errorf("HealthState(%d).String() = %s, want %s", tt.state, result, tt.expected)
		}
	}
}

func TestHTTPCheck(t *testing.T) {
	// Start test server
	server := &http.Server{
		Addr: ":18080",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/health" {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("OK"))
			} else if r.URL.Path == "/unhealthy" {
				w.WriteHeader(http.StatusInternalServerError)
			} else {
				w.WriteHeader(http.StatusNotFound)
			}
		}),
	}

	ln, err := net.Listen("tcp", ":18080")
	if err != nil {
		t.Skipf("Cannot start test server: %v", err)
	}

	go server.Serve(ln)
	defer server.Shutdown(context.Background())

	time.Sleep(100 * time.Millisecond) // Wait for server to start

	// Test healthy endpoint
	healthy, err := HTTPCheck("localhost:18080", "/health", 2*time.Second)
	if err != nil {
		t.Fatalf("HTTPCheck failed: %v", err)
	}
	if !healthy {
		t.Error("Expected healthy=true for /health endpoint")
	}

	// Test unhealthy endpoint
	healthy, err = HTTPCheck("localhost:18080", "/unhealthy", 2*time.Second)
	if err != nil {
		t.Fatalf("HTTPCheck failed: %v", err)
	}
	if healthy {
		t.Error("Expected healthy=false for /unhealthy endpoint")
	}

	// Test non-existent endpoint (404)
	healthy, err = HTTPCheck("localhost:18080", "/nonexistent", 2*time.Second)
	if err != nil {
		t.Fatalf("HTTPCheck failed: %v", err)
	}
	if healthy {
		t.Error("Expected healthy=false for 404 response")
	}
}

func TestHTTPCheckTimeout(t *testing.T) {
	// Start slow server
	server := &http.Server{
		Addr: ":18081",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(2 * time.Second) // Delay
			w.WriteHeader(http.StatusOK)
		}),
	}

	ln, err := net.Listen("tcp", ":18081")
	if err != nil {
		t.Skipf("Cannot start test server: %v", err)
	}

	go server.Serve(ln)
	defer server.Shutdown(context.Background())

	time.Sleep(100 * time.Millisecond)

	// Test with short timeout
	healthy, err := HTTPCheck("localhost:18081", "/slow", 100*time.Millisecond)
	if err == nil {
		t.Error("Expected timeout error")
	}
	if healthy {
		t.Error("Expected healthy=false on timeout")
	}
}

func TestHTTPCheckConnectionRefused(t *testing.T) {
	healthy, err := HTTPCheck("localhost:59999", "/health", 1*time.Second)
	if err == nil {
		t.Error("Expected connection refused error")
	}
	if healthy {
		t.Error("Expected healthy=false on connection refused")
	}
}

func TestHTTPCheckInvalidURL(t *testing.T) {
	// Test with control character in URL that causes request creation to fail
	// \x00 is not valid in URL
	healthy, err := HTTPCheck("localhost\x00invalid", "/health", 1*time.Second)
	if err == nil {
		t.Error("Expected error for invalid URL")
	}
	if healthy {
		t.Error("Expected healthy=false for invalid URL")
	}
}

func TestCheckOneStateTransitions(t *testing.T) {
	checker := NewChecker(10*time.Second, 5*time.Second)

	// Start test server
	successCount := 0
	server := &http.Server{
		Addr: ":18082",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			successCount++
			if successCount <= 3 {
				// First 3 requests fail
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
		}),
	}

	ln, err := net.Listen("tcp", ":18082")
	if err != nil {
		t.Skipf("Cannot start test server: %v", err)
	}

	go server.Serve(ln)
	defer server.Shutdown(context.Background())
	time.Sleep(100 * time.Millisecond)

	// Register health check
	check := HealthCheck{
		Target:    "localhost:18082",
		Path:      "/",
		Threshold: 3,
		Recovery:  2,
	}
	checker.Register("localhost:18082", check)

	// Get the check to verify state transitions
	checker.mu.Lock()
	hc := checker.checks["localhost:18082"]
	checker.mu.Unlock()

	// Simulate 3 failures (should become unhealthy)
	for i := 0; i < 3; i++ {
		checker.checkOne("localhost:18082", hc)
	}

	if hc.State != StateUnhealthy {
		t.Errorf("Expected StateUnhealthy after 3 failures, got %v", hc.State)
	}

	// Reset success counter and simulate recovery
	successCount = 3 // So next requests succeed

	// First success - should be recovering
	checker.checkOne("localhost:18082", hc)
	if hc.State != StateRecovering {
		t.Errorf("Expected StateRecovering after 1 success, got %v", hc.State)
	}

	// Second success - should be healthy
	checker.checkOne("localhost:18082", hc)
	if hc.State != StateHealthy {
		t.Errorf("Expected StateHealthy after 2 successes, got %v", hc.State)
	}
}

func TestCheckOneDegradedState(t *testing.T) {
	checker := NewChecker(10*time.Second, 5*time.Second)

	// Start test server that always fails
	server := &http.Server{
		Addr: ":18083",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}),
	}

	ln, err := net.Listen("tcp", ":18083")
	if err != nil {
		t.Skipf("Cannot start test server: %v", err)
	}

	go server.Serve(ln)
	defer server.Shutdown(context.Background())
	time.Sleep(100 * time.Millisecond)

	check := HealthCheck{
		Target:    "localhost:18083",
		Path:      "/",
		Threshold: 4,
	}
	checker.Register("localhost:18083", check)

	checker.mu.Lock()
	hc := checker.checks["localhost:18083"]
	checker.mu.Unlock()

	// With threshold=4, degraded should trigger at 2 failures
	for i := 0; i < 2; i++ {
		checker.checkOne("localhost:18083", hc)
	}

	if hc.State != StateDegraded {
		t.Errorf("Expected StateDegraded after 2 failures (threshold=4), got %v", hc.State)
	}
}

func TestCheckerStartStop(t *testing.T) {
	checker := NewChecker(100*time.Millisecond, 50*time.Millisecond)

	// Start test server
	server := &http.Server{
		Addr: ":18084",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	}

	ln, err := net.Listen("tcp", ":18084")
	if err != nil {
		t.Skipf("Cannot start test server: %v", err)
	}

	go server.Serve(ln)
	defer server.Shutdown(context.Background())
	time.Sleep(100 * time.Millisecond)

	check := HealthCheck{
		Target:    "localhost:18084",
		Path:      "/",
		Threshold: 3,
	}
	checker.Register("localhost:18084", check)

	ctx, cancel := context.WithCancel(context.Background())

	// Start checker in goroutine
	done := make(chan struct{})
	go func() {
		checker.Start(ctx)
		close(done)
	}()

	// Wait for some checks to run
	time.Sleep(300 * time.Millisecond)

	// Stop checker
	cancel()

	// Wait for checker to stop
	select {
	case <-done:
		// Success
	case <-time.After(1 * time.Second):
		t.Error("Checker did not stop after context cancellation")
	}
}

// Benchmark HTTPCheck
func BenchmarkHTTPCheck(b *testing.B) {
	server := &http.Server{
		Addr: ":18085",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	}

	ln, err := net.Listen("tcp", ":18085")
	if err != nil {
		b.Skipf("Cannot start test server: %v", err)
	}

	go server.Serve(ln)
	defer server.Shutdown(context.Background())
	time.Sleep(100 * time.Millisecond)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		HTTPCheck("localhost:18085", "/", 1*time.Second)
	}
}

func TestTCPCheck(t *testing.T) {
	// Start a simple TCP listener
	ln, err := net.Listen("tcp", ":18086")
	if err != nil {
		t.Skipf("Cannot start test listener: %v", err)
	}
	defer ln.Close()

	// Accept connections in background
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	time.Sleep(100 * time.Millisecond)

	// Test successful TCP check
	healthy, err := TCPCheck("localhost:18086", 2*time.Second)
	if err != nil {
		t.Fatalf("TCPCheck failed: %v", err)
	}
	if !healthy {
		t.Error("Expected healthy=true for listening port")
	}
}

func TestTCPCheckConnectionRefused(t *testing.T) {
	// Test with a port that's not listening
	healthy, err := TCPCheck("localhost:59998", 1*time.Second)
	if err == nil {
		t.Error("Expected connection refused error")
	}
	if healthy {
		t.Error("Expected healthy=false on connection refused")
	}
}

func TestTCPCheckTimeout(t *testing.T) {
	// Start a listener that doesn't accept connections quickly
	ln, err := net.Listen("tcp", ":18087")
	if err != nil {
		t.Skipf("Cannot start test listener: %v", err)
	}
	defer ln.Close()

	// Don't accept connections - just hold the port

	// Test with very short timeout - the connection should succeed
	// because the listener accepts the connection at the TCP level
	healthy, err := TCPCheck("localhost:18087", 1*time.Second)
	if err != nil {
		t.Logf("TCPCheck error (may be expected): %v", err)
	}
	_ = healthy // Result depends on whether connection was accepted
}

func TestCheckerDegradedRecovery(t *testing.T) {
	// Start test server
	server := &http.Server{
		Addr: ":18088",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	}

	ln, err := net.Listen("tcp", ":18088")
	if err != nil {
		t.Skipf("Cannot start test server: %v", err)
	}

	go server.Serve(ln)
	defer server.Shutdown(context.Background())
	time.Sleep(100 * time.Millisecond)

	checker := NewChecker(time.Minute, time.Minute)

	check := HealthCheck{
		Target:    "localhost:18088",
		Path:      "/",
		Threshold: 4,
		Recovery:  2,
	}
	checker.Register("localhost:18088", check)

	checker.mu.Lock()
	hc := checker.checks["localhost:18088"]
	// Set to degraded state with ConsecPass = 0
	hc.State = StateDegraded
	hc.ConsecPass = 0
	checker.mu.Unlock()

	// First success should increment ConsecPass
	checker.checkOne("localhost:18088", hc)
	if hc.ConsecPass != 1 {
		t.Errorf("ConsecPass = %d, want 1", hc.ConsecPass)
	}
	if hc.State != StateDegraded {
		t.Errorf("State = %v, want StateDegraded (not yet 2 passes)", hc.State)
	}

	// Second success should transition to healthy (ConsecPass >= 2)
	checker.checkOne("localhost:18088", hc)
	if hc.ConsecPass != 2 {
		t.Errorf("ConsecPass = %d, want 2", hc.ConsecPass)
	}
	if hc.State != StateHealthy {
		t.Errorf("State = %v, want StateHealthy after 2 consecutive passes from degraded", hc.State)
	}
}
