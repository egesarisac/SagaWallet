FROM golang:1.25-alpine AS builder

WORKDIR /app
COPY pkg/ pkg/
COPY api/ api/
COPY services/notification-service/ services/notification-service/

WORKDIR /app/services/notification-service
RUN go build -o /notification-service ./cmd/main.go

FROM alpine:latest
WORKDIR /app
COPY --from=builder /notification-service .
COPY services/notification-service/config/config.yaml ./config/config.yaml

EXPOSE 8084
CMD ["./notification-service"]
