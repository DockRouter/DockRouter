// Package log provides structured logging
package log

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

	fields, ok := entry["Fields"].(map[string]interface{})
	if !ok {
		t.Error("Should contain Fields map")
	}
	if fields["request_id"] != "123" {
		t.Error("Should contain request_id in Fields")
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

	fields, ok := entry["Fields"].(map[string]interface{})
	if !ok {
		t.Fatal("Should contain Fields map")
	}
	if fields["key1"] != "value1" {
		t.Error("Should contain key1")
	}
	if fields["key2"] != "value2" {
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
