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
	"github.com/egesarisac/SagaWallet/services/transaction-service/internal/service"
)

// TransferConsumer handles Kafka events for transfer saga observation.
type TransferConsumer struct {
	consumer *kafka.Consumer
	producer *kafka.Producer
	svc      *service.TransferService
	log      *logger.Logger
}

// NewTransferConsumer creates a new transfer consumer.
func NewTransferConsumer(consumer *kafka.Consumer, producer *kafka.Producer, svc *service.TransferService, log *logger.Logger) *TransferConsumer {
	return &TransferConsumer{
		consumer: consumer,
		producer: producer,
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
	middleware.RecordKafkaEvent(models.TopicTransferDebitSuccess, "success")
	transferID, _ := uuid.Parse(event.CorrelationID)
	return c.svc.UpdateStatus(ctx, transferID, "DEBITED", "")
}

func (c *TransferConsumer) handleDebitFailed(ctx context.Context, event *models.Event) error {
	transferID, _ := uuid.Parse(event.CorrelationID)

	payloadBytes, _ := json.Marshal(event.Payload)
	var payload models.DebitResultPayload
	_ = json.Unmarshal(payloadBytes, &payload)

	if err := c.svc.UpdateStatus(ctx, transferID, "FAILED", payload.Reason); err != nil {
		return err
	}

	// Publish transfer.failed event for notification service
	failPayload := models.TransferFailedPayload{
		TransferID:     payload.TransferID,
		SenderWalletID: payload.WalletID,
		Reason:         payload.Reason,
	}
	failPayloadMap := make(map[string]interface{})
	b, _ := json.Marshal(failPayload)
	_ = json.Unmarshal(b, &failPayloadMap)

	failEvent := models.NewEvent(models.TopicTransferFailed, event.CorrelationID, "transaction-service", failPayloadMap)
	return c.producer.Publish(ctx, models.TopicTransferFailed, failEvent)
}

func (c *TransferConsumer) handleCreditSuccess(ctx context.Context, event *models.Event) error {
	middleware.RecordKafkaEvent(models.TopicTransferCreditSuccess, "success")
	transferID, _ := uuid.Parse(event.CorrelationID)

	if err := c.svc.UpdateStatus(ctx, transferID, "COMPLETED", ""); err != nil {
		return err
	}

	// Record successful transfer
	middleware.RecordTransfer("COMPLETED")

	// Publish transfer.completed event for notification service
	payloadBytes, _ := json.Marshal(event.Payload)
	var payload models.CreditResultPayload
	_ = json.Unmarshal(payloadBytes, &payload)

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
	return c.producer.Publish(ctx, models.TopicTransferCompleted, completedEvent)
}

func (c *TransferConsumer) handleCreditFailed(ctx context.Context, event *models.Event) error {
	transferID, _ := uuid.Parse(event.CorrelationID)
	// Update status to REFUNDING (refund is triggered by Wallet Service in choreography)
	return c.svc.UpdateStatus(ctx, transferID, "REFUNDING", "Credit failed, awaiting refund")
}

func (c *TransferConsumer) handleRefundSuccess(ctx context.Context, event *models.Event) error {
	middleware.RecordKafkaEvent(models.TopicTransferRefundSuccess, "success")
	transferID, _ := uuid.Parse(event.CorrelationID)

	if err := c.svc.UpdateStatus(ctx, transferID, "FAILED", "Refunded after credit failure"); err != nil {
		return err
	}

	// Record failed transfer
	middleware.RecordTransfer("FAILED")

	// Publish transfer.failed with refund info
	payloadBytes, _ := json.Marshal(event.Payload)
	var payload models.RefundResultPayload
	_ = json.Unmarshal(payloadBytes, &payload)

	failPayload := models.TransferFailedPayload{
		TransferID:     payload.TransferID,
		SenderWalletID: payload.WalletID,
		Reason:         "Refunded after credit failure",
	}
	failPayloadMap := make(map[string]interface{})
	b, _ := json.Marshal(failPayload)
	_ = json.Unmarshal(b, &failPayloadMap)

	failEvent := models.NewEvent(models.TopicTransferFailed, event.CorrelationID, "transaction-service", failPayloadMap)
	return c.producer.Publish(ctx, models.TopicTransferFailed, failEvent)
}
