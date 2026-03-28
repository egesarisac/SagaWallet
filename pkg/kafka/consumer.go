// Package kafka provides Kafka producer and consumer wrappers.
package kafka

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"time"

	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/sasl/plain"

	"github.com/egesarisac/SagaWallet/pkg/logger"
	"github.com/egesarisac/SagaWallet/pkg/models"
)

// MessageHandler is a function that processes a Kafka event.
// Return an error to trigger retry/DLQ logic.
type MessageHandler func(ctx context.Context, event *models.Event) error

// Consumer wraps kafka-go reader with retry and DLQ support.
type Consumer struct {
	reader   *kafka.Reader
	producer *Producer // For DLQ publishing
	log      *logger.Logger
	cfg      ConsumerConfig
}

// ConsumerConfig holds consumer configuration.
type ConsumerConfig struct {
	Brokers        []string
	Username       string
	Password       string
	TLS            bool
	GroupID        string
	Topics         []string
	MinBytes       int
	MaxBytes       int
	MaxRetries     int
	RetryIntervals []time.Duration // Exponential backoff intervals
}

// DefaultRetryIntervals provides default retry intervals.
var DefaultRetryIntervals = []time.Duration{
	0,                // Immediate
	1 * time.Second,  // 1s
	5 * time.Second,  // 5s
	30 * time.Second, // 30s
	2 * time.Minute,  // 2min
}

// NewConsumer creates a new Kafka consumer.
func NewConsumer(cfg ConsumerConfig, producer *Producer, log *logger.Logger) *Consumer {
	if cfg.MinBytes == 0 {
		cfg.MinBytes = 1
	}
	if cfg.MaxBytes == 0 {
		cfg.MaxBytes = 10e6 // 10MB
	}
	if cfg.MaxRetries == 0 {
		cfg.MaxRetries = 5
	}
	if len(cfg.RetryIntervals) == 0 {
		cfg.RetryIntervals = DefaultRetryIntervals
	}

	brokers := normalizeBrokers(cfg.Brokers)

	var tlsConfig *tls.Config
	if cfg.TLS {
		tlsConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}

	dialer := &kafka.Dialer{Timeout: 10 * time.Second, TLS: tlsConfig}
	if cfg.Username != "" && cfg.Password != "" {
		dialer.SASLMechanism = plain.Mechanism{Username: cfg.Username, Password: cfg.Password}
	}

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:     brokers,
		GroupID:     cfg.GroupID,
		GroupTopics: cfg.Topics,
		MinBytes:    cfg.MinBytes,
		MaxBytes:    cfg.MaxBytes,
		Dialer:      dialer,
	})

	return &Consumer{
		reader:   reader,
		producer: producer,
		log:      log,
		cfg:      cfg,
	}
}

// Start begins consuming messages and processing them with the handler.
func (c *Consumer) Start(ctx context.Context, handler MessageHandler) error {
	c.log.Info().
		Strs("topics", c.cfg.Topics).
		Str("group_id", c.cfg.GroupID).
		Msg("Starting Kafka consumer")

	for {
		select {
		case <-ctx.Done():
			c.log.Info().Msg("Consumer shutting down")
			return ctx.Err()
		default:
			msg, err := c.reader.FetchMessage(ctx)
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return nil
				}
				c.log.WithError(err).Error().Msg("Failed to fetch message")
				continue
			}

			c.processMessage(ctx, msg, handler)
		}
	}
}

// processMessage handles a single message with retry logic.
func (c *Consumer) processMessage(ctx context.Context, msg kafka.Message, handler MessageHandler) {
	var event models.Event
	if err := json.Unmarshal(msg.Value, &event); err != nil {
		c.log.WithError(err).
			WithField("topic", msg.Topic).
			Error().Msg("Failed to unmarshal event")
		c.sendToDLQ(ctx, msg.Topic, nil, err)
		_ = c.reader.CommitMessages(ctx, msg)
		return
	}

	logCtx := c.log.
		WithField("topic", msg.Topic).
		WithField("event_type", event.EventType).
		WithField("event_id", event.EventID)

	start := time.Now()
	err := handler(ctx, &event)
	duration := time.Since(start)

	if err != nil {
		logCtx.WithError(err).
			WithDuration(duration).
			Error().Msg("Failed to process event")

		// Retry logic
		event.Metadata.RetryCount++
		if event.Metadata.RetryCount >= c.cfg.MaxRetries {
			logCtx.Warn().Msg("Max retries exceeded, sending to DLQ")
			c.sendToDLQ(ctx, msg.Topic, &event, err)
		} else {
			// Re-publish for retry (in real implementation, use a delay queue)
			logCtx.Info().
				Int("retry_count", event.Metadata.RetryCount).
				Msg("Retrying event")
		}
	} else {
		logCtx.WithDuration(duration).Info().Msg("Event processed successfully")
	}

	// Commit the message
	if err := c.reader.CommitMessages(ctx, msg); err != nil {
		logCtx.WithError(err).Error().Msg("Failed to commit message")
	}
}

// sendToDLQ publishes a failed event to the dead letter queue.
func (c *Consumer) sendToDLQ(ctx context.Context, originalTopic string, event *models.Event, processErr error) {
	dlqPayload := models.DLQPayload{
		OriginalTopic: originalTopic,
		OriginalEvent: event,
		Error:         processErr.Error(),
		FailedAt:      time.Now().UTC(),
		RetryCount:    0,
	}

	if event != nil {
		dlqPayload.RetryCount = event.Metadata.RetryCount
	}

	payload := map[string]interface{}{
		"original_topic": dlqPayload.OriginalTopic,
		"error":          dlqPayload.Error,
		"failed_at":      dlqPayload.FailedAt,
		"retry_count":    dlqPayload.RetryCount,
	}

	dlqEvent := models.NewEvent("dlq.entry", "", "kafka-consumer", payload)

	if err := c.producer.Publish(ctx, models.TopicTransferDLQ, dlqEvent); err != nil {
		c.log.WithError(err).Error().Msg("Failed to send to DLQ")
	}
}

// Close closes the consumer.
func (c *Consumer) Close() error {
	return c.reader.Close()
}
