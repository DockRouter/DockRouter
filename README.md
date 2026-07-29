# 🚢 DockRouter

> **Zero-dependency, single-binary Docker-native ingress router with automatic TLS.**

<p align="center">
  <img src="dockrouter_.jpeg" alt="DockRouter — Zero-dependency Docker ingress router" width="100%" />
</p>

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![CI](https://github.com/DockRouter/dockrouter/actions/workflows/ci.yml/badge.svg)](https://github.com/DockRouter/dockrouter/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)](https://golang.org/dl/)
[![Docker](https://img.shields.io/badge/Docker-Ready-2496ED?style=flat&logo=docker)](https://hub.docker.com/r/dockrouter/dockrouter)
[![GitHub Release](https://img.shields.io/github/v/release/DockRouter/dockrouter?include_prereleases)](https://github.com/DockRouter/dockrouter/releases)
[![Coverage](https://img.shields.io/badge/Coverage-92.2%25-brightgreen)](.)

---

## ✨ Features

- **🚀 Zero external dependencies** — Pure Go stdlib, no external packages
- **🔒 Automatic TLS** — Let's Encrypt HTTP-01 challenge built-in
- **🏷️ Label-based discovery** — Configure routing via Docker labels
- **📦 Single binary** — <12MB, runs anywhere
- **🎛️ Built-in dashboard** — Admin UI on port 9090
- **🔥 Hot reload** — Routes update instantly when containers start/stop
- **🛡️ Production-ready** — Rate limiting, health checks, circuit breaker, CORS
- **🔌 WebSocket support** — Transparent WebSocket proxying
- **📊 Prometheus metrics** — Built-in metrics endpoint
- **⚖️ Load Balancing** — Round-robin, weighted, IP hash, least connections
- **🌐 Proxy Support** — X-Forwarded-For, X-Real-IP, Cloudflare headers

---

## 🚀 Quick Start

### Option 1: Docker (Recommended)

```bash
docker run -d \
  --name dockrouter \
  -p 80:80 -p 443:443 -p 9090:9090 \
  -v /var/run/docker.sock:/var/run/docker.sock:ro \
  -v dockrouter-data:/data \
  -e DR_ACME_EMAIL=you@example.com \
  -e DR_ADMIN_BIND=0.0.0.0 \
  -e DR_ADMIN_USER=admin -e DR_ADMIN_PASS=change-me \
  dockrouter/dockrouter:latest
```

> **Note:** the admin server binds to `127.0.0.1` by default, which inside a
> container is unreachable from a published port. Set `DR_ADMIN_BIND=0.0.0.0` to
> use the dashboard — and set `DR_ADMIN_USER`/`DR_ADMIN_PASS` when you do.

### Option 2: Docker Compose

```bash
git clone https://github.com/DockRouter/dockrouter.git
cd dockrouter
docker-compose up -d
```

### Option 3: Binary Download

```bash
# Linux/macOS
curl -sL https://github.com/DockRouter/dockrouter/releases/latest/download/dockrouter-$(uname -s)-$(uname -m) -o dockrouter
chmod +x dockrouter
sudo ./dockrouter --docker-socket /var/run/docker.sock

# Or with automatic installation
curl -sL https://raw.githubusercontent.com/DockRouter/dockrouter/main/install.sh | bash
```

---

## 📖 Usage

### Basic Example

Add labels to your containers:

```yaml
# docker-compose.yml
services:
  api:
    image: myapp/api
    labels:
      dr.enable: "true"
      dr.host: "api.example.com"
      dr.tls: "auto"
```

```bash
# Or with docker run
docker run -d \
  --label dr.enable=true \
  --label dr.host=api.example.com \
  --label dr.tls=auto \
  myapp/api
```

### Complete Docker Compose Example

```yaml
version: "3.8"

services:
  # DockRouter - The ingress router
  dockrouter:
    image: dockrouter/dockrouter:latest
    container_name: dockrouter
    restart: unless-stopped
    ports:
      - "80:80"
      - "443:443"
      - "9090:9090"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - dockrouter-data:/data
    environment:
      - DR_ACME_EMAIL=admin@example.com
      - DR_LOG_LEVEL=info
      # Required for the published 9090 port to reach the dashboard
      - DR_ADMIN_BIND=0.0.0.0
      - DR_ADMIN_USER=admin
      - DR_ADMIN_PASS=change-me
    labels:
      - "com.dockrouter.description=Ingress Router"

  # Example API Service
  api:
    image: nginx:alpine
    labels:
      dr.enable: "true"
      dr.host: "api.example.com"
      dr.tls: "auto"
      dr.healthcheck.path: "/health"
      dr.ratelimit: "100/m"

  # Example Web Service
  web:
    image: nginx:alpine
    labels:
      dr.enable: "true"
      dr.host: "www.example.com"
      dr.tls: "auto"
      dr.cors.origins: "https://example.com"
      dr.compress: "true"

volumes:
  dockrouter-data:
```

---

## 🏷️ Label Reference

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
| `dr.loadbalancer` | `roundrobin` | LB strategy: `roundrobin`, `iphash`, `leastconn`, `weighted` |
| `dr.weight` | `1` | Backend weight (used with `weighted` strategy) |

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
| `dr.healthcheck.threshold` | `3` | Failures before the backend leaves rotation |
| `dr.healthcheck.recovery` | `2` | Consecutive passes before it returns to rotation |

Backends are health checked actively. A backend that starts failing is taken out
of rotation and put back automatically once it recovers; the last healthy backend
of a route is never ejected, so a partial outage cannot become a total one.

---

## ⚙️ Configuration

### Environment Variables

All configuration can be set via environment variables with `DR_` prefix:

| Variable | Default | Description |
|----------|---------|-------------|
| `DR_HTTP_PORT` | `80` | HTTP listener port |
| `DR_HTTPS_PORT` | `443` | HTTPS listener port |
| `DR_ADMIN_PORT` | `9090` | Admin dashboard port |
| `DR_ADMIN_BIND` | `127.0.0.1` | Admin bind address |
| `DR_ADMIN_USER` | — | Admin username |
| `DR_ADMIN_PASS` | — | Admin password |
| `DR_ADMIN` | `true` | Enable admin interface |
| `DR_DOCKER_SOCKET` | `/var/run/docker.sock` | Docker socket path |
| `DR_DATA_DIR` | `/data` | Data directory |
| `DR_ACME_EMAIL` | — | ACME account email |
| `DR_ACME_PROVIDER` | `letsencrypt` | ACME provider |
| `DR_ACME_STAGING` | `false` | Use staging server |
| `DR_LOG_LEVEL` | `info` | Log level |
| `DR_ACCESS_LOG` | `true` | Enable access logging |
| `DR_DEFAULT_TLS` | `auto` | Default TLS mode for routes that set no `dr.tls` |
| `DR_TRUSTED_IPS` | — | Proxy CIDRs whose forwarded-for headers are trusted |

### CLI Flags

Same options available as CLI flags:

```bash
dockrouter --http-port=8080 --https-port=8443 --acme-email=you@example.com
```

### CLI Commands

```bash
# Show version information
dockrouter version

# Docker healthcheck
dockrouter healthcheck

# Show help
dockrouter --help
```

---

## 🔒 TLS Configuration

### Auto TLS (Let's Encrypt)

```yaml
labels:
  dr.enable: "true"
  dr.host: "api.example.com"
  dr.tls: "auto"
  dr.tls.domains: "api.example.com,www.example.com"
```

Set ACME email:
```bash
docker run -e DR_ACME_EMAIL=admin@example.com dockrouter/dockrouter
```

### Manual TLS

Point `dr.tls.cert` and `dr.tls.key` at the certificate and key files **as seen
from inside the DockRouter container**. Both labels are required when
`dr.tls: "manual"`.

```yaml
labels:
  dr.enable: "true"
  dr.host: "api.example.com"
  dr.tls: "manual"
  dr.tls.cert: "/certs/api.example.com/cert.pem"
  dr.tls.key: "/certs/api.example.com/key.pem"
```

Mount the certificates into DockRouter:
```bash
docker run -v /path/to/certs:/certs:ro dockrouter/dockrouter
```

Any extra domains listed in `dr.tls.domains` are served with the same pair, so a
SAN certificate covers all of them. Manual TLS does not require `DR_ACME_EMAIL`;
replacing the files on disk takes effect on the next discovery sync.

---

## 📊 Monitoring

### Health Endpoints

```bash
# Health check
curl http://localhost:9090/health

# Readiness check
curl http://localhost:9090/ready
```

### Prometheus Metrics

```bash
curl http://localhost:9090/metrics
```

Available metrics:

| Metric | Type | Description |
|--------|------|-------------|
| `dockrouter_http_requests_total` | counter | Total requests handled |
| `dockrouter_http_errors_total` | counter | Responses with status >= 400 |
| `dockrouter_http_request_duration_seconds` | histogram | Request latency |
| `dockrouter_http_requests_active` | gauge | Requests in flight |
| `dockrouter_active_connections` | gauge | Active backend connections |
| `dockrouter_backend_requests_total` | gauge | Requests forwarded to backends |
| `dockrouter_backend_errors_total` | gauge | Backend failures |
| `dockrouter_routes_total` | gauge | Active routes |
| `dockrouter_containers_total` | gauge | Discovered containers |
| `dockrouter_certificates_total` | gauge | Loaded certificates |

`/metrics` and `/api/v1/*` require admin credentials when `DR_ADMIN_USER` is set.
`/health` and `/ready` are always unauthenticated so container and orchestrator
probes work.

### Admin API

```bash
# List routes
curl http://localhost:9090/api/v1/routes

# List containers
curl http://localhost:9090/api/v1/containers

# List certificates
curl http://localhost:9090/api/v1/certificates

# Get status
curl http://localhost:9090/api/v1/status

# Get config
curl http://localhost:9090/api/v1/config

# Get metrics
curl http://localhost:9090/api/v1/metrics
```

---

## 🛡️ Security Features

### IP Filtering

```yaml
labels:
  dr.ipwhitelist: "192.168.0.0/16,10.0.0.0/8"
  dr.ipblacklist: "192.168.100.1/32"
```

**Behind Load Balancers:** IP filtering reads `X-Forwarded-For`, `X-Real-IP` and
`CF-Connecting-IP`, but **only from proxies you explicitly trust** — otherwise any
client could spoof its own address. List the balancer's CIDRs:

```bash
# Set trusted proxy IPs (via environment variable)
DR_TRUSTED_IPS=10.0.0.0/8,172.16.0.0/12
```

Without `DR_TRUSTED_IPS`, filtering uses the peer address of the connection.

### CORS

```yaml
labels:
  dr.cors.origins: "https://example.com,https://app.example.com"
  dr.cors.methods: "GET,POST,PUT,DELETE"
```

### Rate Limiting

```yaml
labels:
  dr.ratelimit: "100/m"
  dr.ratelimit.by: "client_ip"
```

### Circuit Breaker

```yaml
labels:
  dr.circuitbreaker: "5/30s"  # Open after 5 failures in 30s
```

### Basic Auth

```yaml
labels:
  dr.auth.basic.users: "admin:$2a$10$N9qo8uLOickgx2ZMRZoMy..."
```

---

## 🏗️ Architecture

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

---

## 🔧 Development

### Prerequisites

- Go 1.21+
- Docker & Docker Compose
- Make (optional)

### Build from Source

```bash
# Clone
git clone https://github.com/DockRouter/dockrouter.git
cd dockrouter

# Build
go build -o dockrouter ./cmd/dockrouter

# Or use Make
make build

# Run tests
make test

# Run with coverage
make test-coverage

# Run locally
make run
```

### Docker Build

```bash
# Build Docker image
docker build -t dockrouter:dev .

# Run locally
docker run -d \
  -v /var/run/docker.sock:/var/run/docker.sock:ro \
  -p 80:80 -p 443:443 -p 9090:9090 \
  dockrouter:dev
```

---

## 🏆 Code Quality

- **All 12 packages pass** `go vet`, the full test suite, and `go test -race ./...`
- **Behaviour verified end-to-end** — `internal/integration/` drives a real proxy
  through the production middleware chain to prove WebSocket upgrades, SSE
  streaming, replica load balancing, scale-down, retry body replay and backend
  health recovery actually work, rather than asserting against mocks
- **Security hardened** — CSP and HSTS headers, TLS 1.3, CORS origin checks,
  constant-time basic auth, unauthenticated surface limited to `/health` and `/ready`
- **Responses are streamed, not buffered** — a slow or large upstream response
  cannot pin memory in the router

---

## 📁 Project Structure

```
dockrouter/
├── cmd/dockrouter/          # Main application (84.1% coverage)
│   ├── main.go              # Entry point
│   └── dashboard/           # Admin dashboard
├── internal/
│   ├── admin/               # Admin server (95.7% coverage)
│   ├── config/              # Configuration (95.1% coverage)
│   ├── discovery/           # Docker discovery (96.6% coverage)
│   ├── health/              # Health checking (79.8% coverage)
│   ├── log/                 # Logging (96.5% coverage)
│   ├── metrics/             # Prometheus metrics (96.9% coverage)
│   ├── middleware/          # HTTP middleware (89.8% coverage)
│   ├── proxy/               # Reverse proxy (96.0% coverage)
│   ├── router/              # Route management (95.2% coverage)
│   ├── tls/                 # TLS/ACME (90.0% coverage)
│   └── integration/         # Cross-package behaviour tests
├── examples/                # Example configurations
├── docs/                    # Additional documentation
├── Dockerfile               # Multi-stage Docker build
├── docker-compose.yml       # Quick start compose file
├── Makefile                 # Build automation
└── README.md                # This file
```

**Overall test coverage: 92.2%** (CI gate: 80%), verified with `go test -race ./...`

---

## 📝 Examples

See [examples/](./examples/) directory:

| Example | Description |
|---------|-------------|
| [basic](./examples/basic/) | Simple HTTP routing |
| [tls-auto](./examples/tls-auto/) | Auto TLS with Let's Encrypt |
| [multi-app](./examples/multi-app/) | Multiple applications with path-based routing |
| [microservices](./examples/microservices/) | Full microservices architecture with middleware |
| [websocket](./examples/websocket/) | WebSocket proxying with sticky sessions |
| [rate-limiting](./examples/rate-limiting/) | Rate limiting and circuit breaker examples |
| [loadbalancing](./examples/loadbalancing/) | Round-robin, weighted, IP hash, least connections |

---

## 🆚 Comparison

| Feature | DockRouter | Traefik | Caddy | Nginx |
|---------|------------|---------|-------|-------|
| Zero dependencies | ✅ | ❌ | ❌ | ❌ |
| Single binary | ✅ | ✅ | ✅ | ❌ |
| Docker-native | ✅ | ✅ | ❌ | ❌ |
| Auto TLS | ✅ | ✅ | ✅ | ❌ |
| Built-in dashboard | ✅ | ✅ | ❌ | ❌ |
| No config files | ✅ | ❌ | ❌ | ❌ |
| Label-based config | ✅ | ✅ | ❌ | ❌ |
| <10MB binary | ✅ | ❌ | ❌ | ❌ |

---

## 🤝 Contributing

Contributions are welcome!

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

Please read [CONTRIBUTING.md](CONTRIBUTING.md) for details.

---

## 📄 License

MIT License - see [LICENSE](LICENSE) file.

---

## 📞 Support

- **Issues**: [GitHub Issues](https://github.com/DockRouter/dockrouter/issues)
- **Discussions**: [GitHub Discussions](https://github.com/DockRouter/dockrouter/discussions)

---

## 🙏 Acknowledgments

- Built with ❤️ using Go
- Inspired by Traefik and Nginx Proxy Manager
- Powered by Docker
