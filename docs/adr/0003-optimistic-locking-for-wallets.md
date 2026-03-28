# 3. Optimistic Locking for Wallet Operations

Date: 2026-03-25

## Status

Accepted

## Context

Financial transactions demand ACID guarantees. When thousands of rapid requests attempt to modify a wallet's balance concurrently (e.g., in a high-traffic e-commerce scenario), standard transactional updates might suffer from lost updates if isolation levels are inadequately configured. While pessimistic locking (`SELECT FOR UPDATE`) prevents concurrent modifications entirely, it can cause severe lock contention and affect throughput negatively.

## Decision

We will use **Optimistic Locking** to handle concurrent updates to a wallet's balance. A `version` column is added to the `wallets` table.
When an update is executed:
```sql
UPDATE wallets 
SET balance = balance + :amount, version = version + 1
WHERE id = :id AND version = :version;
```
If the number of affected rows is 0, a concurrent modification exception is raised, and the application orchestrates a retry.

## Consequences

**Positive:**
- Better performance under moderate to low concurrency because no database row locks are held over multiple transactions.
- Zero risk of database deadlocks caused by row locks.
- Simplifies scaling database nodes since reads aren't blocked.

**Negative:**
- Contention under heavy load on the same row can lead to excessive retries overhead on the application side.
- Go code needs reliable retry logic to handle the `0 rows affected` scenario securely.
