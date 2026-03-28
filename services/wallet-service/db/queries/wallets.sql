-- name: CreateWallet :one
INSERT INTO wallets (user_id, balance, currency, status)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetWalletByID :one
SELECT * FROM wallets
WHERE id = $1;

-- name: GetWalletByUserID :one
SELECT * FROM wallets
WHERE user_id = $1
LIMIT 1;

-- name: GetWalletByUserIDAndCurrency :one
SELECT * FROM wallets
WHERE user_id = $1 AND currency = $2;

-- name: ListWalletsByUserID :many
SELECT * FROM wallets
WHERE user_id = $1
ORDER BY currency;

-- name: GetWalletForUpdate :one
-- Lock the row for update (pessimistic lock when needed)
SELECT * FROM wallets
WHERE id = $1
FOR UPDATE;

-- name: UpdateWalletBalance :one
-- Optimistic locking: only update if version matches
UPDATE wallets
SET 
    balance = $2,
    version = version + 1,
    updated_at = NOW()
WHERE id = $1 AND version = $3
RETURNING *;

-- name: CreditWallet :one
-- Add funds to wallet with optimistic locking
UPDATE wallets
SET 
    balance = balance + $2,
    version = version + 1,
    updated_at = NOW()
WHERE id = $1 AND version = $3 AND status = 'ACTIVE'
RETURNING *;

-- name: DebitWallet :one
-- Subtract funds from wallet with balance check and optimistic locking
UPDATE wallets
SET 
    balance = balance - $2,
    version = version + 1,
    updated_at = NOW()
WHERE id = $1 
    AND version = $3 
    AND status = 'ACTIVE'
    AND balance >= $2
RETURNING *;

-- name: UpdateWalletStatus :one
UPDATE wallets
SET 
    status = $2,
    version = version + 1,
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: ListWallets :many
SELECT * FROM wallets
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: CountWallets :one
SELECT COUNT(*) FROM wallets;

-- name: DeleteWallet :exec
DELETE FROM wallets WHERE id = $1;

-- name: DeleteWalletTransactions :exec
DELETE FROM wallet_transactions WHERE wallet_id = $1;
