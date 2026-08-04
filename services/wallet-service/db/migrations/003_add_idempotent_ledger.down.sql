ALTER TABLE wallet_transactions
    DROP CONSTRAINT IF EXISTS wallet_transactions_reference_operation_unique;
DROP TABLE IF EXISTS processed_events;
