// Package service provides DLQ (Dead Letter Queue) processing for the transaction service.
package service

import (
	"context"
	"encoding/json"
	"time"

	"github.com/segmentio/kafka-go"

	"github.com/egesarisac/SagaWallet/pkg/logger"
)

// DLQWorker processes messages from the dead letter queue Kafka topic.
// Messages land here when consumers fail to process them after all retries.
type DLQWorker struct {
	reader *kafka.Reader
	log    *logger.Logger
}

// NewDLQWorker creates a new DLQ processor reading from the transfer.dlq topic.
func NewDLQWorker(brokers []string, groupID string, log *logger.Logger) *DLQWorker {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        brokers,
		GroupID:        groupID + "-dlq",
		Topic:          "transfer.dlq",
		MinBytes:       10e3,
		MaxBytes:       10e6,
		CommitInterval: time.Second,
	})

	return &DLQWorker{reader: reader, log: log}
}

// Run starts the DLQ consumer loop. It blocks until ctx is cancelled.
func (w *DLQWorker) Run(ctx context.Context) {
	w.log.Info().Msg("Starting DLQ worker...")

	for {
		msg, err := w.reader.ReadMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				// Context cancelled — clean shutdown.
				break
			}
			w.log.WithError(err).Error().Msg("DLQ: error reading message")
			continue
		}

		w.handleDeadLetter(msg)
	}

	if err := w.reader.Close(); err != nil {
		w.log.WithError(err).Error().Msg("DLQ: error closing reader")
	}
	w.log.Info().Msg("DLQ worker stopped")
}

// handleDeadLetter logs the dead letter event for operator inspection.
func (w *DLQWorker) handleDeadLetter(msg kafka.Message) {
	// Attempt to pretty-print as JSON for easier debugging.
	var payload map[string]interface{}
	if err := json.Unmarshal(msg.Value, &payload); err != nil {
		// Not valid JSON — log raw bytes.
		w.log.Error().
			Str("topic", msg.Topic).
			Int("partition", msg.Partition).
			Int64("offset", msg.Offset).
			Bytes("raw_payload", msg.Value).
			Msg("💀 DLQ: dead-lettered message (non-JSON)")
		return
	}

	w.log.Error().
		Str("topic", msg.Topic).
		Int("partition", msg.Partition).
		Int64("offset", msg.Offset).
		Interface("payload", payload).
		Msg("💀 DLQ: dead-lettered message — requires manual review")
}
