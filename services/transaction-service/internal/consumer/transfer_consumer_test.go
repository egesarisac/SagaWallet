package consumer

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/egesarisac/SagaWallet/pkg/logger"
	"github.com/egesarisac/SagaWallet/pkg/models"
)

type fakeSagaTransitions struct {
	changed     bool
	err         error
	expected    string
	target      string
	sourceTopic string
	sourceEvent string
	outputEvent *models.Event
	withOutbox  bool
}

func (f *fakeSagaTransitions) TransitionStatus(_ context.Context, _ uuid.UUID, expected, target, _ string, sourceTopic, sourceEvent string) (bool, error) {
	f.expected = expected
	f.target = target
	f.sourceTopic = sourceTopic
	f.sourceEvent = sourceEvent
	return f.changed, f.err
}

func (f *fakeSagaTransitions) TransitionStatusWithOutbox(_ context.Context, _ uuid.UUID, expected, target, _ string, sourceTopic, sourceEvent string, output *models.Event) (bool, error) {
	f.expected = expected
	f.target = target
	f.sourceTopic = sourceTopic
	f.sourceEvent = sourceEvent
	f.outputEvent = output
	f.withOutbox = true
	return f.changed, f.err
}

func TestCreditSuccessUsesAtomicTransitionOutbox(t *testing.T) {
	transferID := uuid.NewString()
	event := models.NewEvent(models.TopicTransferCreditSuccess, transferID, "test", map[string]interface{}{
		"transfer_id":      transferID,
		"wallet_id":        uuid.NewString(),
		"sender_wallet_id": uuid.NewString(),
		"amount":           "10.00",
	})
	transitions := &fakeSagaTransitions{changed: true}
	consumer := NewTransferConsumer(nil, transitions, logger.New(logger.Config{Level: "disabled", ServiceName: "transfer-consumer-test"}))

	if err := consumer.handleCreditSuccess(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if !transitions.withOutbox || transitions.outputEvent == nil {
		t.Fatal("expected completion event to be committed through the outbox")
	}
	if transitions.expected != "DEBITED" || transitions.target != "COMPLETED" {
		t.Fatalf("unexpected transition %s -> %s", transitions.expected, transitions.target)
	}
	if transitions.outputEvent.EventType != models.TopicTransferCompleted {
		t.Fatalf("expected %s output, got %s", models.TopicTransferCompleted, transitions.outputEvent.EventType)
	}
	if transitions.sourceTopic != event.EventType || transitions.sourceEvent != event.EventID {
		t.Fatal("expected source topic and event ID to be preserved")
	}
}

func TestInvalidTerminalPayloadDoesNotTransition(t *testing.T) {
	event := models.NewEvent(models.TopicTransferCreditSuccess, uuid.NewString(), "test", map[string]interface{}{
		"transfer_id": 42,
	})
	transitions := &fakeSagaTransitions{changed: true}
	consumer := NewTransferConsumer(nil, transitions, logger.New(logger.Config{Level: "disabled", ServiceName: "transfer-consumer-test"}))

	if err := consumer.handleCreditSuccess(context.Background(), event); err == nil {
		t.Fatal("expected malformed terminal event to fail validation")
	}
	if transitions.withOutbox || transitions.sourceEvent != "" {
		t.Fatal("state changed before terminal payload validation")
	}
}
