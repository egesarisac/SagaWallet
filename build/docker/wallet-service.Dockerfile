FROM golang:1.25-alpine AS builder

WORKDIR /app
COPY pkg/ pkg/
COPY api/ api/
COPY services/wallet-service/ services/wallet-service/

WORKDIR /app/services/wallet-service
# Build both the service and the migrate tool
RUN go build -o /wallet-service ./cmd/main.go && \
    go build -o /migrate ./cmd/migrate/main.go

FROM alpine:latest
WORKDIR /app

COPY --from=builder /wallet-service .
COPY --from=builder /migrate .

# Config
COPY services/wallet-service/config/config.yaml ./config/config.yaml

# SQL migration files
COPY services/wallet-service/db/migrations/ ./db/migrations/

# Entrypoint: run migrations then start service
COPY build/docker/wallet-service-entrypoint.sh ./entrypoint.sh
RUN chmod +x ./entrypoint.sh

EXPOSE 8080 9081
ENTRYPOINT ["./entrypoint.sh"]
