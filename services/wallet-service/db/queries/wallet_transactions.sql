-- name: CreateWalletTransaction :one
INSERT INTO wallet_transactions (
    wallet_id, amount, type, reference_id, description, balance_after
)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetWalletTransactionByID :one
SELECT * FROM wallet_transactions
WHERE id = $1;

-- name: GetWalletTransactionByReferenceID :one
SELECT * FROM wallet_transactions
WHERE reference_id = $1 AND wallet_id = $2;

-- name: ListWalletTransactions :many
SELECT * FROM wallet_transactions
WHERE wallet_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: ListWalletTransactionsByType :many
SELECT * FROM wallet_transactions
WHERE wallet_id = $1 AND type = $2
ORDER BY created_at DESC
LIMIT $3 OFFSET $4;

-- name: CountWalletTransactions :one
SELECT COUNT(*) FROM wallet_transactions
WHERE wallet_id = $1;
