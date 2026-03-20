// Package router handles HTTP routing
package router

import (
	"html"
	"net/http"
	"strings"
)

// Router matches requests to routes and delegates to proxy
type Router struct {
	table             *Table
	proxy             Proxy
	logger            Logger
	maxRetries        int
	middlewareBuilder *RouteMiddlewareBuilder
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
		table:             table,
		proxy:             proxy,
		logger:            logger,
		maxRetries:        3, // Default to 3 retries
		middlewareBuilder: NewRouteMiddlewareBuilder(),
	}
}

// NewRouterWithMiddleware creates a new router with a shared middleware builder
func NewRouterWithMiddleware(table *Table, proxy Proxy, logger Logger, builder *RouteMiddlewareBuilder) *Router {
	return &Router{
		table:             table,
		proxy:             proxy,
		logger:            logger,
		maxRetries:        3,
		middlewareBuilder: builder,
	}
}

// SetMaxRetries sets the maximum number of retry attempts
func (r *Router) SetMaxRetries(n int) {
	if n > 0 {
		r.maxRetries = n
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

	// Check if we have any backends at all
	if route.Backend.HealthyCount() == 0 {
		r.handleNoBackend(w, req, route)
		return
	}

	// Determine max retries for this route (use route config if set, otherwise default)
	maxRetries := r.maxRetries
	if route.MiddlewareConfig.Retry > 0 {
		maxRetries = route.MiddlewareConfig.Retry
	}

	// Create the proxy handler with retry logic
	proxyHandler := r.createProxyHandler(route, host, path, maxRetries)

	// Apply per-route middleware and execute
	chain := r.middlewareBuilder.BuildChain(route, proxyHandler)
	chain.ServeHTTP(w, req)
}

// createProxyHandler creates a handler that proxies to backends with retry logic
func (r *Router) createProxyHandler(route *Route, host, path string, maxRetries int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// Try backends with retry logic
		triedBackends := make(map[string]bool)

		for attempt := 0; attempt < maxRetries; attempt++ {
			// Get backend from pool
			backend := route.Backend.Select(req.RemoteAddr)
			if backend == nil {
				break
			}

			// Skip if already tried
			if triedBackends[backend.Address] {
				break
			}
			triedBackends[backend.Address] = true

			// Record request (increments active connections)
			route.Backend.RecordRequest(backend.Address)

			// Log the match
			r.logger.Debug("Route matched",
				"host", host,
				"path", path,
				"backend", backend.Address,
				"container", route.ContainerName,
				"attempt", attempt+1,
			)

			// Proxy the request
			err := r.proxy.ServeHTTP(w, req, backend.Address)

			// Decrement active connections
			route.Backend.CompleteRequest(backend.Address)

			if err == nil {
				return
			}

			// Record failure
			r.logger.Warn("Proxy error",
				"error", err,
				"backend", backend.Address,
				"path", path,
				"attempt", attempt+1,
			)
			route.Backend.RecordFailure(backend.Address)
			route.Backend.MarkUnhealthy(backend.Address)

			// The proxy's error handler already wrote the error response
			// to the ResponseWriter, so we cannot retry with the same writer.
			// Log the failure and return -- the client already received
			// the error page from the first failed attempt.
			return
		}

		// No backends were available to try
		r.handleNoBackend(w, req, route)
	})
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

// CleanupRoute cleans up middleware state for a route
func (r *Router) CleanupRoute(routeID string) {
	r.middlewareBuilder.RemoveRateLimiter(routeID)
	r.middlewareBuilder.RemoveCircuitBreaker(routeID)
}

// buildErrorPage generates a branded error page
func buildErrorPage(code int, title, message, requestID string) string {
	safeTitle := html.EscapeString(title)
	safeMessage := html.EscapeString(message)
	safeRequestID := html.EscapeString(requestID)

	return `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>` + intToStr(code) + ` ` + safeTitle + `</title>
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
        <div class="message">` + safeTitle + `</div>
        <div class="message">` + safeMessage + `</div>
        ` + func() string {
		if safeRequestID != "" {
			return `<div class="request-id">Request ID: ` + safeRequestID + `</div>`
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
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	var s []byte
	for n > 0 {
		s = append([]byte{byte('0' + n%10)}, s...)
		n /= 10
	}
	if neg {
		s = append([]byte{'-'}, s...)
	}
	return string(s)
}
