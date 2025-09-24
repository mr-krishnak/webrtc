# Multi-stage Dockerfile for SIP Server

# Build stage
FROM golang:1.21-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN make build

# Runtime stage
FROM alpine:latest

# Install runtime dependencies
RUN apk add --no-cache \
    ca-certificates \
    ffmpeg \
    tzdata

# Create user for security
RUN addgroup -S sipserver && adduser -S sipserver -G sipserver

# Set working directory
WORKDIR /app

# Copy binary from builder stage
COPY --from=builder /app/bin/sip-server /app/sip-server

# Copy configuration and media files
COPY --from=builder /app/configs /app/configs
COPY --from=builder /app/media-files /app/media-files

# Create directories for runtime
RUN mkdir -p /app/logs /app/tmp

# Change ownership
RUN chown -R sipserver:sipserver /app

# Switch to non-root user
USER sipserver

# Expose ports
EXPOSE 5060/udp
EXPOSE 10000-40000/udp

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD timeout 3s nc -u localhost 5060 < /dev/null || exit 1

# Set environment variables
ENV CONFIG_FILE=/app/configs/config.yaml
ENV LOG_LEVEL=info

# Run the application
CMD ["/app/sip-server", "-config", "/app/configs/config.yaml"]