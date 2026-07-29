// Package proxy handles reverse proxying to backends
package proxy

import (
	"context"
	"fmt"
	"html"
	"net"
	"net/http"
	"net/http/httputil"
	"strings"
	"time"
)

type contextKey string

const (
	targetKey   contextKey = "target"
	errorKey    contextKey = "proxyError"
	originalKey contextKey = "original"
	// suppressKey marks a request whose failure must not produce an error page,
	// so that the caller can retry against another backend instead.
	suppressKey contextKey = "suppressErrorPage"
)

// Proxy handles reverse proxying to backend containers
type Proxy struct {
	transport      http.RoundTripper
	bufferPool     *bufferPool
	logger         Logger
	websocketProxy *WebSocketProxy
	rp             *httputil.ReverseProxy
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
	p := &Proxy{
		transport:  newTransport(),
		bufferPool: newBufferPool(),
		logger:     logger,
	}
	p.websocketProxy = NewWebSocketProxy(logger)

	p.rp = &httputil.ReverseProxy{
		Transport:  p.transport,
		BufferPool: p.bufferPool,
		Director: func(req *http.Request) {
			target := req.Context().Value(targetKey).(string)
			original := req.Context().Value(originalKey).(*http.Request)

			req.URL.Scheme = "http"
			req.URL.Host = target
			p.setForwardedHeaders(req, original)
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			if errPtr, ok := r.Context().Value(errorKey).(*error); ok {
				*errPtr = err
			}
			// When failover is possible the caller renders the final response,
			// so writing an error page here would burn the ResponseWriter.
			if suppress, ok := r.Context().Value(suppressKey).(bool); ok && suppress {
				return
			}
			p.errorHandler(w, r, err)
		},
		ModifyResponse: func(resp *http.Response) error {
			p.logger.Debug("Response received",
				"status", resp.StatusCode,
				"target", resp.Request.URL.Host,
			)
			return nil
		},
	}
	return p
}

// ServeHTTP proxies the request to the target backend. On failure it renders a
// branded error page and also returns the error.
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request, target string) error {
	return p.serve(w, r, target, false)
}

// ServeHTTPFailover proxies the request but never writes an error response of
// its own. On failure it returns the error and leaves w untouched, so the caller
// can retry against a different backend or render its own error page.
func (p *Proxy) ServeHTTPFailover(w http.ResponseWriter, r *http.Request, target string) error {
	return p.serve(w, r, target, true)
}

func (p *Proxy) serve(w http.ResponseWriter, r *http.Request, target string, suppress bool) error {
	// Route WebSocket requests through WebSocketProxy
	if IsWebSocketRequest(r) {
		return p.websocketProxy.ServeHTTP(w, r, target)
	}

	var proxyErr error
	ctx := context.WithValue(r.Context(), targetKey, target)
	ctx = context.WithValue(ctx, errorKey, &proxyErr)
	ctx = context.WithValue(ctx, originalKey, r)
	ctx = context.WithValue(ctx, suppressKey, suppress)

	p.rp.ServeHTTP(w, r.WithContext(ctx))
	return proxyErr
}

// setForwardedHeaders sets standard proxy headers
func (p *Proxy) setForwardedHeaders(req *http.Request, original *http.Request) {
	// Get client IP from RemoteAddr
	clientIP := original.RemoteAddr
	if host, _, err := net.SplitHostPort(clientIP); err == nil {
		clientIP = host
	}

	// X-Forwarded-For - always overwrite with real client IP to prevent injection
	req.Header.Set("X-Forwarded-For", clientIP)

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
	w.Write([]byte(buildErrorPage(status, http.StatusText(status), "The upstream server is not available", requestID)))
}

// SetTimeout sets the proxy timeout
func (p *Proxy) SetTimeout(d time.Duration) {
	if t, ok := p.transport.(*http.Transport); ok {
		t.ResponseHeaderTimeout = d
	}
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
	safeTitle := html.EscapeString(title)
	safeMessage := html.EscapeString(message)
	safeRequestID := html.EscapeString(requestID)

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
</html>`, code, safeTitle, code, safeMessage, func() string {
		if safeRequestID != "" {
			return `<div class="request-id">Request ID: ` + safeRequestID + `</div>`
		}
		return ""
	}())
}
