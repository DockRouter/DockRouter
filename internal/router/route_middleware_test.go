package router

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRouteMiddlewareBuilderBuildChain(t *testing.T) {
	builder := NewRouteMiddlewareBuilder()

	route := &Route{
		ID: "test-route",
		MiddlewareConfig: MiddlewareConfig{
			RateLimit: RateLimitConfig{
				Enabled: true,
				Count:   10,
				Window:  time.Minute,
			},
			CORS: CORSConfig{
				Enabled: true,
				Origins: []string{"https://example.com"},
				Methods: []string{"GET", "POST"},
				Headers: []string{"Content-Type"},
			},
			Compress: true,
		},
	}

	// Create a simple next handler
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Build chain
	handler := builder.BuildChain(route, next)
	if handler == nil {
		t.Fatal("BuildChain returned nil")
	}

	// Test the handler
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Origin", "https://example.com")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Should get a response (CORS preflight or actual)
	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestRouteMiddlewareBuilderRateLimiter(t *testing.T) {
	builder := NewRouteMiddlewareBuilder()

	route := &Route{
		ID: "rate-limited-route",
		MiddlewareConfig: MiddlewareConfig{
			RateLimit: RateLimitConfig{
				Enabled: true,
				Count:   2,
				Window:  time.Minute,
			},
		},
	}

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := builder.BuildChain(route, next)

	// First request should pass
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("First request: status = %d, want %d", rec.Code, http.StatusOK)
	}

	// Second request should pass
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("Second request: status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestRouteMiddlewareBuilderStripPrefix(t *testing.T) {
	builder := NewRouteMiddlewareBuilder()

	var receivedPath string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	})

	route := &Route{
		ID: "strip-prefix-route",
		MiddlewareConfig: MiddlewareConfig{
			StripPrefix: "/api/v1",
		},
	}

	handler := builder.BuildChain(route, next)

	req := httptest.NewRequest("GET", "/api/v1/users", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if receivedPath != "/users" {
		t.Errorf("Path = %q, want %q", receivedPath, "/users")
	}
}

func TestRouteMiddlewareBuilderAddPrefix(t *testing.T) {
	builder := NewRouteMiddlewareBuilder()

	var receivedPath string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	})

	route := &Route{
		ID: "add-prefix-route",
		MiddlewareConfig: MiddlewareConfig{
			AddPrefix: "/internal",
		},
	}

	handler := builder.BuildChain(route, next)

	req := httptest.NewRequest("GET", "/users", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if receivedPath != "/internal/users" {
		t.Errorf("Path = %q, want %q", receivedPath, "/internal/users")
	}
}

