// Package repository provides database access layer for transaction service.
package repository

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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
		IdempotencyKey:   uuidToPgtype(idempotencyKey),
	})
	if err != nil {
		return nil, apperrors.Wrap(apperrors.CodeDatabaseError, "failed to create transfer", err)
	}
	return &transfer, nil
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
