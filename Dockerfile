# -----------------------
# Build stage
# -----------------------
FROM golang:1.24-bookworm AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build binary
RUN CGO_ENABLED=0 GOOS=linux go build -o lifecycle-monitor ./cmd/main.go

# Install playwright-go driver + browsers
RUN go run github.com/playwright-community/playwright-go/cmd/playwright@v1.52.0 install chromium

# -----------------------
# Runtime stage
# -----------------------
FROM debian:bookworm-slim

WORKDIR /app

# Chromium runtime deps
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

# App
COPY --from=builder /app/lifecycle-monitor .

# Playwright driver (ESSENCIAL)
COPY --from=builder /go/bin/playwright /usr/local/bin/playwright

# Browsers
COPY --from=builder /root/.cache/ms-playwright /root/.cache/ms-playwright

ENV PLAYWRIGHT_BROWSERS_PATH=/root/.cache/ms-playwright

EXPOSE 5000

CMD ["./lifecycle-monitor"]
