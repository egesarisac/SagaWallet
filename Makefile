GOLANGCI_LINT_VERSION := v2.1.5
SAGAWALLET_GO_CACHE ?= /tmp/sagawallet-go-build-cache
SAGAWALLET_LINT_CACHE ?= /tmp/sagawallet-golangci-lint-cache
SAGAWALLET_NPM_CACHE ?= /tmp/sagawallet-npm-cache
TEST_MODULES := api:0 pkg:25 pkg/errors:0 pkg/middleware:50 services/auth-service:30 services/wallet-service:5 services/transaction-service:5 services/notification-service:20 tools/tokengen:0 tools/eventreplay:10
LINT_MODULES := api pkg pkg/errors pkg/middleware services/auth-service services/wallet-service services/transaction-service services/notification-service tests/integration tools/tokengen tools/eventreplay

.PHONY: help proto migrate test test-unit test-race test-module test-integration test-integration-stack ci fmt-check contract security sbom run-wallet run-transaction run-notification retry-outbox docker-up docker-down lint lint-module

# Default target
help:
	@echo "Go-Fintech Makefile"
	@echo ""
	@echo "Usage:"
	@echo "  make proto              Generate Go code from proto files"
	@echo "  make migrate-wallet     Run wallet service migrations"
	@echo "  make migrate-txn        Run transaction service migrations"
	@echo "  make test               Run all unit tests"
	@echo "  make test-race          Run all unit tests with the race detector"
	@echo "  make test-integration   Run integration tests against the full local stack"
	@echo "  make ci                 Run the complete local validation pipeline used by CI"
	@echo "  make lint               Run linter"
	@echo "  make run-wallet         Run wallet service"
	@echo "  make run-auth           Run auth service"
	@echo "  make run-transaction    Run transaction service"
	@echo "  make run-notification   Run notification service"
	@echo "  make retry-outbox       Retry OUTBOX_ID with REASON (optional ACTOR)"
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

retry-outbox:
	@test -n "$(OUTBOX_ID)" || (echo "OUTBOX_ID is required" && exit 2)
	@test -n "$(REASON)" || (echo "REASON is required" && exit 2)
	@cd services/transaction-service && go run ./cmd/outbox-retry -id "$(OUTBOX_ID)" -reason "$(REASON)" $(if $(ACTOR),-actor "$(ACTOR)")

# ===================
# Testing
# ===================

test:
	@echo "Running all unit tests..."
	@cd pkg && go test ./... -v -cover
	@cd services/auth-service && go test ./... -v -cover
	@cd services/wallet-service && go test ./... -v -cover
	@cd services/transaction-service && go test ./... -v -cover
	@cd services/notification-service && go test ./... -v -cover

test-unit: test

test-race:
	@echo "Running unit tests with race detection and coverage floors..."
	@set -eu; for spec in $(TEST_MODULES); do \
		module="$${spec%:*}"; minimum="$${spec##*:}"; \
		GOCACHE="$(SAGAWALLET_GO_CACHE)" bash scripts/test-module.sh "$$module" "$$minimum"; \
	done

test-module:
	@test -n "$(MODULE)" || (echo "MODULE is required" && exit 2)
	@test -n "$(COVERAGE_MIN)" || (echo "COVERAGE_MIN is required" && exit 2)
	@GOCACHE="$(SAGAWALLET_GO_CACHE)" bash scripts/test-module.sh "$(MODULE)" "$(COVERAGE_MIN)"

fmt-check:
	@unformatted="$$(find . -type f -name '*.go' -not -path './.codebase-memory/*' -exec gofmt -l {} +)"; \
	if [ -n "$$unformatted" ]; then \
		echo "Run gofmt on:"; echo "$$unformatted"; exit 1; \
	fi

ci: fmt-check lint test-race contract security sbom test-integration-stack

test-integration:
	@echo "Running integration tests against the configured services..."
	@cd tests/integration && RUN_INTEGRATION=1 GOWORK=off go test -count=1 -timeout 2m -v

test-integration-stack:
	@command -v docker >/dev/null 2>&1 || (echo "docker is required for integration CI" && exit 2)
	@set -eu; \
		cleanup() { docker compose -f docker-compose.full.yml down --volumes --remove-orphans; }; \
		trap cleanup EXIT INT TERM; \
		docker compose -f docker-compose.full.yml up --build --detach --wait; \
		$(MAKE) test-integration

contract:
	@npm_config_cache="$(SAGAWALLET_NPM_CACHE)" npx --yes @redocly/cli@1.25.0 lint docs/openapi.yaml docs/wallet-openapi.yaml docs/transaction-openapi.yaml

security:
	@command -v trivy >/dev/null 2>&1 || (echo "trivy is required; install the CI-pinned release before running make ci" && exit 2)
	@trivy fs --format table --exit-code 1 --severity HIGH,CRITICAL .

sbom:
	@command -v syft >/dev/null 2>&1 || (echo "syft is required; install the CI-pinned release before running make ci" && exit 2)
	@syft dir:. -o cyclonedx-json=sbom.cdx.json

test-wallet:
	@echo "Running wallet service tests..."
	@cd services/wallet-service && go test ./... -v -cover

test-auth:
	@echo "Running auth service tests..."
	@cd services/auth-service && go test ./... -v -cover

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

run-auth:
	@echo "Starting auth service..."
	@cd services/auth-service && go run cmd/main.go

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
	@cd services/auth-service && go mod tidy
	@cd services/wallet-service && go mod tidy
	@cd services/transaction-service && go mod tidy
	@cd services/notification-service && go mod tidy

lint:
	@echo "Running pinned golangci-lint..."
	@set -eu; for module in $(LINT_MODULES); do \
		$(MAKE) --no-print-directory lint-module MODULE="$$module"; \
	done

lint-module:
	@test -n "$(MODULE)" || (echo "MODULE is required" && exit 2)
	@cd "$(MODULE)" && GOCACHE="$(SAGAWALLET_GO_CACHE)" GOLANGCI_LINT_CACHE="$(SAGAWALLET_LINT_CACHE)" go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run --timeout=5m ./...

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
