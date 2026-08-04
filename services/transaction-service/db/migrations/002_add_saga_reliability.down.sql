DROP TABLE IF EXISTS outbox_events;
ALTER TABLE transfers DROP COLUMN IF EXISTS last_event_id;
