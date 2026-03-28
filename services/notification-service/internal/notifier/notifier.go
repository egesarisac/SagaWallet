// Package notifier provides mock notification delivery.
package notifier

import (
	"github.com/egesarisac/SagaWallet/pkg/logger"
)

// Notifier sends notifications (mock implementation).
type Notifier struct {
	log *logger.Logger
}

// NewNotifier creates a new notifier.
func NewNotifier(log *logger.Logger) *Notifier {
	return &Notifier{log: log}
}

// NotifyTransferCompleted sends a notification for a completed transfer.
func (n *Notifier) NotifyTransferCompleted(transferID, senderWalletID, receiverWalletID, amount string) {
	n.log.Info().
		Str("transfer_id", transferID).
		Str("sender", senderWalletID).
		Str("receiver", receiverWalletID).
		Str("amount", amount).
		Msg("📧 NOTIFICATION: Transfer completed successfully")

	// In production: Send email, push notification, SMS, etc.
}

// NotifyTransferFailed sends a notification for a failed transfer.
func (n *Notifier) NotifyTransferFailed(transferID, senderWalletID, reason string) {
	n.log.Info().
		Str("transfer_id", transferID).
		Str("sender", senderWalletID).
		Str("reason", reason).
		Msg("📧 NOTIFICATION: Transfer failed")

	// In production: Send email, push notification, SMS, etc.
}
