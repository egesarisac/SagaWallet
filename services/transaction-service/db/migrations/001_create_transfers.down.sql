-- Rollback Transaction Service Schema
DROP INDEX IF EXISTS idx_saga_events_transfer_id;
DROP INDEX IF EXISTS idx_transfers_created_at;
DROP INDEX IF EXISTS idx_transfers_status;
DROP INDEX IF EXISTS idx_transfers_receiver;
DROP INDEX IF EXISTS idx_transfers_sender;
DROP TABLE IF EXISTS saga_events;
DROP TABLE IF EXISTS transfers;
