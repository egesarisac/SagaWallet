// Package consumer handles Kafka events for transaction service.
package consumer

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"

	"github.com/egesarisac/SagaWallet/pkg/kafka"
	"github.com/egesarisac/SagaWallet/pkg/logger"
	"github.com/egesarisac/SagaWallet/pkg/middleware"
	"github.com/egesarisac/SagaWallet/pkg/models"
)

type eventConsumer interface {
	Start(context.Context, kafka.MessageHandler) error
}

type sagaTransitions interface {
	TransitionStatus(context.Context, uuid.UUID, string, string, string, string, string) (bool, error)
	TransitionStatusWithOutbox(context.Context, uuid.UUID, string, string, string, string, string, *models.Event) (bool, error)
}

// TransferConsumer handles Kafka events for transfer saga observation.
type TransferConsumer struct {
	consumer eventConsumer
	svc      sagaTransitions
	log      *logger.Logger
}

// NewTransferConsumer creates a new transfer consumer.
func NewTransferConsumer(consumer eventConsumer, svc sagaTransitions, log *logger.Logger) *TransferConsumer {
	return &TransferConsumer{
		consumer: consumer,
		svc:      svc,
		log:      log,
	}
}

// Start starts the consumer loop.
func (c *TransferConsumer) Start(ctx context.Context) error {
	c.log.Info().
		Strs("topics", []string{
			models.TopicTransferDebitSuccess,
			models.TopicTransferDebitFailed,
			models.TopicTransferCreditSuccess,
			models.TopicTransferCreditFailed,
			models.TopicTransferRefundSuccess,
		}).
		Msg("Starting Transaction Consumer (Saga Observer)")

	return c.consumer.Start(ctx, func(ctx context.Context, event *models.Event) error {
		c.log.Info().
			Str("event_type", event.EventType).
			Str("correlation_id", event.CorrelationID).
			Msg("Processing saga event")

		switch event.EventType {
		case models.TopicTransferDebitSuccess:
			return c.handleDebitSuccess(ctx, event)
		case models.TopicTransferDebitFailed:
			return c.handleDebitFailed(ctx, event)
		case models.TopicTransferCreditSuccess:
			return c.handleCreditSuccess(ctx, event)
		case models.TopicTransferCreditFailed:
			return c.handleCreditFailed(ctx, event)
		case models.TopicTransferRefundSuccess:
			return c.handleRefundSuccess(ctx, event)
		default:
			return nil
		}
	})
}

func (c *TransferConsumer) handleDebitSuccess(ctx context.Context, event *models.Event) error {
	transferID, err := transferIDFromEvent(event)
	if err != nil {
		return err
	}
	changed, err := c.svc.TransitionStatus(ctx, transferID, "PENDING", "DEBITED", "", event.EventType, event.EventID)
	if err == nil && changed {
		middleware.RecordKafkaEvent(models.TopicTransferDebitSuccess, "success")
	}
	return err
}

func (c *TransferConsumer) handleDebitFailed(ctx context.Context, event *models.Event) error {
	transferID, err := transferIDFromEvent(event)
	if err != nil {
		return err
	}

	payloadBytes, err := json.Marshal(event.Payload)
	if err != nil {
		return err
	}
	var payload models.DebitResultPayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return err
	}

	failPayload := models.TransferFailedPayload{
		TransferID:     payload.TransferID,
		SenderWalletID: payload.WalletID,
		Reason:         payload.Reason,
	}
	failPayloadMap := make(map[string]interface{})
	b, _ := json.Marshal(failPayload)
	_ = json.Unmarshal(b, &failPayloadMap)

	failEvent := models.NewEvent(models.TopicTransferFailed, event.CorrelationID, "transaction-service", failPayloadMap)
	changed, err := c.svc.TransitionStatusWithOutbox(ctx, transferID, "PENDING", "FAILED", payload.Reason, event.EventType, event.EventID, failEvent)
	if err == nil && changed {
		middleware.RecordTransfer("FAILED")
	}
	return err
}

func (c *TransferConsumer) handleCreditSuccess(ctx context.Context, event *models.Event) error {
	transferID, err := transferIDFromEvent(event)
	if err != nil {
		return err
	}
	payloadBytes, err := json.Marshal(event.Payload)
	if err != nil {
		return err
	}
	var payload models.CreditResultPayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return err
	}

	completedPayload := models.TransferCompletedPayload{
		TransferID:       payload.TransferID,
		SenderWalletID:   payload.SenderWalletID,
		ReceiverWalletID: payload.WalletID,
		Amount:           payload.Amount,
	}
	completedPayloadMap := make(map[string]interface{})
	b, _ := json.Marshal(completedPayload)
	_ = json.Unmarshal(b, &completedPayloadMap)

	completedEvent := models.NewEvent(models.TopicTransferCompleted, event.CorrelationID, "transaction-service", completedPayloadMap)
	changed, err := c.svc.TransitionStatusWithOutbox(ctx, transferID, "DEBITED", "COMPLETED", "", event.EventType, event.EventID, completedEvent)
	if err == nil && changed {
		middleware.RecordKafkaEvent(models.TopicTransferCreditSuccess, "success")
		middleware.RecordTransfer("COMPLETED")
	}
	return err
}

func (c *TransferConsumer) handleCreditFailed(ctx context.Context, event *models.Event) error {
	transferID, err := transferIDFromEvent(event)
	if err != nil {
		return err
	}
	// Update status to REFUNDING (refund is triggered by Wallet Service in choreography)
	_, err = c.svc.TransitionStatus(ctx, transferID, "DEBITED", "REFUNDING", "Credit failed, awaiting refund", event.EventType, event.EventID)
	return err
}

func (c *TransferConsumer) handleRefundSuccess(ctx context.Context, event *models.Event) error {
	transferID, err := transferIDFromEvent(event)
	if err != nil {
		return err
	}
	payloadBytes, err := json.Marshal(event.Payload)
	if err != nil {
		return err
	}
	var payload models.RefundResultPayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return err
	}

	failPayload := models.TransferFailedPayload{
		TransferID:     payload.TransferID,
		SenderWalletID: payload.WalletID,
		Reason:         "Refunded after credit failure",
	}
	failPayloadMap := make(map[string]interface{})
	b, _ := json.Marshal(failPayload)
	_ = json.Unmarshal(b, &failPayloadMap)

	failEvent := models.NewEvent(models.TopicTransferFailed, event.CorrelationID, "transaction-service", failPayloadMap)
	changed, err := c.svc.TransitionStatusWithOutbox(ctx, transferID, "REFUNDING", "FAILED", "Refunded after credit failure", event.EventType, event.EventID, failEvent)
	if err == nil && changed {
		middleware.RecordKafkaEvent(models.TopicTransferRefundSuccess, "success")
		middleware.RecordTransfer("FAILED")
	}
	return err
}

func transferIDFromEvent(event *models.Event) (uuid.UUID, error) {
	if _, err := uuid.Parse(event.EventID); err != nil {
		return uuid.Nil, err
	}
	return uuid.Parse(event.CorrelationID)
}
