# DockRouter — TASKS.md

> Task breakdown for Claude Code implementation sessions.  
> Each task has a unique ID, clear scope, and dependencies.  
> **Reference SPECIFICATION.md and IMPLEMENTATION.md before starting any task.**

---

## Task Status Legend

- 🔴 Not Started
- 🟡 In Progress
- 🟢 Complete
- ⚪ Blocked

---

## Phase 0: Project Scaffolding

### DR-001 — Initialize Go Module & Directory Structure 🟢
**Scope:** Create the full directory structure from SPECIFICATION.md §10, initialize `go.mod`, create placeholder files.
**Output:** Complete directory tree with package declarations, `go.mod` with `module github.com/DockRouter/dockrouter`, empty `go.sum`.
**Dependencies:** None
**Effort:** Small

### DR-002 — CLI & Configuration System 🟢
**Scope:** Implement `cmd/dockrouter/main.go` with CLI flag parsing (using `flag` stdlib), environment variable loading (`DR_*` prefix), config struct, validation, and defaults per SPECIFICATION.md §5.
**Output:** `internal/config/config.go`, `defaults.go`, `validate.go`, `main.go` with clean startup sequence.
**Dependencies:** DR-001
**Effort:** Medium

### DR-003 — Structured Logger 🟢
**Scope:** Implement JSON structured logger per SPECIFICATION.md §3.8. Support log levels (debug/info/warn/error/fatal), structured fields, and `log/slog` compatible interface. Zero external deps — use `encoding/json` directly.
**Output:** `internal/log/logger.go`, `access.go`
**Dependencies:** DR-001
**Effort:** Small

### DR-004 — Makefile & Dockerfile 🟢
**Scope:** Create `Makefile` (build, build-all, docker, test, lint), multi-stage `Dockerfile` (builder + scratch), and `.github/workflows/build.yml`.
**Output:** `Makefile`, `Dockerfile`, `.github/workflows/build.yml`
**Dependencies:** DR-001
**Effort:** Small

---

## Phase 1: Docker Discovery Engine

### DR-010 — Docker Socket HTTP Client 🟢
**Scope:** Implement raw HTTP-over-Unix-socket client per IMPLEMENTATION.md §2. Methods: `ListContainers`, `InspectContainer`, `Ping`. No Docker SDK.
**Output:** `internal/discovery/docker.go` with full Docker API communication.
**Dependencies:** DR-001, DR-003
**Effort:** Medium

### DR-011 — Label Parser 🟢
**Scope:** Implement the complete label parsing system per SPECIFICATION.md §4 and IMPLEMENTATION.md §4. Parse all `dr.*` labels into `RouteConfig` struct. Handle edge cases: missing labels, malformed values, duration parsing, CIDR parsing.
**Output:** `internal/discovery/labels.go` with comprehensive unit tests in `labels_test.go`.
**Dependencies:** DR-001
**Effort:** Medium

### DR-012 — Event Stream Handler 🟢
**Scope:** Implement Docker event stream listener per IMPLEMENTATION.md §3. Watch for container start/stop/die/health_status events. Automatic reconnect with exponential backoff. Filter by `dr.enable=true`.
**Output:** `internal/discovery/events.go`
**Dependencies:** DR-010, DR-003
**Effort:** Medium

### DR-013 — Polling Fallback 🟢
**Scope:** Implement periodic polling of `ListContainers` as fallback when event stream is unavailable. Configurable interval (default 10s).
**Output:** `internal/discovery/poller.go`
**Dependencies:** DR-010
**Effort:** Small

### DR-014 — Reconciler & Discovery Engine 🟢
**Scope:** Implement the reconciler per IMPLEMENTATION.md §3. Full sync on startup, event-driven updates, route table diffing. Container address resolution (shared network IP, published port, label override). Wire up event stream + poller as the Discovery Engine.
**Output:** `internal/discovery/reconciler.go`, `internal/discovery/engine.go`
**Dependencies:** DR-010, DR-011, DR-012, DR-013
**Effort:** Large

---

## Phase 2: Routing Engine

### DR-020 — Route & Backend Types 🟢
**Scope:** Define core types: `Route`, `BackendPool`, `BackendTarget`, `LoadBalanceStrategy`, `HealthCheckConfig`. Include methods for backend management (add/remove target, select target by strategy).
**Output:** `internal/router/route.go`, `internal/router/backend.go`
**Dependencies:** DR-001
**Effort:** Small

