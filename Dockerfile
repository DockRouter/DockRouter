# Build stage
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Copy go.mod first for caching
COPY go.mod ./
RUN go mod download || true

# Copy source
COPY . .

# Build static binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /dockrouter ./cmd/dockrouter

# Final stage - minimal scratch image
FROM scratch

# Copy binary
COPY --from=builder /dockrouter /dockrouter

# Copy dashboard files
COPY dashboard/ /dashboard/

# Create data directory
VOLUME /data

# Expose ports
EXPOSE 80 443 9090

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD ["/dockrouter", "healthcheck"] || exit 1

ENTRYPOINT ["/dockrouter"]
