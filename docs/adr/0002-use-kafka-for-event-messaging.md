# 2. Use Kafka for Event Messaging

Date: 2026-03-25

## Status

Accepted

## Context

Because we selected the Saga Choreography pattern for orchestrating complex business processes (like transfers), we need a resilient, ordered, and scalable messaging system. The broker must safely transport events between `transaction-service`, `wallet-service`, and `notification-service`.

## Decision

We will use **Apache Kafka** (or Redpanda as a drop-in capable alternative) as our primary message broker.

## Consequences

**Positive:**
- Persistent and replayable message logs, which is highly beneficial for auditing and error recovery.
- Partitioning provides scalability and strict ordering when keyed correctly (e.g., using `wallet_id`).
- Extensive Go ecosystem support (e.g., confluent-kafka-go, segmentio/kafka-go).

**Negative:**
- Operational complexity is higher compared to simpler queues like RabbitMQ or Redis Pub/Sub.
- We must manually model event schemas, dead letter queues (DLQs), and handle idempotency locally.
