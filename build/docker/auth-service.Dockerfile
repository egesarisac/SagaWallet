FROM golang:1.25-alpine AS builder

WORKDIR /app
COPY pkg/ pkg/
COPY services/auth-service/ services/auth-service/

WORKDIR /app/services/auth-service
RUN go build -o /auth-service ./cmd/main.go

FROM alpine:latest
WORKDIR /app

COPY --from=builder /auth-service .
COPY services/auth-service/config/config.yaml ./config/config.yaml

EXPOSE 8080
ENTRYPOINT ["./auth-service"]
