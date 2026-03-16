// Package router handles HTTP routing
package router

import (
	"net/http"
	"strings"
)

// Router matches requests to routes and delegates to proxy
type Router struct {
	table   *Table
	proxy   Proxy
	logger  Logger
}

// Proxy is the interface for proxying requests
type Proxy interface {
	ServeHTTP(w http.ResponseWriter, r *http.Request, target string) error
}

// Logger interface for router
type Logger interface {
	Debug(msg string, fields ...interface{})
	Info(msg string, fields ...interface{})
	Warn(msg string, fields ...interface{})
	Error(msg string, fields ...interface{})
}

// NewRouter creates a new router
func NewRouter(table *Table, proxy Proxy, logger Logger) *Router {
	return &Router{
		table:  table,
		proxy:  proxy,
		logger: logger,
	}
}

// ServeHTTP implements http.Handler
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// Extract host and path
	host := req.Host
	path := req.URL.Path

	// Normalize host (remove port)
	if idx := strings.LastIndex(host, ":"); idx > 0 {
		host = host[:idx]
	}
	host = strings.ToLower(host)

	// Match route
	route := r.table.Match(host, path)
	if route == nil {
		r.handleNoMatch(w, req, host, path)
		return
	}

	// Get backend from pool
	backend := route.Backend.Select(req.RemoteAddr)
	if backend == nil {
		r.handleNoBackend(w, req, route)
		return
	}

	// Record request
	route.Backend.RecordRequest(backend.Address)

	// Log the match
	r.logger.Debug("Route matched",
		"host", host,
		"path", path,
		"backend", backend.Address,
		"container", route.ContainerName,
	)

	// Proxy the request
	if err := r.proxy.ServeHTTP(w, req, backend.Address); err != nil {
		r.logger.Error("Proxy error",
			"error", err,
			"backend", backend.Address,
			"path", path,
		)
		route.Backend.RecordFailure(backend.Address)

		// Return error page
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
	}
}

// handleNoMatch handles requests with no matching route
func (r *Router) handleNoMatch(w http.ResponseWriter, req *http.Request, host, path string) {
	r.logger.Debug("No route matched",
		"host", host,
		"path", path,
	)

	// Return 502 with branded error page
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusBadGateway)
	w.Write([]byte(buildErrorPage(502, "Bad Gateway", "No route found for this host", req.Header.Get("X-Request-Id"))))
}

// handleNoBackend handles requests with no healthy backend
func (r *Router) handleNoBackend(w http.ResponseWriter, req *http.Request, route *Route) {
	r.logger.Warn("No healthy backend",
		"host", route.Host,
		"path", route.PathPrefix,
		"container", route.ContainerName,
	)

	// Return 503 with branded error page
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusServiceUnavailable)
	w.Write([]byte(buildErrorPage(503, "Service Unavailable", "No healthy backends available", req.Header.Get("X-Request-Id"))))
}

// GetTable returns the route table (for admin API)
func (r *Router) GetTable() *Table {
	return r.table
}

// buildErrorPage generates a branded error page
func buildErrorPage(code int, title, message, requestID string) string {
	return `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>` + intToStr(code) + ` ` + title + `</title>
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
        <div class="code">` + intToStr(code) + `</div>
        <div class="message">` + title + `</div>
        <div class="message">` + message + `</div>
        ` + func() string {
		if requestID != "" {
			return `<div class="request-id">Request ID: ` + requestID + `</div>`
		}
		return ""
	}() + `
    </div>
</body>
</html>`
}

func intToStr(n int) string {
	if n == 0 {
		return "0"
	}
	var s []byte
	for n > 0 {
		s = append([]byte{byte('0' + n%10)}, s...)
		n /= 10
	}
	return string(s)
}
