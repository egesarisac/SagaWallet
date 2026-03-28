// Package kafka provides Kafka producer and consumer wrappers.
package kafka

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"strings"
	"time"

	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/sasl/plain"

	"github.com/egesarisac/SagaWallet/pkg/logger"
	"github.com/egesarisac/SagaWallet/pkg/models"
)

// Producer wraps kafka-go writer with retry logic.
type Producer struct {
	writer *kafka.Writer
	log    *logger.Logger
}

// ProducerConfig holds producer configuration.
type ProducerConfig struct {
	Brokers      []string
	Username     string
	Password     string
	TLS          bool
	BatchSize    int
	BatchTimeout time.Duration
}

func normalizeBrokers(brokers []string) []string {
	normalized := make([]string, 0, len(brokers))
	for _, b := range brokers {
		b = strings.TrimSpace(b)
		b = strings.TrimPrefix(b, "SASL_SSL://")
		b = strings.TrimPrefix(b, "SSL://")
		b = strings.TrimPrefix(b, "PLAINTEXT://")
		b = strings.TrimPrefix(b, "PLAINTEXT_HOST://")
		if b != "" {
			normalized = append(normalized, b)
		}
	}
	return normalized
}

func buildTransport(username, password string, useTLS bool) *kafka.Transport {
	var tlsConfig *tls.Config
	if useTLS {
		tlsConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}

	tr := &kafka.Transport{TLS: tlsConfig}
	if username != "" && password != "" {
		tr.SASL = plain.Mechanism{Username: username, Password: password}
	}
	return tr
}

// NewProducer creates a new Kafka producer.
func NewProducer(cfg ProducerConfig, log *logger.Logger) *Producer {
	if cfg.BatchSize == 0 {
		cfg.BatchSize = 1
	}
	if cfg.BatchTimeout == 0 {
		cfg.BatchTimeout = 10 * time.Millisecond
	}

	brokers := normalizeBrokers(cfg.Brokers)
	writer := &kafka.Writer{
		Addr:         kafka.TCP(brokers...),
		BatchSize:    cfg.BatchSize,
		BatchTimeout: cfg.BatchTimeout,
		RequiredAcks: kafka.RequireAll,
		Async:        false, // Sync for reliability
		Transport:    buildTransport(cfg.Username, cfg.Password, cfg.TLS),
	}

	return &Producer{
		writer: writer,
		log:    log,
	}
}

// Publish sends an event to the specified topic.
func (p *Producer) Publish(ctx context.Context, topic string, event *models.Event) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}

	msg := kafka.Message{
		Topic: topic,
		Key:   []byte(event.CorrelationID), // Partition by correlation ID
		Value: data,
	}

	start := time.Now()
	err = p.writer.WriteMessages(ctx, msg)
	duration := time.Since(start)

	if err != nil {
		p.log.WithError(err).
			WithField("topic", topic).
			WithField("event_type", event.EventType).
			Error().Msg("Failed to publish event")
		return err
	}

	p.log.WithField("topic", topic).
		WithField("event_type", event.EventType).
		WithField("event_id", event.EventID).
		WithDuration(duration).
		Info().Msg("Event published")

	return nil
}

// PublishWithKey sends an event with a custom partition key.
func (p *Producer) PublishWithKey(ctx context.Context, topic string, key string, event *models.Event) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}

	msg := kafka.Message{
		Topic: topic,
		Key:   []byte(key),
		Value: data,
	}

	return p.writer.WriteMessages(ctx, msg)
}

// Close closes the producer.
func (p *Producer) Close() error {
	return p.writer.Close()
}
