# Build stage
FROM golang:1.22-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /build

# Copy go mod files first for caching
COPY go.mod go.sum ./
RUN go mod download || true

# Copy source code
COPY . .

# Build arguments for version info
ARG VERSION=dev
ARG BUILD_TIME=unknown
ARG COMMIT=unknown

# Build static binary with optimizations
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X main.Version=${VERSION} -X main.BuildTime=${BUILD_TIME} -X main.Commit=${COMMIT}" \
    -o /dockrouter ./cmd/dockrouter

# Final stage - minimal scratch image
FROM scratch

# Copy CA certificates for HTTPS
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy timezone data
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

# Copy binary
COPY --from=builder /dockrouter /dockrouter

# Copy dashboard files
COPY --from=builder /build/cmd/dockrouter/dashboard /dashboard

# Create data directory
VOLUME /data

# Expose ports
# 80  - HTTP
# 443 - HTTPS
# 9090 - Admin Dashboard
EXPOSE 80 443 9090

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD ["/dockrouter", "healthcheck"] || exit 1

# Labels
LABEL org.opencontainers.image.title="DockRouter" \
      org.opencontainers.image.description="Zero-dependency Docker-native ingress router with automatic TLS" \
      org.opencontainers.image.vendor="DockRouter" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.source="https://github.com/DockRouter/dockrouter"

ENTRYPOINT ["/dockrouter"]
