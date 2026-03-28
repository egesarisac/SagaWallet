package consumer

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"

	"github.com/egesarisac/SagaWallet/pkg/kafka"
	"github.com/egesarisac/SagaWallet/pkg/logger"
	"github.com/egesarisac/SagaWallet/pkg/middleware"
	"github.com/egesarisac/SagaWallet/pkg/models"
	"github.com/egesarisac/SagaWallet/services/wallet-service/internal/service"
)

// WalletConsumer handles Kafka events for wallet operations.
type WalletConsumer struct {
	consumer *kafka.Consumer
	producer *kafka.Producer
	svc      *service.WalletService
	log      *logger.Logger
}

// NewWalletConsumer creates a new wallet consumer.
func NewWalletConsumer(consumer *kafka.Consumer, producer *kafka.Producer, svc *service.WalletService, log *logger.Logger) *WalletConsumer {
	return &WalletConsumer{
		consumer: consumer,
		producer: producer,
		svc:      svc,
		log:      log,
	}
}

// Start starts the consumer loop.
func (c *WalletConsumer) Start(ctx context.Context) error {
	c.log.Info().
		Strs("topics", []string{
			models.TopicTransferCreated,
			models.TopicTransferDebitSuccess, // To trigger credit
			models.TopicTransferCreditFailed, // To trigger refund
		}).
		Msg("Starting Kafka consumer")

	return c.consumer.Start(ctx, func(ctx context.Context, event *models.Event) error {
		c.log.Info().
			Str("event_type", event.EventType).
			Str("event_id", event.EventID).
			Str("correlation_id", event.CorrelationID).
			Msg("Processing event")

		switch event.EventType {
		case models.TopicTransferCreated:
			return c.handleTransferCreated(ctx, event)
		case models.TopicTransferDebitSuccess:
			return c.handleDebitSuccess(ctx, event)
		case models.TopicTransferCreditFailed:
			return c.handleCreditFailed(ctx, event)
		default:
			return nil // Ignore irrelevant events
		}
	})
}

// handleTransferCreated handles the start of a transfer saga (Debit Sender).
func (c *WalletConsumer) handleTransferCreated(ctx context.Context, event *models.Event) error {
	middleware.RecordKafkaEvent(models.TopicTransferCreated, "received")
	payloadBytes, _ := json.Marshal(event.Payload)
	var payload models.TransferCreatedPayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return err
	}

	senderID, _ := uuid.Parse(payload.SenderWalletID)
	transferID, _ := uuid.Parse(payload.TransferID)

	// Call service to debit wallet
	_, err := c.svc.Debit(ctx, service.DebitInput{
		WalletID:    senderID,
		Amount:      payload.Amount,
		ReferenceID: transferID,
		Description: "Transfer Debit",
	})

	if err != nil {
		// Publish Failure Event
		failPayload := models.DebitResultPayload{
			TransferID: payload.TransferID,
			WalletID:   payload.SenderWalletID,
			Reason:     err.Error(),
		}
		failPayloadMap := make(map[string]interface{})
		b, _ := json.Marshal(failPayload)
		_ = json.Unmarshal(b, &failPayloadMap)

		failEvent := models.NewEvent(models.TopicTransferDebitFailed, event.CorrelationID, "wallet-service", failPayloadMap)
		return c.producer.Publish(ctx, models.TopicTransferDebitFailed, failEvent)
	}

	// Publish Success Event
	successPayload := models.DebitResultPayload{
		TransferID:       payload.TransferID,
		WalletID:         payload.SenderWalletID,
		SenderWalletID:   payload.SenderWalletID,
		ReceiverWalletID: payload.ReceiverWalletID,
		Amount:           payload.Amount,
	}
	successPayloadMap := make(map[string]interface{})
	b, _ := json.Marshal(successPayload)
	_ = json.Unmarshal(b, &successPayloadMap)

	// Send success event which will trigger the Credit step (choreography)
	successEvent := models.NewEvent(models.TopicTransferDebitSuccess, event.CorrelationID, "wallet-service", successPayloadMap)
	return c.producer.Publish(ctx, models.TopicTransferDebitSuccess, successEvent)
}

