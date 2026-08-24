// Package kafka provides Kafka producer and consumer wrappers.
package kafka

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"math/rand/v2"
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
	reader   messageReader
	producer eventPublisher // For retry and DLQ publishing
	log      *logger.Logger
	cfg      ConsumerConfig
}

type messageReader interface {
	FetchMessage(context.Context) (kafka.Message, error)
	CommitMessages(context.Context, ...kafka.Message) error
	Close() error
}

type eventPublisher interface {
	Publish(context.Context, string, *models.Event) error
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
	RetryJitter    float64         // Fractional jitter applied to delayed retries
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
	if cfg.RetryJitter <= 0 {
		cfg.RetryJitter = 0.2
	}
	if cfg.RetryJitter > 1 {
		cfg.RetryJitter = 1
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

			c.observeLag(msg.Topic)
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
		if err := c.sendToDLQ(ctx, msg, nil, "invalid_event", err); err != nil {
			return
		}
		_ = c.commit(ctx, msg, c.log)
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
			if err := c.sendToDLQ(ctx, msg, &event, "handler_error", err); err != nil {
				return
			}
		} else {
			delay := c.retryDelay(event.Metadata.RetryCount)
			logCtx.WithField("delay", delay.String()).Info().
				Int("retry_count", event.Metadata.RetryCount).
				Msg("Retrying event")
			if !waitForRetry(ctx, delay) {
				return
			}
			if err := c.producer.Publish(ctx, msg.Topic, &event); err != nil {
				logCtx.WithError(err).Error().Msg("Failed to persist retry event")
				return
			}
		}
	} else {
		logCtx.WithDuration(duration).Info().Msg("Event processed successfully")
	}

	// Commit the message
	_ = c.commit(ctx, msg, logCtx)
}

// sendToDLQ publishes a failed event to the dead letter queue.
func (c *Consumer) sendToDLQ(ctx context.Context, msg kafka.Message, event *models.Event, failureClass string, processErr error) error {
	headers := make(map[string]string, len(msg.Headers))
	for _, header := range msg.Headers {
		headers[header.Key] = string(header.Value)
	}
	dlqPayload := models.DLQPayload{
		OriginalTopic:     msg.Topic,
		OriginalEvent:     event,
		OriginalValue:     msg.Value,
		OriginalPartition: msg.Partition,
		OriginalOffset:    msg.Offset,
		Headers:           headers,
		FailureClass:      failureClass,
		Error:             processErr.Error(),
		FailedAt:          time.Now().UTC(),
		RetryCount:        0,
	}

	if event != nil {
		dlqPayload.RetryCount = event.Metadata.RetryCount
	}

	payloadBytes, err := json.Marshal(dlqPayload)
	if err != nil {
		return err
	}
	payload := make(map[string]interface{})
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return err
	}

	dlqEvent := models.NewEvent("dlq.entry", "", "kafka-consumer", payload)

	if err := c.producer.Publish(ctx, models.TopicTransferDLQ, dlqEvent); err != nil {
		c.log.WithError(err).Error().Msg("Failed to send to DLQ")
		return err
	}
	return nil
}

func (c *Consumer) retryDelay(retryCount int) time.Duration {
	if retryCount <= 0 || len(c.cfg.RetryIntervals) == 0 {
		return 0
	}
	index := retryCount - 1
	var base time.Duration
	if index < len(c.cfg.RetryIntervals) {
		base = c.cfg.RetryIntervals[index]
	} else {
		base = c.cfg.RetryIntervals[len(c.cfg.RetryIntervals)-1]
	}
	if base <= 0 || c.cfg.RetryJitter <= 0 {
		return base
	}
	factor := 1 + ((rand.Float64()*2)-1)*c.cfg.RetryJitter
	return time.Duration(float64(base) * factor)
}

func waitForRetry(ctx context.Context, delay time.Duration) bool {
	if delay == 0 {
		return true
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

type commitLogger interface {
	WithError(error) *logger.Logger
}

func (c *Consumer) commit(ctx context.Context, msg kafka.Message, logCtx commitLogger) error {
	if err := c.reader.CommitMessages(ctx, msg); err != nil {
		logCtx.WithError(err).Error().Msg("Failed to commit message")
		return err
	}
	return nil
}

// Close closes the consumer.
func (c *Consumer) Close() error {
	return c.reader.Close()
}
