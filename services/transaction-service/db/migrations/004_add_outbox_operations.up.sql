ALTER TABLE outbox_events
    ADD COLUMN locked_by TEXT;

CREATE TABLE outbox_retry_audit (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    outbox_event_id UUID NOT NULL REFERENCES outbox_events(id),
    actor TEXT NOT NULL,
    reason TEXT NOT NULL,
    retried_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_outbox_retry_audit_event
    ON outbox_retry_audit (outbox_event_id, retried_at DESC);
