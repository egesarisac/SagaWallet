-- Drop indexes first
DROP INDEX IF EXISTS idx_wallet_transactions_wallet_id;
DROP INDEX IF EXISTS idx_wallet_transactions_reference_id;
DROP INDEX IF EXISTS idx_wallet_transactions_created_at;
DROP INDEX IF EXISTS idx_wallet_transactions_type;

-- Drop table
DROP TABLE IF EXISTS wallet_transactions;
