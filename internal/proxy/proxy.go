// Package proxy handles reverse proxying to backends
package proxy

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"
)

// Proxy handles reverse proxying to backend containers
type Proxy struct {
	transport  http.RoundTripper
	bufferPool *bufferPool
	logger     Logger
}

// Logger interface for proxy
type Logger interface {
	Debug(msg string, fields ...interface{})
	Info(msg string, fields ...interface{})
	Warn(msg string, fields ...interface{})
	Error(msg string, fields ...interface{})
}

// NewProxy creates a new reverse proxy
func NewProxy(logger Logger) *Proxy {
	return &Proxy{
		transport:  newTransport(),
		bufferPool: newBufferPool(),
		logger:     logger,
	}
}

// ServeHTTP proxies the request to the target backend
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request, target string) error {
	// Parse target URL
	targetURL, err := url.Parse("http://" + target)
	if err != nil {
		return fmt.Errorf("invalid target URL: %w", err)
	}

	// Create reverse proxy
	proxy := httputil.NewSingleHostReverseProxy(targetURL)
	proxy.Transport = p.transport
	proxy.BufferPool = p.bufferPool
	proxy.ErrorHandler = p.errorHandler

	// Director to modify request before forwarding
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)

		// Set X-Forwarded headers
		p.setForwardedHeaders(req, r)

		// Handle WebSocket upgrade
		if IsWebSocketRequest(req) {
			req.Header.Set("Connection", "Upgrade")
			req.Header.Set("Upgrade", "websocket")
		}
	}

	// Modify response to capture status
	proxy.ModifyResponse = func(resp *http.Response) error {
		// Log response status
		p.logger.Debug("Response received",
			"status", resp.StatusCode,
			"target", target,
		)
		return nil
	}

	// Forward the request
	proxy.ServeHTTP(w, r)

	return nil
}

// setForwardedHeaders sets standard proxy headers
func (p *Proxy) setForwardedHeaders(req *http.Request, original *http.Request) {
	// Get client IP from RemoteAddr
	clientIP := original.RemoteAddr
	if idx := strings.LastIndex(clientIP, ":"); idx != -1 {
		clientIP = clientIP[:idx]
	}
	// Remove brackets from IPv6
	clientIP = strings.Trim(clientIP, "[]")

	// X-Forwarded-For
	xff := original.Header.Get("X-Forwarded-For")
	if xff != "" {
		xff = xff + ", " + clientIP
	} else {
		xff = clientIP
	}
	req.Header.Set("X-Forwarded-For", xff)

	// X-Forwarded-Proto
	if original.TLS != nil {
		req.Header.Set("X-Forwarded-Proto", "https")
	} else {
		req.Header.Set("X-Forwarded-Proto", "http")
	}

	// X-Forwarded-Host
	if host := original.Header.Get("Host"); host != "" {
		req.Header.Set("X-Forwarded-Host", host)
	} else {
		req.Header.Set("X-Forwarded-Host", original.Host)
	}

	// X-Real-IP
	if req.Header.Get("X-Real-IP") == "" {
		req.Header.Set("X-Real-IP", clientIP)
	}
}

// errorHandler handles proxy errors
func (p *Proxy) errorHandler(w http.ResponseWriter, r *http.Request, err error) {
	p.logger.Error("Proxy error",
		"error", err,
		"path", r.URL.Path,
		"method", r.Method,
	)

	// Determine status code
	status := http.StatusBadGateway
	if strings.Contains(err.Error(), "timeout") {
		status = http.StatusGatewayTimeout
	} else if strings.Contains(err.Error(), "connection refused") {
		status = http.StatusServiceUnavailable
	}

	// Return error page
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)

	requestID := r.Header.Get("X-Request-Id")
	w.Write([]byte(buildErrorPage(status, http.StatusText(status), err.Error(), requestID)))
}

// SetTimeout sets the proxy timeout
func (p *Proxy) SetTimeout(d time.Duration) {
	if t, ok := p.transport.(*http.Transport); ok {
		t.ResponseHeaderTimeout = d
	}
}

// StreamProxy handles streaming responses (SSE, chunked)
type StreamProxy struct {
	proxy *Proxy
}

// NewStreamProxy creates a streaming proxy
func NewStreamProxy(proxy *Proxy) *StreamProxy {
	return &StreamProxy{proxy: proxy}
}

// ServeHTTP handles streaming requests with flush support
func (sp *StreamProxy) ServeHTTP(w http.ResponseWriter, r *http.Request, target string) error {
	// Check if flusher is supported
	flusher, ok := w.(http.Flusher)
	if !ok {
		return sp.proxy.ServeHTTP(w, r, target)
	}

	// Create request
	targetURL, err := url.Parse("http://" + target)
	if err != nil {
		return err
	}

	// Copy request
	req := r.Clone(r.Context())
	req.URL = targetURL
	req.Host = targetURL.Host

	// Remove hop-by-hop headers
	removeHopHeaders(req.Header)

	// Set forwarded headers
	sp.proxy.setForwardedHeaders(req, r)

	// Send request
	client := &http.Client{
		Transport: sp.proxy.transport,
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Copy headers
	for k, v := range resp.Header {
		w.Header()[k] = v
	}
	w.WriteHeader(resp.StatusCode)

	// Stream response
	flusher.Flush()

	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			w.Write(buf[:n])
			flusher.Flush()
		}
		if err != nil {
			if err != io.EOF {
				return err
			}
			break
		}
	}

	return nil
}

// removeHopHeaders removes hop-by-hop headers
func removeHopHeaders(hdr http.Header) {
	hopHeaders := []string{
		"Connection",
		"Keep-Alive",
		"Proxy-Authenticate",
		"Proxy-Authorization",
		"Te",
		"Trailer",
		"Transfer-Encoding",
		"Upgrade",
	}

	for _, h := range hopHeaders {
		hdr.Del(h)
	}
}

// buildErrorPage generates a branded error page
func buildErrorPage(code int, title, message, requestID string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>%d %s</title>
    <style>
        body { background: #0F172A; color: #F1F5F9; font-family: system-ui; display: flex; align-items: center; justify-content: center; min-height: 100vh; margin: 0; }
        .container { text-align: center; }
        .code { font-size: 4rem; font-weight: bold; color: #F97316; }
        .message { margin: 1rem 0; color: #94A3B8; }
        .request-id { font-family: monospace; font-size: 0.875rem; color: #64748B; }
    </style>
</head>
<body>
    <div class="container">
        <div class="code">%d</div>
        <div class="message">%s</div>
        %s
    </div>
</body>
</html>`, code, title, code, title, func() string {
		if requestID != "" {
			return `<div class="request-id">Request ID: ` + requestID + `</div>`
		}
		return ""
	}())
}
