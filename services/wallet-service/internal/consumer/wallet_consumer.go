package consumer

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"

	"github.com/egesarisac/SagaWallet/pkg/kafka"
	"github.com/egesarisac/SagaWallet/pkg/logger"
	"github.com/egesarisac/SagaWallet/pkg/middleware"
	"github.com/egesarisac/SagaWallet/pkg/models"
	db "github.com/egesarisac/SagaWallet/services/wallet-service/db/generated"
	"github.com/egesarisac/SagaWallet/services/wallet-service/internal/service"
)

type eventConsumer interface {
	Start(context.Context, kafka.MessageHandler) error
}

type eventPublisher interface {
	Publish(context.Context, string, *models.Event) error
}

type walletCommands interface {
	DebitForEvent(context.Context, uuid.UUID, string, service.DebitInput) (*db.Wallet, bool, error)
	CreditForEvent(context.Context, uuid.UUID, string, service.CreditInput) (*db.Wallet, bool, error)
}

// WalletConsumer handles Kafka events for wallet operations.
type WalletConsumer struct {
	consumer eventConsumer
	producer eventPublisher
	svc      walletCommands
	log      *logger.Logger
}

// NewWalletConsumer creates a new wallet consumer.
func NewWalletConsumer(consumer eventConsumer, producer eventPublisher, svc walletCommands, log *logger.Logger) *WalletConsumer {
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
	payloadBytes, err := json.Marshal(event.Payload)
	if err != nil {
		return err
	}
	var payload models.TransferCreatedPayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return err
	}

	eventID, senderID, transferID, err := sagaIDs(event, payload.SenderWalletID, payload.TransferID)
	if err != nil {
		return err
	}

	// Call service to debit wallet
	_, duplicate, err := c.svc.DebitForEvent(ctx, eventID, event.EventType, service.DebitInput{
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
	if duplicate {
		c.log.Info().Str("event_id", event.EventID).Msg("Re-emitting debit result for an already applied command")
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

	payloadBytes, err := json.Marshal(event.Payload)
	if err != nil {
		return err
	}
	var payload models.DebitResultPayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return err
	}

	if payload.ReceiverWalletID == "" {
		return nil
	}

	eventID, receiverID, transferID, err := sagaIDs(event, payload.ReceiverWalletID, payload.TransferID)
	if err != nil {
		return err
	}

	// Call service to credit wallet
	_, duplicate, err := c.svc.CreditForEvent(ctx, eventID, event.EventType, service.CreditInput{
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
	if duplicate {
		c.log.Info().Str("event_id", event.EventID).Msg("Re-emitting credit result for an already applied command")
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
	payloadBytes, err := json.Marshal(event.Payload)
	if err != nil {
		return err
	}
	var payload models.CreditResultPayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return err
	}

	eventID, senderID, transferID, err := sagaIDs(event, payload.SenderWalletID, payload.TransferID)
	if err != nil {
		return err
	}

	// Call service to refund sender
	_, duplicate, err := c.svc.CreditForEvent(ctx, eventID, event.EventType, service.CreditInput{
		WalletID:    senderID,
		Amount:      payload.Amount,
		ReferenceID: transferID,
		Description: "Transfer Refund",
	})
	if err != nil {
		c.log.WithError(err).Error().Msg("CRITICAL: Failed to refund sender")
		return err
	}
	if duplicate {
		c.log.Info().Str("event_id", event.EventID).Msg("Re-emitting refund result for an already applied command")
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

func sagaIDs(event *models.Event, walletID, transferID string) (uuid.UUID, uuid.UUID, uuid.UUID, error) {
	eventUUID, err := uuid.Parse(event.EventID)
	if err != nil {
		return uuid.Nil, uuid.Nil, uuid.Nil, err
	}
	walletUUID, err := uuid.Parse(walletID)
	if err != nil {
		return uuid.Nil, uuid.Nil, uuid.Nil, err
	}
	transferUUID, err := uuid.Parse(transferID)
	if err != nil {
		return uuid.Nil, uuid.Nil, uuid.Nil, err
	}
	return eventUUID, walletUUID, transferUUID, nil
}
