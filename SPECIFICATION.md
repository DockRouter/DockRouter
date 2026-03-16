# DockRouter — SPECIFICATION.md

> **Zero-dependency, single-binary Docker-native ingress router with automatic TLS, label-based discovery, and built-in admin dashboard.**

**Repository:** github.com/DockRouter  
**Website:** dockrouter.com  
**Language:** Go (1.22+)  
**License:** MIT  
**Philosophy:** Zero external dependencies, Docker socket only, single binary, production-ready from day one.

---

## Table of Contents

1. [Vision & Goals](#1-vision--goals)
2. [Architecture Overview](#2-architecture-overview)
3. [Core Subsystems](#3-core-subsystems)
   - 3.1 [Docker Discovery Engine](#31-docker-discovery-engine)
   - 3.2 [Routing Engine](#32-routing-engine)
   - 3.3 [TLS / ACME Manager](#33-tls--acme-manager)
   - 3.4 [Middleware Pipeline](#34-middleware-pipeline)
   - 3.5 [Rate Limiter](#35-rate-limiter)
   - 3.6 [Admin Dashboard & API](#36-admin-dashboard--api)
   - 3.7 [Health Checker](#37-health-checker)
   - 3.8 [Logging & Metrics](#38-logging--metrics)
4. [Docker Label Convention](#4-docker-label-convention)
5. [Configuration](#5-configuration)
6. [Deployment Model](#6-deployment-model)
7. [Data Flow](#7-data-flow)
8. [Security Model](#8-security-model)
9. [Performance Targets](#9-performance-targets)
10. [Directory Structure](#10-directory-structure)
11. [Future Scope (Post-MVP)](#11-future-scope-post-mvp)

---

## 1. Vision & Goals

### What DockRouter Is

DockRouter is a **Docker-native ingress router** that automatically discovers containers via Docker labels, provisions TLS certificates via ACME (Let's Encrypt / ZeroSSL), and routes HTTP/HTTPS traffic — all from a **single Go binary with zero external dependencies**.

Think of it as **Traefik's philosophy, rebuilt from scratch** with:
- No YAML/TOML config sprawl — labels are the config
- No external dependencies — pure Go stdlib + Docker socket
- No complex provider system — Docker-first, Docker-only (MVP)
- Built-in admin dashboard — no separate UI container needed

### Goals

| Priority | Goal |
|----------|------|
| G1 | Zero-config container routing via Docker labels |
| G2 | Automatic TLS with Let's Encrypt (HTTP-01 challenge) |
| G3 | Sub-millisecond routing decision overhead |
| G4 | Single static binary, <20MB, runs anywhere Docker runs |
| G5 | Built-in admin dashboard on dedicated port |
| G6 | Production-grade rate limiting per route/IP |
| G7 | Hot-reload on container start/stop — zero downtime |
| G8 | Sensible defaults — works out of the box with minimal config |

### Non-Goals (MVP)

- Kubernetes / Swarm orchestrator support (future)
- TCP/UDP (L4) raw proxying (future)
- gRPC-specific routing (future)
- Distributed / multi-node clustering (future — Raft consensus)
- DNS-01 ACME challenge (future — requires provider plugins)

---

## 2. Architecture Overview

```
                    ┌─────────────────────────────────────┐
                    │           DockRouter Binary          │
                    │                                      │
  :80 HTTP ────────▶│  ┌───────────┐   ┌───────────────┐  │
                    │  │  Entrypoint│──▶│  Middleware    │  │
  :443 HTTPS ──────▶│  │  Listener  │   │  Pipeline     │  │
                    │  └───────────┘   └───────┬───────┘  │
                    │                          │          │
                    │                  ┌───────▼───────┐  │
                    │                  │  Router Core   │  │
                    │                  │  (host+path)   │  │
                    │                  └───────┬───────┘  │
                    │                          │          │
                    │                  ┌───────▼───────┐  │
                    │                  │  Backend Pool  │──│──▶ Container :3000
                    │                  │  (upstreams)   │──│──▶ Container :8080
                    │                  └───────────────┘  │
                    │                                      │
  :9090 Admin ─────▶│  ┌───────────────────────────────┐  │
                    │  │  Admin API + Embedded Web UI   │  │
                    │  └───────────────────────────────┘  │
                    │                                      │
                    │  ┌──────────┐  ┌──────────────────┐ │
                    │  │ Discovery│  │  ACME/TLS Manager │ │
                    │  │ Engine   │  │  (Let's Encrypt)  │ │
                    │  └────┬─────┘  └──────────────────┘ │
                    │       │                              │
                    └───────│──────────────────────────────┘
                            │
                   /var/run/docker.sock
                            │
               ┌────────────▼────────────────┐
               │    Docker Daemon             │
               │  ┌─────┐ ┌─────┐ ┌─────┐   │
               │  │web-1│ │api-1│ │app-1│   │
               │  └─────┘ └─────┘ └─────┘   │
               └─────────────────────────────┘
```

### Component Interaction

1. **Discovery Engine** watches Docker socket for container events
2. On container start/stop, it parses labels and updates the **Route Table**
3. **Entrypoint Listeners** accept connections on :80, :443, :9090
4. Incoming requests flow through the **Middleware Pipeline** (rate limit, headers, CORS, compression, auth)
5. **Router Core** matches request to a route by Host header + path prefix
6. Request is proxied to the **Backend Pool** (the target container)
7. **ACME Manager** handles certificate provisioning when a new domain appears
8. **Health Checker** periodically verifies backend availability
9. **Admin Dashboard** exposes real-time state via REST API + embedded SPA

---

## 3. Core Subsystems

### 3.1 Docker Discovery Engine

**Purpose:** Watch Docker daemon for container lifecycle events and extract routing configuration from container labels.

#### Discovery Modes

| Mode | Mechanism | Use Case |
|------|-----------|----------|
| **Event Stream** (primary) | `GET /events` on Docker API via Unix socket | Real-time discovery, <100ms reaction |
| **Polling** (fallback) | `GET /containers/json` periodic scan | Reconnect safety net, interval: 10s |
| **Initial Sync** | Full container list on startup | Catch already-running containers |

#### Docker API Communication

All Docker API communication happens over **Unix socket** (`/var/run/docker.sock`) using Go's `net.Dial("unix", ...)` and raw HTTP/1.1 over the connection. No Docker SDK dependency.

**API Endpoints Used:**

| Endpoint | Purpose |
|----------|---------|
| `GET /v1.44/events?filters=...` | Stream container start/stop/die events |
| `GET /v1.44/containers/json?filters=...` | List running containers with labels |
| `GET /v1.44/containers/{id}/json` | Inspect single container (get IP, ports, labels) |
| `GET /v1.44/networks/{id}` | Resolve container network for direct routing |

#### Event Processing Flow

```
Docker Event (container start)
    │
    ▼
Parse Event JSON
    │
    ▼
Filter: has "dr.enable=true" label?
    │ NO → ignore
    │ YES ↓
    ▼
Inspect container → get labels, IP, ports, networks
    │
    ▼
Build RouteEntry from labels
    │
    ▼
Register in Route Table (thread-safe)
    │
    ▼
If dr.tls=auto → trigger ACME cert provisioning
    │
    ▼
Notify Health Checker → start monitoring this backend
```

#### Container Network Resolution

DockRouter needs to reach containers by their internal IP. Resolution strategy:

1. If DockRouter and target share a Docker network → use container's IP on that network
2. If target exposes a published port → use `host.docker.internal:{published_port}` or `172.17.0.1:{published_port}`
3. Label override: `dr.address=10.0.0.5:3000` for explicit addressing

#### Reconnection & Resilience

- Event stream drops → automatic reconnect with exponential backoff (1s, 2s, 4s, max 30s)
- On reconnect → full resync (list all containers, diff with current route table)
- Docker socket unavailable at startup → retry loop with clear error logging
- Graceful degradation: existing routes remain active if Docker API is temporarily unreachable

---

### 3.2 Routing Engine

**Purpose:** Match incoming HTTP requests to backend containers based on Host header and path prefix.

#### Route Table Structure

```go
type RouteTable struct {
    mu     sync.RWMutex
    routes map[string]*Route  // key: host or host+path
    index  *RadixTree         // fast path matching
}

type Route struct {
    ID          string       // container ID (short)
    Host        string       // "api.example.com"
    PathPrefix  string       // "/api/v2" (optional)
    Backend     BackendPool
    TLS         TLSConfig
    Middlewares []string     // ["ratelimit", "cors", "compress"]
    Priority    int          // higher wins on conflict
    CreatedAt   time.Time
    Labels      map[string]string // raw labels for reference
}

type BackendPool struct {
    Targets     []BackendTarget
    Strategy    LoadBalanceStrategy // roundrobin, random, iphash
    HealthCheck HealthCheckConfig
}

type BackendTarget struct {
    Address     string    // "172.18.0.5:3000"
    ContainerID string
    Weight      int
    Healthy     bool
    LastCheck   time.Time
}
```

#### Matching Algorithm

1. Extract `Host` header from request
2. Normalize: lowercase, strip port
3. **Exact host match** → check path prefix
4. **Wildcard match** (`*.example.com`) → check path prefix
5. **Catch-all** route (if configured) → default backend
6. No match → 502 Bad Gateway with branded error page

**Path matching** uses longest-prefix-wins:
- `/api/v2/users` matches `/api/v2` over `/api`
- Trailing slash normalization: `/api/` and `/api` are equivalent

#### Multi-Container Same-Host Support

Multiple containers can register for the same host:
- Same host + same path → load balance between them (backend pool)
- Same host + different path → path-based routing
- Container stop → remove from pool, if pool empty → route goes "degraded"

#### Hot Reload Semantics

Route table updates are **lock-free for reads** (RWMutex, read-heavy workload):
- Container start → add route (write lock, <1μs)
- Container stop → remove route (write lock, <1μs)
- In-flight requests complete on old config (no interruption)
- New requests immediately use new config

---

### 3.3 TLS / ACME Manager

**Purpose:** Automatic TLS certificate provisioning and renewal via ACME protocol (Let's Encrypt, ZeroSSL).

#### ACME Implementation

Pure Go ACME client — no `certbot`, no `acme.sh`, no external tools.

**Supported challenges (MVP):**

| Challenge | How It Works | When Used |
|-----------|-------------|-----------|
| **HTTP-01** | Serve token at `/.well-known/acme-challenge/{token}` on :80 | Default, single-domain certs |
| **TLS-ALPN-01** | Serve special self-signed cert with ACME extension on :443 | When :80 is unavailable |

**Not in MVP:** DNS-01 (requires provider API plugins for Cloudflare, Route53 etc.)

#### Certificate Lifecycle

```
New domain detected (dr.tls=auto)
    │
    ▼
Check cert store → exists and valid (>30 days to expiry)?
    │ YES → use existing cert
    │ NO ↓
    ▼
Create ACME account (or reuse existing)
    │
    ▼
Request new order for domain(s)
    │
    ▼
Solve HTTP-01 challenge
    │   ├─ Register challenge token in memory
    │   ├─ ACME server validates http://{domain}/.well-known/acme-challenge/{token}
    │   └─ Challenge solved
    ▼
Finalize order → receive certificate chain
    │
    ▼
Store cert + key to filesystem
    │
    ▼
Load into TLS config (hot swap, no restart)
    │
    ▼
Schedule renewal check (daily cron, renew at 30 days before expiry)
```

#### Certificate Storage

```
/data/certs/
├── accounts/
│   └── acme-v02.api.letsencrypt.org/
│       └── account.json          # ACME account key
├── certificates/
│   ├── api.example.com/
│   │   ├── cert.pem              # Full chain
│   │   ├── key.pem               # Private key
│   │   └── meta.json             # Expiry, domains, issuer
│   └── app.example.com/
│       ├── cert.pem
│       ├── key.pem
│       └── meta.json
└── staging/                       # Let's Encrypt staging certs (for testing)
```

#### TLS Configuration

```go
type TLSConfig struct {
    Mode       string   // "auto", "manual", "off"
    Domains    []string // from dr.host + dr.tls.domains
    MinVersion uint16   // tls.VersionTLS12
    MaxVersion uint16   // tls.VersionTLS13
    CertFile   string   // for manual mode
    KeyFile    string   // for manual mode
    ALPN       []string // ["h2", "http/1.1"]
}
```

#### Dynamic TLS with `GetCertificate`

Go's `tls.Config.GetCertificate` callback is used for per-connection certificate selection:

```go
tlsConfig := &tls.Config{
    GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
        return certManager.GetCertificate(hello.ServerName)
    },
    MinVersion: tls.VersionTLS12,
}
```

This allows:
- Different certs per domain
- Hot-swap on renewal (no restart)
- Fallback to self-signed if ACME fails (with warning log)

#### ACME Providers

| Provider | URL | Default |
|----------|-----|---------|
| Let's Encrypt Production | `https://acme-v02.api.letsencrypt.org/directory` | ✅ |
| Let's Encrypt Staging | `https://acme-staging-v02.api.letsencrypt.org/directory` | Testing |
| ZeroSSL | `https://acme.zerossl.com/v2/DV90` | Alternative |

Configurable via `--acme-provider` flag or `DR_ACME_PROVIDER` env var.

---

### 3.4 Middleware Pipeline

**Purpose:** Composable request/response processing chain applied per-route.

#### Pipeline Architecture

```
Request In
    │
    ▼
[Recovery] → panic protection, always first
    │
    ▼
[Request ID] → inject X-Request-Id header
    │
    ▼
[Access Log] → log request start
    │
    ▼
[Rate Limiter] → check rate limit (may short-circuit → 429)
    │
    ▼
[IP Whitelist/Blacklist] → check IP rules (may short-circuit → 403)
    │
    ▼
[CORS] → handle preflight, set headers
    │
    ▼
[Security Headers] → HSTS, X-Frame-Options, CSP etc.
    │
    ▼
[Compression] → gzip/brotli response compression
    │
    ▼
[Auth] → Basic auth, JWT validation, API key (may short-circuit → 401)
    │
    ▼
[Custom Headers] → add/remove/set headers
    │
    ▼
[Proxy] → forward to backend
    │
    ▼
Response Out (back through chain for response modification)
```

#### Built-in Middlewares

| Middleware | Label Config | Description |
|-----------|-------------|-------------|
| `ratelimit` | `dr.ratelimit=100/m` | Token bucket rate limiter |
| `cors` | `dr.cors.origins=*` | CORS headers |
| `compress` | `dr.compress=true` | Gzip/Brotli compression |
| `headers` | `dr.headers.X-Custom=value` | Custom header injection |
| `basicauth` | `dr.auth.basic.users=user:hash` | HTTP Basic authentication |
| `ipwhitelist` | `dr.ipwhitelist=10.0.0.0/8` | IP allow list |
| `ipblacklist` | `dr.ipblacklist=1.2.3.4` | IP deny list |
| `redirect-https` | `dr.redirect.https=true` | HTTP→HTTPS redirect (default: true when TLS is on) |
| `stripprefix` | `dr.stripprefix=/api` | Strip path prefix before forwarding |
| `addprefix` | `dr.addprefix=/v2` | Add path prefix before forwarding |
| `maxbody` | `dr.maxbody=10mb` | Request body size limit |
| `retry` | `dr.retry=3` | Retry failed requests to other backends |
| `circuitbreaker` | `dr.circuitbreaker=5/30s` | Circuit breaker (5 failures in 30s → open) |

#### Middleware Resolution

Per-route middleware list is determined by:
1. **Global defaults** (recovery, requestid, accesslog — always on)
2. **Auto-applied** (redirect-https when TLS is active)
3. **Label-declared** via specific `dr.*` labels
4. **Explicit pipeline** via `dr.middlewares=ratelimit,cors,compress`

---

### 3.5 Rate Limiter

**Purpose:** Protect backends from excessive traffic with configurable rate limiting.

#### Algorithm: Token Bucket

```go
type TokenBucket struct {
    mu         sync.Mutex
    tokens     float64
    maxTokens  float64
    refillRate float64    // tokens per second
    lastRefill time.Time
}
```

**Why Token Bucket over Sliding Window:**
- Allows bursts up to bucket size (more realistic traffic)
- O(1) memory per key
- Simple and battle-tested

#### Rate Limit Scopes

| Scope | Label | Key | Example |
|-------|-------|-----|---------|
| Per-IP | `dr.ratelimit=100/m` | Client IP | 100 requests/min per IP |
| Per-Route | `dr.ratelimit.route=1000/m` | Route ID | 1000 req/min for entire route |
| Per-Header | `dr.ratelimit.by=X-API-Key` | Header value | Rate limit by API key |
| Global | config file | — | Overall DockRouter throughput cap |

#### Rate Limit Label Syntax

```
dr.ratelimit={count}/{window}

Windows:
  s = second
  m = minute
  h = hour

Examples:
  dr.ratelimit=100/m      → 100 requests per minute per IP
  dr.ratelimit=10/s        → 10 requests per second per IP
  dr.ratelimit=5000/h      → 5000 requests per hour per IP
```

#### Rate Limit Response

When rate limited:
- Status: `429 Too Many Requests`
- Headers:
  - `X-RateLimit-Limit: 100`
  - `X-RateLimit-Remaining: 0`
  - `X-RateLimit-Reset: 1699999999` (Unix timestamp)
  - `Retry-After: 45` (seconds)

#### Memory Management

- Rate limit buckets are stored in `sync.Map` keyed by `{route_id}:{client_ip}`
- Expired buckets are garbage collected every 60 seconds
- Maximum 100K active buckets (configurable), LRU eviction beyond that

---

### 3.6 Admin Dashboard & API

**Purpose:** Real-time visibility into DockRouter state, routes, certificates, and traffic.

#### Admin Server

Runs on `:9090` (configurable via `--admin-port` or `DR_ADMIN_PORT`).

**Security:**
- Binds to `127.0.0.1:9090` by default (localhost only)
- Optional: bind to `0.0.0.0:9090` with `--admin-bind=0.0.0.0`
- Basic auth protection: `--admin-user` / `--admin-pass`
- Can be disabled entirely: `--admin=false`

#### REST API Endpoints

```
GET  /api/v1/status                  → DockRouter status & uptime
GET  /api/v1/routes                  → List all active routes
GET  /api/v1/routes/{id}             → Route detail + backends
GET  /api/v1/containers              → Discovered containers
GET  /api/v1/certificates            → TLS certificate status
GET  /api/v1/certificates/{domain}   → Certificate detail for domain
POST /api/v1/certificates/{domain}/renew → Force certificate renewal
GET  /api/v1/metrics                 → Prometheus-format metrics
GET  /api/v1/health                  → Health check endpoint
GET  /api/v1/config                  → Running configuration
GET  /api/v1/middlewares             → Available middlewares
GET  /api/v1/ratelimit/status        → Rate limiter statistics
```

#### Embedded Web UI

The admin dashboard is a **single-page application** embedded into the Go binary at compile time using `embed.FS`.

**Dashboard Pages:**

| Page | Content |
|------|---------|
| **Overview** | Active routes count, total requests, cert status, uptime, error rate |
| **Routes** | Table of all routes: host, path, backend(s), TLS status, middleware list |
| **Route Detail** | Specific route: backends with health, request stats, latency percentiles |
| **Certificates** | All certs: domain, issuer, expiry date, auto-renew status |
| **Containers** | Discovered Docker containers, their labels, network info |
| **Logs** | Real-time log stream (last 1000 entries, filterable) |
| **Configuration** | Running config, env vars (sensitive values masked) |

**Tech stack for embedded UI:**
- Vanilla HTML/CSS/JS (no build step, no npm, no framework)
- CSS: minimal custom CSS, dark/light mode via `prefers-color-scheme`
- JS: native `fetch()` for API calls, `EventSource` for real-time updates
- Embedded via `//go:embed dashboard/*`

#### Real-Time Updates

Admin dashboard uses **Server-Sent Events (SSE)** for live updates:

```
GET /api/v1/events → SSE stream

Event types:
- route.added
- route.removed
- route.updated
- container.started
- container.stopped
- certificate.issued
- certificate.renewed
- certificate.expiring
- health.changed
- ratelimit.triggered
```

---

### 3.7 Health Checker

**Purpose:** Monitor backend container health and remove unhealthy targets from routing.

#### Health Check Modes

| Mode | Mechanism | Default |
|------|-----------|---------|
| **HTTP** | `GET /` or custom path, expect 2xx | ✅ Default |
| **TCP** | TCP connect to backend port | Fallback |
| **Docker** | Use container health status from Docker API | If available |

#### Configuration via Labels

```
dr.healthcheck.path=/health       # HTTP health check path
dr.healthcheck.interval=10s       # Check every 10 seconds
dr.healthcheck.timeout=5s         # Timeout per check
dr.healthcheck.threshold=3        # 3 consecutive failures → unhealthy
dr.healthcheck.recovery=2         # 2 consecutive successes → healthy again
```

#### Health State Machine

```
                 ┌──────────┐
    startup ────▶│ HEALTHY  │◀─── recovery threshold met
                 └────┬─────┘
                      │ failure
                      ▼
                 ┌──────────┐
                 │ DEGRADED │   (still receives traffic, warned)
                 └────┬─────┘
                      │ failure threshold met
                      ▼
                 ┌──────────┐
                 │ UNHEALTHY│   (removed from pool)
                 └────┬─────┘
                      │ success
                      ▼
                 ┌──────────┐
                 │RECOVERING│   (not yet in pool)
                 └──────────┘
```

---

### 3.8 Logging & Metrics

#### Structured Logging

JSON-formatted structured logs to stdout:

```json
{
  "ts": "2025-01-15T10:30:00.123Z",
  "level": "info",
  "msg": "request completed",
  "method": "GET",
  "host": "api.example.com",
  "path": "/users",
  "status": 200,
  "duration_ms": 12.5,
  "client_ip": "1.2.3.4",
  "request_id": "abc123",
  "backend": "172.18.0.5:3000",
  "container": "api-server-1"
}
```

#### Log Levels

| Level | Usage |
|-------|-------|
| `debug` | Detailed internal state (route matching, label parsing) |
| `info` | Request logs, container events, cert operations |
| `warn` | Health check failures, rate limit triggers, cert expiry warnings |
| `error` | Proxy failures, ACME errors, Docker socket errors |
| `fatal` | Cannot start (port conflict, socket permission denied) |

#### Metrics (Prometheus-compatible)

Exposed at `GET /api/v1/metrics`:

```
# Request metrics
dockrouter_requests_total{host, method, status}
dockrouter_request_duration_seconds{host, method} (histogram)
dockrouter_request_size_bytes{host} (histogram)
dockrouter_response_size_bytes{host} (histogram)

# Backend metrics
dockrouter_backend_up{host, backend}
dockrouter_backend_requests_total{host, backend, status}
dockrouter_backend_duration_seconds{host, backend} (histogram)

# Route metrics
dockrouter_routes_active
dockrouter_containers_discovered

# Certificate metrics
dockrouter_certificate_expiry_timestamp{domain}
dockrouter_certificate_renewal_total{domain, status}

# Rate limiter metrics
dockrouter_ratelimit_rejected_total{host, client_ip}

# System metrics
dockrouter_uptime_seconds
dockrouter_goroutines
dockrouter_memory_bytes
```

---

## 4. Docker Label Convention

### Prefix: `dr.`

All DockRouter labels use the `dr.` prefix to avoid conflicts.

### Complete Label Reference

#### Required Labels

| Label | Type | Description | Example |
|-------|------|-------------|---------|
| `dr.enable` | `bool` | Enable routing for this container | `"true"` |
| `dr.host` | `string` | Domain(s) to route to this container | `"api.example.com"` |

#### Routing Labels

| Label | Type | Default | Description | Example |
|-------|------|---------|-------------|---------|
| `dr.port` | `int` | Auto-detect | Container port to proxy to | `"3000"` |
| `dr.path` | `string` | `/` | Path prefix for routing | `"/api"` |
| `dr.priority` | `int` | `0` | Route priority (higher wins) | `"10"` |
| `dr.address` | `string` | Auto | Explicit backend address | `"10.0.0.5:3000"` |
| `dr.loadbalancer` | `string` | `roundrobin` | LB strategy | `"iphash"` |
| `dr.weight` | `int` | `1` | Backend weight for weighted LB | `"5"` |

#### TLS Labels

| Label | Type | Default | Description | Example |
|-------|------|---------|-------------|---------|
| `dr.tls` | `string` | `auto` | TLS mode: auto, manual, off | `"auto"` |
| `dr.tls.domains` | `string` | Same as `dr.host` | Additional SAN domains | `"www.example.com,example.com"` |
| `dr.tls.cert` | `string` | — | Path to manual cert file | `"/certs/cert.pem"` |
| `dr.tls.key` | `string` | — | Path to manual key file | `"/certs/key.pem"` |

#### Middleware Labels

| Label | Type | Default | Description |
|-------|------|---------|-------------|
| `dr.ratelimit` | `string` | — | Rate limit: `{count}/{window}` |
| `dr.ratelimit.by` | `string` | `client_ip` | Rate limit key |
| `dr.cors.origins` | `string` | — | Allowed CORS origins |
| `dr.cors.methods` | `string` | `GET,POST,PUT,DELETE` | Allowed CORS methods |
| `dr.cors.headers` | `string` | — | Allowed CORS headers |
| `dr.compress` | `bool` | `false` | Enable gzip/brotli |
| `dr.headers.{name}` | `string` | — | Add custom response header |
| `dr.redirect.https` | `bool` | `true` (when TLS on) | HTTP→HTTPS redirect |
| `dr.stripprefix` | `string` | — | Strip path prefix |
| `dr.addprefix` | `string` | — | Add path prefix |
| `dr.maxbody` | `string` | `10mb` | Max request body size |
| `dr.auth.basic.users` | `string` | — | Basic auth `user:bcrypt_hash` |
| `dr.ipwhitelist` | `string` | — | Allowed IPs (CIDR) |
| `dr.ipblacklist` | `string` | — | Blocked IPs (CIDR) |
| `dr.retry` | `int` | `0` | Retry count on failure |
| `dr.circuitbreaker` | `string` | — | Circuit breaker `{failures}/{window}` |
| `dr.middlewares` | `string` | — | Explicit middleware list |

#### Health Check Labels

| Label | Type | Default | Description |
|-------|------|---------|-------------|
| `dr.healthcheck.path` | `string` | `/` | Health check HTTP path |
| `dr.healthcheck.interval` | `duration` | `10s` | Check interval |
| `dr.healthcheck.timeout` | `duration` | `5s` | Check timeout |
| `dr.healthcheck.threshold` | `int` | `3` | Failures before unhealthy |
| `dr.healthcheck.recovery` | `int` | `2` | Successes before healthy |

### Example docker-compose.yml

```yaml
version: "3.8"

services:
  dockrouter:
    image: ghcr.io/dockrouter/dockrouter:latest
    ports:
      - "80:80"
      - "443:443"
      - "9090:9090"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - dockrouter-data:/data
    environment:
      - DR_ACME_EMAIL=admin@example.com
      - DR_ADMIN_USER=admin
      - DR_ADMIN_PASS=secretpassword
    restart: unless-stopped

  api:
    image: myapp/api:latest
    labels:
      dr.enable: "true"
      dr.host: "api.example.com"
      dr.port: "3000"
      dr.tls: "auto"
      dr.ratelimit: "100/m"
      dr.cors.origins: "https://app.example.com"
      dr.compress: "true"
      dr.healthcheck.path: "/health"

  frontend:
    image: myapp/frontend:latest
    labels:
      dr.enable: "true"
      dr.host: "app.example.com"
      dr.port: "3000"
      dr.tls: "auto"
      dr.compress: "true"

  blog:
    image: ghost:latest
    labels:
      dr.enable: "true"
      dr.host: "blog.example.com"
      dr.port: "2368"
      dr.tls: "auto"
      dr.ratelimit: "200/m"

volumes:
  dockrouter-data:
```

---

## 5. Configuration

### Configuration Hierarchy (priority order)

1. **CLI flags** (highest priority)
2. **Environment variables** (`DR_` prefix)
3. **Config file** (`/etc/dockrouter/config.yaml` or `--config` flag)
4. **Defaults** (lowest priority)

### Configuration Options

| CLI Flag | Env Var | Default | Description |
|----------|---------|---------|-------------|
| `--http-port` | `DR_HTTP_PORT` | `80` | HTTP listener port |
| `--https-port` | `DR_HTTPS_PORT` | `443` | HTTPS listener port |
| `--admin-port` | `DR_ADMIN_PORT` | `9090` | Admin dashboard port |
| `--admin-bind` | `DR_ADMIN_BIND` | `127.0.0.1` | Admin bind address |
| `--admin-user` | `DR_ADMIN_USER` | — | Admin basic auth user |
| `--admin-pass` | `DR_ADMIN_PASS` | — | Admin basic auth password |
| `--admin` | `DR_ADMIN` | `true` | Enable admin dashboard |
| `--docker-socket` | `DR_DOCKER_SOCKET` | `/var/run/docker.sock` | Docker socket path |
| `--data-dir` | `DR_DATA_DIR` | `/data` | Data directory (certs, state) |
| `--acme-email` | `DR_ACME_EMAIL` | — (required for ACME) | ACME account email |
| `--acme-provider` | `DR_ACME_PROVIDER` | `letsencrypt` | ACME provider |
| `--acme-staging` | `DR_ACME_STAGING` | `false` | Use LE staging (testing) |
| `--log-level` | `DR_LOG_LEVEL` | `info` | Log level |
| `--log-format` | `DR_LOG_FORMAT` | `json` | Log format: json, text |
| `--access-log` | `DR_ACCESS_LOG` | `true` | Enable access logging |
| `--default-tls` | `DR_DEFAULT_TLS` | `auto` | Default TLS mode for new routes |
| `--poll-interval` | `DR_POLL_INTERVAL` | `10s` | Docker polling fallback interval |
| `--max-body-size` | `DR_MAX_BODY_SIZE` | `10mb` | Global max request body |
| `--trusted-ips` | `DR_TRUSTED_IPS` | — | Trusted proxy IPs (for X-Forwarded-For) |

### Minimal Startup

```bash
# Absolute minimum — just works
docker run -d \
  -p 80:80 -p 443:443 \
  -v /var/run/docker.sock:/var/run/docker.sock:ro \
  -v dockrouter-data:/data \
  -e DR_ACME_EMAIL=you@example.com \
  ghcr.io/dockrouter/dockrouter:latest
```

---

## 6. Deployment Model

### Docker Container (Primary)

```dockerfile
FROM scratch
COPY dockrouter /dockrouter
COPY dashboard/ /dashboard/
VOLUME /data
EXPOSE 80 443 9090
ENTRYPOINT ["/dockrouter"]
```

- `FROM scratch` — minimal attack surface
- Static binary — no libc dependency
- `/data` volume for certs and state persistence

### Standalone Binary

```bash
# Direct download
curl -fsSL https://get.dockrouter.com | sh

# Or from GitHub releases
wget https://github.com/DockRouter/dockrouter/releases/latest/download/dockrouter-linux-amd64
chmod +x dockrouter-linux-amd64
sudo mv dockrouter-linux-amd64 /usr/local/bin/dockrouter

# Run
dockrouter --acme-email=you@example.com
```

### Supported Platforms

| OS | Arch | Status |
|----|------|--------|
| Linux | amd64 | ✅ Primary |
| Linux | arm64 | ✅ |
| macOS | amd64 | ✅ (dev) |
| macOS | arm64 | ✅ (dev) |

---

## 7. Data Flow

### Request Lifecycle (Happy Path)

```
Client (HTTPS) ──▶ :443 TLS Listener
    │
    ├─ TLS handshake (GetCertificate → cert for SNI hostname)
    │
    ▼
HTTP/2 or HTTP/1.1 request parsed
    │
    ▼
Middleware: Recovery (panic guard)
    │
    ▼
Middleware: Request ID (generate + inject X-Request-Id)
    │
    ▼
Middleware: Access Log (record start time)
    │
    ▼
Router: Match Host + Path → Route
    │
    ├─ No match → 502 branded error page
    │
    ▼
Middleware: Rate Limiter → check bucket for client IP
    │
    ├─ Exceeded → 429 Too Many Requests
    │
    ▼
Middleware: CORS (if configured)
    │
    ▼
Middleware: Security Headers (HSTS, X-Frame-Options, etc.)
    │
    ▼
Middleware: Compression (negotiate Accept-Encoding)
    │
    ▼
Backend Selection: Round-robin / IP-hash / Weighted
    │
    ├─ All backends unhealthy → 503 Service Unavailable
    │
    ▼
Reverse Proxy:
    ├─ Set X-Forwarded-For, X-Forwarded-Proto, X-Real-IP
    ├─ Open connection to backend (connection pooling)
    ├─ Forward request headers + body
    ├─ Stream response back to client
    ├─ Handle WebSocket upgrade if requested
    │
    ▼
Middleware: Access Log (record status, duration)
    │
    ▼
Response sent to client
```

### ACME Challenge Flow

```
Let's Encrypt Server
    │
    ├─ Validates: GET http://api.example.com/.well-known/acme-challenge/{token}
    │
    ▼
:80 HTTP Listener
    │
    ▼
Router: Check if path matches /.well-known/acme-challenge/*
    │
    ├─ YES → ACME handler responds with challenge token
    │
    ├─ NO → Normal HTTP→HTTPS redirect or serve
    │
    ▼
(ACME handler is injected before normal routing, highest priority)
```

---

## 8. Security Model

### Attack Surface Minimization

| Measure | Implementation |
|---------|---------------|
| `FROM scratch` Docker image | No shell, no tools, no OS |
| Read-only Docker socket | Mount with `:ro` — DockRouter only reads |
| Admin on localhost | Default bind `127.0.0.1:9090` |
| No external deps | No supply chain attack vector |
| TLS 1.2 minimum | No SSLv3, TLS 1.0, TLS 1.1 |
| Security headers by default | HSTS, X-Content-Type-Options, X-Frame-Options |

### Header Sanitization

DockRouter sets/overwrites these headers before proxying:

```
X-Forwarded-For:   client IP (or append to existing if trusted proxy)
X-Forwarded-Proto: https
X-Forwarded-Host:  original Host header
X-Real-IP:         client IP
```

Hop-by-hop headers are stripped:
`Connection`, `Keep-Alive`, `Proxy-Authenticate`, `Proxy-Authorization`, `TE`, `Trailer`, `Transfer-Encoding`, `Upgrade` (except WebSocket).

### Trusted Proxies

When behind another LB (e.g., cloud LB), configure trusted proxy IPs:

```
--trusted-ips=10.0.0.0/8,172.16.0.0/12
```

Only IPs in this range are trusted for `X-Forwarded-For` header values.

### Rate Limiting as DDoS Mitigation

- Per-IP rate limiting prevents single-source flood
- Global rate limit prevents total overload
- Circuit breaker protects individual backends

---

## 9. Performance Targets

| Metric | Target | Notes |
|--------|--------|-------|
| Routing decision | <100μs | Radix tree lookup |
| Request overhead (proxy added latency) | <1ms p99 | Compared to direct connection |
| Concurrent connections | 50K+ | Per-listener, with connection pooling |
| Requests/second | 100K+ | Single core, small payload |
| Memory (idle, 100 routes) | <30MB | Including admin dashboard |
| Memory (active, 100 routes, 10K RPS) | <100MB | With rate limiter state |
| Binary size | <20MB | Static, stripped, with embedded dashboard |
| Startup time | <500ms | To first ready-to-serve state |
| Route update latency | <5ms | Container event → route active |
| TLS handshake overhead | <5ms | With session resumption |
| Container discovery | <100ms | Event to route registration |

### Connection Pooling

DockRouter maintains connection pools to backends:
- Default: 100 idle connections per backend
- Max connections per backend: 250
- Idle timeout: 90s
- Configurable via labels: `dr.pool.maxidle=50`

---

## 10. Directory Structure

```
dockrouter/
├── cmd/
│   └── dockrouter/
│       └── main.go                 # Entry point, CLI parsing, wire-up
│
├── internal/
│   ├── config/
│   │   ├── config.go               # Configuration struct & loading
│   │   ├── defaults.go             # Default values
│   │   └── validate.go             # Config validation
│   │
│   ├── discovery/
│   │   ├── docker.go               # Docker socket client (raw HTTP)
│   │   ├── events.go               # Event stream handler
│   │   ├── poller.go               # Polling fallback
│   │   ├── labels.go               # Label parser → RouteConfig
│   │   └── reconciler.go           # Diff & reconcile route table
│   │
│   ├── router/
│   │   ├── router.go               # HTTP router (host + path matching)
│   │   ├── table.go                # Route table (concurrent-safe)
│   │   ├── radix.go                # Radix tree for path matching
│   │   ├── route.go                # Route struct & methods
│   │   └── backend.go              # Backend pool & load balancing
│   │
│   ├── proxy/
│   │   ├── proxy.go                # Reverse proxy implementation
│   │   ├── transport.go            # Custom transport with connection pooling
│   │   ├── websocket.go            # WebSocket upgrade handling
│   │   ├── bufferpool.go           # Shared buffer pool for proxying
│   │   └── errorpage.go            # Branded error pages (502, 503, etc.)
│   │
│   ├── tls/
│   │   ├── manager.go              # TLS certificate manager
│   │   ├── acme.go                 # ACME client (account, order, challenge)
│   │   ├── challenge.go            # HTTP-01 / TLS-ALPN-01 solvers
│   │   ├── store.go                # Certificate filesystem storage
│   │   └── renewal.go              # Auto-renewal scheduler
│   │
│   ├── middleware/
│   │   ├── chain.go                # Middleware chain builder
│   │   ├── recovery.go             # Panic recovery
│   │   ├── requestid.go            # X-Request-Id injection
│   │   ├── accesslog.go            # Access logging
│   │   ├── ratelimit.go            # Token bucket rate limiter
│   │   ├── cors.go                 # CORS handler
│   │   ├── compress.go             # Gzip/Brotli compression
│   │   ├── headers.go              # Custom header injection
│   │   ├── security.go             # Security headers (HSTS, CSP, etc.)
│   │   ├── basicauth.go            # HTTP Basic auth
│   │   ├── ipfilter.go             # IP whitelist/blacklist
│   │   ├── redirect.go             # HTTP→HTTPS redirect
│   │   ├── pathmod.go              # Strip/Add prefix
│   │   ├── maxbody.go              # Request body size limit
│   │   ├── retry.go                # Retry on failure
│   │   └── circuitbreaker.go       # Circuit breaker
│   │
│   ├── health/
│   │   ├── checker.go              # Health check orchestrator
│   │   ├── http.go                 # HTTP health check
│   │   ├── tcp.go                  # TCP health check
│   │   └── state.go                # Health state machine
│   │
│   ├── admin/
│   │   ├── server.go               # Admin HTTP server
│   │   ├── api.go                  # REST API handlers
│   │   ├── sse.go                  # Server-Sent Events for live updates
│   │   └── auth.go                 # Admin authentication
│   │
│   ├── metrics/
│   │   ├── collector.go            # Metrics collection
│   │   └── prometheus.go           # Prometheus exposition format
│   │
│   └── log/
│       ├── logger.go               # Structured JSON logger
│       └── access.go               # Access log formatter
│
├── dashboard/                       # Embedded admin web UI
│   ├── index.html
│   ├── app.js
│   ├── style.css
│   └── embed.go                    # go:embed directive
│
├── Dockerfile
├── docker-compose.yml               # DockRouter + example services
├── Makefile
├── go.mod
├── go.sum
├── LICENSE                          # MIT
├── README.md
├── SPECIFICATION.md                 # This file
├── IMPLEMENTATION.md                # Implementation details
├── TASKS.md                         # Task breakdown for Claude Code
├── BRANDING.md                      # Brand guidelines
└── .github/
    └── workflows/
        ├── build.yml                # Build + test
        └── release.yml              # GoReleaser for multi-platform
```

---

## 11. Future Scope (Post-MVP)

### Phase 2: Advanced Routing

- **TCP/UDP (L4) proxying** — raw TCP/UDP passthrough for databases, MQTT, etc.
- **gRPC routing** — gRPC-specific load balancing and health checking
- **Header-based routing** — route by custom headers (A/B testing, canary)
- **Weighted canary routing** — `dr.canary=10%` to new version
- **Request mirroring** — shadow traffic to test containers

### Phase 3: Clustering

- **Raft consensus** — multi-node DockRouter with shared route table
- **Distributed rate limiting** — consistent rate limits across nodes
- **Leader election** — ACME operations on leader only
- **Cert replication** — certs synced across cluster nodes

### Phase 4: Extended Provider Support

- **DNS-01 ACME challenge** — plugin system for DNS providers (Cloudflare, Route53, etc.)
- **Docker Swarm discovery** — service-level routing (not just containers)
- **Kubernetes Ingress Controller** — k8s CRD-based configuration

### Phase 5: Observability

- **OpenTelemetry traces** — distributed tracing support
- **Grafana dashboard templates** — pre-built dashboards
- **Alerting** — webhook alerts for cert expiry, health changes, error spikes

### Phase 6: MCP Integration

- **MCP Server** — expose DockRouter management as MCP tools
- **AI-assisted routing** — natural language route configuration
- **Anomaly detection** — AI-powered traffic analysis