// handleDebitSuccess handles triggering the credit step after a successful debit.
func (c *WalletConsumer) handleDebitSuccess(ctx context.Context, event *models.Event) error {
	middleware.RecordKafkaEvent(models.TopicTransferDebitSuccess, "received")
	// In choreography, Wallet Service listens to its own DebitSuccess (or assumes it) to trigger Credit.

	payloadBytes, _ := json.Marshal(event.Payload)
	var payload models.DebitResultPayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return err
	}

	if payload.ReceiverWalletID == "" {
		return nil
	}

	receiverID, _ := uuid.Parse(payload.ReceiverWalletID)
	transferID, _ := uuid.Parse(payload.TransferID)

	// Call service to credit wallet
	_, err := c.svc.Credit(ctx, service.CreditInput{
		WalletID:    receiverID,
		Amount:      payload.Amount,
		ReferenceID: transferID,
		Description: "Transfer Credit",
	})

	if err != nil {
		// Publish Credit Failure Event -> Triggers Refund
		failPayload := models.CreditResultPayload{
			TransferID:     payload.TransferID,
			WalletID:       payload.ReceiverWalletID,
			SenderWalletID: payload.SenderWalletID, // Needed for refund
			Amount:         payload.Amount,         // Needed for refund
			Reason:         err.Error(),
		}
		failPayloadMap := make(map[string]interface{})
		b, _ := json.Marshal(failPayload)
		_ = json.Unmarshal(b, &failPayloadMap)

		failEvent := models.NewEvent(models.TopicTransferCreditFailed, event.CorrelationID, "wallet-service", failPayloadMap)
		return c.producer.Publish(ctx, models.TopicTransferCreditFailed, failEvent)
	}

	// Publish Credit Success Event -> Saga Complete
	successPayload := models.CreditResultPayload{
		TransferID:     payload.TransferID,
		WalletID:       payload.ReceiverWalletID,
		SenderWalletID: payload.SenderWalletID,
		Amount:         payload.Amount,
	}
	successPayloadMap := make(map[string]interface{})
	b, _ := json.Marshal(successPayload)
	_ = json.Unmarshal(b, &successPayloadMap)

	successEvent := models.NewEvent(models.TopicTransferCreditSuccess, event.CorrelationID, "wallet-service", successPayloadMap)
	return c.producer.Publish(ctx, models.TopicTransferCreditSuccess, successEvent)
}

// handleCreditFailed handles triggering the refund step after a failed credit.
func (c *WalletConsumer) handleCreditFailed(ctx context.Context, event *models.Event) error {
	middleware.RecordKafkaEvent(models.TopicTransferCreditFailed, "received")
	payloadBytes, _ := json.Marshal(event.Payload)
	var payload models.CreditResultPayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return err
	}

	senderID, _ := uuid.Parse(payload.SenderWalletID)
	transferID, _ := uuid.Parse(payload.TransferID)

	// Call service to refund sender
	_, err := c.svc.Credit(ctx, service.CreditInput{
		WalletID:    senderID,
		Amount:      payload.Amount,
		ReferenceID: transferID,
		Description: "Transfer Refund",
	})

	if err != nil {
		c.log.WithError(err).Error().Msg("CRITICAL: Failed to refund sender")
		return err
	}

	// Publish Refund Success Event
	successPayload := models.RefundResultPayload{
		TransferID: payload.TransferID,
		WalletID:   payload.SenderWalletID,
	}
	successPayloadMap := make(map[string]interface{})
	b, _ := json.Marshal(successPayload)
	_ = json.Unmarshal(b, &successPayloadMap)

	successEvent := models.NewEvent(models.TopicTransferRefundSuccess, event.CorrelationID, "wallet-service", successPayloadMap)
	return c.producer.Publish(ctx, models.TopicTransferRefundSuccess, successEvent)
}
