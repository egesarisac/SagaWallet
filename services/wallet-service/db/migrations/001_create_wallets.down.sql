-- Drop indexes first
DROP INDEX IF EXISTS idx_wallets_user_id;
DROP INDEX IF EXISTS idx_wallets_status;

-- Drop table
DROP TABLE IF EXISTS wallets;
