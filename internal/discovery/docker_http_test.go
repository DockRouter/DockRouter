package discovery

import (
	"context"
	"testing"
	"time"
)

// TestDoRequestContextDeadline tests doRequest with context deadline
func TestDoRequestContextDeadline(t *testing.T) {
	client, _ := NewDockerClient("/nonexistent/docker.sock")

	// Create context with very short deadline
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	// Give time for context to timeout
	time.Sleep(10 * time.Millisecond)

	// doRequest should fail due to context timeout or connection error
	_, err := client.doRequest(ctx, "GET", "/containers/json")
	if err == nil {
		t.Error("doRequest should fail with deadline exceeded or connection error")
	}
}

// TestDoStreamRequestContextDeadline tests doStreamRequest with context deadline
func TestDoStreamRequestContextDeadline(t *testing.T) {
	client, _ := NewDockerClient("/nonexistent/docker.sock")

	// Create context with very short deadline
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	// Give time for context to timeout
	time.Sleep(10 * time.Millisecond)

	// doStreamRequest should fail due to context timeout or connection error
	_, err := client.doStreamRequest(ctx, "GET", "/events")
	if err == nil {
		t.Error("doStreamRequest should fail with deadline exceeded or connection error")
	}
}

// TestDockerClientDoRequestConnectionFailure tests doRequest connection failure
func TestDockerClientDoRequestConnectionFailure(t *testing.T) {
	client, _ := NewDockerClient("/nonexistent/docker.sock")
	ctx := context.Background()

	_, err := client.doRequest(ctx, "GET", "/containers/json")
	if err == nil {
		t.Error("doRequest should fail with nonexistent socket")
	}

	// Verify error message contains expected text
	if err != nil && err.Error() == "" {
		t.Error("Error should have a message")
	}
}

// TestDockerClientDoStreamRequestConnectionFailure tests doStreamRequest connection failure
func TestDockerClientDoStreamRequestConnectionFailure(t *testing.T) {
	client, _ := NewDockerClient("/nonexistent/docker.sock")
	ctx := context.Background()

	_, err := client.doStreamRequest(ctx, "GET", "/events")
	if err == nil {
		t.Error("doStreamRequest should fail with nonexistent socket")
	}

	// Verify error message contains expected text
	if err != nil && err.Error() == "" {
		t.Error("Error should have a message")
	}
}

// TestDockerClientDoRequestWithTimeout tests doRequest respects client timeout
func TestDockerClientDoRequestWithTimeout(t *testing.T) {
	client, _ := NewDockerClient("/nonexistent/docker.sock")
	client.SetTimeout(50 * time.Millisecond)

	ctx := context.Background()

	start := time.Now()
	_, err := client.doRequest(ctx, "GET", "/containers/json")
	elapsed := time.Since(start)

	if err == nil {
		t.Error("doRequest should fail")
	}

	// Should fail quickly due to timeout, not hang
	if elapsed > 5*time.Second {
		t.Error("doRequest should respect timeout")
	}
}

// TestDockerClientDoStreamRequestWithTimeout tests doStreamRequest respects client timeout
func TestDockerClientDoStreamRequestWithTimeout(t *testing.T) {
	client, _ := NewDockerClient("/nonexistent/docker.sock")
	client.SetTimeout(50 * time.Millisecond)

	ctx := context.Background()

	start := time.Now()
	_, err := client.doStreamRequest(ctx, "GET", "/events")
	elapsed := time.Since(start)

	if err == nil {
		t.Error("doStreamRequest should fail")
	}

	// Should fail quickly due to timeout, not hang
	if elapsed > 5*time.Second {
		t.Error("doStreamRequest should respect timeout")
	}
}

// TestDockerClientPingWithTimeout tests Ping with custom timeout
func TestDockerClientPingWithTimeout(t *testing.T) {
	client, _ := NewDockerClient("/nonexistent/docker.sock")
	client.SetTimeout(100 * time.Millisecond)

	ctx := context.Background()

	start := time.Now()
	err := client.Ping(ctx)
	elapsed := time.Since(start)

	if err == nil {
		t.Error("Ping should fail with nonexistent socket")
	}

	// Should fail quickly, not hang
	if elapsed > 5*time.Second {
		t.Error("Ping should respect timeout")
	}
}
