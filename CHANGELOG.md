# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

An end-to-end audit found that several documented features did not work at
runtime even though the unit test suite passed, because the tests asserted
against mocks that mirrored the same faulty assumptions. The behaviour is now
covered by `internal/integration/`, which drives a real proxy through the
production middleware chain.

### Fixed

#### Request handling
- **WebSocket proxying returned `503` and never upgraded.** Responses were
  buffered through an `httptest.ResponseRecorder`, which does not implement
  `http.Hijacker`, so the upgrade could not be performed. Responses are now
  streamed directly to the client
- **A WebSocket request marked its backend unhealthy**, letting any client take
  a backend out of rotation by sending an `Upgrade` header
- **Server-Sent Events and streaming responses were not delivered incrementally** —
  the whole response was held in memory until the upstream finished, so a large
  download consumed memory proportional to its size
- **Five middleware `ResponseWriter` wrappers dropped `Hijacker` and `Flusher`**,
  which broke upgrades and flushes for any route with middleware attached
- `Timeout` middleware and the servers' `WriteTimeout` severed long-lived
  connections; upgrades and event streams are now exempt and the servers use
  `ReadHeaderTimeout` instead
- **Retries lost the request body.** A bounded body (1 MB) is now buffered so a
  failed attempt can be replayed against the next backend; larger bodies stream
  to a single backend instead

#### Routing and load balancing
- **Replicas of the same service overwrote each other.** Routes were keyed so
  that only the last container for a host and path received traffic, making the
  round-robin, weighted, IP-hash and least-connections strategies unreachable.
  Containers sharing a host and path now merge into one backend pool
- **Stopping one replica removed the whole route**, taking healthy replicas
  offline. A route is now dropped only when its last backend is gone
- Failing backends were ejected permanently after a single error and the last
  healthy backend could be ejected, turning a partial outage into a total one.
  Ejection now requires consecutive failures and never removes the last backend

#### Health checking
- **The health checker was never connected.** `Register` was not called and
  `MarkHealthy` had no callers, so a backend taken out of rotation never
  returned. Check results now feed back into the route table, honouring
  `dr.healthcheck.*` including the previously unused `recovery` setting

#### TLS
- **`dr.tls: manual` never loaded the certificate.** `dr.tls.cert` and
  `dr.tls.key` were parsed and validated, then ignored
- **HTTPS did not start at all without `DR_ACME_EMAIL`**, so manual certificates
  could not be served. TLS now initialises whenever TLS is enabled, with ACME
  as an optional component
- **`dr.tls.domains` was ignored.** Additional SAN domains are now included in
  the ACME order and CSR, and resolve to the same certificate over SNI
- HTTP-to-HTTPS redirects were decided globally, so a route with `dr.tls: off`
  was still redirected to a port holding no certificate for it

#### Observability
- **Prometheus metrics were never collected** — the metrics middleware was not
  in the chain and `/metrics` returned an empty body
- **`/health`, `/ready` and `/metrics` returned `404`**; they are now served
- **The Docker `HEALTHCHECK` failed whenever `DR_ADMIN_USER` was set**, marking
  healthy containers unhealthy. Probes are now unauthenticated while the admin
  API stays protected
- `dockrouter healthcheck` ignored `DR_ADMIN_PORT`/`DR_ADMIN_BIND`

#### Configuration and packaging
- **`DR_TRUSTED_IPS` was parsed but never reached the IP filter**, so forwarded
  client addresses were not honoured behind a load balancer
- **The Dockerfile passed `-X main.Version`/`main.BuildTime`/`main.Commit` while
  the binary declares `version`/`buildTime`/`commit`**, so the linker silently
  ignored them and published images always reported `dev`
- The admin server binds to `127.0.0.1`, which is unreachable from a published
  container port; `docker-compose.yml` and the README now set `DR_ADMIN_BIND`

#### Concurrency
- Fixed two data races in the test suite (an unsynchronised SSE mock writer and
  a `time.Sleep`-synchronised assertion against `App.start`)

### Added
- `internal/integration/` — behaviour tests covering WebSocket upgrade and data
  relay, SSE streaming latency, round-robin distribution across replicas,
  scale-down, retry body replay, backend health recovery and manual TLS
- `dockrouter version` now reports the build commit
- `/ready` reports readiness separately from liveness, returning `503` while
  Docker discovery is down

### Changed
- Overall coverage 92.2%, verified with `go test -race ./...`
- Repository cleaned of build artifacts, profiling output and scratch files;
  `.gitignore` extended to keep them out

## [1.1.0] - 2024-03-18

### Added
- Router radix tree optimization with sync.Pool (reduced allocations from 18 to 16 allocs/op)
- Code of Conduct for community standards
- GitHub issue templates (bug report, feature request, question)
- Comprehensive benchmark suite for router package
- Additional TLS manager tests for error handling

### Changed
- WebSocket ServeHTTP coverage improved from 28.6% to 76.2%
- Proxy package coverage increased to 95.7%
- Example documentation with detailed README files
- CI badge added to README

## [1.0.0] - 2024-03-17

### Added

#### Core Features
- Docker container auto-discovery via socket or HTTP API
- Host-based routing (exact and wildcard matching)
- Path-based routing with radix tree for O(k) lookups
- Automatic TLS certificates via Let's Encrypt (ACME)
- WebSocket passthrough support
- Multi-backend load balancing (round-robin, least-connections, ip-hash, weighted)

#### Middleware
- Rate limiting (token bucket, per-IP/per-route/per-header)
- CORS with full preflight support
- Basic authentication with bcrypt
- IP whitelist/blacklist with CIDR notation
- Gzip compression
- Circuit breaker pattern
- Request retry with backoff
- Path prefix stripping/adding
- Request body size limiting
- Security headers (HSTS, X-Frame-Options, CSP, etc.)

#### Observability
- Prometheus metrics endpoint
- Structured JSON logging
- Access logging with request IDs
- Health check system (HTTP/TCP)
- Admin REST API
- Real-time dashboard with SSE

#### Operations
- Graceful shutdown
- Hot certificate reload
- Docker Compose examples
- One-click install script
- Multi-platform releases (Linux, macOS, Windows)
- Docker healthcheck command (`dockrouter healthcheck`)
- Version command (`dockrouter version`)

#### Tooling and documentation
- `ParseLoadBalanceStrategy()` helper and `dr.weight` label for weighted balancing
- Trusted proxy configuration for IP filtering (`AddTrustedProxy()`)
- Benchmarks for routing and load balancing
- Per-example README files and a load balancing example
- Dependabot configuration and a CI benchmark job

### Fixed
- ACME thumbprint calculation now follows RFC 7638 (JWK Thumbprint)
- Graceful shutdown now waits for active connections

### Changed
- Refactored duplicate IP network parsing into a `parseIPNetworks()` helper
- Updated linter configuration (`.golangci.yml`)

### Security
- Constant-time bcrypt comparison for auth
- X-Forwarded-For validation with trusted proxy support
- No external dependencies (stdlib only)
- Minimal attack surface with scratch-based Docker image

[Unreleased]: https://github.com/DockRouter/dockrouter/compare/v1.1.0...HEAD
[1.1.0]: https://github.com/DockRouter/dockrouter/compare/v1.0.0...v1.1.0
[1.0.0]: https://github.com/DockRouter/dockrouter/releases/tag/v1.0.0
