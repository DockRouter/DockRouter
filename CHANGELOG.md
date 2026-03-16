# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Initial release

## [1.0.0] - 2024-03-17

### Added

#### Core Features
- Docker container auto-discovery via socket or HTTP API
- Host-based routing (exact and wildcard matching)
- Path-based routing with radix tree for O(k) lookups
- Automatic TLS certificates via Let's Encrypt (ACME)
- WebSocket passthrough support
- Multi-backend load balancing (round-robin, least-connections, ip-hash)

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
