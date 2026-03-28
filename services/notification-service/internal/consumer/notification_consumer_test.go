package consumer

import (
	"encoding/json"
	"testing"

	"github.com/egesarisac/SagaWallet/pkg/logger"
	"github.com/egesarisac/SagaWallet/pkg/models"
	"github.com/egesarisac/SagaWallet/services/notification-service/internal/notifier"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestNotificationConsumer_HandleTransferCompleted(t *testing.T) {
	l := logger.New(logger.Config{
		Level:       "debug",
		Format:      "console",
		ServiceName: "test-consumer",
	})
	notif := notifier.NewNotifier(l)

	// Kafka consumer instance is not needed for unit testing the handler logic
	consumer := NewNotificationConsumer(nil, notif, l)

	payload := models.TransferCompletedPayload{
		TransferID:       uuid.New().String(),
		SenderWalletID:   uuid.New().String(),
		ReceiverWalletID: uuid.New().String(),
		Amount:           "500.00",
		Currency:         "USD",
	}

	// In the real system, json is unmarshaled into an interface{} initially via the Kafka consumer
	payloadMap := make(map[string]interface{})
	bytes, _ := json.Marshal(payload)
	_ = json.Unmarshal(bytes, &payloadMap)

	event := &models.Event{
		EventID:       uuid.New().String(),
		EventType:     models.TopicTransferCompleted,
		CorrelationID: uuid.New().String(),
		Payload:       payloadMap,
	}

	err := consumer.handleTransferCompleted(event)
	assert.NoError(t, err)
}

func TestNotificationConsumer_HandleTransferFailed(t *testing.T) {
	l := logger.New(logger.Config{
		Level:       "debug",
		Format:      "console",
		ServiceName: "test-consumer",
	})
	notif := notifier.NewNotifier(l)

	consumer := NewNotificationConsumer(nil, notif, l)

	payload := models.TransferFailedPayload{
		TransferID:     uuid.New().String(),
		SenderWalletID: uuid.New().String(),
		Reason:         "Insufficient Funds",
	}

	payloadMap := make(map[string]interface{})
	bytes, _ := json.Marshal(payload)
	_ = json.Unmarshal(bytes, &payloadMap)

	event := &models.Event{
		EventID:       uuid.New().String(),
		EventType:     models.TopicTransferFailed,
		CorrelationID: uuid.New().String(),
		Payload:       payloadMap,
	}

	err := consumer.handleTransferFailed(event)
	assert.NoError(t, err)
}
