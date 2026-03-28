#!/bin/sh
set -e

if [ "${SKIP_MIGRATIONS}" != "true" ]; then
	echo "Running database migrations..."
	./migrate up
else
	echo "Skipping database migrations (SKIP_MIGRATIONS=true)"
fi

echo "Starting wallet service..."
exec ./wallet-service
