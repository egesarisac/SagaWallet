// Package repository provides database access layer for transaction service.
package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	apperrors "github.com/egesarisac/SagaWallet/pkg/errors"
	db "github.com/egesarisac/SagaWallet/services/transaction-service/db/generated"
)

// TransferRepository handles transfer database operations.
type TransferRepository struct {
	pool    *pgxpool.Pool
	queries *db.Queries
}

// NewTransferRepository creates a new transfer repository.
func NewTransferRepository(pool *pgxpool.Pool) *TransferRepository {
	return &TransferRepository{
		pool:    pool,
		queries: db.New(pool),
	}
}

// Helper functions for type conversion
func uuidToPgtype(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

func nullableUUID(id uuid.UUID) pgtype.UUID {
	if id == uuid.Nil {
		return pgtype.UUID{}
	}
	return uuidToPgtype(id)
}

func stringToNumeric(s string) pgtype.Numeric {
	var n pgtype.Numeric
	_ = n.Scan(s)
	return n
}

func pgtypeToUUID(p pgtype.UUID) uuid.UUID {
	if !p.Valid {
		return uuid.Nil
	}
	return p.Bytes
}

// CreateTransfer creates a new transfer record.
func (r *TransferRepository) CreateTransfer(
	ctx context.Context,
	senderWalletID, receiverWalletID uuid.UUID,
	amount, currency string,
	idempotencyKey uuid.UUID,
) (*db.Transfer, error) {
	transfer, err := r.queries.CreateTransfer(ctx, db.CreateTransferParams{
		SenderWalletID:   uuidToPgtype(senderWalletID),
		ReceiverWalletID: uuidToPgtype(receiverWalletID),
		Amount:           stringToNumeric(amount),
		Currency:         currency,
		IdempotencyKey:   nullableUUID(idempotencyKey),
	})
	if err != nil {
		return nil, apperrors.Wrap(apperrors.CodeDatabaseError, "failed to create transfer", err)
	}
	return &transfer, nil
}

// OutboxEvent is a claimed event that must be published to Kafka.
type OutboxEvent struct {
	ID         uuid.UUID
	Topic      string
	MessageKey string
	Payload    []byte
	Attempts   int
}

// PendingOutboxEvent is written atomically with a transfer state transition.
type PendingOutboxEvent struct {
	ID         uuid.UUID
	Topic      string
	MessageKey string
	Payload    []byte
}

// TransitionOutcome describes how an incoming saga event affected state.
type TransitionOutcome string

const (
	TransitionApplied   TransitionOutcome = "applied"
	TransitionDuplicate TransitionOutcome = "duplicate"
	TransitionIgnored   TransitionOutcome = "ignored"
	TransitionDeferred  TransitionOutcome = "deferred"
)

// CreateTransferWithOutbox atomically records a transfer, audit record, and start event.
func (r *TransferRepository) CreateTransferWithOutbox(
	ctx context.Context,
	transferID uuid.UUID,
	senderWalletID, receiverWalletID uuid.UUID,
	amount, currency string,
	idempotencyKey uuid.UUID,
	eventID uuid.UUID,
	topic string,
	eventPayload interface{},
) (*db.Transfer, bool, error) {
	payload, err := json.Marshal(eventPayload)
	if err != nil {
		return nil, false, apperrors.Wrap(apperrors.CodeInternalError, "failed to marshal outbox event", err)
	}

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, false, apperrors.Wrap(apperrors.CodeDatabaseError, "failed to begin transfer transaction", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const transferColumns = `id, sender_wallet_id, receiver_wallet_id, amount, currency, status, failure_reason, idempotency_key, created_at, updated_at`
	row := tx.QueryRow(ctx, `
		INSERT INTO transfers (id, sender_wallet_id, receiver_wallet_id, amount, currency, status, idempotency_key)
		VALUES ($1, $2, $3, $4, $5, 'PENDING', $6)
		RETURNING `+transferColumns,
		uuidToPgtype(transferID), uuidToPgtype(senderWalletID), uuidToPgtype(receiverWalletID), stringToNumeric(amount), currency, nullableUUID(idempotencyKey),
	)
	transfer, err := scanTransfer(row)
	if err != nil {
		var pgErr *pgconn.PgError
		if idempotencyKey != uuid.Nil && errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "transfers_idempotency_key_key" {
			if rollbackErr := tx.Rollback(ctx); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
				return nil, false, apperrors.Wrap(apperrors.CodeDatabaseError, "failed to roll back duplicate transfer", rollbackErr)
			}
			existing, getErr := r.GetTransferByIdempotencyKey(ctx, idempotencyKey)
			if getErr != nil {
				return nil, false, getErr
			}
			if existing == nil {
				return nil, false, apperrors.Wrap(apperrors.CodeDatabaseError, "duplicate transfer disappeared", err)
			}
			return existing, false, nil
		}
		return nil, false, apperrors.Wrap(apperrors.CodeDatabaseError, "failed to create transfer", err)
	}

	if _, err := tx.Exec(ctx, `
			INSERT INTO saga_events (transfer_id, event_type, payload)
			VALUES ($1, 'TRANSFER_CREATED', $2)`, transfer.ID, payload); err != nil {
		return nil, false, apperrors.Wrap(apperrors.CodeDatabaseError, "failed to create transfer audit event", err)
	}

	if _, err := tx.Exec(ctx, `
			INSERT INTO outbox_events (id, aggregate_id, topic, message_key, payload)
			VALUES ($1, $2, $3, $4, $5)`, uuidToPgtype(eventID), transfer.ID, topic, GetTransferID(transfer).String(), payload); err != nil {
		return nil, false, apperrors.Wrap(apperrors.CodeDatabaseError, "failed to create outbox event", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, false, apperrors.Wrap(apperrors.CodeDatabaseError, "failed to commit transfer transaction", err)
	}
	return transfer, true, nil
}

type transferRow interface {
	Scan(dest ...any) error
}

func scanTransfer(row transferRow) (*db.Transfer, error) {
	var transfer db.Transfer
	err := row.Scan(
		&transfer.ID,
		&transfer.SenderWalletID,
		&transfer.ReceiverWalletID,
		&transfer.Amount,
		&transfer.Currency,
		&transfer.Status,
		&transfer.FailureReason,
		&transfer.IdempotencyKey,
		&transfer.CreatedAt,
		&transfer.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &transfer, nil
}

// ClaimOutbox leases a batch so multiple publishers can work safely.
func (r *TransferRepository) ClaimOutbox(ctx context.Context, batchSize, lockSeconds int, workerID string) ([]OutboxEvent, error) {
	rows, err := r.pool.Query(ctx, `
		WITH candidates AS (
			SELECT id
			FROM outbox_events
			WHERE published_at IS NULL
			  AND next_attempt_at <= NOW()
			  AND (locked_until IS NULL OR locked_until < NOW())
			ORDER BY created_at
			FOR UPDATE SKIP LOCKED
			LIMIT $1
		)
		UPDATE outbox_events AS outbox
			SET locked_until = NOW() + ($2 * INTERVAL '1 second'), locked_by = $3, attempts = outbox.attempts + 1
			FROM candidates
			WHERE outbox.id = candidates.id
			RETURNING outbox.id, outbox.topic, outbox.message_key, outbox.payload, outbox.attempts`, batchSize, lockSeconds, workerID)
	if err != nil {
		return nil, apperrors.Wrap(apperrors.CodeDatabaseError, "failed to claim outbox events", err)
	}
	defer rows.Close()

	events := make([]OutboxEvent, 0, batchSize)
	for rows.Next() {
		var event OutboxEvent
		var id pgtype.UUID
		if err := rows.Scan(&id, &event.Topic, &event.MessageKey, &event.Payload, &event.Attempts); err != nil {
			return nil, apperrors.Wrap(apperrors.CodeDatabaseError, "failed to scan outbox event", err)
		}
		event.ID = pgtypeToUUID(id)
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, apperrors.Wrap(apperrors.CodeDatabaseError, "failed to read claimed outbox events", err)
	}
	return events, nil
}

// MarkOutboxPublished marks a successfully delivered event immutable.
func (r *TransferRepository) MarkOutboxPublished(ctx context.Context, eventID uuid.UUID, workerID string) error {
	command, err := r.pool.Exec(ctx, `
			UPDATE outbox_events
			SET published_at = NOW(), locked_until = NULL, locked_by = NULL, last_error = NULL
			WHERE id = $1 AND locked_by = $2 AND published_at IS NULL`, uuidToPgtype(eventID), workerID)
	if err != nil {
		return apperrors.Wrap(apperrors.CodeDatabaseError, "failed to mark outbox event published", err)
	}
	if command.RowsAffected() != 1 {
		return apperrors.New(apperrors.CodeDatabaseError, "outbox lease was lost before publish acknowledgement")
	}
	return nil
}

// ReleaseOutbox schedules a failed event for a later retry.
func (r *TransferRepository) ReleaseOutbox(ctx context.Context, eventID uuid.UUID, attempts int, workerID string, cause error) error {
	delay := time.Duration(1<<min(attempts, 6)) * time.Second
	command, err := r.pool.Exec(ctx, `
			UPDATE outbox_events
			SET locked_until = NULL, locked_by = NULL, next_attempt_at = NOW() + ($2 * INTERVAL '1 second'), last_error = $3
			WHERE id = $1 AND locked_by = $4 AND published_at IS NULL`, uuidToPgtype(eventID), int(delay.Seconds()), cause.Error(), workerID)
	if err != nil {
		return apperrors.Wrap(apperrors.CodeDatabaseError, "failed to release outbox event", err)
	}
	if command.RowsAffected() != 1 {
		return apperrors.New(apperrors.CodeDatabaseError, "outbox lease was lost before retry scheduling")
	}
	return nil
}

// RetryOutbox makes one unpublished outbox event immediately claimable and records the operator action.
func (r *TransferRepository) RetryOutbox(ctx context.Context, eventID uuid.UUID, actor, reason string) (bool, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return false, apperrors.Wrap(apperrors.CodeDatabaseError, "failed to begin outbox retry transaction", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	command, err := tx.Exec(ctx, `
			UPDATE outbox_events
			SET next_attempt_at = NOW(), locked_until = NULL, locked_by = NULL, last_error = NULL
			WHERE id = $1 AND published_at IS NULL`, uuidToPgtype(eventID))
	if err != nil {
		return false, apperrors.Wrap(apperrors.CodeDatabaseError, "failed to schedule outbox retry", err)
	}
	if command.RowsAffected() != 1 {
		return false, nil
	}
	if _, err := tx.Exec(ctx, `
			INSERT INTO outbox_retry_audit (outbox_event_id, actor, reason)
			VALUES ($1, $2, $3)`, uuidToPgtype(eventID), actor, reason); err != nil {
		return false, apperrors.Wrap(apperrors.CodeDatabaseError, "failed to record outbox retry audit", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, apperrors.Wrap(apperrors.CodeDatabaseError, "failed to commit outbox retry", err)
	}
	return true, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// GetTransferByID retrieves a transfer by its ID.
func (r *TransferRepository) GetTransferByID(ctx context.Context, transferID uuid.UUID) (*db.Transfer, error) {
	transfer, err := r.queries.GetTransferByID(ctx, uuidToPgtype(transferID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperrors.TransferNotFound(transferID.String())
		}
		return nil, apperrors.Wrap(apperrors.CodeDatabaseError, "failed to get transfer", err)
	}
	return &transfer, nil
}

// GetTransferByIdempotencyKey retrieves a transfer by idempotency key.
func (r *TransferRepository) GetTransferByIdempotencyKey(ctx context.Context, key uuid.UUID) (*db.Transfer, error) {
	transfer, err := r.queries.GetTransferByIdempotencyKey(ctx, uuidToPgtype(key))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil // Not found is not an error for idempotency check
		}
		return nil, apperrors.Wrap(apperrors.CodeDatabaseError, "failed to get transfer by idempotency key", err)
	}
	return &transfer, nil
}

// UpdateTransferStatus updates the status of a transfer.
func (r *TransferRepository) UpdateTransferStatus(ctx context.Context, transferID uuid.UUID, status string, failureReason string) (*db.Transfer, error) {
	var reason pgtype.Text
	if failureReason != "" {
		reason = pgtype.Text{String: failureReason, Valid: true}
	}

	transfer, err := r.queries.UpdateTransferStatus(ctx, db.UpdateTransferStatusParams{
		ID:            uuidToPgtype(transferID),
		Status:        status,
		FailureReason: reason,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperrors.TransferNotFound(transferID.String())
		}
		return nil, apperrors.Wrap(apperrors.CodeDatabaseError, "failed to update transfer status", err)
	}
	return &transfer, nil
}

// TransitionStatus updates a transfer only from its expected current state.
// A false result means a duplicate, stale, or out-of-order event was ignored.
func (r *TransferRepository) TransitionStatus(
	ctx context.Context,
	transferID uuid.UUID,
	expectedStatus, status, failureReason string,
	eventID uuid.UUID,
	sourceTopic string,
	outbox *PendingOutboxEvent,
) (TransitionOutcome, error) {
	if !IsAllowedTransition(expectedStatus, status) {
		return TransitionIgnored, apperrors.New(apperrors.CodeValidationFailed, fmt.Sprintf("invalid transfer transition %s -> %s", expectedStatus, status))
	}

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return TransitionIgnored, apperrors.Wrap(apperrors.CodeDatabaseError, "failed to begin transition transaction", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var recorded pgtype.UUID
	err = tx.QueryRow(ctx, `
			INSERT INTO processed_events (event_id, topic)
			VALUES ($1, $2)
			ON CONFLICT (event_id) DO NOTHING
			RETURNING event_id`, uuidToPgtype(eventID), sourceTopic).Scan(&recorded)
	if errors.Is(err, pgx.ErrNoRows) {
		if err := tx.Commit(ctx); err != nil {
			return TransitionIgnored, apperrors.Wrap(apperrors.CodeDatabaseError, "failed to commit duplicate event", err)
		}
		return TransitionDuplicate, nil
	}
	if err != nil {
		return TransitionIgnored, apperrors.Wrap(apperrors.CodeDatabaseError, "failed to record processed event", err)
	}

	command, err := tx.Exec(ctx, `
		UPDATE transfers
		SET status = $3, failure_reason = NULLIF($4, ''), last_event_id = $5, updated_at = NOW()
			WHERE id = $1 AND status = $2`, uuidToPgtype(transferID), expectedStatus, status, failureReason, uuidToPgtype(eventID))
	if err != nil {
		return TransitionIgnored, apperrors.Wrap(apperrors.CodeDatabaseError, "failed to transition transfer", err)
	}
	if command.RowsAffected() == 1 {
		auditPayload, err := json.Marshal(map[string]interface{}{
			"event_id":        eventID.String(),
			"source_topic":    sourceTopic,
			"previous_status": expectedStatus,
			"new_status":      status,
			"failure_reason":  failureReason,
		})
		if err != nil {
			return TransitionIgnored, apperrors.Wrap(apperrors.CodeInternalError, "failed to marshal transition audit", err)
		}
		if _, err := tx.Exec(ctx, `
				INSERT INTO saga_events (transfer_id, event_type, payload)
				VALUES ($1, 'STATUS_TRANSITION', $2)`, uuidToPgtype(transferID), auditPayload); err != nil {
			return TransitionIgnored, apperrors.Wrap(apperrors.CodeDatabaseError, "failed to record transition audit", err)
		}
		if outbox != nil {
			if _, err := tx.Exec(ctx, `
					INSERT INTO outbox_events (id, aggregate_id, topic, message_key, payload)
					VALUES ($1, $2, $3, $4, $5)`, uuidToPgtype(outbox.ID), uuidToPgtype(transferID), outbox.Topic, outbox.MessageKey, outbox.Payload); err != nil {
				return TransitionIgnored, apperrors.Wrap(apperrors.CodeDatabaseError, "failed to create transition outbox event", err)
			}
		}
		if err := tx.Commit(ctx); err != nil {
			return TransitionIgnored, apperrors.Wrap(apperrors.CodeDatabaseError, "failed to commit transfer transition", err)
		}
		return TransitionApplied, nil
	}

	var currentStatus string
	if err := tx.QueryRow(ctx, `SELECT status FROM transfers WHERE id = $1`, uuidToPgtype(transferID)).Scan(&currentStatus); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return TransitionIgnored, apperrors.TransferNotFound(transferID.String())
		}
		return TransitionIgnored, apperrors.Wrap(apperrors.CodeDatabaseError, "failed to read transfer state", err)
	}
	outcome := classifyTransition(currentStatus, expectedStatus, status)
	if outcome == TransitionDeferred {
		return outcome, nil
	}
	if err := tx.Commit(ctx); err != nil {
		return TransitionIgnored, apperrors.Wrap(apperrors.CodeDatabaseError, "failed to commit ignored transition", err)
	}
	return outcome, nil
}

func classifyTransition(currentStatus, expectedStatus, targetStatus string) TransitionOutcome {
	if currentStatus == targetStatus || isTerminalStatus(currentStatus) {
		return TransitionIgnored
	}
	if canReachStatus(currentStatus, expectedStatus) {
		return TransitionDeferred
	}
	return TransitionIgnored
}

func canReachStatus(from, target string) bool {
	if from == target {
		return true
	}
	statuses := []string{"PENDING", "DEBITED", "COMPLETED", "REFUNDING", "FAILED", "MANUAL_REVIEW"}
	visited := map[string]bool{from: true}
	queue := []string{from}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, next := range statuses {
			if visited[next] || !IsAllowedTransition(current, next) {
				continue
			}
			if next == target {
				return true
			}
			visited[next] = true
			queue = append(queue, next)
		}
	}
	return false
}

func isTerminalStatus(status string) bool {
	return status == "COMPLETED" || status == "FAILED" || status == "MANUAL_REVIEW"
}

// IsAllowedTransition centralizes the saga's legal state machine edges.
func IsAllowedTransition(from, to string) bool {
	switch from {
	case "PENDING":
		return to == "DEBITED" || to == "FAILED"
	case "DEBITED":
		return to == "COMPLETED" || to == "REFUNDING"
	case "REFUNDING":
		return to == "FAILED" || to == "MANUAL_REVIEW"
	default:
		return false
	}
}

// ListTransfersBySender lists transfers by sender wallet ID.
func (r *TransferRepository) ListTransfersBySender(ctx context.Context, senderWalletID uuid.UUID, limit, offset int32) ([]db.Transfer, error) {
	transfers, err := r.queries.ListTransfersBySender(ctx, db.ListTransfersBySenderParams{
		SenderWalletID: uuidToPgtype(senderWalletID),
		Limit:          limit,
		Offset:         offset,
	})
	if err != nil {
		return nil, apperrors.Wrap(apperrors.CodeDatabaseError, "failed to list transfers by sender", err)
	}
	return transfers, nil
}

// ListTransfersByReceiver lists transfers by receiver wallet ID.
func (r *TransferRepository) ListTransfersByReceiver(ctx context.Context, receiverWalletID uuid.UUID, limit, offset int32) ([]db.Transfer, error) {
	transfers, err := r.queries.ListTransfersByReceiver(ctx, db.ListTransfersByReceiverParams{
		ReceiverWalletID: uuidToPgtype(receiverWalletID),
		Limit:            limit,
		Offset:           offset,
	})
	if err != nil {
		return nil, apperrors.Wrap(apperrors.CodeDatabaseError, "failed to list transfers by receiver", err)
	}
	return transfers, nil
}

// ListStuckTransfers lists transfers stuck in intermediate saga states.
func (r *TransferRepository) ListStuckTransfers(ctx context.Context, pendingThreshold, debitedThreshold, refundingThreshold pgtype.Timestamptz, limit int32) ([]db.Transfer, error) {
	transfers, err := r.queries.ListStuckTransfers(ctx, db.ListStuckTransfersParams{
		UpdatedAt:   pendingThreshold,
		UpdatedAt_2: debitedThreshold,
		UpdatedAt_3: refundingThreshold,
		Limit:       limit,
	})
	if err != nil {
		return nil, apperrors.Wrap(apperrors.CodeDatabaseError, "failed to list stuck transfers", err)
	}
	return transfers, nil
}

// CreateSagaEvent logs a saga event.
func (r *TransferRepository) CreateSagaEvent(ctx context.Context, transferID uuid.UUID, eventType string, payload interface{}) (*db.SagaEvent, error) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, apperrors.Wrap(apperrors.CodeInternalError, "failed to marshal payload", err)
	}

	event, err := r.queries.CreateSagaEvent(ctx, db.CreateSagaEventParams{
		TransferID: uuidToPgtype(transferID),
		EventType:  eventType,
		Payload:    payloadBytes,
	})
	if err != nil {
		return nil, apperrors.Wrap(apperrors.CodeDatabaseError, "failed to create saga event", err)
	}
	return &event, nil
}

// ListSagaEventsByTransfer returns all saga events for a transfer.
func (r *TransferRepository) ListSagaEventsByTransfer(ctx context.Context, transferID uuid.UUID) ([]db.SagaEvent, error) {
	events, err := r.queries.ListSagaEventsByTransfer(ctx, uuidToPgtype(transferID))
	if err != nil {
		return nil, apperrors.Wrap(apperrors.CodeDatabaseError, "failed to list saga events", err)
	}
	return events, nil
}

// GetTransferID extracts UUID from pgtype.UUID
func GetTransferID(transfer *db.Transfer) uuid.UUID {
	return pgtypeToUUID(transfer.ID)
}