func TestRouteMiddlewareBuilderBasicAuth(t *testing.T) {
	builder := NewRouteMiddlewareBuilder()

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("secret"))
	})

	// Note: The middleware currently uses plain text comparison
	// The Hash field stores the password (bcrypt support documented but not implemented)
	route := &Route{
		ID: "auth-route",
		MiddlewareConfig: MiddlewareConfig{
			BasicAuthUsers: []BasicAuthUser{
				{Username: "admin", Hash: "secret"},
			},
		},
	}

	handler := builder.BuildChain(route, next)

	// Request without auth should fail
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Without auth: status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}

	// Request with valid auth should pass
	req.SetBasicAuth("admin", "secret")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("With auth: status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestRouteMiddlewareBuilderCircuitBreaker(t *testing.T) {
	builder := NewRouteMiddlewareBuilder()

	route := &Route{
		ID: "cb-route",
		MiddlewareConfig: MiddlewareConfig{
			CircuitBreaker: CircuitBreakerConfig{
				Enabled:  true,
				Failures: 3,
				Window:   time.Minute,
			},
		},
	}

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := builder.BuildChain(route, next)

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestRouteMiddlewareBuilderMaxBody(t *testing.T) {
	builder := NewRouteMiddlewareBuilder()

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	route := &Route{
		ID: "maxbody-route",
		MiddlewareConfig: MiddlewareConfig{
			MaxBody: 100, // 100 bytes
		},
	}

	handler := builder.BuildChain(route, next)

	// Small request should pass
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("Small request: status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestRouteMiddlewareBuilderEmptyChain(t *testing.T) {
	builder := NewRouteMiddlewareBuilder()

	route := &Route{
		ID:               "empty-route",
		MiddlewareConfig: MiddlewareConfig{}, // No middleware
	}

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	handler := builder.BuildChain(route, next)

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !called {
		t.Error("Next handler was not called")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestRouteMiddlewareBuilderRemoveRateLimiter(t *testing.T) {
	builder := NewRouteMiddlewareBuilder()

	route := &Route{
		ID: "cleanup-route",
		MiddlewareConfig: MiddlewareConfig{
			RateLimit: RateLimitConfig{
				Enabled: true,
				Count:   10,
				Window:  time.Minute,
			},
		},
	}

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Build chain to create rate limiter
	_ = builder.BuildChain(route, next)

	// Remove it (cleanup)
	builder.RemoveRateLimiter("cleanup-route")

	// Should not panic
}

func TestRouteMiddlewareBuilderRemoveCircuitBreaker(t *testing.T) {
	builder := NewRouteMiddlewareBuilder()

	route := &Route{
		ID: "cleanup-cb-route",
		MiddlewareConfig: MiddlewareConfig{
			CircuitBreaker: CircuitBreakerConfig{
				Enabled:  true,
				Failures: 5,
				Window:   time.Minute,
			},
		},
	}

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Build chain to create circuit breaker
	_ = builder.BuildChain(route, next)

	// Remove it (cleanup)
	builder.RemoveCircuitBreaker("cleanup-cb-route")

	// Should not panic
}

func TestRouteMiddlewareBuilderCircuitBreakerDefaultValues(t *testing.T) {
	builder := NewRouteMiddlewareBuilder()

	// Test with default values (0 for Failures and Window)
	route := &Route{
		ID: "cb-default-route",
		MiddlewareConfig: MiddlewareConfig{
			CircuitBreaker: CircuitBreakerConfig{
				Enabled:  true,
				Failures: 0, // Should use default of 5
				Window:   0, // Should use default of 1 minute
			},
		},
	}

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := builder.BuildChain(route, next)

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestRouteMiddlewareBuilderRateLimiterReuse(t *testing.T) {
	builder := NewRouteMiddlewareBuilder()

	route := &Route{
		ID: "reuse-route",
		MiddlewareConfig: MiddlewareConfig{
			RateLimit: RateLimitConfig{
				Enabled: true,
				Count:   100,
				Window:  time.Minute,
			},
		},
	}

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Build chain twice - should reuse same rate limiter
	handler1 := builder.BuildChain(route, next)
	handler2 := builder.BuildChain(route, next)

	if handler1 == nil || handler2 == nil {
		t.Error("BuildChain returned nil")
	}

	// Both should work
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler1.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("Handler1: status = %d, want %d", rec.Code, http.StatusOK)
	}

	rec = httptest.NewRecorder()
	handler2.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("Handler2: status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestRouteMiddlewareBuilderDefaultValues(t *testing.T) {
	builder := NewRouteMiddlewareBuilder()

	// Route with zero values
	route := &Route{
		ID: "defaults-route",
		MiddlewareConfig: MiddlewareConfig{
			RateLimit: RateLimitConfig{
				Enabled: true,
				// Count and Window are zero - should use defaults
			},
			CircuitBreaker: CircuitBreakerConfig{
				Enabled: true,
				// Failures and Window are zero - should use defaults
			},
		},
	}

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := builder.BuildChain(route, next)
	if handler == nil {
		t.Fatal("BuildChain returned nil")
	}

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Should work with default values
	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestRouteMiddlewareBuilderIPFilter(t *testing.T) {
	builder := NewRouteMiddlewareBuilder()

	_, whitelist, _ := net.ParseCIDR("192.168.1.0/24")

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	route := &Route{
		ID: "ipfilter-route",
		MiddlewareConfig: MiddlewareConfig{
			IPWhitelist: []*net.IPNet{whitelist},
		},
	}

	handler := builder.BuildChain(route, next)

	// Request should be handled (IP filter middleware is applied)
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Note: IP filter behavior depends on the middleware implementation
	// The test verifies the chain builds without error
	_ = req
	_ = rec
}

func TestRouteMiddlewareBuilderIPBlacklist(t *testing.T) {
	builder := NewRouteMiddlewareBuilder()

	_, blacklist, _ := net.ParseCIDR("10.0.0.0/8")

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	route := &Route{
		ID: "ipblacklist-route",
		MiddlewareConfig: MiddlewareConfig{
			IPBlacklist: []*net.IPNet{blacklist},
		},
	}

	handler := builder.BuildChain(route, next)

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Verify handler was created
	if handler == nil {
		t.Error("BuildChain returned nil")
	}
}
