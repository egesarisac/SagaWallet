#!/bin/sh
set -e

if [ "${SKIP_MIGRATIONS}" != "true" ]; then
	echo "Running database migrations..."
	./migrate up
else
	echo "Skipping database migrations (SKIP_MIGRATIONS=true)"
fi

echo "Starting transaction service..."
exec ./transaction-service
