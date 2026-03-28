#!/bin/bash
# Run database migrations

set -e

SERVICE=$1
DIRECTION=${2:-up}

if [ -z "$SERVICE" ]; then
  echo "Usage: ./migrate.sh <service> [up|down]"
  echo "  service: wallet or transaction"
  exit 1
fi

case $SERVICE in
  wallet)
    DB_URL="postgres://wallet_user:wallet_secret@localhost:5432/wallet_db?sslmode=disable"
    MIGRATIONS_DIR="services/wallet-service/db/migrations"
    ;;
  transaction)
    DB_URL="postgres://transaction_user:transaction_secret@localhost:5433/transaction_db?sslmode=disable"
    MIGRATIONS_DIR="services/transaction-service/db/migrations"
    ;;
  *)
    echo "Unknown service: $SERVICE"
    exit 1
    ;;
esac

echo "Running migrations for $SERVICE service ($DIRECTION)..."

# Using golang-migrate
migrate -path $MIGRATIONS_DIR -database "$DB_URL" $DIRECTION

echo "Migration complete!"
