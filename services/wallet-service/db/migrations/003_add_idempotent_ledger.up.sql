CREATE TABLE processed_events (
    event_id UUID PRIMARY KEY,
    topic TEXT NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE wallet_transactions
    ADD CONSTRAINT wallet_transactions_reference_operation_unique
    UNIQUE (wallet_id, reference_id, type);
