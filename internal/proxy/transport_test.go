package proxy

import (
	"net/http"
	"testing"
	"time"
)

func TestNewTransport(t *testing.T) {
	transport := newTransport()

	if transport == nil {
		t.Fatal("expected transport to not be nil")
	}

	// Verify transport configuration
	if transport.MaxIdleConns != MaxIdleConns {
		t.Errorf("MaxIdleConns: expected %d, got %d", MaxIdleConns, transport.MaxIdleConns)
	}

	if transport.MaxIdleConnsPerHost != MaxIdleConnsPerHost {
		t.Errorf("MaxIdleConnsPerHost: expected %d, got %d", MaxIdleConnsPerHost, transport.MaxIdleConnsPerHost)
	}

	if transport.IdleConnTimeout != IdleConnTimeout {
		t.Errorf("IdleConnTimeout: expected %v, got %v", IdleConnTimeout, transport.IdleConnTimeout)
	}

	if transport.ResponseHeaderTimeout != ResponseTimeout {
		t.Errorf("ResponseHeaderTimeout: expected %v, got %v", ResponseTimeout, transport.ResponseHeaderTimeout)
	}

	// Verify HTTP/2 is disabled
	if transport.ForceAttemptHTTP2 != false {
		t.Error("ForceAttemptHTTP2 should be false")
	}

	// Verify keep-alives are enabled
	if transport.DisableKeepAlives != false {
		t.Error("DisableKeepAlives should be false")
	}

	// Verify compression passthrough is enabled (DisableCompression=true for proxy)
	if transport.DisableCompression != true {
		t.Error("DisableCompression should be true for transparent proxy passthrough")
	}
}

func TestTransportConstants(t *testing.T) {
	// Verify all constants are set correctly
	tests := []struct {
		name     string
		got      int
		expected int
	}{
		{"MaxIdleConns", MaxIdleConns, 100},
		{"MaxIdleConnsPerHost", MaxIdleConnsPerHost, 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("%s: expected %d, got %d", tt.name, tt.expected, tt.got)
			}
		})
	}

	// Verify timeouts
	if IdleConnTimeout != 90*time.Second {
		t.Errorf("IdleConnTimeout: expected 90s, got %v", IdleConnTimeout)
	}

	if HandshakeTimeout != 10*time.Second {
		t.Errorf("HandshakeTimeout: expected 10s, got %v", HandshakeTimeout)
	}

	if ResponseTimeout != 30*time.Second {
		t.Errorf("ResponseTimeout: expected 30s, got %v", ResponseTimeout)
	}
}

func TestNewTransportDialer(t *testing.T) {
	transport := newTransport()

	// Verify DialContext is set (not nil)
	if transport.DialContext == nil {
		t.Error("DialContext should not be nil")
	}

	// Create a test request to verify transport works
	client := &http.Client{
		Transport: transport,
		Timeout:   1 * time.Second,
	}

	// We can't actually make requests, but we can verify the client is configured
	if client.Transport != transport {
		t.Error("client transport not set correctly")
	}
}

func TestTransportReusability(t *testing.T) {
	// Ensure multiple calls return independent transports
	t1 := newTransport()
	t2 := newTransport()

	if t1 == t2 {
		t.Error("expected different transport instances")
	}

	// Verify they have same configuration
	if t1.MaxIdleConns != t2.MaxIdleConns {
		t.Error("transports should have same MaxIdleConns")
	}
}
