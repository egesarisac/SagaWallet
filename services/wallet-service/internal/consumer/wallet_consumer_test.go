package consumer

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/egesarisac/SagaWallet/pkg/logger"
	"github.com/egesarisac/SagaWallet/pkg/models"
	db "github.com/egesarisac/SagaWallet/services/wallet-service/db/generated"
	"github.com/egesarisac/SagaWallet/services/wallet-service/internal/service"
)

type fakeWalletCommands struct {
	duplicate bool
	err       error
}

func (f *fakeWalletCommands) DebitForEvent(context.Context, uuid.UUID, string, service.DebitInput) (*db.Wallet, bool, error) {
	return &db.Wallet{}, f.duplicate, f.err
}

func (f *fakeWalletCommands) CreditForEvent(context.Context, uuid.UUID, string, service.CreditInput) (*db.Wallet, bool, error) {
	return &db.Wallet{}, f.duplicate, f.err
}

type fakeEventPublisher struct {
	topic string
	event *models.Event
	err   error
}

func (f *fakeEventPublisher) Publish(_ context.Context, topic string, event *models.Event) error {
	f.topic = topic
	f.event = event
	return f.err
}

func testWalletConsumer(commands walletCommands, publisher eventPublisher) *WalletConsumer {
	return NewWalletConsumer(nil, publisher, commands, logger.New(logger.Config{Level: "disabled", ServiceName: "wallet-consumer-test"}))
}

func TestDuplicateDebitReemitsSuccess(t *testing.T) {
	transferID := uuid.NewString()
	senderID := uuid.NewString()
	receiverID := uuid.NewString()
	event := models.NewEvent(models.TopicTransferCreated, transferID, "test", map[string]interface{}{
		"transfer_id":        transferID,
		"sender_wallet_id":   senderID,
		"receiver_wallet_id": receiverID,
		"amount":             "10.00",
		"currency":           "TRY",
	})
	publisher := &fakeEventPublisher{}
	consumer := testWalletConsumer(&fakeWalletCommands{duplicate: true}, publisher)

	if err := consumer.handleTransferCreated(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if publisher.topic != models.TopicTransferDebitSuccess || publisher.event == nil {
		t.Fatalf("expected duplicate debit to re-emit success, got topic %q", publisher.topic)
	}
	if publisher.event.CorrelationID != transferID {
		t.Fatalf("expected correlation ID %s, got %s", transferID, publisher.event.CorrelationID)
	}
}

func TestDuplicateCreditReemitsSuccess(t *testing.T) {
	transferID := uuid.NewString()
	event := models.NewEvent(models.TopicTransferDebitSuccess, transferID, "test", map[string]interface{}{
		"transfer_id":        transferID,
		"wallet_id":          uuid.NewString(),
		"sender_wallet_id":   uuid.NewString(),
		"receiver_wallet_id": uuid.NewString(),
		"amount":             "10.00",
	})
	publisher := &fakeEventPublisher{}
	consumer := testWalletConsumer(&fakeWalletCommands{duplicate: true}, publisher)

	if err := consumer.handleDebitSuccess(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if publisher.topic != models.TopicTransferCreditSuccess {
		t.Fatalf("expected duplicate credit to re-emit success, got topic %q", publisher.topic)
	}
}
