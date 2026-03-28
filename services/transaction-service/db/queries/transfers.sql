-- name: CreateTransfer :one
INSERT INTO transfers (
    sender_wallet_id,
    receiver_wallet_id,
    amount,
    currency,
    status,
    idempotency_key
) VALUES (
    $1, $2, $3, $4, 'PENDING', $5
)
RETURNING *;

-- name: GetTransferByID :one
SELECT * FROM transfers WHERE id = $1;

-- name: GetTransferByIdempotencyKey :one
SELECT * FROM transfers WHERE idempotency_key = $1;

-- name: UpdateTransferStatus :one
UPDATE transfers
SET status = $2,
    failure_reason = $3,
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: ListTransfersBySender :many
SELECT * FROM transfers
WHERE sender_wallet_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: ListTransfersByReceiver :many
SELECT * FROM transfers
WHERE receiver_wallet_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: CreateSagaEvent :one
INSERT INTO saga_events (
    transfer_id,
    event_type,
    payload
) VALUES (
    $1, $2, $3
)
RETURNING *;

-- name: ListSagaEventsByTransfer :many
SELECT * FROM saga_events
WHERE transfer_id = $1
ORDER BY created_at ASC;

-- name: ListStuckTransfers :many
SELECT * FROM transfers
WHERE (status = 'PENDING' AND updated_at < $1)
   OR (status = 'DEBITED' AND updated_at < $2)
   OR (status = 'REFUNDING' AND updated_at < $3)
ORDER BY updated_at ASC
LIMIT $4;
