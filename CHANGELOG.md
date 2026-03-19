# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed
- Discovery package test coverage improved from 69.9% to 72.2%
- CMD package test coverage improved from 72.4% to 77.1%
- Router package test coverage improved from 96.7% to 97.9%
- TLS package test coverage improved from 80.9% to 83.0%
- Middleware package test coverage improved from 95.6% to 97.4%
- Overall project coverage improved to 88.5%
- Added comprehensive tests for GetContainerIP, Changed, handleEvent, printVersion, admin handlers, and weighted round robin edge cases
- Added TLS tests for processAuthorization, provisionCertificate error paths, and challenge solver edge cases
- Fixed duplicate test declarations in discovery package
- Added TLS edge case tests: needsRenewal nil cases, GetCertificate validation, Load/SaveAccountKey errors
- Added CMD edge case tests: handleRoutes with routes, handleStatus with components, admin handler auth tests
- Added discovery HTTP request tests for context deadlines and timeouts
- Added router backend tests: weighted round robin zero/negative weight edge cases
- Added middleware tests: AddTrustedProxy edge cases, IP extraction with trusted proxies, rate limiter zero rate
- Added discovery extended tests: GetContainerIP network priority, container state tests, network/subnet struct tests
- Added router tests: getNode/putNode pool functions, radix tree edge cases, common prefix edge cases
- Added TLS tests: provisionCertificate nil ACME, generateCSR, encodePrivateKey, Renew nil ACME
- Added CMD tests: start with HTTP only, initialize with data dir, shutdown edge cases, certificates handler
- Added discovery tests: doRequest/doStreamRequest error handling, Sync context cancellation, pollLoop/watchEvents cancellation
- Added TLS tests: Store.List empty, ACME Initialize invalid URL, Challenge solver handler tests

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
  - `cmd/dockrouter`: 67.5% → 72.4% (+4.9%)
  - `internal/proxy`: 89.6% → 95.7% (+6.1%)
  - `internal/tls`: 80.3% → 80.9% (+0.6%)
- New benchmark tests for router package (radix tree, backend pool selection)
- WebSocket test coverage improvements with error handling tests
- Rate limiter cleanup logic tests
- CI badge added to README for build status visibility
- Detailed README files for all examples:
  - `examples/websocket/` - WebSocket proxying with sticky sessions
  - `examples/rate-limiting/` - Rate limiting and circuit breaker patterns
  - `examples/microservices/` - Complete microservices architecture
- Dashboard embedded files tests
- Proxy transport and error page tests
- Extended poller tests for discovery package

### Changed
- WebSocket ServeHTTP coverage improved from 28.6% to 76.2%
- Example documentation now includes architecture diagrams and testing instructions

### Fixed
- ACME thumbprint calculation now follows RFC 7638 (JWK Thumbprint)
- Graceful shutdown now properly waits for active connections
- Retry logic verified working in router package
- `ParseLoadBalanceStrategy()` helper function for strategy parsing
- `dr.weight` label support for weighted load balancing
- X-Forwarded-For, X-Real-IP, CF-Connecting-IP header support in IP filtering
- Trusted proxy configuration for IP filtering (`AddTrustedProxy()`)
- Docker healthcheck command (`dockrouter healthcheck`)
- Detailed version command (`dockrouter version`)
- Performance benchmarks for load balancing and routing
- Load balancing example in `examples/loadbalancing/`
- Dependabot configuration for automated dependency updates
- CI benchmark job for performance tracking

### Fixed
- ACME thumbprint calculation now follows RFC 7638 (JWK Thumbprint)
- Graceful shutdown now properly waits for active connections
- Retry logic verified working in router package

### Changed
- Refactored duplicate IP network parsing code into `parseIPNetworks()` helper
- Updated linter configuration (`.golangci.yml`)
- Improved code formatting across all files

### Security
- X-Forwarded-For header validation with trusted proxy support

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

### Security
- Constant-time bcrypt comparison for auth
- No external dependencies (stdlib only)
- Minimal attack surface with scratch-based Docker image

[Unreleased]: https://github.com/DockRouter/dockrouter/compare/v1.0.0...HEAD
[1.0.0]: https://github.com/DockRouter/dockrouter/releases/tag/v1.0.0
