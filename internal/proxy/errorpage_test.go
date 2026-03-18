package proxy

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRenderError(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		requestID  string
		wantCode   int
		wantText   string
	}{
		{
			name:       "404 Not Found",
			statusCode: 404,
			requestID:  "req-123-abc",
			wantCode:   404,
			wantText:   "Not Found",
		},
		{
			name:       "500 Internal Server Error",
			statusCode: 500,
			requestID:  "req-456-def",
			wantCode:   500,
			wantText:   "Internal Server Error",
		},
		{
			name:       "502 Bad Gateway",
			statusCode: 502,
			requestID:  "req-789-ghi",
			wantCode:   502,
			wantText:   "Bad Gateway",
		},
		{
			name:       "503 Service Unavailable",
			statusCode: 503,
			requestID:  "req-000-jkl",
			wantCode:   503,
			wantText:   "Service Unavailable",
		},
		{
			name:       "empty request ID",
			statusCode: 404,
			requestID:  "",
			wantCode:   404,
			wantText:   "Not Found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			RenderError(rec, tt.statusCode, tt.requestID)

			// Check status code
			if rec.Code != tt.wantCode {
				t.Errorf("status code: got %d, want %d", rec.Code, tt.wantCode)
			}

			// Check content type
			contentType := rec.Header().Get("Content-Type")
			if contentType != "text/html; charset=utf-8" {
				t.Errorf("content-type: got %q, want %q", contentType, "text/html; charset=utf-8")
			}

			// Check body contains expected elements
			body := rec.Body.String()

			// Should contain status code
			if !strings.Contains(body, fmt.Sprintf("%d", tt.statusCode)) {
				t.Errorf("body should contain status code %d", tt.statusCode)
			}

			// Should contain status text
			if !strings.Contains(body, tt.wantText) {
				t.Errorf("body should contain status text %q", tt.wantText)
			}

			// Should contain request ID
			if !strings.Contains(body, tt.requestID) {
				t.Errorf("body should contain request ID %q", tt.requestID)
			}

			// Should be valid HTML
			if !strings.HasPrefix(body, "<!DOCTYPE html>") {
				t.Error("body should start with <!DOCTYPE html>")
			}
		})
	}
}

func TestRenderErrorTemplateExecution(t *testing.T) {
	rec := httptest.NewRecorder()
	RenderError(rec, http.StatusServiceUnavailable, "test-req-id")

	body := rec.Body.String()

	// Verify HTML structure
	checks := []string{
		"<html",
		"<head>",
		"<body>",
		"</body>",
		"</html>",
	}

	for _, check := range checks {
		if !strings.Contains(body, check) {
			t.Errorf("body should contain %q", check)
		}
	}
}

func TestErrorPageDataStruct(t *testing.T) {
	// Test that ErrorPageData can be created and fields are accessible
	data := ErrorPageData{
		StatusCode: 500,
		StatusText: "Internal Server Error",
		RequestID:  "abc-123",
		Message:    "something went wrong",
	}

	if data.StatusCode != 500 {
		t.Error("StatusCode not set correctly")
	}
	if data.StatusText != "Internal Server Error" {
		t.Error("StatusText not set correctly")
	}
	if data.RequestID != "abc-123" {
		t.Error("RequestID not set correctly")
	}
	if data.Message != "something went wrong" {
		t.Error("Message not set correctly")
	}
}
