// Package consumer handles Kafka events for notification service.
package consumer

import (
	"context"
	"encoding/json"

	"github.com/egesarisac/SagaWallet/pkg/kafka"
	"github.com/egesarisac/SagaWallet/pkg/logger"
	"github.com/egesarisac/SagaWallet/pkg/models"
	"github.com/egesarisac/SagaWallet/services/notification-service/internal/notifier"
)

// NotificationConsumer handles Kafka events for notifications.
type NotificationConsumer struct {
	consumer *kafka.Consumer
	notifier *notifier.Notifier
	log      *logger.Logger
}

// NewNotificationConsumer creates a new notification consumer.
func NewNotificationConsumer(consumer *kafka.Consumer, notifier *notifier.Notifier, log *logger.Logger) *NotificationConsumer {
	return &NotificationConsumer{
		consumer: consumer,
		notifier: notifier,
		log:      log,
	}
}

// Start starts the consumer loop.
func (c *NotificationConsumer) Start(ctx context.Context) error {
	c.log.Info().
		Strs("topics", []string{
			models.TopicTransferCompleted,
			models.TopicTransferFailed,
		}).
		Msg("Starting Notification Consumer")

	return c.consumer.Start(ctx, func(ctx context.Context, event *models.Event) error {
		c.log.Info().
			Str("event_type", event.EventType).
			Str("correlation_id", event.CorrelationID).
			Msg("Received notification event")

		switch event.EventType {
		case models.TopicTransferCompleted:
			return c.handleTransferCompleted(event)
		case models.TopicTransferFailed:
			return c.handleTransferFailed(event)
		default:
			return nil
		}
	})
}

func (c *NotificationConsumer) handleTransferCompleted(event *models.Event) error {
	payloadBytes, _ := json.Marshal(event.Payload)
	var payload models.TransferCompletedPayload
	_ = json.Unmarshal(payloadBytes, &payload)

	c.notifier.NotifyTransferCompleted(
		payload.TransferID,
		payload.SenderWalletID,
		payload.ReceiverWalletID,
		payload.Amount,
	)
	return nil
}

func (c *NotificationConsumer) handleTransferFailed(event *models.Event) error {
	payloadBytes, _ := json.Marshal(event.Payload)
	var payload models.TransferFailedPayload
	_ = json.Unmarshal(payloadBytes, &payload)

	c.notifier.NotifyTransferFailed(
		payload.TransferID,
		payload.SenderWalletID,
		payload.Reason,
	)
	return nil
}
