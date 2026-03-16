# DockRouter — IMPLEMENTATION.md

> Technical implementation guide for Claude Code development sessions.  
> **Read SPECIFICATION.md first** for full feature descriptions.

---

## Table of Contents

1. [Build & Toolchain](#1-build--toolchain)
2. [Docker Socket Client](#2-docker-socket-client)
3. [Event Stream & Reconciliation](#3-event-stream--reconciliation)
4. [Label Parser](#4-label-parser)
5. [Routing Engine Internals](#5-routing-engine-internals)
6. [Reverse Proxy Core](#6-reverse-proxy-core)
7. [ACME Client Implementation](#7-acme-client-implementation)
8. [Middleware System](#8-middleware-system)
9. [Rate Limiter Details](#9-rate-limiter-details)
10. [Health Checker Implementation](#10-health-checker-implementation)
11. [Admin Server & Dashboard](#11-admin-server--dashboard)
12. [Metrics Collection](#12-metrics-collection)
13. [Graceful Shutdown](#13-graceful-shutdown)
14. [Testing Strategy](#14-testing-strategy)
15. [Error Handling Patterns](#15-error-handling-patterns)

---

## 1. Build & Toolchain

### Go Version & Module

```
go 1.22
module github.com/DockRouter/dockrouter
```

**Zero external dependencies** — `go.sum` should remain empty. Only Go stdlib packages.

### Build Commands

```makefile
VERSION := $(shell git describe --tags --always --dirty)
LDFLAGS := -s -w -X main.version=$(VERSION)

build:
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/dockrouter ./cmd/dockrouter

build-all:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/dockrouter-linux-amd64 ./cmd/dockrouter
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/dockrouter-linux-arm64 ./cmd/dockrouter
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/dockrouter-darwin-amd64 ./cmd/dockrouter
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/dockrouter-darwin-arm64 ./cmd/dockrouter

docker:
	docker build -t ghcr.io/dockrouter/dockrouter:$(VERSION) .
	docker tag ghcr.io/dockrouter/dockrouter:$(VERSION) ghcr.io/dockrouter/dockrouter:latest

test:
	go test -v -race -count=1 ./...

lint:
	go vet ./...
```

### Binary Embedding

Dashboard files are embedded at compile time:

```go
// dashboard/embed.go
package dashboard

import "embed"

//go:embed index.html app.js style.css
var Assets embed.FS
```

---

## 2. Docker Socket Client

### Raw HTTP Over Unix Socket

No Docker SDK. We implement a minimal HTTP client over the Unix socket.

```go
// internal/discovery/docker.go
package discovery

import (
    "bufio"
    "context"
    "encoding/json"
    "fmt"
    "net"
    "net/http"
    "net/url"
    "time"
)

const (
    defaultDockerSocket = "/var/run/docker.sock"
    dockerAPIVersion    = "v1.44"
)

type DockerClient struct {
    socketPath string
    httpClient *http.Client
}

func NewDockerClient(socketPath string) *DockerClient {
    if socketPath == "" {
        socketPath = defaultDockerSocket
    }
    
    transport := &http.Transport{
        DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
            return net.DialTimeout("unix", socketPath, 5*time.Second)
        },
    }
    
    return &DockerClient{
        socketPath: socketPath,
        httpClient: &http.Client{
            Transport: transport,
            Timeout:   30 * time.Second,
        },
    }
}

// ListContainers returns all running containers with dr.enable=true label
func (c *DockerClient) ListContainers(ctx context.Context) ([]Container, error) {
    filters := url.QueryEscape(`{"label":["dr.enable=true"],"status":["running"]}`)
    path := fmt.Sprintf("/%s/containers/json?filters=%s", dockerAPIVersion, filters)
    
    req, err := http.NewRequestWithContext(ctx, "GET", "http://localhost"+path, nil)
    if err != nil {
        return nil, err
    }
    
    resp, err := c.httpClient.Do(req)
    if err != nil {
        return nil, fmt.Errorf("docker api: %w", err)
    }
    defer resp.Body.Close()
    
    var containers []Container
    if err := json.NewDecoder(resp.Body).Decode(&containers); err != nil {
        return nil, fmt.Errorf("docker api decode: %w", err)
    }
    
    return containers, nil
}

// InspectContainer returns detailed info about a specific container
func (c *DockerClient) InspectContainer(ctx context.Context, id string) (*ContainerDetail, error) {
    path := fmt.Sprintf("/%s/containers/%s/json", dockerAPIVersion, id)
    
    req, err := http.NewRequestWithContext(ctx, "GET", "http://localhost"+path, nil)
    if err != nil {
        return nil, err
    }
    
    resp, err := c.httpClient.Do(req)
    if err != nil {
        return nil, fmt.Errorf("docker inspect: %w", err)
    }
    defer resp.Body.Close()
    
    var detail ContainerDetail
    if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
        return nil, fmt.Errorf("docker inspect decode: %w", err)
    }
    
    return &detail, nil
}

// Container represents the minimal container info from list API
type Container struct {
    ID      string            `json:"Id"`
    Names   []string          `json:"Names"`
    Labels  map[string]string `json:"Labels"`
    State   string            `json:"State"`
    Status  string            `json:"Status"`
    Ports   []PortBinding     `json:"Ports"`
    NetworkSettings struct {
        Networks map[string]NetworkInfo `json:"Networks"`
    } `json:"NetworkSettings"`
}

type PortBinding struct {
    IP          string `json:"IP"`
    PrivatePort int    `json:"PrivatePort"`
    PublicPort  int    `json:"PublicPort"`
    Type        string `json:"Type"`
}

type NetworkInfo struct {
    IPAddress string `json:"IPAddress"`
    Gateway   string `json:"Gateway"`
    NetworkID string `json:"NetworkID"`
}

type ContainerDetail struct {
    ID     string `json:"Id"`
    Name   string `json:"Name"`
    Config struct {
        Labels       map[string]string `json:"Labels"`
        ExposedPorts map[string]struct{} `json:"ExposedPorts"`
    } `json:"Config"`
    NetworkSettings struct {
        Networks map[string]NetworkInfo `json:"Networks"`
    } `json:"NetworkSettings"`
    State struct {
        Status  string `json:"Status"`
        Running bool   `json:"Running"`
        Health  *struct {
            Status string `json:"Status"`
        } `json:"Health"`
    } `json:"State"`
}
```

### Resolving Container Address

```go
// ResolveAddress determines how to reach the container
func ResolveAddress(detail *ContainerDetail, labelPort string) (string, error) {
    // 1. Explicit address from label
    if addr, ok := detail.Config.Labels["dr.address"]; ok {
        return addr, nil
    }
    
    // 2. Determine port
    port := labelPort
    if port == "" {
        port = detectPort(detail)
    }
    if port == "" {
        return "", fmt.Errorf("cannot determine port for container %s", detail.ID[:12])
    }
    
    // 3. Find IP on shared network
    for _, net := range detail.NetworkSettings.Networks {
        if net.IPAddress != "" {
            return net.IPAddress + ":" + port, nil
        }
    }
    
    return "", fmt.Errorf("cannot resolve address for container %s", detail.ID[:12])
}

// detectPort tries to figure out the container's service port
func detectPort(detail *ContainerDetail) string {
    // Check ExposedPorts (from Dockerfile EXPOSE)
    for portProto := range detail.Config.ExposedPorts {
        // portProto format: "3000/tcp"
        parts := strings.SplitN(portProto, "/", 2)
        if len(parts) > 0 {
            return parts[0]
        }
    }
    return ""
}
```

---

## 3. Event Stream & Reconciliation

### Event Stream Handler

```go
// internal/discovery/events.go
package discovery

import (
    "bufio"
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "net/url"
    "time"
)

type DockerEvent struct {
    Type   string `json:"Type"`   // "container"
    Action string `json:"Action"` // "start", "stop", "die", "health_status"
    Actor  struct {
        ID         string            `json:"ID"`
        Attributes map[string]string `json:"Attributes"`
    } `json:"Actor"`
    Time int64 `json:"time"`
}

// WatchEvents opens a streaming connection to Docker events API
// It automatically reconnects on failure with exponential backoff
func (c *DockerClient) WatchEvents(ctx context.Context, handler func(DockerEvent)) error {
    backoff := time.Second
    maxBackoff := 30 * time.Second
    
    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        default:
        }
        
        err := c.streamEvents(ctx, handler)
        if err != nil {
            if ctx.Err() != nil {
                return ctx.Err()
            }
            
            log.Warn("docker event stream disconnected, reconnecting",
                "error", err, "backoff", backoff)
            
            select {
            case <-time.After(backoff):
                backoff = min(backoff*2, maxBackoff)
            case <-ctx.Done():
                return ctx.Err()
            }
            continue
        }
        
        // Reset backoff on clean disconnect
        backoff = time.Second
    }
}

func (c *DockerClient) streamEvents(ctx context.Context, handler func(DockerEvent)) error {
    filters := url.QueryEscape(`{"type":["container"],"event":["start","stop","die","kill","health_status"]}`)
    path := fmt.Sprintf("/%s/events?filters=%s", dockerAPIVersion, filters)
    
    // Use a transport without timeout for streaming
    transport := &http.Transport{
        DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
            return net.DialTimeout("unix", c.socketPath, 5*time.Second)
        },
    }
    streamClient := &http.Client{Transport: transport}
    
    req, err := http.NewRequestWithContext(ctx, "GET", "http://localhost"+path, nil)
    if err != nil {
        return err
    }
    
    resp, err := streamClient.Do(req)
    if err != nil {
        return fmt.Errorf("event stream connect: %w", err)
    }
    defer resp.Body.Close()
    
    scanner := bufio.NewScanner(resp.Body)
    for scanner.Scan() {
        var event DockerEvent
        if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
            continue // skip malformed events
        }
        
        // Only process containers with dr.enable label
        if event.Actor.Attributes["dr.enable"] != "true" {
            continue
        }
        
        handler(event)
    }
    
    return scanner.Err()
}
```

### Reconciler

The reconciler keeps the route table in sync with actual Docker state:

```go
// internal/discovery/reconciler.go
package discovery

type Reconciler struct {
    docker     *DockerClient
    routeTable *router.RouteTable
    certMgr    *tls.Manager
    healthChk  *health.Checker
    logger     *log.Logger
}

// FullSync lists all running containers and rebuilds the route table
// Called on startup and on event stream reconnect
func (r *Reconciler) FullSync(ctx context.Context) error {
    containers, err := r.docker.ListContainers(ctx)
    if err != nil {
        return fmt.Errorf("full sync: %w", err)
    }
    
    // Build desired state from Docker
    desired := make(map[string]*router.Route)
    for _, c := range containers {
        detail, err := r.docker.InspectContainer(ctx, c.ID)
        if err != nil {
            r.logger.Warn("cannot inspect container", "id", c.ID[:12], "error", err)
            continue
        }
        
        route, err := r.buildRoute(detail)
        if err != nil {
            r.logger.Warn("cannot build route", "id", c.ID[:12], "error", err)
            continue
        }
        
        desired[route.ID] = route
    }
    
    // Diff with current state
    current := r.routeTable.All()
    
    // Add new routes
    for id, route := range desired {
        if _, exists := current[id]; !exists {
            r.addRoute(ctx, route)
        }
    }
    
    // Remove stale routes
    for id := range current {
        if _, exists := desired[id]; !exists {
            r.removeRoute(id)
        }
    }
    
    r.logger.Info("full sync complete", "routes", len(desired))
    return nil
}

// HandleEvent processes a single Docker event
func (r *Reconciler) HandleEvent(ctx context.Context, event DockerEvent) {
    switch event.Action {
    case "start":
        detail, err := r.docker.InspectContainer(ctx, event.Actor.ID)
        if err != nil {
            r.logger.Warn("cannot inspect started container", "id", event.Actor.ID[:12])
            return
        }
        route, err := r.buildRoute(detail)
        if err != nil {
            r.logger.Warn("cannot build route for started container", "id", event.Actor.ID[:12])
            return
        }
        r.addRoute(ctx, route)
        
    case "stop", "die", "kill":
        r.removeRoute(event.Actor.ID[:12])
        
    case "health_status":
        // Update health state for this container's backend
        status := event.Actor.Attributes["health_status"]
        r.healthChk.UpdateDockerHealth(event.Actor.ID[:12], status)
    }
}

func (r *Reconciler) addRoute(ctx context.Context, route *router.Route) {
    r.routeTable.Add(route)
    r.logger.Info("route added",
        "host", route.Host,
        "path", route.PathPrefix,
        "backend", route.Backend.Targets[0].Address,
        "container", route.ID)
    
    // Trigger ACME if auto-TLS
    if route.TLS.Mode == "auto" {
        go r.certMgr.EnsureCertificate(ctx, route.Host, route.TLS.Domains)
    }
    
    // Start health checks
    for _, target := range route.Backend.Targets {
        r.healthChk.Register(target.ContainerID, target.Address, route.HealthCheck)
    }
}

func (r *Reconciler) removeRoute(containerID string) {
    removed := r.routeTable.RemoveByContainer(containerID)
    if removed != nil {
        r.logger.Info("route removed",
            "host", removed.Host,
            "container", containerID)
        r.healthChk.Deregister(containerID)
    }
}
```

---

## 4. Label Parser

```go
// internal/discovery/labels.go
package discovery

import (
    "strconv"
    "strings"
    "time"
)

const labelPrefix = "dr."

// ParseLabels converts Docker labels into a RouteConfig
func ParseLabels(labels map[string]string) (*RouteConfig, error) {
    if labels[labelPrefix+"enable"] != "true" {
        return nil, ErrNotEnabled
    }
    
    host := labels[labelPrefix+"host"]
    if host == "" {
        return nil, ErrMissingHost
    }
    
    cfg := &RouteConfig{
        Host:        host,
        Port:        labels[labelPrefix+"port"],
        Path:        labels[labelPrefix+"path"],
        Address:     labels[labelPrefix+"address"],
        TLS:         parseTLSLabels(labels),
        RateLimit:   parseRateLimitLabel(labels[labelPrefix+"ratelimit"]),
        HealthCheck: parseHealthCheckLabels(labels),
        Middlewares: parseMiddlewareLabels(labels),
        Priority:    parseIntLabel(labels[labelPrefix+"priority"], 0),
        Weight:      parseIntLabel(labels[labelPrefix+"weight"], 1),
        LBStrategy:  labels[labelPrefix+"loadbalancer"],
    }
    
    if cfg.Path == "" {
        cfg.Path = "/"
    }
    if cfg.LBStrategy == "" {
        cfg.LBStrategy = "roundrobin"
    }
    
    return cfg, nil
}

type RouteConfig struct {
    Host        string
    Port        string
    Path        string
    Address     string
    TLS         TLSLabelConfig
    RateLimit   *RateLimitConfig
    HealthCheck HealthCheckConfig
    Middlewares MiddlewareConfig
    Priority    int
    Weight      int
    LBStrategy  string
}

type TLSLabelConfig struct {
    Mode    string   // auto, manual, off
    Domains []string
    Cert    string
    Key     string
}

func parseTLSLabels(labels map[string]string) TLSLabelConfig {
    cfg := TLSLabelConfig{
        Mode: labels[labelPrefix+"tls"],
    }
    if cfg.Mode == "" {
        cfg.Mode = "auto" // default to auto
    }
    
    if domains := labels[labelPrefix+"tls.domains"]; domains != "" {
        cfg.Domains = strings.Split(domains, ",")
        for i := range cfg.Domains {
            cfg.Domains[i] = strings.TrimSpace(cfg.Domains[i])
        }
    }
    
    cfg.Cert = labels[labelPrefix+"tls.cert"]
    cfg.Key = labels[labelPrefix+"tls.key"]
    
    return cfg
}

type RateLimitConfig struct {
    Count  int
    Window time.Duration
    By     string // "client_ip", header name
}

// parseRateLimitLabel parses "100/m" format
func parseRateLimitLabel(val string) *RateLimitConfig {
    if val == "" {
        return nil
    }
    
    parts := strings.SplitN(val, "/", 2)
    if len(parts) != 2 {
        return nil
    }
    
    count, err := strconv.Atoi(parts[0])
    if err != nil {
        return nil
    }
    
    var window time.Duration
    switch parts[1] {
    case "s":
        window = time.Second
    case "m":
        window = time.Minute
    case "h":
        window = time.Hour
    default:
        return nil
    }
    
    return &RateLimitConfig{
        Count:  count,
        Window: window,
        By:     "client_ip",
    }
}

func parseHealthCheckLabels(labels map[string]string) HealthCheckConfig {
    return HealthCheckConfig{
        Path:      getOrDefault(labels, labelPrefix+"healthcheck.path", "/"),
        Interval:  parseDurationLabel(labels[labelPrefix+"healthcheck.interval"], 10*time.Second),
        Timeout:   parseDurationLabel(labels[labelPrefix+"healthcheck.timeout"], 5*time.Second),
        Threshold: parseIntLabel(labels[labelPrefix+"healthcheck.threshold"], 3),
        Recovery:  parseIntLabel(labels[labelPrefix+"healthcheck.recovery"], 2),
    }
}

type MiddlewareConfig struct {
    Compress     bool
    CORS         *CORSConfig
    Headers      map[string]string
    RedirectHTTPS bool
    StripPrefix  string
    AddPrefix    string
    MaxBody      string
    BasicAuth    string
    IPWhitelist  []string
    IPBlacklist  []string
    Retry        int
    CircuitBreaker *CircuitBreakerConfig
}

func parseMiddlewareLabels(labels map[string]string) MiddlewareConfig {
    cfg := MiddlewareConfig{
        Compress:      labels[labelPrefix+"compress"] == "true",
        RedirectHTTPS: labels[labelPrefix+"redirect.https"] != "false", // default true
        StripPrefix:   labels[labelPrefix+"stripprefix"],
        AddPrefix:     labels[labelPrefix+"addprefix"],
        MaxBody:       getOrDefault(labels, labelPrefix+"maxbody", "10mb"),
        BasicAuth:     labels[labelPrefix+"auth.basic.users"],
        Retry:         parseIntLabel(labels[labelPrefix+"retry"], 0),
        Headers:       make(map[string]string),
    }
    
    // Parse CORS
    if origins := labels[labelPrefix+"cors.origins"]; origins != "" {
        cfg.CORS = &CORSConfig{
            Origins: strings.Split(origins, ","),
            Methods: strings.Split(getOrDefault(labels, labelPrefix+"cors.methods", "GET,POST,PUT,DELETE"), ","),
            Headers: strings.Split(labels[labelPrefix+"cors.headers"], ","),
        }
    }
    
    // Parse custom headers (dr.headers.X-Custom=value)
    for k, v := range labels {
        if strings.HasPrefix(k, labelPrefix+"headers.") {
            headerName := strings.TrimPrefix(k, labelPrefix+"headers.")
            cfg.Headers[headerName] = v
        }
    }
    
    // Parse IP lists
    if wl := labels[labelPrefix+"ipwhitelist"]; wl != "" {
        cfg.IPWhitelist = strings.Split(wl, ",")
    }
    if bl := labels[labelPrefix+"ipblacklist"]; bl != "" {
        cfg.IPBlacklist = strings.Split(bl, ",")
    }
    
    return cfg
}

// Helper functions
func parseIntLabel(val string, defaultVal int) int {
    if val == "" {
        return defaultVal
    }
    n, err := strconv.Atoi(val)
    if err != nil {
        return defaultVal
    }
    return n
}

func parseDurationLabel(val string, defaultVal time.Duration) time.Duration {
    if val == "" {
        return defaultVal
    }
    d, err := time.ParseDuration(val)
    if err != nil {
        return defaultVal
    }
    return d
}

func getOrDefault(m map[string]string, key, defaultVal string) string {
    if v, ok := m[key]; ok && v != "" {
        return v
    }
    return defaultVal
}
```

---

## 5. Routing Engine Internals

### Radix Tree for Path Matching

```go
// internal/router/radix.go
package router

// RadixNode is a node in the radix tree for fast path prefix matching
type RadixNode struct {
    prefix   string
    route    *Route
    children []*RadixNode
    // Longest prefix wins — deeper nodes have higher specificity
}

// Insert adds a path prefix → route mapping
func (n *RadixNode) Insert(path string, route *Route) {
    // Standard radix tree insertion with prefix splitting
    // ... (full implementation in code)
}

// Match finds the longest matching prefix for a given path
func (n *RadixNode) Match(path string) *Route {
    // Walk the tree, track the deepest match
    // ... (full implementation in code)
}
```

### Route Table with RWMutex

```go
// internal/router/table.go
package router

import "sync"

type RouteTable struct {
    mu       sync.RWMutex
    byHost   map[string]*HostRoutes  // hostname → routes for that host
    catchAll *Route                  // fallback route
}

type HostRoutes struct {
    exact    *Route      // exact path "/" route
    pathTree *RadixNode  // path prefix tree
}

func NewRouteTable() *RouteTable {
    return &RouteTable{
        byHost: make(map[string]*HostRoutes),
    }
}

// Match finds the best route for a request
func (rt *RouteTable) Match(host, path string) *Route {
    rt.mu.RLock()
    defer rt.mu.RUnlock()
    
    // Normalize host
    host = normalizeHost(host)
    
    // 1. Exact host match
    if hr, ok := rt.byHost[host]; ok {
        if route := hr.matchPath(path); route != nil {
            return route
        }
    }
    
    // 2. Wildcard match (*.example.com)
    if route := rt.matchWildcard(host, path); route != nil {
        return route
    }
    
    // 3. Catch-all
    return rt.catchAll
}

// Add adds or merges a route into the table
func (rt *RouteTable) Add(route *Route) {
    rt.mu.Lock()
    defer rt.mu.Unlock()
    
    host := normalizeHost(route.Host)
    
    hr, ok := rt.byHost[host]
    if !ok {
        hr = &HostRoutes{
            pathTree: &RadixNode{},
        }
        rt.byHost[host] = hr
    }
    
    // If same host+path exists, add to backend pool (multi-container LB)
    existing := hr.pathTree.Match(route.PathPrefix)
    if existing != nil && existing.PathPrefix == route.PathPrefix {
        existing.Backend.AddTarget(route.Backend.Targets[0])
        return
    }
    
    hr.pathTree.Insert(route.PathPrefix, route)
}

// RemoveByContainer removes all routes associated with a container ID
func (rt *RouteTable) RemoveByContainer(containerID string) *Route {
    rt.mu.Lock()
    defer rt.mu.Unlock()
    
    // Find and remove (or remove from backend pool if multi-target)
    for _, hr := range rt.byHost {
        if removed := hr.pathTree.RemoveByContainer(containerID); removed != nil {
            return removed
        }
    }
    return nil
}

func normalizeHost(host string) string {
    // Strip port, lowercase
    host = strings.ToLower(host)
    if idx := strings.LastIndex(host, ":"); idx != -1 {
        host = host[:idx]
    }
    return host
}
```

---

## 6. Reverse Proxy Core

### httputil.ReverseProxy Wrapper

We use Go's `net/http/httputil.ReverseProxy` as the base but wrap it for our needs:

```go
// internal/proxy/proxy.go
package proxy

import (
    "net"
    "net/http"
    "net/http/httputil"
    "time"
)

type ReverseProxy struct {
    transport    *http.Transport
    bufferPool   httputil.BufferPool
    errorHandler func(http.ResponseWriter, *http.Request, error)
}

func New() *ReverseProxy {
    transport := &http.Transport{
        DialContext: (&net.Dialer{
            Timeout:   5 * time.Second,
            KeepAlive: 30 * time.Second,
        }).DialContext,
        MaxIdleConns:          1000,
        MaxIdleConnsPerHost:   100,
        IdleConnTimeout:       90 * time.Second,
        TLSHandshakeTimeout:  5 * time.Second,
        ResponseHeaderTimeout: 30 * time.Second,
        ForceAttemptHTTP2:     false, // backends are HTTP/1.1
    }
    
    return &ReverseProxy{
        transport:  transport,
        bufferPool: newBufferPool(),
    }
}

// ProxyRequest creates an httputil.ReverseProxy for a specific backend target
func (rp *ReverseProxy) ServeHTTP(w http.ResponseWriter, r *http.Request, target string) {
    proxy := &httputil.ReverseProxy{
        Director: func(req *http.Request) {
            req.URL.Scheme = "http"
            req.URL.Host = target
            req.Host = r.Host // preserve original Host header
            
            // Set forwarding headers
            clientIP, _, _ := net.SplitHostPort(r.RemoteAddr)
            req.Header.Set("X-Forwarded-For", clientIP)
            req.Header.Set("X-Forwarded-Proto", scheme(r))
            req.Header.Set("X-Forwarded-Host", r.Host)
            req.Header.Set("X-Real-IP", clientIP)
        },
        Transport:    rp.transport,
        BufferPool:   rp.bufferPool,
        ErrorHandler: rp.handleProxyError,
    }
    
    proxy.ServeHTTP(w, r)
}

func scheme(r *http.Request) string {
    if r.TLS != nil {
        return "https"
    }
    return "http"
}
```

### WebSocket Support

WebSocket upgrades are handled transparently by `httputil.ReverseProxy` — the `Upgrade: websocket` and `Connection: Upgrade` headers are forwarded, and the connection is hijacked for bidirectional streaming. No special handling needed beyond ensuring hop-by-hop headers aren't stripped for WebSocket requests.

### Buffer Pool

```go
// internal/proxy/bufferpool.go
package proxy

import "sync"

type bufferPool struct {
    pool sync.Pool
}

func newBufferPool() *bufferPool {
    return &bufferPool{
        pool: sync.Pool{
            New: func() interface{} {
                buf := make([]byte, 32*1024) // 32KB buffers
                return &buf
            },
        },
    }
}

func (bp *bufferPool) Get() []byte {
    return *bp.pool.Get().(*[]byte)
}

func (bp *bufferPool) Put(buf []byte) {
    bp.pool.Put(&buf)
}
```

### Error Pages

```go
// internal/proxy/errorpage.go
package proxy

import (
    "fmt"
    "net/http"
)

var errorPages = map[int]string{
    502: "Bad Gateway — DockRouter cannot reach the backend service.",
    503: "Service Unavailable — All backends are unhealthy.",
    504: "Gateway Timeout — Backend did not respond in time.",
    429: "Too Many Requests — Rate limit exceeded.",
}

func ServeErrorPage(w http.ResponseWriter, statusCode int, requestID string) {
    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    w.Header().Set("X-Request-Id", requestID)
    w.WriteHeader(statusCode)
    
    msg := errorPages[statusCode]
    if msg == "" {
        msg = http.StatusText(statusCode)
    }
    
    // Minimal branded HTML error page
    fmt.Fprintf(w, `<!DOCTYPE html>
<html><head><title>%d — DockRouter</title>
<style>
body{font-family:system-ui,sans-serif;display:flex;align-items:center;justify-content:center;min-height:100vh;margin:0;background:#0f172a;color:#e2e8f0}
.box{text-align:center;padding:2rem}
h1{font-size:4rem;margin:0;color:#f97316}
p{font-size:1.1rem;opacity:.8}
code{font-size:.75rem;opacity:.5}
</style></head>
<body><div class="box">
<h1>%d</h1>
<p>%s</p>
<code>Request ID: %s</code>
</div></body></html>`, statusCode, statusCode, msg, requestID)
}
```

---

## 7. ACME Client Implementation

### ACME Protocol Flow (Pure Go)

```go
// internal/tls/acme.go
package tls

import (
    "crypto/ecdsa"
    "crypto/elliptic"
    "crypto/rand"
    "crypto/x509"
    "crypto/x509/pkix"
    "encoding/json"
    "encoding/pem"
    "fmt"
    "net/http"
)

const (
    letsEncryptProd    = "https://acme-v02.api.letsencrypt.org/directory"
    letsEncryptStaging = "https://acme-staging-v02.api.letsencrypt.org/directory"
)

type ACMEClient struct {
    directoryURL string
    directory    *ACMEDirectory
    accountKey   *ecdsa.PrivateKey
    accountURL   string
    httpClient   *http.Client
}

type ACMEDirectory struct {
    NewNonce   string `json:"newNonce"`
    NewAccount string `json:"newAccount"`
    NewOrder   string `json:"newOrder"`
    RevokeCert string `json:"revokeCert"`
}

// NewACMEClient creates an ACME client
// directoryURL: production or staging Let's Encrypt URL
func NewACMEClient(directoryURL string) (*ACMEClient, error) {
    client := &ACMEClient{
        directoryURL: directoryURL,
        httpClient:   &http.Client{Timeout: 30 * time.Second},
    }
    
    // Fetch directory
    resp, err := client.httpClient.Get(directoryURL)
    if err != nil {
        return nil, fmt.Errorf("acme directory: %w", err)
    }
    defer resp.Body.Close()
    
    if err := json.NewDecoder(resp.Body).Decode(&client.directory); err != nil {
        return nil, fmt.Errorf("acme directory decode: %w", err)
    }
    
    return client, nil
}

// ObtainCertificate performs the full ACME flow for a domain
func (c *ACMEClient) ObtainCertificate(ctx context.Context, domains []string, solver ChallengeSolver) (*CertBundle, error) {
    // 1. Get fresh nonce
    nonce, err := c.getNonce()
    if err != nil {
        return nil, err
    }
    
    // 2. Create order
    order, err := c.createOrder(nonce, domains)
    if err != nil {
        return nil, err
    }
    
    // 3. Process authorizations
    for _, authzURL := range order.Authorizations {
        authz, err := c.getAuthorization(authzURL)
        if err != nil {
            return nil, err
        }
        
        // Find HTTP-01 challenge
        var challenge *ACMEChallenge
        for i, ch := range authz.Challenges {
            if ch.Type == "http-01" {
                challenge = &authz.Challenges[i]
                break
            }
        }
        if challenge == nil {
            return nil, fmt.Errorf("no http-01 challenge for %s", authz.Identifier.Value)
        }
        
        // Solve challenge
        keyAuth := c.keyAuthorization(challenge.Token)
        solver.Present(challenge.Token, keyAuth)
        defer solver.CleanUp(challenge.Token)
        
        // Notify ACME server
        if err := c.respondChallenge(challenge.URL); err != nil {
            return nil, err
        }
        
        // Poll until valid
        if err := c.waitForValid(authzURL); err != nil {
            return nil, err
        }
    }
    
    // 4. Generate CSR
    certKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
    if err != nil {
        return nil, err
    }
    
    csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
        Subject: pkix.Name{CommonName: domains[0]},
        DNSNames: domains,
    }, certKey)
    if err != nil {
        return nil, err
    }
    
    // 5. Finalize order
    cert, err := c.finalizeOrder(order.FinalizeURL, csr)
    if err != nil {
        return nil, err
    }
    
    return &CertBundle{
        Certificate: cert,
        PrivateKey:  certKey,
        Domains:     domains,
    }, nil
}
```

### HTTP-01 Challenge Solver

```go
// internal/tls/challenge.go
package tls

import (
    "net/http"
    "sync"
)

type ChallengeSolver interface {
    Present(token, keyAuth string) error
    CleanUp(token string) error
}

// HTTP01Solver serves ACME challenges on :80 /.well-known/acme-challenge/
type HTTP01Solver struct {
    mu     sync.RWMutex
    tokens map[string]string // token → keyAuthorization
}

func NewHTTP01Solver() *HTTP01Solver {
    return &HTTP01Solver{
        tokens: make(map[string]string),
    }
}

func (s *HTTP01Solver) Present(token, keyAuth string) error {
    s.mu.Lock()
    s.tokens[token] = keyAuth
    s.mu.Unlock()
    return nil
}

func (s *HTTP01Solver) CleanUp(token string) error {
    s.mu.Lock()
    delete(s.tokens, token)
    s.mu.Unlock()
    return nil
}

// ServeHTTP handles /.well-known/acme-challenge/ requests
// This is registered on the :80 HTTP listener with highest priority
func (s *HTTP01Solver) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    token := strings.TrimPrefix(r.URL.Path, "/.well-known/acme-challenge/")
    
    s.mu.RLock()
    keyAuth, ok := s.tokens[token]
    s.mu.RUnlock()
    
    if !ok {
        http.NotFound(w, r)
        return
    }
    
    w.Header().Set("Content-Type", "text/plain")
    w.Write([]byte(keyAuth))
}
```

### Certificate Store & Manager

```go
// internal/tls/manager.go
package tls

import (
    "crypto/tls"
    "sync"
)

type Manager struct {
    mu         sync.RWMutex
    certs      map[string]*tls.Certificate // domain → cert
    store      *CertStore
    acme       *ACMEClient
    solver     *HTTP01Solver
    acmeEmail  string
}

func NewManager(dataDir, acmeEmail, provider string, staging bool) (*Manager, error) {
    dirURL := letsEncryptProd
    if staging {
        dirURL = letsEncryptStaging
    }
    
    acme, err := NewACMEClient(dirURL)
    if err != nil {
        return nil, err
    }
    
    return &Manager{
        certs:     make(map[string]*tls.Certificate),
        store:     NewCertStore(dataDir),
        acme:      acme,
        solver:    NewHTTP01Solver(),
        acmeEmail: acmeEmail,
    }, nil
}

// GetCertificate is the tls.Config callback for per-SNI cert selection
func (m *Manager) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
    m.mu.RLock()
    cert, ok := m.certs[hello.ServerName]
    m.mu.RUnlock()
    
    if ok {
        return cert, nil
    }
    
    // Try loading from disk
    cert, err := m.store.Load(hello.ServerName)
    if err == nil {
        m.mu.Lock()
        m.certs[hello.ServerName] = cert
        m.mu.Unlock()
        return cert, nil
    }
    
    // Not found — return nil, will fall through to default cert or error
    return nil, fmt.Errorf("no certificate for %s", hello.ServerName)
}

// EnsureCertificate provisions a certificate if needed
func (m *Manager) EnsureCertificate(ctx context.Context, host string, extraDomains []string) error {
    domains := []string{host}
    domains = append(domains, extraDomains...)
    
    // Check if we already have a valid cert
    if m.store.IsValid(host) {
        return nil
    }
    
    // Obtain new certificate
    bundle, err := m.acme.ObtainCertificate(ctx, domains, m.solver)
    if err != nil {
        return fmt.Errorf("acme obtain: %w", err)
    }
    
    // Store to disk
    if err := m.store.Save(host, bundle); err != nil {
        return fmt.Errorf("cert store: %w", err)
    }
    
    // Load into memory
    tlsCert, err := tls.X509KeyPair(bundle.CertPEM, bundle.KeyPEM)
    if err != nil {
        return err
    }
    
    m.mu.Lock()
    m.certs[host] = &tlsCert
    m.mu.Unlock()
    
    return nil
}
```

---

## 8. Middleware System

### Chain Builder

```go
// internal/middleware/chain.go
package middleware

import "net/http"

type Middleware func(http.Handler) http.Handler

// Chain builds a middleware chain, executing in order
func Chain(handler http.Handler, middlewares ...Middleware) http.Handler {
    // Apply in reverse so first middleware in list executes first
    for i := len(middlewares) - 1; i >= 0; i-- {
        handler = middlewares[i](handler)
    }
    return handler
}

// BuildChain creates the middleware chain for a specific route based on its config
func BuildChain(cfg *MiddlewareConfig, rl *RateLimiter) []Middleware {
    var chain []Middleware
    
    // Always-on middlewares
    chain = append(chain, RecoveryMiddleware())
    chain = append(chain, RequestIDMiddleware())
    chain = append(chain, AccessLogMiddleware())
    
    // Conditional middlewares
    if cfg.RateLimit != nil {
        chain = append(chain, RateLimitMiddleware(rl, cfg.RateLimit))
    }
    
    if len(cfg.IPWhitelist) > 0 {
        chain = append(chain, IPFilterMiddleware(cfg.IPWhitelist, true))
    }
    if len(cfg.IPBlacklist) > 0 {
        chain = append(chain, IPFilterMiddleware(cfg.IPBlacklist, false))
    }
    
    if cfg.CORS != nil {
        chain = append(chain, CORSMiddleware(cfg.CORS))
    }
    
    chain = append(chain, SecurityHeadersMiddleware())
    
    if cfg.Compress {
        chain = append(chain, CompressMiddleware())
    }
    
    if cfg.StripPrefix != "" {
        chain = append(chain, StripPrefixMiddleware(cfg.StripPrefix))
    }
    if cfg.AddPrefix != "" {
        chain = append(chain, AddPrefixMiddleware(cfg.AddPrefix))
    }
    
    for name, value := range cfg.Headers {
        chain = append(chain, CustomHeaderMiddleware(name, value))
    }
    
    return chain
}
```

---

## 9. Rate Limiter Details

### Token Bucket Implementation

```go
// internal/middleware/ratelimit.go
package middleware

import (
    "net/http"
    "sync"
    "sync/atomic"
    "time"
)

type RateLimiter struct {
    mu      sync.RWMutex
    buckets map[string]*tokenBucket
    
    // GC tracking
    lastGC  time.Time
    gcEvery time.Duration
    maxSize int
}

type tokenBucket struct {
    tokens     float64
    maxTokens  float64
    refillRate float64 // tokens per second
    lastRefill time.Time
    lastAccess time.Time
    mu         sync.Mutex
}

func NewRateLimiter(maxBuckets int) *RateLimiter {
    rl := &RateLimiter{
        buckets: make(map[string]*tokenBucket),
        lastGC:  time.Now(),
        gcEvery: 60 * time.Second,
        maxSize: maxBuckets,
    }
    
    // Background GC
    go rl.gcLoop()
    
    return rl
}

// Allow checks if a request is allowed under the rate limit
func (rl *RateLimiter) Allow(key string, limit int, window time.Duration) (allowed bool, remaining int, resetAt time.Time) {
    bucket := rl.getBucket(key, limit, window)
    
    bucket.mu.Lock()
    defer bucket.mu.Unlock()
    
    now := time.Now()
    bucket.lastAccess = now
    
    // Refill tokens based on elapsed time
    elapsed := now.Sub(bucket.lastRefill).Seconds()
    bucket.tokens += elapsed * bucket.refillRate
    if bucket.tokens > bucket.maxTokens {
        bucket.tokens = bucket.maxTokens
    }
    bucket.lastRefill = now
    
    if bucket.tokens >= 1 {
        bucket.tokens--
        return true, int(bucket.tokens), now.Add(time.Duration(1/bucket.refillRate) * time.Second)
    }
    
    // Calculate when next token is available
    waitTime := time.Duration((1 - bucket.tokens) / bucket.refillRate * float64(time.Second))
    return false, 0, now.Add(waitTime)
}

func (rl *RateLimiter) getBucket(key string, limit int, window time.Duration) *tokenBucket {
    rl.mu.RLock()
    b, ok := rl.buckets[key]
    rl.mu.RUnlock()
    
    if ok {
        return b
    }
    
    rl.mu.Lock()
    defer rl.mu.Unlock()
    
    // Double-check after write lock
    if b, ok := rl.buckets[key]; ok {
        return b
    }
    
    refillRate := float64(limit) / window.Seconds()
    b = &tokenBucket{
        tokens:     float64(limit),
        maxTokens:  float64(limit),
        refillRate: refillRate,
        lastRefill: time.Now(),
        lastAccess: time.Now(),
    }
    rl.buckets[key] = b
    return b
}

func (rl *RateLimiter) gcLoop() {
    ticker := time.NewTicker(rl.gcEvery)
    defer ticker.Stop()
    
    for range ticker.C {
        rl.gc()
    }
}

func (rl *RateLimiter) gc() {
    rl.mu.Lock()
    defer rl.mu.Unlock()
    
    cutoff := time.Now().Add(-5 * time.Minute) // remove buckets idle for 5 min
    for key, bucket := range rl.buckets {
        bucket.mu.Lock()
        idle := bucket.lastAccess.Before(cutoff)
        bucket.mu.Unlock()
        
        if idle {
            delete(rl.buckets, key)
        }
    }
}

// RateLimitMiddleware creates the HTTP middleware
func RateLimitMiddleware(rl *RateLimiter, cfg *RateLimitConfig) Middleware {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            // Determine rate limit key
            key := clientIP(r) // default: per-IP
            if cfg.By != "client_ip" {
                key = r.Header.Get(cfg.By)
            }
            
            allowed, remaining, resetAt := rl.Allow(key, cfg.Count, cfg.Window)
            
            // Set rate limit headers
            w.Header().Set("X-RateLimit-Limit", strconv.Itoa(cfg.Count))
            w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
            w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(resetAt.Unix(), 10))
            
            if !allowed {
                w.Header().Set("Retry-After", strconv.Itoa(int(time.Until(resetAt).Seconds())+1))
                ServeErrorPage(w, http.StatusTooManyRequests, requestID(r))
                return
            }
            
            next.ServeHTTP(w, r)
        })
    }
}
```

---

## 10. Health Checker Implementation

```go
// internal/health/checker.go
package health

import (
    "context"
    "net"
    "net/http"
    "sync"
    "time"
)

type State int

const (
    StateHealthy     State = iota
    StateDegraded
    StateUnhealthy
    StateRecovering
)

type Checker struct {
    mu      sync.RWMutex
    targets map[string]*target // containerID → target
    client  *http.Client
    notify  func(containerID string, healthy bool) // callback to update route table
}

type target struct {
    containerID    string
    address        string
    config         HealthCheckConfig
    state          State
    failCount      int
    successCount   int
    lastCheck      time.Time
    cancel         context.CancelFunc
}

func NewChecker(notify func(string, bool)) *Checker {
    return &Checker{
        targets: make(map[string]*target),
        client: &http.Client{
            Timeout: 5 * time.Second,
            CheckRedirect: func(req *http.Request, via []*http.Request) error {
                return http.ErrUseLastResponse // don't follow redirects
            },
        },
        notify: notify,
    }
}

func (c *Checker) Register(containerID, address string, cfg HealthCheckConfig) {
    c.mu.Lock()
    defer c.mu.Unlock()
    
    ctx, cancel := context.WithCancel(context.Background())
    
    t := &target{
        containerID: containerID,
        address:     address,
        config:      cfg,
        state:       StateHealthy, // assume healthy on registration
        cancel:      cancel,
    }
    
    c.targets[containerID] = t
    go c.checkLoop(ctx, t)
}

func (c *Checker) Deregister(containerID string) {
    c.mu.Lock()
    defer c.mu.Unlock()
    
    if t, ok := c.targets[containerID]; ok {
        t.cancel()
        delete(c.targets, containerID)
    }
}

func (c *Checker) checkLoop(ctx context.Context, t *target) {
    ticker := time.NewTicker(t.config.Interval)
    defer ticker.Stop()
    
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            healthy := c.doCheck(t)
            c.updateState(t, healthy)
        }
    }
}

func (c *Checker) doCheck(t *target) bool {
    url := "http://" + t.address + t.config.Path
    
    ctx, cancel := context.WithTimeout(context.Background(), t.config.Timeout)
    defer cancel()
    
    req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
    if err != nil {
        return false
    }
    req.Header.Set("User-Agent", "DockRouter/HealthCheck")
    
    resp, err := c.client.Do(req)
    if err != nil {
        return false
    }
    defer resp.Body.Close()
    
    return resp.StatusCode >= 200 && resp.StatusCode < 400
}

func (c *Checker) updateState(t *target, healthy bool) {
    t.lastCheck = time.Now()
    
    if healthy {
        t.successCount++
        t.failCount = 0
        
        switch t.state {
        case StateUnhealthy, StateRecovering:
            t.state = StateRecovering
            if t.successCount >= t.config.Recovery {
                t.state = StateHealthy
                c.notify(t.containerID, true)
            }
        case StateDegraded:
            t.state = StateHealthy
        }
    } else {
        t.failCount++
        t.successCount = 0
        
        switch t.state {
        case StateHealthy:
            t.state = StateDegraded
        case StateDegraded:
            if t.failCount >= t.config.Threshold {
                t.state = StateUnhealthy
                c.notify(t.containerID, false)
            }
        }
    }
}
```

---

## 11. Admin Server & Dashboard

### Server Setup

```go
// internal/admin/server.go
package admin

import (
    "embed"
    "net/http"
)

type Server struct {
    mux         *http.ServeMux
    routeTable  *router.RouteTable
    certMgr     *tls.Manager
    healthChk   *health.Checker
    metrics     *metrics.Collector
    sseHub      *SSEHub
    config      AdminConfig
}

type AdminConfig struct {
    Bind     string
    Port     int
    Username string
    Password string
    Enabled  bool
}

func NewServer(cfg AdminConfig, rt *router.RouteTable, cm *tls.Manager, hc *health.Checker, mc *metrics.Collector) *Server {
    s := &Server{
        mux:        http.NewServeMux(),
        routeTable: rt,
        certMgr:    cm,
        healthChk:  hc,
        metrics:    mc,
        sseHub:     NewSSEHub(),
        config:     cfg,
    }
    
    s.registerRoutes()
    return s
}

func (s *Server) registerRoutes() {
    // API endpoints
    s.mux.HandleFunc("GET /api/v1/status", s.handleStatus)
    s.mux.HandleFunc("GET /api/v1/routes", s.handleRoutes)
    s.mux.HandleFunc("GET /api/v1/routes/{id}", s.handleRouteDetail)
    s.mux.HandleFunc("GET /api/v1/containers", s.handleContainers)
    s.mux.HandleFunc("GET /api/v1/certificates", s.handleCertificates)
    s.mux.HandleFunc("GET /api/v1/certificates/{domain}", s.handleCertDetail)
    s.mux.HandleFunc("POST /api/v1/certificates/{domain}/renew", s.handleCertRenew)
    s.mux.HandleFunc("GET /api/v1/metrics", s.handleMetrics)
    s.mux.HandleFunc("GET /api/v1/health", s.handleHealth)
    s.mux.HandleFunc("GET /api/v1/config", s.handleConfig)
    s.mux.HandleFunc("GET /api/v1/events", s.sseHub.ServeHTTP)
    
    // Embedded dashboard (catch-all for SPA)
    s.mux.Handle("/", http.FileServer(http.FS(dashboard.Assets)))
}

func (s *Server) Handler() http.Handler {
    var handler http.Handler = s.mux
    
    // Basic auth if configured
    if s.config.Username != "" {
        handler = basicAuthMiddleware(handler, s.config.Username, s.config.Password)
    }
    
    // CORS for API
    handler = corsMiddleware(handler)
    
    return handler
}
```

### SSE Hub for Live Updates

```go
// internal/admin/sse.go
package admin

import (
    "encoding/json"
    "fmt"
    "net/http"
    "sync"
)

type SSEHub struct {
    mu      sync.RWMutex
    clients map[chan SSEEvent]struct{}
}

type SSEEvent struct {
    Type string      `json:"type"`
    Data interface{} `json:"data"`
}

func NewSSEHub() *SSEHub {
    return &SSEHub{
        clients: make(map[chan SSEEvent]struct{}),
    }
}

func (h *SSEHub) Broadcast(event SSEEvent) {
    h.mu.RLock()
    defer h.mu.RUnlock()
    
    for ch := range h.clients {
        select {
        case ch <- event:
        default:
            // Drop if client is slow
        }
    }
}

func (h *SSEHub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    flusher, ok := w.(http.Flusher)
    if !ok {
        http.Error(w, "streaming not supported", http.StatusInternalServerError)
        return
    }
    
    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")
    
    ch := make(chan SSEEvent, 100)
    
    h.mu.Lock()
    h.clients[ch] = struct{}{}
    h.mu.Unlock()
    
    defer func() {
        h.mu.Lock()
        delete(h.clients, ch)
        h.mu.Unlock()
        close(ch)
    }()
    
    for {
        select {
        case event := <-ch:
            data, _ := json.Marshal(event.Data)
            fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, data)
            flusher.Flush()
        case <-r.Context().Done():
            return
        }
    }
}
```

---

## 12. Metrics Collection

```go
// internal/metrics/collector.go
package metrics

import (
    "fmt"
    "sync"
    "sync/atomic"
    "time"
)

type Collector struct {
    counters   sync.Map // string → *int64
    histograms sync.Map // string → *Histogram
    gauges     sync.Map // string → *int64
    startTime  time.Time
}

func NewCollector() *Collector {
    return &Collector{
        startTime: time.Now(),
    }
}

func (c *Collector) IncrCounter(name string, labels map[string]string) {
    key := metricKey(name, labels)
    val, _ := c.counters.LoadOrStore(key, new(int64))
    atomic.AddInt64(val.(*int64), 1)
}

func (c *Collector) ObserveHistogram(name string, labels map[string]string, value float64) {
    key := metricKey(name, labels)
    h, _ := c.histograms.LoadOrStore(key, NewHistogram())
    h.(*Histogram).Observe(value)
}

// PrometheusFormat exports all metrics in Prometheus text format
func (c *Collector) PrometheusFormat() string {
    var buf strings.Builder
    
    c.counters.Range(func(key, value interface{}) bool {
        fmt.Fprintf(&buf, "%s %d\n", key, atomic.LoadInt64(value.(*int64)))
        return true
    })
    
    c.histograms.Range(func(key, value interface{}) bool {
        h := value.(*Histogram)
        fmt.Fprintf(&buf, "%s_count %d\n", key, h.Count())
        fmt.Fprintf(&buf, "%s_sum %f\n", key, h.Sum())
        for _, q := range []float64{0.5, 0.9, 0.95, 0.99} {
            fmt.Fprintf(&buf, "%s{quantile=\"%g\"} %f\n", key, q, h.Quantile(q))
        }
        return true
    })
    
    return buf.String()
}
```

---

## 13. Graceful Shutdown

```go
// cmd/dockrouter/main.go (shutdown section)

func gracefulShutdown(httpServer, httpsServer, adminServer *http.Server, discovery *discovery.Engine) {
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
    
    sig := <-sigCh
    log.Info("shutdown signal received", "signal", sig)
    
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    
    // 1. Stop accepting new connections
    // 2. Stop discovery engine
    discovery.Stop()
    
    // 3. Drain in-flight requests
    var wg sync.WaitGroup
    
    wg.Add(3)
    go func() { defer wg.Done(); httpServer.Shutdown(ctx) }()
    go func() { defer wg.Done(); httpsServer.Shutdown(ctx) }()
    go func() { defer wg.Done(); adminServer.Shutdown(ctx) }()
    
    wg.Wait()
    log.Info("shutdown complete")
}
```

---

## 14. Testing Strategy

### Unit Tests

| Package | Test Focus |
|---------|-----------|
| `discovery/labels_test.go` | Label parsing edge cases |
| `router/table_test.go` | Route matching, wildcards, path precedence |
| `router/radix_test.go` | Radix tree insert/match/delete |
| `middleware/ratelimit_test.go` | Token bucket accuracy, concurrent access |
| `middleware/cors_test.go` | CORS header correctness |
| `tls/acme_test.go` | ACME protocol flow (mock server) |
| `proxy/proxy_test.go` | Header forwarding, WebSocket upgrade |

### Integration Tests

Use `httptest.Server` and mock Docker socket for end-to-end:

```go
func TestFullFlow(t *testing.T) {
    // 1. Create mock Docker socket server
    // 2. Start DockRouter pointing to mock socket
    // 3. "Start" a container (send event)
    // 4. Verify route appears
    // 5. Make HTTP request → verify proxied correctly
    // 6. "Stop" container → verify route removed
}
```

### ACME Testing

- Use Let's Encrypt **staging** environment
- Use Pebble (ACME test server) for CI: `https://github.com/letsencrypt/pebble`

---

## 15. Error Handling Patterns

### Error Types

```go
// internal/errors.go
package internal

import "errors"

var (
    ErrNotEnabled     = errors.New("container does not have dr.enable=true")
    ErrMissingHost    = errors.New("container missing dr.host label")
    ErrNoBackend      = errors.New("no healthy backend available")
    ErrRouteNotFound  = errors.New("no route matches request")
    ErrCertNotFound   = errors.New("no certificate for domain")
    ErrACMEFailed     = errors.New("ACME certificate provisioning failed")
    ErrDockerSocket   = errors.New("cannot connect to Docker socket")
    ErrRateLimited    = errors.New("rate limit exceeded")
)
```

### Recovery Pattern

Every goroutine that handles requests or background tasks wraps in recover:

```go
func safeGo(fn func()) {
    go func() {
        defer func() {
            if r := recover(); r != nil {
                log.Error("panic recovered", "panic", r, "stack", string(debug.Stack()))
            }
        }()
        fn()
    }()
}
```
