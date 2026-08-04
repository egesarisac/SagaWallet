// Package service provides business logic for transaction service.
package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	apperrors "github.com/egesarisac/SagaWallet/pkg/errors"
	"github.com/egesarisac/SagaWallet/pkg/kafka"
	"github.com/egesarisac/SagaWallet/pkg/logger"
	"github.com/egesarisac/SagaWallet/pkg/models"
	grpcclient "github.com/egesarisac/SagaWallet/services/transaction-service/internal/grpc"
	"github.com/egesarisac/SagaWallet/services/transaction-service/internal/repository"
)

// TransferService handles transfer business logic.
type TransferService struct {
	repo     *repository.TransferRepository
	producer *kafka.Producer
	wallet   *grpcclient.WalletClient
	log      *logger.Logger
}

// NewTransferService creates a new transfer service.
func NewTransferService(repo *repository.TransferRepository, producer *kafka.Producer, wallet *grpcclient.WalletClient, log *logger.Logger) *TransferService {
	return &TransferService{
		repo:     repo,
		producer: producer,
		wallet:   wallet,
		log:      log,
	}
}

// CreateTransferInput represents input for creating a transfer.
type CreateTransferInput struct {
	RequestUserID    uuid.UUID
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

func pgUUIDToUUID(id struct {
	Bytes [16]byte
	Valid bool
}) uuid.UUID {
	if !id.Valid {
		return uuid.Nil
	}
	return id.Bytes
}

func (s *TransferService) isWalletOwnedByUser(ctx context.Context, walletID, userID uuid.UUID) (bool, error) {
	if walletID == uuid.Nil {
		return false, nil
	}

	wallet, err := s.wallet.GetWallet(ctx, walletID)
	if err != nil {
		return false, err
	}

	return wallet.GetUserId() == userID.String(), nil
}

// CreateTransfer initiates a new transfer (starts the saga).
func (s *TransferService) CreateTransfer(ctx context.Context, input CreateTransferInput) (*TransferResult, error) {
	// Validate input
	if input.RequestUserID == uuid.Nil {
		return nil, apperrors.New(apperrors.CodeUnauthorized, "missing authenticated user context")
	}
	if input.SenderWalletID == uuid.Nil || input.ReceiverWalletID == uuid.Nil {
		return nil, apperrors.New(apperrors.CodeValidationFailed, "sender and receiver wallet IDs are required")
	}
	if input.SenderWalletID == input.ReceiverWalletID {
		return nil, apperrors.New(apperrors.CodeValidationFailed, "cannot transfer to the same wallet")
	}

	if s.wallet == nil {
		return nil, apperrors.New(apperrors.CodeServiceUnavailable, "wallet verification service unavailable")
	}

	senderWallet, err := s.wallet.GetWallet(ctx, input.SenderWalletID)
	if err != nil {
		return nil, apperrors.New(apperrors.CodeServiceUnavailable, "failed to verify sender wallet ownership")
	}

	if senderWallet.GetUserId() != input.RequestUserID.String() {
		return nil, apperrors.New(apperrors.CodeForbidden, "sender wallet does not belong to authenticated user")
	}

	// Check idempotency
	if input.IdempotencyKey != uuid.Nil {
		existing, err := s.repo.GetTransferByIdempotencyKey(ctx, input.IdempotencyKey)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			owned, err := s.isWalletOwnedByUser(ctx, pgUUIDToUUID(existing.SenderWalletID), input.RequestUserID)
			if err != nil {
				return nil, apperrors.New(apperrors.CodeServiceUnavailable, "failed to verify transfer ownership")
			}
			if !owned {
				return nil, apperrors.New(apperrors.CodeForbidden, "transfer does not belong to authenticated user")
			}

			s.log.Info().
				Str("transfer_id", repository.GetTransferID(existing).String()).
				Msg("Duplicate transfer request, returning existing")
			return &TransferResult{
				TransferID: repository.GetTransferID(existing).String(),
				Status:     existing.Status,
			}, nil
		}
	}

	transferID := uuid.New()
	payload := models.TransferCreatedPayload{
		TransferID:       transferID.String(),
		SenderWalletID:   input.SenderWalletID.String(),
		ReceiverWalletID: input.ReceiverWalletID.String(),
		Amount:           input.Amount,
		Currency:         input.Currency,
	}
	payloadMap, err := payloadToMap(payload)
	if err != nil {
		return nil, err
	}
	event := models.NewEvent(models.TopicTransferCreated, transferID.String(), "transaction-service", payloadMap)
	eventID, err := uuid.Parse(event.EventID)
	if err != nil {
		return nil, apperrors.Wrap(apperrors.CodeInternalError, "failed to create transfer event ID", err)
	}

	// Persist the command and its start event together. Kafka is never a second commit point.
	transfer, err := s.repo.CreateTransferWithOutbox(
		ctx,
		transferID,
		input.SenderWalletID,
		input.ReceiverWalletID,
		input.Amount,
		input.Currency,
		input.IdempotencyKey,
		eventID,
		models.TopicTransferCreated,
		event,
	)
	if err != nil {
		return nil, err
	}
	transferID = repository.GetTransferID(transfer)
	s.log.Info().
		Str("transfer_id", transferID.String()).
		Str("sender", input.SenderWalletID.String()).
		Str("receiver", input.ReceiverWalletID.String()).
		Str("amount", input.Amount).
		Msg("Transfer created, starting saga")

	return &TransferResult{
		TransferID: transferID.String(),
		Status:     transfer.Status,
	}, nil
}

