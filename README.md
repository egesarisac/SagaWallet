# SagaWallet 🏦

<div align="center">

![Go Version](https://img.shields.io/badge/Go-1.25-00ADD8?style=for-the-badge&logo=go)
![Architecture](https://img.shields.io/badge/Architecture-Microservices-purple?style=for-the-badge)
![Cloud](https://img.shields.io/badge/Cloud-GCP-blue?style=for-the-badge&logo=google-cloud)
![License](https://img.shields.io/badge/License-MIT-green?style=for-the-badge)

**A production-ready distributed wallet system built with Go microservices, Saga Choreography, and deployed on GCP Cloud Run.**

[Features](#-features) • [Quick Start](#-quick-start) • [API](#-api-endpoints) • [Architecture](#-architecture) • [Docker](#-docker-deployment)

</div>

---

## ✨ Features

| Feature | Description |
|---------|-------------|
| 🔄 **Saga Pattern** | Event-driven choreography with automatic compensation (rollback) |
| ⚡ **High Performance** | Go's concurrency model for low-latency operations |
| 🔐 **JWT Authentication** | Secure API endpoints with token-based auth |
| 🚦 **Rate Limiting** | Token bucket algorithm (100 req/min per IP) |
| 📊 **Prometheus Metrics** | Built-in `/metrics` endpoints for observability |
| 📨 **Event-Driven** | Kafka (Redpanda) based async communication |
| 💾 **ACID Transactions** | PostgreSQL with optimistic locking |

---

## 🚀 Quick Start

### Prerequisites

- Go 1.25
- Docker & Docker Compose
- gcloud CLI (for GCP deployment)
- Terraform (for IaC)
- Make (optional)

### Option 1: Run with Docker (Recommended)

```bash
# Clone the repository
git clone https://github.com/egesarisac/sagawallet.git
cd sagawallet

# Start everything (databases, Kafka, all 3 services)
docker compose -f docker-compose.full.yml up --build
```

**Services will be available at:**
- Wallet Service: http://localhost:8081
- Transaction Service: http://localhost:8083
- Notification Service: http://localhost:8084
- Kafka UI: http://localhost:8080
- Prometheus: http://localhost:9090

### Option 2: Run Locally (Development)

```bash
# Start infrastructure only
make docker-up

# Run migrations (Wallet and Transaction services)
make migrate-wallet
make migrate-txn

# Run services in separate terminals
cd services/wallet-service && go run cmd/main.go
cd services/transaction-service && go run cmd/main.go
cd services/notification-service && go run cmd/main.go
```

---

## 🔑 Authentication

All `/api/v1/*` endpoints require JWT authentication.

### Generate a Token

```bash
cd tools/tokengen && go run main.go
```

### Use the Token

```bash
curl -H "Authorization: Bearer <your-token>" \
  http://localhost:8081/api/v1/wallets/<wallet-id>
```

---

## 🌐 API Endpoints

### Wallet Service (`:8081`)

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| `GET` | `/health` | ❌ | Health check |
| `GET` | `/metrics` | ❌ | Prometheus metrics |
| `POST` | `/api/v1/wallets` | ✅ | Create wallet |
| `GET` | `/api/v1/wallets/:id` | ✅ | Get wallet details |
| `GET` | `/api/v1/wallets/:id/balance` | ✅ | Get balance |
| `POST` | `/api/v1/wallets/:id/credit` | ✅ | Add funds |
| `POST` | `/api/v1/wallets/:id/debit` | ✅ | Withdraw funds |
| `GET` | `/api/v1/wallets/:id/transactions` | ✅ | Transaction history |
| `PUT` | `/api/v1/wallets/:id/status` | ✅ | Update wallet status |

### Transaction Service (`:8083`)

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| `GET` | `/health` | ❌ | Health check |
| `GET` | `/metrics` | ❌ | Prometheus metrics |
| `POST` | `/api/v1/transfers` | ✅ | Create transfer (initiates saga) |
| `GET` | `/api/v1/transfers/:id` | ✅ | Get transfer status |

### Example: Create a Transfer

```bash
# 1. Get a JWT token
TOKEN=$(cd tools/tokengen && JWT_SECRET=dev-local-jwt-secret-change-me go run main.go | tail -1)

# 2. Create a transfer
curl -X POST http://localhost:8083/api/v1/transfers \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "sender_wallet_id": "aa505eba-d304-4e53-a8ea-c800441b84c9",
    "receiver_wallet_id": "06bee8e7-5985-47a8-ae74-45675c521ade",
    "amount": "50.00"
  }'

# Response: {"data":{"transfer_id":"...","status":"PENDING"}}

# 3. Check transfer status
curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:8083/api/v1/transfers/<transfer-id>

# Response: {"data":{"transfer_id":"...","status":"COMPLETED"}}
```

---

## 🏗 Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         API Gateway                             │
└─────────────────────────────────────────────────────────────────┘
                                │
        ┌───────────────────────┼───────────────────────┐
        ▼                       ▼                       ▼
┌───────────────┐     ┌─────────────────┐     ┌─────────────────┐
│    Wallet     │     │   Transaction   │     │  Notification   │
│  Service:8081 │◀───▶│  Service:8083   │────▶│  Service:8084   │
│  (Go + Gin)   │     │   (Go + Gin)    │     │     (Go)        │
└───────┬───────┘     └────────┬────────┘     └─────────────────┘
        │                      │
        ▼                      ▼
   [PostgreSQL            [PostgreSQL
    wallet-db:5434]      transaction-db:5433]
        │                      │
        └──────────┬───────────┘
                   ▼
           [Kafka/Redpanda:9092]
```

### Saga Choreography Flow

```mermaid
sequenceDiagram
    participant Client
    participant TX as Transaction Service
    participant WS as Wallet Service
    participant NS as Notification Service

    Client->>TX: POST /transfers
    TX->>TX: Create transfer (PENDING)
    TX-->>Kafka: transfer.created
    
    Kafka->>WS: transfer.created
    WS->>WS: Debit sender
    WS-->>Kafka: transfer.debit.success
    
    Kafka->>WS: transfer.debit.success
    WS->>WS: Credit receiver
    WS-->>Kafka: transfer.credit.success
    
    Kafka->>TX: transfer.credit.success
    TX->>TX: Update status (COMPLETED)
    TX-->>Kafka: transfer.completed
    
    Kafka->>NS: transfer.completed
    NS->>NS: Send notification
```

---

## 🐳 Docker Deployment

### Full Stack

```bash
# Start everything
docker compose -f docker-compose.full.yml up -d

# View logs
docker compose -f docker-compose.full.yml logs -f

# Stop everything
docker compose -f docker-compose.full.yml down
```

### Infrastructure Only

```bash
# Start databases and Kafka only
docker compose up -d

# Stop
docker compose down
```

---

## 📊 Monitoring

### OpenTelemetry Tracing

Services generate traces for cross-service calls (gRPC) and Kafka events. In production (GCP), these are exported to **Google Cloud Trace**. For local development, stdout or a local Jaeger instance can be used.

---

## 📁 Project Structure

```
sagawallet/
├── pkg/                    # Shared libraries
│   ├── config/             # Viper configuration (supports PORT/DATABASE_URL)
│   ├── errors/             # Standardized error codes
│   ├── kafka/              # Producer/consumer with DLQ worker
│   ├── logger/             # Structured JSON logging
│   ├── middleware/         # JWT, Rate Limit, Metrics, Tracing
│   └── models/             # Kafka event schemas
├── deployments/            # Infrastructure as Code
│   └── terraform-gcp/      # GCP (Cloud Run, VPC, Cloud SQL)
├── services/
│   ├── wallet-service/     # Wallet management
│   ├── transaction-service/ # Transfer orchestration
│   └── notification-service/ # Notifications (Kafka consumer)
├── tools/
│   └── tokengen/           # JWT token generator
├── docker-compose.yml       # Infrastructure only
├── docker-compose.full.yml  # Full stack
└── Makefile
```

---

## 🔧 Make Commands

| Command | Description |
|---------|-------------|
| `make docker-up` | Start infrastructure (Postgres, Kafka) |
| `make docker-down` | Stop all containers |
| `make run-wallet` | Run Wallet Service |
| `make run-transaction` | Run Transaction Service |
| `make run-notification` | Run Notification Service |
| `make migrate-wallet` | Run wallet service migrations |
| `make migrate-txn` | Run transaction service migrations |
| `make test` | Run all unit tests |
| `make test-integration` | Run full saga flow integration tests |

---

## 📨 Kafka Topics

| Topic | Publisher | Purpose |
|-------|-----------|---------|
| `transfer.created` | Transaction | Saga start |
| `transfer.debit.success` | Wallet | Debit completed |
| `transfer.debit.failed` | Wallet | Debit failed |
| `transfer.credit.success` | Wallet | Credit completed |
| `transfer.credit.failed` | Wallet | Credit failed → triggers refund |
| `transfer.refund.success` | Wallet | Refund completed |
| `transfer.completed` | Transaction | Saga success |
| `transfer.failed` | Transaction | Saga failed |
| `dlq` | All | Dead letter queue |

---

## 🛠 Tech Stack

| Component | Technology |
|-----------|------------|
| Language | Go 1.25 |
| Web Framework | Gin |
| Database | PostgreSQL + sqlc |
| Message Broker | Kafka (Redpanda) |
| Auth | JWT (golang-jwt/jwt) |
| Config | Viper |
| Logging | Zerolog |
| Observability | Prometheus + OpenTelemetry |
| Cloud | GCP (Cloud Run) |

---

## 📄 License

MIT License - see [LICENSE](LICENSE) for details.

---

<div align="center">

**Built with ❤️ using Go**

</div>
