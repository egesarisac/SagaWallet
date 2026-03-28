.PHONY: help proto migrate test run-wallet run-transaction run-notification docker-up docker-down lint

# Default target
help:
	@echo "Go-Fintech Makefile"
	@echo ""
	@echo "Usage:"
	@echo "  make proto              Generate Go code from proto files"
	@echo "  make migrate-wallet     Run wallet service migrations"
	@echo "  make migrate-txn        Run transaction service migrations"
	@echo "  make test               Run all unit tests"
	@echo "  make test-integration   Run integration tests (requires running services)"
	@echo "  make lint               Run linter"
	@echo "  make run-wallet         Run wallet service"
	@echo "  make run-transaction    Run transaction service"
	@echo "  make run-notification   Run notification service"
	@echo "  make docker-up          Start infrastructure (Postgres, Kafka)"
	@echo "  make docker-full        Start full stack with all services"
	@echo "  make docker-down        Stop all Docker containers"
	@echo "  make swagger-serve      Start Swagger UI at localhost:8085"
	@echo "  make token              Generate JWT token"
	@echo "  make tidy               Run go mod tidy for all modules"
	@echo ""

# ===================
# Proto Generation
# ===================

proto:
	@echo "Generating protobuf code..."
	@./scripts/generate-proto.sh

# ===================
# Database Migrations
# ===================

migrate-wallet:
	@echo "Running wallet service migrations..."
	@cd services/wallet-service && go run cmd/migrate/main.go up

migrate-wallet-down:
	@echo "Rolling back wallet service migrations..."
	@cd services/wallet-service && go run cmd/migrate/main.go down

migrate-txn:
	@echo "Running transaction service migrations..."
	@cd services/transaction-service && go run cmd/migrate/main.go up

migrate-txn-down:
	@echo "Rolling back transaction service migrations..."
	@cd services/transaction-service && go run cmd/migrate/main.go down

# ===================
# Testing
# ===================

test:
	@echo "Running all unit tests..."
	@cd pkg/middleware && go test ./... -v -cover
	@cd services/wallet-service && go test ./... -v -cover
	@cd services/transaction-service && go test ./... -v -cover

test-unit:
	@echo "Running unit tests..."
	@cd pkg/middleware && go test ./... -v -cover
	@cd services/wallet-service && go test ./... -v -cover
	@cd services/transaction-service && go test ./... -v -cover

test-integration:
	@echo "Running integration tests (requires running services)..."
	@cd tests/integration && GOWORK=off go mod tidy && JWT_TOKEN=$$(cd ../../tools/tokengen && GOWORK=off JWT_SECRET=dev-local-jwt-secret-change-me go run main.go | tail -1) GOWORK=off go test -v

test-wallet:
	@echo "Running wallet service tests..."
	@cd services/wallet-service && go test ./... -v -cover

test-txn:
	@echo "Running transaction service tests..."
	@cd services/transaction-service && go test ./... -v -cover

test-middleware:
	@echo "Running middleware tests..."
	@cd pkg/middleware && go test ./... -v -cover

test-notification:
	@echo "Running notification service tests..."
	@cd services/notification-service && go test ./... -v -cover

# ===================
# Documentation
# ===================

swagger-serve:
	@echo "Starting Swagger UI at http://localhost:8085..."
	@echo "Available APIs: Wallet Service & Transaction Service (use dropdown)"
	@docker run -p 8085:8080 \
		-e URLS="[{url:'/specs/wallet-openapi.yaml',name:'Wallet Service (8081)'},{url:'/specs/transaction-openapi.yaml',name:'Transaction Service (8083)'}]" \
		-v $(PWD)/docs:/usr/share/nginx/html/specs \
		swaggerapi/swagger-ui

swagger-wallet:
	@echo "Starting Swagger UI for Wallet Service at http://localhost:8085..."
	@docker run -p 8085:8080 -e SWAGGER_JSON=/api/wallet-openapi.yaml -v $(PWD)/docs:/api swaggerapi/swagger-ui

swagger-transaction:
	@echo "Starting Swagger UI for Transaction Service at http://localhost:8085..."
	@docker run -p 8085:8080 -e SWAGGER_JSON=/api/transaction-openapi.yaml -v $(PWD)/docs:/api swaggerapi/swagger-ui

token:
	@cd tools/tokengen && JWT_SECRET=dev-local-jwt-secret-change-me go run main.go


# ===================
# Running Services
# ===================

run-wallet:
	@echo "Starting wallet service..."
	@cd services/wallet-service && go run cmd/main.go

run-transaction:
	@echo "Starting transaction service..."
	@cd services/transaction-service && go run cmd/main.go

run-notification:
	@echo "Starting notification service..."
	@cd services/notification-service && go run cmd/main.go

# ===================
# Docker
# ===================

docker-up:
	@echo "Starting Docker containers..."
	@docker-compose up -d

docker-full:
	@echo "Starting full stack with all microservices..."
	@docker-compose -f docker-compose.full.yml up -d --build

docker-down:
	@echo "Stopping Docker containers..."
	@docker-compose down

docker-logs:
	@docker-compose logs -f

docker-clean:
	@echo "Cleaning up Docker volumes..."
	@docker-compose down -v

# ===================
# Development
# ===================

tidy:
	@echo "Running go mod tidy for all modules..."
	@cd pkg && go mod tidy
	@cd services/wallet-service && go mod tidy
	@cd services/transaction-service && go mod tidy
	@cd services/notification-service && go mod tidy

lint:
	@echo "Running linter..."
	@golangci-lint run ./...

fmt:
	@echo "Formatting code..."
	@gofmt -s -w .

# ===================
# SQLC
# ===================

sqlc-wallet:
	@echo "Generating sqlc code for wallet service..."
	@cd services/wallet-service && sqlc generate

sqlc-txn:
	@echo "Generating sqlc code for transaction service..."
	@cd services/transaction-service && sqlc generate

sqlc:
	@make sqlc-wallet
	@make sqlc-txn