func payloadToMap(payload interface{}) (map[string]interface{}, error) {
	bytes, err := json.Marshal(payload)
	if err != nil {
		return nil, apperrors.Wrap(apperrors.CodeInternalError, "failed to marshal event payload", err)
	}
	result := make(map[string]interface{})
	if err := json.Unmarshal(bytes, &result); err != nil {
		return nil, apperrors.Wrap(apperrors.CodeInternalError, "failed to unmarshal event payload", err)
	}
	return result, nil
}

// GetTransfer retrieves a transfer by ID.
func (s *TransferService) GetTransfer(ctx context.Context, transferID uuid.UUID, requestUserID uuid.UUID) (*TransferResult, error) {
	if requestUserID == uuid.Nil {
		return nil, apperrors.New(apperrors.CodeUnauthorized, "missing authenticated user context")
	}

	if s.wallet == nil {
		return nil, apperrors.New(apperrors.CodeServiceUnavailable, "wallet verification service unavailable")
	}

	transfer, err := s.repo.GetTransferByID(ctx, transferID)
	if err != nil {
		return nil, err
	}

	senderOwned, err := s.isWalletOwnedByUser(ctx, pgUUIDToUUID(transfer.SenderWalletID), requestUserID)
	if err != nil {
		return nil, apperrors.New(apperrors.CodeServiceUnavailable, "failed to verify transfer ownership")
	}
	if !senderOwned {
		receiverOwned, err := s.isWalletOwnedByUser(ctx, pgUUIDToUUID(transfer.ReceiverWalletID), requestUserID)
		if err != nil {
			return nil, apperrors.New(apperrors.CodeServiceUnavailable, "failed to verify transfer ownership")
		}
		if !receiverOwned {
			return nil, apperrors.New(apperrors.CodeForbidden, "transfer does not belong to authenticated user")
		}
	}

	return &TransferResult{
		TransferID: repository.GetTransferID(transfer).String(),
		Status:     transfer.Status,
	}, nil
}

// TransitionStatus applies a legal saga transition once for an incoming event.
func (s *TransferService) TransitionStatus(ctx context.Context, transferID uuid.UUID, expectedStatus, status, failureReason, eventID string) (bool, error) {
	parsedEventID, err := uuid.Parse(eventID)
	if err != nil {
		return false, apperrors.New(apperrors.CodeValidationFailed, fmt.Sprintf("invalid saga event ID: %s", eventID))
	}

	changed, err := s.repo.TransitionStatus(ctx, transferID, expectedStatus, status, failureReason, parsedEventID)
	if err != nil || !changed {
		return changed, err
	}

	_, auditErr := s.repo.CreateSagaEvent(ctx, transferID, "STATUS_UPDATED", map[string]interface{}{
		"event_id":        eventID,
		"previous_status": expectedStatus,
		"new_status":      status,
		"failure_reason":  failureReason,
	})
	if auditErr != nil {
		s.log.WithError(auditErr).WithField("transfer_id", transferID.String()).Warn().Msg("Failed to record status transition audit event")
	}

	s.log.Info().Str("transfer_id", transferID.String()).Str("new_status", status).Msg("Transfer status transitioned")
	return true, nil
}
