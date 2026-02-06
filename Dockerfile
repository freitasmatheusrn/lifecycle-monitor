# Build stage
FROM golang:1.24-bookworm AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Install goose for migrations (before COPY . . so it's cached)
RUN go install github.com/pressly/goose/v3/cmd/goose@latest

# Install playwright-go driver and chromium browser (before COPY . . so it's cached)
RUN go run github.com/playwright-community/playwright-go/cmd/playwright@latest install --with-deps chromium

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o lifecycle-monitor ./cmd/main.go

# Runtime stage - use Debian with all browser dependencies pre-installed
FROM debian:bookworm-slim

WORKDIR /app

# Install runtime dependencies for Chromium
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    libnss3 \
    libnspr4 \
    libatk1.0-0 \
    libatk-bridge2.0-0 \
    libcups2 \
    libdrm2 \
    libdbus-1-3 \
    libxkbcommon0 \
    libatspi2.0-0 \
    libxcomposite1 \
    libxdamage1 \
    libxfixes3 \
    libxrandr2 \
    libgbm1 \
    libasound2 \
    libpango-1.0-0 \
    libcairo2 \
    fonts-liberation \
    && rm -rf /var/lib/apt/lists/*

# Copy binary and assets from builder
COPY --from=builder /app/lifecycle-monitor .
COPY --from=builder /app/assets ./assets

# Copy goose binary and migrations
COPY --from=builder /go/bin/goose /usr/local/bin/goose
COPY --from=builder /app/internal/database/postgres/migrations ./internal/database/postgres/migrations

# Copy playwright browsers and driver from builder
COPY --from=builder /root/.cache/ms-playwright /root/.cache/ms-playwright
COPY --from=builder /root/.cache/ms-playwright-go /root/.cache/ms-playwright-go

# Expose port
EXPOSE 5000

# Run the application
CMD ["./lifecycle-monitor"]
