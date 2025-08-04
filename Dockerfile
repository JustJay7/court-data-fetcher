FROM golang:1.23-bookworm AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -o court-data-fetcher cmd/server/main.go

# Runtime stage
FROM debian:bookworm-slim

# Install dependencies and apply security updates
RUN apt-get update && apt-get upgrade -y && apt-get install -y \
    ca-certificates \
    chromium \
    chromium-driver \
    sqlite3 \
    && rm -rf /var/lib/apt/lists/*

# Create non-root user
RUN useradd -m -u 1000 appuser

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/court-data-fetcher .

# Copy web assets
COPY --from=builder /app/web ./web

# Create data directory
RUN mkdir -p data && chown -R appuser:appuser /app

# Switch to non-root user
USER appuser

# Expose port
EXPOSE 8080

# Set environment variables
ENV ROD_BROWSER_PATH=/usr/bin/chromium

# Run the application
CMD ["./court-data-fetcher"]