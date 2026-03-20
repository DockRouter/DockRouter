// Package log provides structured logging
package log

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestLoggerLevels(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf, LevelDebug)

	logger.Debug("debug message")
	logger.Info("info message")
	logger.Warn("warn message")
	logger.Error("error message")

	output := buf.String()
	if !strings.Contains(output, "debug message") {
		t.Error("Should contain debug message")
	}
	if !strings.Contains(output, "info message") {
		t.Error("Should contain info message")
	}
	if !strings.Contains(output, "warn message") {
		t.Error("Should contain warn message")
	}
	if !strings.Contains(output, "error message") {
		t.Error("Should contain error message")
	}
}

func TestLoggerLevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf, LevelWarn)

	logger.Debug("debug message")
	logger.Info("info message")
	logger.Warn("warn message")
	logger.Error("error message")

	output := buf.String()
	if strings.Contains(output, "debug message") {
		t.Error("Should not contain debug message when level is warn")
	}
	if strings.Contains(output, "info message") {
		t.Error("Should not contain info message when level is warn")
	}
	if !strings.Contains(output, "warn message") {
		t.Error("Should contain warn message")
	}
	if !strings.Contains(output, "error message") {
		t.Error("Should contain error message")
	}
}

func TestLoggerJSONFormat(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf, LevelInfo)

	logger.Info("test message", "key", "value")

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Errorf("Output should be valid JSON: %v", err)
	}

	if entry["msg"] != "test message" {
		t.Errorf("Message = %v, want 'test message'", entry["msg"])
	}
	if entry["level"] != "info" {
		t.Errorf("Level = %v, want 'info'", entry["level"])
	}
}

func TestLoggerWithFields(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf, LevelInfo)

	logger.With("request_id", "123").Info("with field")

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Errorf("Output should be valid JSON: %v", err)
	}

	// With MarshalJSON, fields are inlined into top-level JSON
	if entry["request_id"] != "123" {
		t.Error("Should contain request_id as top-level field")
	}
}

func TestLevelString(t *testing.T) {
	tests := []struct {
		level    Level
		expected string
	}{
		{LevelDebug, "debug"},
		{LevelInfo, "info"},
		{LevelWarn, "warn"},
		{LevelError, "error"},
		{LevelFatal, "fatal"},
		{Level(999), "unknown"}, // Invalid level
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if tt.level.String() != tt.expected {
				t.Errorf("Level.String() = %s, want %s", tt.level.String(), tt.expected)
			}
		})
	}
}

func TestAccessLog(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf, LevelInfo)

	req := httptest.NewRequest("GET", "http://example.com/test?foo=bar", nil)
	req.Header.Set("X-Request-Id", "req-123")
	req.Header.Set("User-Agent", "test-agent")

	logger.AccessLog(req, http.StatusOK, 50*time.Millisecond, "192.168.1.1:8080")

	output := buf.String()
	if !strings.Contains(output, "request completed") {
		t.Error("Should contain 'request completed'")
	}
	if !strings.Contains(output, "GET") {
		t.Error("Should contain method GET")
	}
	if !strings.Contains(output, "/test") {
		t.Error("Should contain path /test")
	}
	if !strings.Contains(output, "200") {
		t.Error("Should contain status 200")
	}
	if !strings.Contains(output, "192.168.1.1:8080") {
		t.Error("Should contain backend address")
	}
}

func TestAccessLogWithQuery(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf, LevelInfo)

	req := httptest.NewRequest("POST", "http://api.example.com/submit?param=value", nil)

	logger.AccessLog(req, http.StatusCreated, 100*time.Microsecond, "10.0.0.1:3000")

	output := buf.String()
	if !strings.Contains(output, "POST") {
		t.Error("Should contain method POST")
	}
	if !strings.Contains(output, "201") {
		t.Error("Should contain status 201")
	}
}

func TestLoggerWithChained(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf, LevelInfo)

	// Test chaining multiple With calls
	logger.With("key1", "value1").With("key2", "value2").Info("chained")

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Errorf("Output should be valid JSON: %v", err)
	}

	// With MarshalJSON, fields are inlined into top-level JSON
	if entry["key1"] != "value1" {
		t.Error("Should contain key1")
	}
	if entry["key2"] != "value2" {
		t.Error("Should contain key2")
	}
}

