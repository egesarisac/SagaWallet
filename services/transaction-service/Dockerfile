# Build stage
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git

# Copy all source code (workspace-aware build)
COPY go.work go.work.sum ./
COPY pkg/ ./pkg/
COPY services/ ./services/

# Build the specific service
WORKDIR /app/services/transaction-service
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /transaction-service ./cmd/main.go

# Runtime stage
FROM alpine:3.19

RUN apk --no-cache add ca-certificates tzdata wget

WORKDIR /app

# Copy binary from builder
COPY --from=builder /transaction-service .

# Copy migrations
COPY services/transaction-service/db/migrations ./db/migrations

EXPOSE 8083 9082

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -q -O /dev/null http://localhost:8083/health || exit 1

CMD ["./transaction-service"]