### DR-021 — Radix Tree for Path Matching 🟢
**Scope:** Implement radix tree for fast longest-prefix path matching. Operations: Insert, Match (longest prefix), Delete, RemoveByContainer. Must handle edge cases: trailing slashes, overlapping prefixes.
**Output:** `internal/router/radix.go` with `radix_test.go`
**Dependencies:** DR-001
**Effort:** Medium

### DR-022 — Route Table (Concurrent-Safe) 🟢
**Scope:** Implement `RouteTable` with `sync.RWMutex`. Match by host (exact + wildcard) then path. Add/Remove routes. Multi-container same-host support (merge into backend pool). Per IMPLEMENTATION.md §5.
**Output:** `internal/router/table.go` with `table_test.go`
**Dependencies:** DR-020, DR-021
**Effort:** Medium

### DR-023 — HTTP Router Handler 🟢
**Scope:** Implement the main HTTP handler that receives requests, matches via RouteTable, selects backend, and delegates to proxy. Handle: host normalization, path matching, no-match error page, host-not-found.
**Output:** `internal/router/router.go`
**Dependencies:** DR-022
**Effort:** Medium

---

## Phase 3: Reverse Proxy Core

### DR-030 — Reverse Proxy Implementation 🟢
**Scope:** Implement reverse proxy per IMPLEMENTATION.md §6. Use `net/http/httputil.ReverseProxy`. Set X-Forwarded-* headers, X-Real-IP. Connection pooling via custom `http.Transport`. Buffer pool.
**Output:** `internal/proxy/proxy.go`, `internal/proxy/transport.go`, `internal/proxy/bufferpool.go`
**Dependencies:** DR-001
**Effort:** Medium

### DR-031 — WebSocket Passthrough 🟢
**Scope:** Ensure WebSocket upgrade (`Connection: Upgrade`, `Upgrade: websocket`) works through the proxy. Test with actual WebSocket connections.
**Output:** `internal/proxy/websocket.go` (or verify stdlib handles it), `websocket_test.go`
**Dependencies:** DR-030
**Effort:** Small

### DR-032 — Error Pages 🟢
**Scope:** Implement branded error pages (502, 503, 504, 429) per IMPLEMENTATION.md §6. Minimal, dark-themed HTML with DockRouter branding. Include Request ID.
**Output:** `internal/proxy/errorpage.go`
**Dependencies:** DR-001
**Effort:** Small

### DR-033 — Entrypoint Listeners 🟢
**Scope:** Create the main HTTP (:80) and HTTPS (:443) listeners. HTTP listener handles: ACME challenges (highest priority), HTTP→HTTPS redirect, plain HTTP serve. HTTPS listener uses `tls.Config` with `GetCertificate` callback. Wire together: listener → middleware chain → router → proxy.
**Output:** Main server setup in `cmd/dockrouter/main.go` (or `internal/server/server.go`)
**Dependencies:** DR-023, DR-030, DR-002
**Effort:** Large

---

## Phase 4: TLS / ACME Manager

### DR-040 — Certificate Store (Filesystem) 🟢
**Scope:** Implement filesystem-based cert storage per SPECIFICATION.md §3.3. Save/Load cert+key PEM files, meta.json with expiry. Check validity (>30 days to expiry). Directory structure: `/data/certs/certificates/{domain}/`.
**Output:** `internal/tls/store.go` with `store_test.go`
**Dependencies:** DR-001
**Effort:** Small

### DR-041 — ACME Client Core 🟢
**Scope:** Implement pure Go ACME client per IMPLEMENTATION.md §7. ACME directory fetch, JWS signing with ECDSA P-256, nonce management, account registration/lookup.
**Key complexity:** JWS (JSON Web Signature) with ACME-specific protected header format. Use `crypto/ecdsa`, `encoding/base64` (URL-safe), `encoding/json`.
**Output:** `internal/tls/acme.go` (core protocol), `internal/tls/jws.go` (JWS signing)
**Dependencies:** DR-001
**Effort:** Large

### DR-042 — ACME Order & Challenge Flow 🟢
**Scope:** Implement order creation, authorization fetching, challenge validation, order finalization, certificate download. Parse certificate chain from ACME response. Handle ACME error responses.
**Output:** Extension of `internal/tls/acme.go`
**Dependencies:** DR-041
**Effort:** Large

