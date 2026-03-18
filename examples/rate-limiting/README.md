# Rate Limiting Example

This example demonstrates advanced rate limiting and circuit breaker patterns.

## Features Demonstrated

- **Per-IP rate limiting** - Token bucket algorithm per client IP
- **Per-header rate limiting** - Custom rate limits based on headers
- **Circuit breaker** - Automatic failure detection and recovery
- **Burst handling** - Configurable burst capacity

## Architecture

```
┌─────────────┐     ┌─────────────┐     ┌─────────────────┐
│   Clients   │────▶│  DockRouter │────▶│  API Backend    │
│  (Multiple) │     │  (Rate Limit│     │  (api-service)  │
└─────────────┘     │  + Circuit  │     └─────────────────┘
                    │  Breaker)   │
                    └─────────────┘
```

## Running the Example

```bash
cd examples/rate-limiting
docker-compose up -d
```

## Testing Rate Limits

### Test Per-IP Limiting (10 req/min)

```bash
# This will work
for i in {1..10}; do
  curl -s http://localhost/api/test | head -1
done

# This should return 429 Too Many Requests
for i in {1..5}; do
  curl -s -w "%{http_code}\n" http://localhost/api/test
done
```

### Test Circuit Breaker

Simulate backend failures:

```bash
# Stop the backend to trigger circuit breaker
docker-compose stop api-service

# Requests should fail fast with 503
curl -s -w "%{http_code}\n" http://localhost/api/test

# Restart backend
docker-compose start api-service

# After recovery period, requests should succeed
```

## Configuration Details

### Per-IP Rate Limiting

```yaml
labels:
  - "dr.ratelimit=100"
  - "dr.ratelimitwindow=60"
```

- **100 requests** per **60 seconds**
- **10 request** burst capacity
- Automatic cleanup of old entries

### Circuit Breaker

```yaml
labels:
  - "dr.circuitbreaker=true"
  - "dr.cbthreshold=5"
  - "dr.cbtimeout=30"
```

| State | Description |
|-------|-------------|
| `Closed` | Normal operation, requests pass through |
| `Open` | Failure threshold reached, requests blocked |
| `Half-Open` | Testing if backend recovered |

## Headers

Response headers indicate rate limit status:

```
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 95
X-RateLimit-Reset: 1710000000
```

## Cleanup

```bash
docker-compose down -v
```
