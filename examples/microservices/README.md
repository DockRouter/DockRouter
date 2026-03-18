# Microservices Example

This example demonstrates a complete microservices architecture with DockRouter.

## Features Demonstrated

- **Path-based routing** - Route by URL path prefix
- **Service discovery** - Automatic container registration
- **Middleware chain** - Auth, CORS, compression, rate limiting
- **Health checks** - HTTP and TCP health monitoring
- **Load balancing** - Multiple strategies per service

## Architecture

```
                    ┌─────────────┐
                    │  DockRouter │
                    │   (Gateway) │
                    └──────┬──────┘
                           │
        ┌──────────────────┼──────────────────┐
        │                  │                  │
        ▼                  ▼                  ▼
┌──────────────┐  ┌──────────────┐  ┌──────────────┐
│  /api/users  │  │  /api/orders │  │   /health    │
│  User Service│  │ Order Service│  │  Health Check│
│   (x2 inst)  │  │   (x2 inst)  │  │              │
└──────────────┘  └──────────────┘  └──────────────┘
```

## Running the Example

```bash
cd examples/microservices
docker-compose up -d
```

## Testing the Services

### User Service

```bash
# List users
curl http://localhost/api/users

# Get specific user
curl http://localhost/api/users/123
```

### Order Service

```bash
# List orders
curl http://localhost/api/orders

# Get order details
curl http://localhost/api/orders/456
```

### Health Checks

```bash
# Check DockRouter health
curl http://localhost:9090/health

# Check individual service health
docker-compose ps
```

## Configuration Details

### User Service

```yaml
labels:
  - "dr.enabled=true"
  - "dr.host=api.example.com"
  - "dr.pathprefix=/api/users"
  - "dr.port=8080"
  - "dr.loadbalancer=roundrobin"
```

### Order Service

```yaml
labels:
  - "dr.enabled=true"
  - "dr.host=api.example.com"
  - "dr.pathprefix=/api/orders"
  - "dr.port=8080"
  - "dr.loadbalancer=leastconn"
```

### Middleware Stack

Each service has a middleware chain:

1. **Request ID** - Unique identifier for tracing
2. **Access Log** - Request logging
3. **Recovery** - Panic handling
4. **CORS** - Cross-origin support
5. **Compression** - Gzip responses
6. **Rate Limit** - 100 req/min per IP
7. **Circuit Breaker** - Fail-fast on errors

## Admin Dashboard

Access the real-time dashboard:

```bash
open http://localhost:9090
```

Dashboard shows:
- Active routes
- Backend health status
- Request metrics
- Recent access logs

## Load Balancing Strategies

| Service | Strategy | Description |
|---------|----------|-------------|
| Users   | Round Robin | Even distribution |
| Orders  | Least Connections | Route to least busy |

## Scaling Services

```bash
# Scale user service to 3 instances
docker-compose up -d --scale user-service=3

# DockRouter automatically discovers new instances
```

## Cleanup

```bash
docker-compose down -v
```

## Production Considerations

- Use TLS certificates (`dr.tls=auto`)
- Configure appropriate rate limits
- Set up monitoring and alerting
- Use health checks for all services
- Implement proper logging
