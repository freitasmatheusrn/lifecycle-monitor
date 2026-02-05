# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o lifecycle-monitor ./cmd/main.go

# Runtime stage
FROM mcr.microsoft.com/playwright:v1.52.0-jammy

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/lifecycle-monitor .
COPY --from=builder /app/assets ./assets

# Install Playwright browsers
RUN npx playwright install chromium

# Expose port
EXPOSE 5000

# Run the application
CMD ["./lifecycle-monitor"]
