# DockRouter Examples

This directory contains example configurations for common use cases.

## Examples

### Basic Setup (`basic/`)
Simple single-service routing configuration.

```bash
cd basic
docker-compose up -d
```

### Multi-App (`multi-app/`)
Multiple services with path-based routing, authentication, and rate limiting.

```bash
cd multi-app
docker-compose up -d
```

### Load Balancing (`loadbalancing/`)
Multiple backends with different load balancing strategies.

```bash
cd loadbalancing
# Add hosts to /etc/hosts:
# 127.0.0.1 rr.example.local weighted.example.local sticky.example.local leastconn.example.local
docker-compose up -d
```

### TLS with Auto-Cert (`tls-auto/`)
Automatic SSL/TLS certificate provisioning with Let's Encrypt.

```bash
cd tls-auto
# Edit docker-compose.yml to set your domain and email
docker-compose up -d
```

### WebSocket (`websocket/`)
WebSocket proxying with sticky sessions for stateful connections.

```bash
cd websocket
# Add hosts to /etc/hosts:
# 127.0.0.1 ws.example.local chat.example.local
docker-compose up -d
# Test: wscat -c ws://ws.example.local/ws
```

### Rate Limiting (`rate-limiting/`)
Per-IP and per-header rate limiting with circuit breaker.

```bash
cd rate-limiting
# Add hosts to /etc/hosts:
# 127.0.0.1 api.example.local public.example.local admin.example.local
docker-compose up -d
# Test: for i in $(seq 1 20); do curl -s -o /dev/null -w "%{http_code}\n" http://api.example.local; done
```

### Microservices (`microservices/`)
Full microservices architecture with path-based routing, per-service middleware, health checks, and load balancing.

```bash
cd microservices
# Add to /etc/hosts: 127.0.0.1 app.example.local
docker-compose up -d
# curl http://app.example.local/api/
```

## Common Labels

| Label | Description | Example |
|-------|-------------|---------|
| `dr.enable` | Enable routing | `true` |
| `dr.host` | Hostname to route | `app.example.com` |
| `dr.port` | Container port | `8080` |
| `dr.path` | Path prefix | `/api` |
| `dr.tls` | TLS mode | `auto`, `off` |
| `dr.auth.basic.users` | Basic auth (htpasswd) | `user:$2a$10$...` |
| `dr.ratelimit` | Rate limit | `100/m` |
| `dr.cors.origins` | CORS origins | `*` |
| `dr.compress` | Enable compression | `true` |
| `dr.loadbalancer` | LB strategy | `roundrobin`, `weighted`, `iphash`, `leastconn` |
| `dr.weight` | Backend weight | `5` |
| `dr.retry` | Retry attempts | `3` |