### DR-043 — HTTP-01 Challenge Solver 🟢
**Scope:** Implement HTTP-01 challenge solver per IMPLEMENTATION.md §7. In-memory token store, HTTP handler for `/.well-known/acme-challenge/{token}`. Register handler on :80 listener with highest priority (before normal routing).
**Output:** `internal/tls/challenge.go`
**Dependencies:** DR-033
**Effort:** Small

### DR-044 — TLS Manager (Certificate Lifecycle) 🟢
**Scope:** Implement `Manager` per IMPLEMENTATION.md §7. `GetCertificate` callback for per-SNI selection. `EnsureCertificate` for on-demand provisioning. Load existing certs from disk on startup. Daily renewal check (renew at 30 days before expiry). Hot-swap certs without restart.
**Output:** `internal/tls/manager.go`, `internal/tls/renewal.go`
**Dependencies:** DR-040, DR-041, DR-042, DR-043
**Effort:** Large

---

## Phase 5: Middleware Pipeline

### DR-050 — Middleware Chain Builder 🟢
**Scope:** Implement middleware chain per IMPLEMENTATION.md §8. `Chain()` function, `BuildChain()` from route config. Middleware type: `func(http.Handler) http.Handler`.
**Output:** `internal/middleware/chain.go`
**Dependencies:** DR-001
**Effort:** Small

### DR-051 — Core Middlewares (Always-On) 🟢
**Scope:** Implement: Recovery (panic guard with stack trace), RequestID (UUID v4 generation, inject X-Request-Id), AccessLog (structured JSON request/response log).
**Output:** `internal/middleware/recovery.go`, `requestid.go`, `accesslog.go`
**Dependencies:** DR-050, DR-003
**Effort:** Medium

### DR-052 — Security & Headers Middleware 🟢
**Scope:** Implement: Security headers (HSTS, X-Content-Type-Options, X-Frame-Options, CSP, Referrer-Policy), Custom header injection (from labels), HTTP→HTTPS redirect.
**Output:** `internal/middleware/security.go`, `headers.go`, `redirect.go`
**Dependencies:** DR-050
**Effort:** Small

### DR-053 — CORS Middleware 🟢
**Scope:** Full CORS implementation: preflight handling (OPTIONS), Access-Control-Allow-Origin/Methods/Headers/Credentials. Support wildcard (*) and specific origin lists. Handle complex CORS flows.
**Output:** `internal/middleware/cors.go` with `cors_test.go`
**Dependencies:** DR-050
**Effort:** Medium

### DR-054 — Compression Middleware 🟢
**Scope:** Gzip compression using `compress/gzip` stdlib. Check Accept-Encoding, skip for already-compressed content types (images, video). Configurable min size. Set Content-Encoding header.
**Output:** `internal/middleware/compress.go`
**Dependencies:** DR-050
**Effort:** Small

### DR-055 — Rate Limit Middleware 🟢
**Scope:** Token bucket rate limiter per IMPLEMENTATION.md §9. Per-IP, per-route, per-header key. Rate limit headers (X-RateLimit-*). 429 response. Background GC for expired buckets. Configurable max buckets with LRU eviction.
**Output:** `internal/middleware/ratelimit.go` with `ratelimit_test.go`
**Dependencies:** DR-050
**Effort:** Large

### DR-056 — Auth & IP Filter Middleware 🟢
**Scope:** Basic auth (bcrypt hash comparison), IP whitelist (CIDR matching), IP blacklist (CIDR matching). Use `net.ParseCIDR` for IP matching.
**Output:** `internal/middleware/basicauth.go`, `ipfilter.go`
**Dependencies:** DR-050
**Effort:** Medium

### DR-057 — Path Modifier & Body Limit Middleware 🟢
**Scope:** StripPrefix (remove path prefix before forwarding), AddPrefix (add path prefix), MaxBody (request body size limit using `http.MaxBytesReader`).
**Output:** `internal/middleware/pathmod.go`, `maxbody.go`
**Dependencies:** DR-050
**Effort:** Small

