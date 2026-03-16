// Package middleware provides HTTP middleware components
package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestChain(t *testing.T) {
	order := make([]string, 0)

	m1 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "m1-before")
			next.ServeHTTP(w, r)
			order = append(order, "m1-after")
		})
	}

	m2 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "m2-before")
			next.ServeHTTP(w, r)
			order = append(order, "m2-after")
		})
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		order = append(order, "handler")
		w.WriteHeader(http.StatusOK)
	})

	chain := Chain(m1, m2)
	finalHandler := chain(handler)

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	finalHandler.ServeHTTP(rec, req)

	expected := []string{"m1-before", "m2-before", "handler", "m2-after", "m1-after"}
	if len(order) != len(expected) {
		t.Errorf("Order length = %d, want %d", len(order), len(expected))
	}
	for i, v := range expected {
		if order[i] != v {
			t.Errorf("order[%d] = %s, want %s", i, order[i], v)
		}
	}
}

func TestRecovery(t *testing.T) {
	panicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	})

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()

	recoveryHandler := Recovery(panicHandler)
	recoveryHandler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestRequestID(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-Id")
		if id == "" {
			t.Error("Request ID not set")
		}
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()

	requestIDHandler := RequestID(handler)
	requestIDHandler.ServeHTTP(rec, req)

	id := rec.Header().Get("X-Request-Id")
	if id == "" {
		t.Error("Request ID not in response")
	}
}

func TestRequestIDPreservesExisting(t *testing.T) {
	existingID := "existing-id-123"
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-Id")
		if id != existingID {
			t.Errorf("Request ID = %s, want %s", id, existingID)
		}
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Request-Id", existingID)
	rec := httptest.NewRecorder()

	requestIDHandler := RequestID(handler)
	requestIDHandler.ServeHTTP(rec, req)
}

func TestSecurityHeaders(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()

	securityHandler := SecurityHeaders(handler)
	securityHandler.ServeHTTP(rec, req)

	tests := []struct {
		header   string
		expected string
	}{
		{"X-Content-Type-Options", "nosniff"},
		{"X-Frame-Options", "DENY"},
		{"X-Xss-Protection", "1; mode=block"},
		{"Referrer-Policy", "strict-origin-when-cross-origin"},
	}

	for _, tt := range tests {
		t.Run(tt.header, func(t *testing.T) {
			got := rec.Header().Get(tt.header)
			if got != tt.expected {
				t.Errorf("%s = %s, want %s", tt.header, got, tt.expected)
			}
		})
	}
}

func TestCORS(t *testing.T) {
	config := CORSConfig{
		Origins: []string{"https://example.com", "https://app.example.com"},
		Methods: []string{"GET", "POST"},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	corsHandler := CORS(config)(handler)

	// Test preflight
	t.Run("preflight", func(t *testing.T) {
		req := httptest.NewRequest("OPTIONS", "/", nil)
		req.Header.Set("Origin", "https://example.com")
		rec := httptest.NewRecorder()

		corsHandler.ServeHTTP(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Errorf("Status = %d, want %d", rec.Code, http.StatusNoContent)
		}
	})

	// Test actual request
	t.Run("actual request", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Origin", "https://example.com")
		rec := httptest.NewRecorder()

		corsHandler.ServeHTTP(rec, req)

		if rec.Header().Get("Access-Control-Allow-Origin") != "https://example.com" {
			t.Errorf("CORS origin = %s", rec.Header().Get("Access-Control-Allow-Origin"))
		}
	})
}

func TestRedirectHTTPS(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	redirectHandler := RedirectHTTPS(handler)

	t.Run("redirects http", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/path?query=1", nil)
		req.Host = "example.com"
		rec := httptest.NewRecorder()

		redirectHandler.ServeHTTP(rec, req)

		if rec.Code != http.StatusMovedPermanently {
			t.Errorf("Status = %d, want %d", rec.Code, http.StatusMovedPermanently)
		}
		location := rec.Header().Get("Location")
		if location != "https://example.com/path?query=1" {
			t.Errorf("Location = %s, want https://example.com/path?query=1", location)
		}
	})
}

func TestStripPrefix(t *testing.T) {
	var receivedPath string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	})

	stripHandler := StripPrefix("/api")(handler)

	req := httptest.NewRequest("GET", "/api/users", nil)
	rec := httptest.NewRecorder()

	stripHandler.ServeHTTP(rec, req)

	if receivedPath != "/users" {
		t.Errorf("Path = %s, want /users", receivedPath)
	}
}

func TestAddPrefix(t *testing.T) {
	var receivedPath string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	})

	addHandler := AddPrefix("/v1")(handler)

	req := httptest.NewRequest("GET", "/users", nil)
	rec := httptest.NewRecorder()

	addHandler.ServeHTTP(rec, req)

	if receivedPath != "/v1/users" {
		t.Errorf("Path = %s, want /v1/users", receivedPath)
	}
}

func TestHeaders(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	headers := map[string]string{
		"X-Custom-Header":  "custom-value",
		"X-Another-Header": "another-value",
	}

	headersHandler := Headers(headers)(handler)

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()

	headersHandler.ServeHTTP(rec, req)

	for key, expected := range headers {
		got := rec.Header().Get(key)
		if got != expected {
			t.Errorf("%s = %s, want %s", key, got, expected)
		}
	}
}
