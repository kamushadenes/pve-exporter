# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /build

# Install build dependencies
RUN apk add --no-cache git

# Copy go mod files
COPY go.mod go.sum* ./
RUN go mod download

# Copy source code
COPY . .

# Build binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo \
    -ldflags "-X main.version=${VERSION:-dev} -X main.commit=${COMMIT:-none} -X main.date=${BUILD_DATE:-unknown} -w -s" \
    -o pve-exporter .

# Runtime stage
FROM alpine:3.19

# Add ca-certificates for HTTPS
RUN apk --no-cache add ca-certificates

# Create non-root user
RUN addgroup -g 1000 exporter && \
    adduser -D -u 1000 -G exporter exporter

WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/pve-exporter .

# Change ownership
RUN chown -R exporter:exporter /app

# Switch to non-root user
USER exporter

# Expose metrics port
EXPOSE 9221

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:9221/health || exit 1

# Run the exporter
ENTRYPOINT ["/app/pve-exporter"]