### DR-058 — Retry & Circuit Breaker Middleware 🟢
**Scope:** Retry: retry failed requests to next backend in pool. Configurable count, backoff. Circuit Breaker: track failure rate, open circuit after threshold, half-open after timeout, close on success.
**Output:** `internal/middleware/retry.go`, `circuitbreaker.go`
**Dependencies:** DR-050
**Effort:** Medium

---

## Phase 6: Health Checker

### DR-060 — Health Check System 🟢
**Scope:** Implement per IMPLEMENTATION.md §10. HTTP health checks (GET path, expect 2xx), TCP health checks (connect), state machine (Healthy → Degraded → Unhealthy → Recovering). Callback to update backend pool health status.
**Output:** `internal/health/checker.go`, `http.go`, `tcp.go`, `state.go` with `checker_test.go`
**Dependencies:** DR-020
**Effort:** Medium

---

## Phase 7: Admin Dashboard & API

### DR-070 — Admin REST API 🟢
**Scope:** Implement all REST API endpoints per SPECIFICATION.md §3.6. JSON responses. Basic auth protection. Bind to configurable port (default :9090, localhost only).
**Output:** `internal/admin/server.go`, `api.go`, `auth.go`
**Dependencies:** DR-022, DR-044, DR-060
**Effort:** Large

### DR-071 — SSE Event Hub 🟢
**Scope:** Implement Server-Sent Events hub per IMPLEMENTATION.md §11. Broadcast events (route.added, route.removed, certificate.issued, etc.). Client management (register/deregister). Backpressure handling (drop slow clients).
**Output:** `internal/admin/sse.go`
**Dependencies:** DR-070
**Effort:** Medium

### DR-072 — Embedded Web Dashboard 🟢
**Scope:** Create admin dashboard SPA per SPECIFICATION.md §3.6. Vanilla HTML/CSS/JS (no framework). Pages: Overview, Routes, Route Detail, Certificates, Containers, Logs, Configuration. Use fetch() for API, EventSource for SSE. Dark/light mode. Embed via `go:embed`.
**Output:** `dashboard/index.html`, `app.js`, `style.css`, `embed.go`
**Dependencies:** DR-070, DR-071
**Effort:** Large

---

## Phase 8: Metrics & Observability

### DR-080 — Metrics Collector 🟢
**Scope:** Implement metrics collection per IMPLEMENTATION.md §12. Counters, histograms (with quantile calculation), gauges. Atomic operations for thread safety. Prometheus text format exposition.
**Output:** `internal/metrics/collector.go`, `prometheus.go` with `collector_test.go`
**Dependencies:** DR-001
**Effort:** Medium

### DR-081 — Request Metrics Integration 🟢
**Scope:** Wire metrics into the request flow. Track: requests_total, request_duration, backend_up, backend_requests, certificate_expiry, ratelimit_rejected, system metrics (goroutines, memory).
**Output:** Metrics integration across proxy, middleware, health, and tls packages.
**Dependencies:** DR-080, DR-030, DR-055, DR-060, DR-044
**Effort:** Medium

---

## Phase 9: Integration & Wiring

### DR-090 — Main Entrypoint Wiring 🟢
**Scope:** Wire all components together in `cmd/dockrouter/main.go`. Startup sequence: parse config → init logger → init cert manager → init discovery → init route table → init middlewares → init health checker → init metrics → init admin server → start listeners → start discovery → ready. Graceful shutdown per IMPLEMENTATION.md §13.
**Output:** Complete `cmd/dockrouter/main.go`
**Dependencies:** All Phase 0-8 tasks
**Effort:** Large

### DR-091 — Docker Compose Example 🟢
**Scope:** Create example `docker-compose.yml` with DockRouter + 3 example services (API, frontend, blog) demonstrating all label features.
**Output:** `docker-compose.yml`, `examples/` directory
**Dependencies:** DR-090
**Effort:** Small

### DR-092 — Integration Tests 🟢
**Scope:** End-to-end tests using mock Docker socket. Test flows: container start → route created → request proxied → container stop → route removed. Test ACME with Pebble test server. Test rate limiting, health checks, middleware chain.
**Output:** `tests/integration/` directory
**Dependencies:** DR-090
**Effort:** Large

---

## Phase 10: Documentation & Release

### DR-100 — README.md 🟢
**Scope:** Comprehensive README: quick start, features, label reference, configuration, examples, comparison with Traefik, contributing guide.
**Output:** `README.md`
**Dependencies:** DR-090
**Effort:** Medium

