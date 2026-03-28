package notifier

import (
	"testing"

	"github.com/egesarisac/SagaWallet/pkg/logger"
	"github.com/stretchr/testify/assert"
)

func TestNotifier_NotifyTransferCompleted(t *testing.T) {
	// Initialize a logger
	l := logger.New(logger.Config{
		Level:       "debug",
		Format:      "console",
		ServiceName: "test-notifier",
	})

	n := NewNotifier(l)

	// Since Notifier is currently a mock that just logs the output,
	// we simply invoke the function to ensure it formats and executes without panicking.
	assert.NotPanics(t, func() {
		n.NotifyTransferCompleted(
			"transfer-123",
			"sender-uuid",
			"receiver-uuid",
			"150.00",
		)
	}, "NotifyTransferCompleted should not panic")
}

func TestNotifier_NotifyTransferFailed(t *testing.T) {
	l := logger.New(logger.Config{
		Level:       "debug",
		Format:      "console",
		ServiceName: "test-notifier",
	})

	n := NewNotifier(l)

	assert.NotPanics(t, func() {
		n.NotifyTransferFailed(
			"transfer-456",
			"sender-uuid",
			"INSUFFICIENT_FUNDS",
		)
	}, "NotifyTransferFailed should not panic")
}
