# DockRouter

**Zero-dependency, single-binary Docker-native ingress router with automatic TLS.**

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

## Features

- **Zero external dependencies** — Pure Go stdlib, no external packages
- **Automatic TLS** — Let's Encrypt HTTP-01 challenge built-in
- **Label-based discovery** — Configure routing via Docker labels
- **Single binary** — <10MB, runs anywhere
- **Built-in dashboard** — Admin UI on port 9090
- **Hot reload** — Routes update instantly when containers start/stop
- **Production-ready** — Rate limiting, health checks, circuit breaker, CORS
- **WebSocket support** — Transparent WebSocket proxying

## Quick Start

```bash
# Run with Docker
docker run -d \
  --name dockrouter \
  -p 80:80 -p 443:443 -p 9090:9090 \
  -v /var/run/docker.sock:/var/run/docker.sock:ro \
  -v dockrouter-data:/data \
  -e DR_ACME_EMAIL=you@example.com \
  dockrouter:latest
```

## Configuration via Labels

Add labels to your containers:

```yaml
services:
  api:
    image: myapp/api
    labels:
      dr.enable: "true"
      dr.host: "api.example.com"
      dr.tls: "auto"
      dr.ratelimit: "100/m"
      dr.cors.origins: "https://app.example.com"
```

## Label Reference

### Required Labels

| Label | Description | Example |
|-------|-------------|---------|
| `dr.enable` | Enable routing for this container | `true` |
| `dr.host` | Domain to route to this container | `api.example.com` |

### Routing Labels

| Label | Default | Description |
|-------|---------|-------------|
| `dr.port` | Auto-detect | Container port to proxy to |
| `dr.path` | `/` | Path prefix for routing |
| `dr.priority` | `0` | Route priority (higher wins) |
| `dr.address` | Auto | Explicit backend address |
| `dr.loadbalancer` | `roundrobin` | LB strategy: `roundrobin`, `iphash` |
| `dr.weight` | `1` | Backend weight for weighted LB |

### TLS Labels

| Label | Default | Description |
|-------|---------|-------------|
| `dr.tls` | `auto` | TLS mode: `auto`, `manual`, `off` |
| `dr.tls.domains` | Same as `dr.host` | Additional SAN domains |
| `dr.tls.cert` | — | Path to manual cert file |
| `dr.tls.key` | — | Path to manual key file |

### Middleware Labels

| Label | Description |
|-------|-------------|
| `dr.ratelimit` | Rate limit: `100/m`, `10/s`, `5000/h` |
| `dr.ratelimit.by` | Rate limit key: `client_ip`, `X-API-Key` |
| `dr.cors.origins` | Allowed CORS origins |
| `dr.cors.methods` | Allowed CORS methods |
| `dr.compress` | Enable gzip compression (`true`) |
| `dr.auth.basic.users` | Basic auth: `user:bcrypt_hash` |
| `dr.ipwhitelist` | Allowed IPs (CIDR) |
| `dr.ipblacklist` | Blocked IPs (CIDR) |
| `dr.stripprefix` | Strip path prefix before forwarding |
| `dr.addprefix` | Add path prefix before forwarding |
| `dr.maxbody` | Max request body size (e.g., `10mb`) |
| `dr.retry` | Retry count on failure |
| `dr.circuitbreaker` | Circuit breaker: `5/30s` |

### Health Check Labels

| Label | Default | Description |
|-------|---------|-------------|
| `dr.healthcheck.path` | `/` | Health check path |
| `dr.healthcheck.interval` | `10s` | Check interval |
| `dr.healthcheck.timeout` | `5s` | Check timeout |
| `dr.healthcheck.threshold` | `3` | Failures before unhealthy |

## Environment Variables

All configuration can be set via environment variables with `DR_` prefix:

| Variable | Default | Description |
|----------|---------|-------------|
| `DR_HTTP_PORT` | `80` | HTTP listener port |
| `DR_HTTPS_PORT` | `443` | HTTPS listener port |
| `DR_ADMIN_PORT` | `9090` | Admin dashboard port |
| `DR_ADMIN_BIND` | `127.0.0.1` | Admin bind address |
| `DR_ADMIN_USER` | — | Admin username |
| `DR_ADMIN_PASS` | — | Admin password |
| `DR_DOCKER_SOCKET` | `/var/run/docker.sock` | Docker socket path |
| `DR_DATA_DIR` | `/data` | Data directory |
| `DR_ACME_EMAIL` | — | ACME account email |
| `DR_ACME_PROVIDER` | `letsencrypt` | ACME provider |
| `DR_ACME_STAGING` | `false` | Use staging server |
| `DR_LOG_LEVEL` | `info` | Log level |
| `DR_ACCESS_LOG` | `true` | Enable access logging |

## CLI Flags

Same options available as CLI flags:

```bash
dockrouter --http-port=8080 --https-port=8443 --acme-email=you@example.com
```

## Docker Compose Example

```yaml
version: "3.8"

services:
  dockrouter:
    image: dockrouter:latest
    ports:
      - "80:80"
      - "443:443"
      - "9090:9090"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - dockrouter-data:/data
    environment:
      - DR_ACME_EMAIL=admin@example.com

  api:
    image: myapp/api
    labels:
      dr.enable: "true"
      dr.host: "api.example.com"
      dr.tls: "auto"

  web:
    image: myapp/web
    labels:
      dr.enable: "true"
      dr.host: "www.example.com"
      dr.tls: "auto"
```

## Admin Dashboard

Access the admin dashboard at `http://localhost:9090`

- View all routes and their status
- Monitor certificates and expiry
- View discovered containers
- Real-time metrics

## Architecture

```
                    ┌─────────────────────────────────────┐
                    │           DockRouter Binary          │
  :80 HTTP ────────▶│  Listener → Middleware → Router      │
  :443 HTTPS ──────▶│              ↓                       │──▶ Container
                    │         Backend Pool                 │
  :9090 Admin ─────▶│  Dashboard + REST API               │
                    └─────────────────────────────────────┘
                                      │
                             /var/run/docker.sock
```

## Building

```bash
# Build
make build

# Build for all platforms
make build-all

# Docker build
make docker
```

## Development

```bash
# Run tests
make test

# Run locally
./bin/dockrouter --acme-email=test@example.com --log-level=debug
```

## Comparison

| Feature | DockRouter | Traefik | Caddy | Nginx |
|---------|------------|---------|-------|-------|
| Zero dependencies | ✅ | ❌ | ❌ | ❌ |
| Single binary | ✅ | ✅ | ✅ | ❌ |
| Docker-native | ✅ | ✅ | ❌ | ❌ |
| Auto TLS | ✅ | ✅ | ✅ | ❌ |
| Built-in dashboard | ✅ | ✅ | ❌ | ❌ |
| No config files | ✅ | ❌ | ❌ | ❌ |
| Label-based config | ✅ | ✅ | ❌ | ❌ |

## License

MIT License - see [LICENSE](LICENSE)