### DR-101 — BRANDING.md 🟢
**Scope:** Brand guidelines: logo concept, color palette, typography, voice/tone, social media templates.
**Output:** `BRANDING.md`
**Dependencies:** None
**Effort:** Small

### DR-102 — GitHub Release Workflow 🟢
**Scope:** GoReleaser config, GitHub Actions release workflow (on tag push), multi-platform binaries, Docker image push to GHCR.
**Output:** `.goreleaser.yml`, `.github/workflows/release.yml`
**Dependencies:** DR-004
**Effort:** Medium

### DR-103 — Website Landing Page (dockrouter.com) 🟢
**Scope:** Single-page landing site: hero, features, quick start code block, comparison table, footer. Deploy to GitHub Pages or Cloudflare Pages.
**Output:** `website/` directory
**Dependencies:** DR-101
**Effort:** Medium

---

## Dependency Graph (Critical Path)

```
DR-001 (scaffold)
  ├─ DR-002 (config)
  ├─ DR-003 (logger)
  ├─ DR-004 (makefile)
  │
  ├─ DR-010 (docker client)
  │   ├─ DR-011 (labels)
  │   ├─ DR-012 (events)
  │   ├─ DR-013 (poller)
  │   └─ DR-014 (reconciler) ← depends on 010,011,012,013
  │
  ├─ DR-020 (route types)
  │   ├─ DR-021 (radix tree)
  │   ├─ DR-022 (route table) ← depends on 020,021
  │   └─ DR-023 (router handler) ← depends on 022
  │
  ├─ DR-030 (proxy)
  │   ├─ DR-031 (websocket)
  │   ├─ DR-032 (error pages)
  │   └─ DR-033 (listeners) ← depends on 023,030,002
  │
  ├─ DR-040 (cert store)
  │   ├─ DR-041 (acme core)
  │   ├─ DR-042 (acme flow) ← depends on 041
  │   ├─ DR-043 (challenge solver) ← depends on 033
  │   └─ DR-044 (tls manager) ← depends on 040,041,042,043
  │
  ├─ DR-050 (middleware chain)
  │   ├─ DR-051 (core middlewares)
  │   ├─ DR-052 (security headers)
  │   ├─ DR-053 (cors)
  │   ├─ DR-054 (compression)
  │   ├─ DR-055 (rate limiter)
  │   ├─ DR-056 (auth/ip filter)
  │   ├─ DR-057 (path mod/body)
  │   └─ DR-058 (retry/circuit breaker)
  │
  ├─ DR-060 (health checker)
  │
  ├─ DR-070 (admin api) ← depends on 022,044,060
  │   ├─ DR-071 (sse hub)
  │   └─ DR-072 (dashboard) ← depends on 070,071
  │
  ├─ DR-080 (metrics)
  │   └─ DR-081 (metrics integration)
  │
  └─ DR-090 (wiring) ← depends on ALL above
      ├─ DR-091 (docker-compose example)
      └─ DR-092 (integration tests)
```

## Recommended Implementation Order (Claude Code Sessions)

**Session 1:** DR-001, DR-002, DR-003, DR-004 (scaffold + config + logger + build)
**Session 2:** DR-010, DR-011, DR-012, DR-013 (Docker discovery)
**Session 3:** DR-020, DR-021, DR-022, DR-023 (routing engine)
**Session 4:** DR-030, DR-031, DR-032 (reverse proxy)
**Session 5:** DR-050, DR-051, DR-052, DR-053, DR-054 (middleware core)
**Session 6:** DR-055, DR-056, DR-057, DR-058 (middleware advanced)
**Session 7:** DR-060 (health checker)
**Session 8:** DR-040, DR-041, DR-042, DR-043, DR-044 (TLS/ACME — largest effort)
**Session 9:** DR-033 (wire listeners — needs proxy + TLS + router)
**Session 10:** DR-080, DR-081 (metrics)
**Session 11:** DR-070, DR-071 (admin API + SSE)
**Session 12:** DR-072 (admin dashboard UI)
**Session 13:** DR-014 (reconciler — needs all subsystems)
**Session 14:** DR-090, DR-091 (final wiring + examples)
**Session 15:** DR-092 (integration tests)
**Session 16:** DR-100, DR-101, DR-102, DR-103 (docs + release + website)
