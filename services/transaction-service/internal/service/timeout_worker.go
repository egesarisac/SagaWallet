package service

import (
	"context"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/egesarisac/SagaWallet/pkg/logger"
	"github.com/egesarisac/SagaWallet/pkg/models"
	"github.com/egesarisac/SagaWallet/services/transaction-service/internal/repository"
)

// TimeoutWorker periodically checks for and resolves stuck saga transactions.
type TimeoutWorker struct {
	repo *repository.TransferRepository
	svc  *TransferService
	log  *logger.Logger
}

// NewTimeoutWorker creates a new TimeoutWorker.
func NewTimeoutWorker(repo *repository.TransferRepository, svc *TransferService, log *logger.Logger) *TimeoutWorker {
	return &TimeoutWorker{
		repo: repo,
		svc:  svc,
		log:  log,
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
			reason := "Saga timeout: no response after transfer.created"
			output, err := w.transferFailedEvent(transferID.String(), w.uuidFromPgtype(t.SenderWalletID), reason)
			if err != nil {
				w.log.WithError(err).Error().Str("transfer_id", transferID.String()).Msg("Failed to build timeout event")
				continue
			}
			if _, err := w.svc.TransitionStatusWithOutbox(ctx, transferID, string(models.TransferStatusPending), string(models.TransferStatusFailed), reason, "saga.timeout", uuid.NewString(), output); err != nil {
				w.log.WithError(err).Warn().Str("transfer_id", transferID.String()).Msg("Failed to resolve pending transfer timeout")
			}

		case string(models.TransferStatusDebited):
			reason := "Saga timeout: no response after transfer.debit.success"
			output, err := w.creditFailedEvent(transferID.String(), w.uuidFromPgtype(t.ReceiverWalletID), w.uuidFromPgtype(t.SenderWalletID), w.numericToString(t.Amount), reason)
			if err != nil {
				w.log.WithError(err).Error().Str("transfer_id", transferID.String()).Msg("Failed to build refund request")
				continue
			}
			if _, err := w.svc.TransitionStatusWithOutbox(ctx, transferID, string(models.TransferStatusDebited), string(models.TransferStatusRefunding), reason, "saga.timeout", uuid.NewString(), output); err != nil {
				w.log.WithError(err).Warn().Str("transfer_id", transferID.String()).Msg("Failed to resolve debited transfer timeout")
			}

		case string(models.TransferStatusRefunding):
			reason := "Saga timeout: no response after transfer.credit.failed"
			output, err := w.transferFailedEvent(transferID.String(), w.uuidFromPgtype(t.SenderWalletID), reason+"; manual review required")
			if err != nil {
				w.log.WithError(err).Error().Str("transfer_id", transferID.String()).Msg("Failed to build manual review event")
				continue
			}
			changed, err := w.svc.TransitionStatusWithOutbox(ctx, transferID, string(models.TransferStatusRefunding), string(models.TransferStatusManualReview), reason, "saga.timeout", uuid.NewString(), output)
			if err != nil {
				w.log.WithError(err).Warn().Str("transfer_id", transferID.String()).Msg("Failed to escalate refund timeout")
			} else if changed {
				w.log.Error().
					Str("transfer_id", transferID.String()).
					Msg("CRITICAL: Transfer stuck in REFUNDING state, escalating to MANUAL_REVIEW")
			}
		}
	}
}

func (w *TimeoutWorker) transferFailedEvent(transferID, senderWalletID, reason string) (*models.Event, error) {
	payload := models.TransferFailedPayload{
		TransferID:     transferID,
		SenderWalletID: senderWalletID,
		Reason:         reason,
	}
	payloadMap, err := payloadToMap(payload)
	if err != nil {
		return nil, err
	}
	return models.NewEvent(models.TopicTransferFailed, transferID, "transaction-service-timeout", payloadMap), nil
}

func (w *TimeoutWorker) creditFailedEvent(transferID, receiverWalletID, senderWalletID, amount, reason string) (*models.Event, error) {
	payload := models.CreditResultPayload{
		TransferID:     transferID,
		WalletID:       receiverWalletID,
		SenderWalletID: senderWalletID,
		Amount:         amount,
		Reason:         reason,
	}
	payloadMap, err := payloadToMap(payload)
	if err != nil {
		return nil, err
	}
	return models.NewEvent(models.TopicTransferCreditFailed, transferID, "transaction-service-timeout", payloadMap), nil
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