func TestLoggerWithEmptyValues(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf, LevelInfo)

	// With odd number of args should not panic
	logger.With("single_key").Info("test")

	// Output should still be valid
	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Errorf("Output should be valid JSON: %v", err)
	}
}

func TestNewLoggerNilWriter(t *testing.T) {
	// NewLogger with nil writer should not panic
	logger := NewLogger(nil, LevelInfo)
	if logger == nil {
		t.Error("NewLogger should not return nil")
	}
}

func TestLoggerFieldsOddArgs(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf, LevelInfo)

	// Test with odd number of field arguments (should be handled gracefully)
	logger.Info("message", "key1", "value1", "key2")

	// Should still produce valid JSON
	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Errorf("Output should be valid JSON even with odd args: %v", err)
	}
}

func TestLoggerWithNonStringKey(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf, LevelInfo)

	// Test with non-string key (should be skipped)
	logger.Info("message", 123, "value", "valid_key", "valid_value")

	// Should still produce valid JSON
	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Errorf("Output should be valid JSON: %v", err)
	}

	// With MarshalJSON, fields are inlined into top-level JSON
	// Non-string key should be skipped, but valid key should be present
	if entry["valid_key"] != "valid_value" {
		t.Error("Should contain valid_key")
	}
}

func TestLoggerWithErrorValue(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf, LevelInfo)

	// Test with error value (should be converted to string)
	testErr := fmt.Errorf("test error message")
	logger.Info("message", "error_field", testErr)

	// Should produce valid JSON with error as string
	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Errorf("Output should be valid JSON: %v", err)
	}

	// With MarshalJSON, fields are inlined into top-level JSON
	// Error should be stored as its string representation
	if entry["error_field"] != "test error message" {
		t.Errorf("error_field = %v, want 'test error message'", entry["error_field"])
	}
}

func TestLoggerFatalSubprocess(t *testing.T) {
	// This test runs in a subprocess to test Fatal which calls os.Exit
	if os.Getenv("TEST_FATAL") == "1" {
		// nil writer defaults to stdout, which we capture
		logger := NewLogger(nil, LevelFatal)
		logger.Fatal("fatal message", "key", "value")
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestLoggerFatalSubprocess")
	cmd.Env = append(os.Environ(), "TEST_FATAL=1")
	output, err := cmd.CombinedOutput()

	// Fatal should cause the process to exit with code 1
	if err == nil {
		t.Error("Fatal should cause process to exit with non-zero status")
	}

	// Check that the fatal message was logged
	if !strings.Contains(string(output), "fatal message") {
		t.Errorf("Output should contain 'fatal message', got: %s", output)
	}

	// Check exit code
	if exitErr, ok := err.(*exec.ExitError); ok {
		if exitErr.ExitCode() != 1 {
			t.Errorf("Exit code = %d, want 1", exitErr.ExitCode())
		}
	}
}

func TestLoggerFatalJSONFormat(t *testing.T) {
	// Test that fatal logs in correct JSON format (subprocess)
	if os.Getenv("TEST_FATAL_JSON") == "1" {
		// nil writer defaults to stdout
		logger := NewLogger(nil, LevelFatal)
		logger.Fatal("fatal json test")
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestLoggerFatalJSONFormat")
	cmd.Env = append(os.Environ(), "TEST_FATAL_JSON=1")
	output, _ := cmd.CombinedOutput()

	// Should contain valid JSON with level "fatal"
	var entry map[string]interface{}
	if err := json.Unmarshal(output, &entry); err != nil {
		t.Errorf("Output should be valid JSON: %v, got: %s", err, output)
	}

	if entry["level"] != "fatal" {
		t.Errorf("level = %v, want 'fatal'", entry["level"])
	}

	if entry["msg"] != "fatal json test" {
		t.Errorf("msg = %v, want 'fatal json test'", entry["msg"])
	}
}
