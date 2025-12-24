# Multi-stage build for API server
# Stage 1: Build
FROM golang:1.25-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /build

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary
# CGO_ENABLED=0 creates a statically linked binary
# -ldflags="-w -s" strips debug info to reduce size
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s" \
    -o /build/bin/api \
    ./cmd/api

# Stage 2: Runtime
FROM alpine:latest

# Install CA certificates for HTTPS requests, wget for health checks, and grpc_health_probe for Envoy
RUN apk --no-cache add ca-certificates tzdata wget curl && \
    wget -qO /usr/local/bin/grpc_health_probe https://github.com/grpc-ecosystem/grpc-health-probe/releases/download/v0.4.24/grpc_health_probe-linux-amd64 && \
    chmod +x /usr/local/bin/grpc_health_probe

# Create non-root user
RUN addgroup -g 1000 appuser && \
    adduser -D -u 1000 -G appuser appuser

WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/bin/api /app/api

# Change ownership to non-root user
RUN chown -R appuser:appuser /app

# Switch to non-root user
USER appuser

# Expose ports
# 8081: gRPC server
# 2112: Metrics/health endpoints
EXPOSE 8081 2112

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:2112/health || exit 1

# Run the application
CMD ["/app/api"]
