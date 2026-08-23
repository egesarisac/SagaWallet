DROP TABLE IF EXISTS outbox_retry_audit;
ALTER TABLE outbox_events DROP COLUMN IF EXISTS locked_by;
