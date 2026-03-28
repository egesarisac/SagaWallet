// Package service provides business logic for transaction service.
package service

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"

	apperrors "github.com/egesarisac/SagaWallet/pkg/errors"
	"github.com/egesarisac/SagaWallet/pkg/kafka"
	"github.com/egesarisac/SagaWallet/pkg/logger"
	"github.com/egesarisac/SagaWallet/pkg/models"
	"github.com/egesarisac/SagaWallet/services/transaction-service/internal/repository"
)

// TransferService handles transfer business logic.
type TransferService struct {
	repo     *repository.TransferRepository
	producer *kafka.Producer
	log      *logger.Logger
}

// NewTransferService creates a new transfer service.
func NewTransferService(repo *repository.TransferRepository, producer *kafka.Producer, log *logger.Logger) *TransferService {
	return &TransferService{
		repo:     repo,
		producer: producer,
		log:      log,
	}
}

// CreateTransferInput represents input for creating a transfer.
type CreateTransferInput struct {
	SenderWalletID   uuid.UUID
	ReceiverWalletID uuid.UUID
	Amount           string
	Currency         string
	IdempotencyKey   uuid.UUID
}

// TransferResult represents the result of a transfer operation.
type TransferResult struct {
	TransferID string `json:"transfer_id"`
	Status     string `json:"status"`
}

// CreateTransfer initiates a new transfer (starts the saga).
func (s *TransferService) CreateTransfer(ctx context.Context, input CreateTransferInput) (*TransferResult, error) {
	// Validate input
	if input.SenderWalletID == uuid.Nil || input.ReceiverWalletID == uuid.Nil {
		return nil, apperrors.New(apperrors.CodeValidationFailed, "sender and receiver wallet IDs are required")
	}
	if input.SenderWalletID == input.ReceiverWalletID {
		return nil, apperrors.New(apperrors.CodeValidationFailed, "cannot transfer to the same wallet")
	}

	// Check idempotency
	if input.IdempotencyKey != uuid.Nil {
		existing, err := s.repo.GetTransferByIdempotencyKey(ctx, input.IdempotencyKey)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			s.log.Info().
				Str("transfer_id", repository.GetTransferID(existing).String()).
				Msg("Duplicate transfer request, returning existing")
			return &TransferResult{
				TransferID: repository.GetTransferID(existing).String(),
				Status:     existing.Status,
			}, nil
		}
	}

	// Create transfer record
	transfer, err := s.repo.CreateTransfer(
		ctx,
		input.SenderWalletID,
		input.ReceiverWalletID,
		input.Amount,
		input.Currency,
		input.IdempotencyKey,
	)
	if err != nil {
		return nil, err
	}

	transferID := repository.GetTransferID(transfer)
	s.log.Info().
		Str("transfer_id", transferID.String()).
		Str("sender", input.SenderWalletID.String()).
		Str("receiver", input.ReceiverWalletID.String()).
		Str("amount", input.Amount).
		Msg("Transfer created, starting saga")

	// Log saga event
	_, _ = s.repo.CreateSagaEvent(ctx, transferID, "TRANSFER_CREATED", map[string]interface{}{
		"sender_wallet_id":   input.SenderWalletID.String(),
		"receiver_wallet_id": input.ReceiverWalletID.String(),
		"amount":             input.Amount,
	})

	// Publish transfer.created event to start the saga
	payload := models.TransferCreatedPayload{
		TransferID:       transferID.String(),
		SenderWalletID:   input.SenderWalletID.String(),
		ReceiverWalletID: input.ReceiverWalletID.String(),
		Amount:           input.Amount,
		Currency:         input.Currency,
	}
	payloadMap := make(map[string]interface{})
	b, _ := json.Marshal(payload)
	_ = json.Unmarshal(b, &payloadMap)

	event := models.NewEvent(models.TopicTransferCreated, transferID.String(), "transaction-service", payloadMap)
	if err := s.producer.Publish(ctx, models.TopicTransferCreated, event); err != nil {
		s.log.WithError(err).Error().Msg("Failed to publish transfer.created event")
		// Don't fail the request, the transfer is still created
	}

	return &TransferResult{
		TransferID: transferID.String(),
		Status:     transfer.Status,
	}, nil
}

// GetTransfer retrieves a transfer by ID.
func (s *TransferService) GetTransfer(ctx context.Context, transferID uuid.UUID) (*TransferResult, error) {
	transfer, err := s.repo.GetTransferByID(ctx, transferID)
	if err != nil {
		return nil, err
	}

	return &TransferResult{
		TransferID: repository.GetTransferID(transfer).String(),
		Status:     transfer.Status,
	}, nil
}

// UpdateStatus updates the status of a transfer (called by consumer).
func (s *TransferService) UpdateStatus(ctx context.Context, transferID uuid.UUID, status, failureReason string) error {
	_, err := s.repo.UpdateTransferStatus(ctx, transferID, status, failureReason)
	if err != nil {
		return err
	}

	// Log saga event
	_, _ = s.repo.CreateSagaEvent(ctx, transferID, "STATUS_UPDATED", map[string]interface{}{
		"new_status":     status,
		"failure_reason": failureReason,
	})

	s.log.Info().
		Str("transfer_id", transferID.String()).
		Str("new_status", status).
		Msg("Transfer status updated")

	return nil
}
