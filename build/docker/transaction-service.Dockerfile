FROM golang:1.25-alpine AS builder

WORKDIR /app
COPY pkg/ pkg/
COPY api/ api/
COPY services/transaction-service/ services/transaction-service/

WORKDIR /app/services/transaction-service
# Build both the service and the migrate tool
RUN go build -o /transaction-service ./cmd/main.go && \
    go build -o /migrate ./cmd/migrate/main.go

FROM alpine:latest
WORKDIR /app

COPY --from=builder /transaction-service .
COPY --from=builder /migrate .

# Config
COPY services/transaction-service/config/config.yaml ./config/config.yaml

# SQL migration files
COPY services/transaction-service/db/migrations/ ./db/migrations/

# Entrypoint: run migrations then start service
COPY build/docker/transaction-service-entrypoint.sh ./entrypoint.sh
RUN chmod +x ./entrypoint.sh

EXPOSE 8080
ENTRYPOINT ["./entrypoint.sh"]
