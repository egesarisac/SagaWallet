// Package models provides shared domain models and Kafka event structures.
package models

import (
	"time"

	"github.com/google/uuid"
)

// TransferStatus represents the current state of a transfer.
type TransferStatus string

const (
	TransferStatusPending      TransferStatus = "PENDING"
	TransferStatusDebited      TransferStatus = "DEBITED"
	TransferStatusCompleted    TransferStatus = "COMPLETED"
	TransferStatusRefunding    TransferStatus = "REFUNDING"
	TransferStatusFailed       TransferStatus = "FAILED"
	TransferStatusManualReview TransferStatus = "MANUAL_REVIEW"
)

// WalletStatus represents the current state of a wallet.
type WalletStatus string

const (
	WalletStatusActive    WalletStatus = "ACTIVE"
	WalletStatusSuspended WalletStatus = "SUSPENDED"
	WalletStatusFrozen    WalletStatus = "FROZEN"
	WalletStatusClosed    WalletStatus = "CLOSED"
)

// TransactionType represents the type of wallet transaction.
type TransactionType string

const (
	TransactionTypeDebit  TransactionType = "DEBIT"
	TransactionTypeCredit TransactionType = "CREDIT"
)

// Event represents a Kafka event envelope.
type Event struct {
	EventID       string                 `json:"event_id"`
	EventType     string                 `json:"event_type"`
	Timestamp     time.Time              `json:"timestamp"`
	Version       string                 `json:"version"`
	CorrelationID string                 `json:"correlation_id"`
	Payload       map[string]interface{} `json:"payload"`
	Metadata      EventMetadata          `json:"metadata"`
}

// EventMetadata contains event metadata.
type EventMetadata struct {
	Source     string `json:"source"`
	RetryCount int    `json:"retry_count"`
}

// NewEvent creates a new event with generated ID and timestamp.
func NewEvent(eventType string, correlationID string, source string, payload map[string]interface{}) *Event {
	return &Event{
		EventID:       uuid.New().String(),
		EventType:     eventType,
		Timestamp:     time.Now().UTC(),
		Version:       "1.0",
		CorrelationID: correlationID,
		Payload:       payload,
		Metadata: EventMetadata{
			Source:     source,
			RetryCount: 0,
		},
	}
}

// Kafka topic names as constants.
const (
	TopicTransferCreated       = "transfer.created"
	TopicTransferDebitSuccess  = "transfer.debit.success"
	TopicTransferDebitFailed   = "transfer.debit.failed"
	TopicTransferCreditSuccess = "transfer.credit.success"
	TopicTransferCreditFailed  = "transfer.credit.failed"
	TopicTransferRefundSuccess = "transfer.refund.success"
	TopicTransferCompleted     = "transfer.completed"
	TopicTransferFailed        = "transfer.failed"
	TopicTransferDLQ           = "transfer.dlq"
)

// TransferCreatedPayload is the payload for transfer created events (triggers debit).
type TransferCreatedPayload struct {
	TransferID       string `json:"transfer_id"`
	SenderWalletID   string `json:"sender_wallet_id"`
	ReceiverWalletID string `json:"receiver_wallet_id"`
	Amount           string `json:"amount"`
	Currency         string `json:"currency"`
}

// DebitResultPayload is the payload for debit success/failed events (failure triggers end/compensation).
type DebitResultPayload struct {
	TransferID       string `json:"transfer_id"`
	WalletID         string `json:"wallet_id"`
	SenderWalletID   string `json:"sender_wallet_id,omitempty"`
	ReceiverWalletID string `json:"receiver_wallet_id,omitempty"`
	Amount           string `json:"amount,omitempty"`
	Reason           string `json:"reason,omitempty"` // Only for failed
}

// CreditResultPayload is the payload for credit success/failed events (failure triggers refund).
type CreditResultPayload struct {
	TransferID     string `json:"transfer_id"`
	WalletID       string `json:"wallet_id"`
	SenderWalletID string `json:"sender_wallet_id,omitempty"` // Needed for refund
	Amount         string `json:"amount,omitempty"`           // Needed for refund
	Reason         string `json:"reason,omitempty"`           // Only for failed
}

// RefundResultPayload is the payload for refund success events.
type RefundResultPayload struct {
	TransferID string `json:"transfer_id"`
	WalletID   string `json:"wallet_id"`
}

// TransferCompletedPayload is the payload for transfer completed events.
type TransferCompletedPayload struct {
	TransferID       string `json:"transfer_id"`
	SenderWalletID   string `json:"sender_wallet_id"`
	ReceiverWalletID string `json:"receiver_wallet_id"`
	Amount           string `json:"amount"`
	Currency         string `json:"currency"`
}

// TransferFailedPayload is the payload for transfer failed events.
type TransferFailedPayload struct {
	TransferID     string `json:"transfer_id"`
	SenderWalletID string `json:"sender_wallet_id"`
	Reason         string `json:"reason"`
}

// DLQPayload wraps a failed event for the dead letter queue.
type DLQPayload struct {
	OriginalTopic string    `json:"original_topic"`
	OriginalEvent *Event    `json:"original_event"`
	Error         string    `json:"error"`
	FailedAt      time.Time `json:"failed_at"`
	RetryCount    int       `json:"retry_count"`
}
