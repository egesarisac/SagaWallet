# 1. Use Saga Choreography Pattern

Date: 2026-03-25

## Status

Accepted

## Context

The Go Fintech application needs to process money transfers between wallets. Since wallets and transactions are managed by distinct continuous microservices (`wallet-service` and `transaction-service`) backed by different databases, a distributed transaction mechanism is required. Conventional 2PC (Two-Phase Commit) is known to scale poorly and reduce availability in distributed systems.

## Decision

We have decided to implement the **Saga Choreography Pattern** for distributed transactions, specifically for the transfer flow:
1. `transaction-service` creates a `PENDING` transfer and publishes an event.
2. `wallet-service` consumes the event, attempts to debit the sender's wallet, and publishes a success or failure event.
3. If the debit succeeds, `wallet-service` triggers a credit to the receiver.
4. If the credit fails, a rollback mechanism is triggered to refund the sender.
5. All services listen to the relevant events to update their own state without a central orchestrator.

## Consequences

**Positive:**
- Decentralized logic ensures single-responsibility principle: each service reacts to events it cares about.
- Improved fault tolerance; if one service fails, messages remain in the queue until the service recovers.
- High scalability.

**Negative:**
- Event flow can be complicated to debug. Tracking a complete transfer workflow requires tracing spanning over multiple asynchronous operations.
- Requires building mechanisms for retries, timeouts, and Dead Letter Queues (DLQ).
- Requires comprehensive distributed tracing (OpenTelemetry).
