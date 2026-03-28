package service

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/egesarisac/SagaWallet/pkg/kafka"
	"github.com/egesarisac/SagaWallet/pkg/logger"
	"github.com/egesarisac/SagaWallet/pkg/models"
	"github.com/egesarisac/SagaWallet/services/transaction-service/internal/repository"
)

// TimeoutWorker periodically checks for and resolves stuck saga transactions.
type TimeoutWorker struct {
	repo     *repository.TransferRepository
	svc      *TransferService
	producer *kafka.Producer
	log      *logger.Logger
}

// NewTimeoutWorker creates a new TimeoutWorker.
func NewTimeoutWorker(repo *repository.TransferRepository, svc *TransferService, producer *kafka.Producer, log *logger.Logger) *TimeoutWorker {
	return &TimeoutWorker{
		repo:     repo,
		svc:      svc,
		producer: producer,
		log:      log,
	}
}

// Start begins the background worker loop.
func (w *TimeoutWorker) Start(ctx context.Context, interval time.Duration) {
	w.log.Info().Dur("interval", interval).Msg("Starting Saga Timeout Worker")

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.log.Info().Msg("Saga Timeout Worker stopping")
			return
		case <-ticker.C:
			w.processTimeouts(ctx)
		}
	}
}

func (w *TimeoutWorker) processTimeouts(ctx context.Context) {
	now := time.Now().UTC()
	pendingThreshold := w.timeToPgTimestamptz(now.Add(-30 * time.Second))
	debitedThreshold := w.timeToPgTimestamptz(now.Add(-60 * time.Second))
	refundingThreshold := w.timeToPgTimestamptz(now.Add(-120 * time.Second))

	// Get up to 100 stuck transfers
	transfers, err := w.repo.ListStuckTransfers(ctx, pendingThreshold, debitedThreshold, refundingThreshold, 100)
	if err != nil {
		w.log.WithError(err).Error().Msg("Failed to list stuck transfers")
		return
	}

	if len(transfers) > 0 {
		w.log.Info().Int("count", len(transfers)).Msg("Found stuck transfers to process")
	}

	for _, t := range transfers {
		transferID := repository.GetTransferID(&t)
		w.log.Info().
			Str("transfer_id", transferID.String()).
			Str("status", t.Status).
			Msg("Processing stuck transfer")

		switch t.Status {
		case string(models.TransferStatusPending):
			// PENDING -> FAILED
			reason := "Saga timeout: no response after transfer.created"
			err := w.svc.UpdateStatus(ctx, transferID, string(models.TransferStatusFailed), reason)
			if err == nil {
				w.publishTransferFailed(ctx, transferID.String(), w.uuidFromPgtype(t.SenderWalletID), reason)
			}

		case string(models.TransferStatusDebited):
			// DEBITED -> REFUNDING
			reason := "Saga timeout: no response after transfer.debit.success"
			err := w.svc.UpdateStatus(ctx, transferID, string(models.TransferStatusRefunding), reason)
			if err == nil {
				w.publishCreditFailed(ctx, transferID.String(), w.uuidFromPgtype(t.ReceiverWalletID), w.uuidFromPgtype(t.SenderWalletID), w.numericToString(t.Amount), reason)
			}

		case string(models.TransferStatusRefunding):
			// REFUNDING -> MANUAL_REVIEW
			reason := "Saga timeout: no response after transfer.credit.failed"
			err := w.svc.UpdateStatus(ctx, transferID, string(models.TransferStatusManualReview), reason)
			if err == nil {
				w.log.Error().
					Str("transfer_id", transferID.String()).
					Msg("CRITICAL: Transfer stuck in REFUNDING state, escalating to MANUAL_REVIEW")
			}
		}
	}
}

func (w *TimeoutWorker) publishTransferFailed(ctx context.Context, transferID, senderWalletID, reason string) {
	payload := models.TransferFailedPayload{
		TransferID:     transferID,
		SenderWalletID: senderWalletID,
		Reason:         reason,
	}
	payloadMap := make(map[string]interface{})
	b, _ := json.Marshal(payload)
	_ = json.Unmarshal(b, &payloadMap)

	event := models.NewEvent(models.TopicTransferFailed, transferID, "transaction-service-timeout", payloadMap)
	if err := w.producer.Publish(ctx, models.TopicTransferFailed, event); err != nil {
		w.log.WithError(err).Error().Str("transfer_id", transferID).Msg("Failed to publish transfer.failed due to timeout")
	}
}

func (w *TimeoutWorker) publishCreditFailed(ctx context.Context, transferID, receiverWalletID, senderWalletID, amount, reason string) {
	// Publish as transfer.credit.failed so the Wallet Service picks it up and refunds the sender.
	payload := models.CreditResultPayload{
		TransferID:     transferID,
		WalletID:       receiverWalletID,
		SenderWalletID: senderWalletID,
		Amount:         amount,
		Reason:         reason,
	}
	payloadMap := make(map[string]interface{})
	b, _ := json.Marshal(payload)
	_ = json.Unmarshal(b, &payloadMap)

	event := models.NewEvent(models.TopicTransferCreditFailed, transferID, "transaction-service-timeout", payloadMap)
	if err := w.producer.Publish(ctx, models.TopicTransferCreditFailed, event); err != nil {
		w.log.WithError(err).Error().Str("transfer_id", transferID).Msg("Failed to publish transfer.credit.failed due to timeout")
	}
}

// Helpers
func (w *TimeoutWorker) timeToPgTimestamptz(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

func (w *TimeoutWorker) uuidFromPgtype(p pgtype.UUID) string {
	if !p.Valid {
		return ""
	}
	return uuid.UUID(p.Bytes).String()
}

func (w *TimeoutWorker) numericToString(n pgtype.Numeric) string {
	if !n.Valid {
		return "0.00"
	}
	val, err := n.Float64Value()
	if err != nil || !val.Valid {
		return "0.00"
	}
	return strconv.FormatFloat(val.Float64, 'f', 2, 64)
}
