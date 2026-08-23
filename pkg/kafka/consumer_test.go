package kafka

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	kafkago "github.com/segmentio/kafka-go"

	"github.com/egesarisac/SagaWallet/pkg/logger"
	"github.com/egesarisac/SagaWallet/pkg/models"
)

type recordedPublish struct {
	topic string
	event *models.Event
}

type fakePublisher struct {
	publishes []recordedPublish
	err       error
}

func (p *fakePublisher) Publish(_ context.Context, topic string, event *models.Event) error {
	if p.err != nil {
		return p.err
	}
	p.publishes = append(p.publishes, recordedPublish{topic: topic, event: event})
	return nil
}

type fakeReader struct {
	commits []kafkago.Message
}

func (r *fakeReader) FetchMessage(context.Context) (kafkago.Message, error) {
	return kafkago.Message{}, context.Canceled
}

func (r *fakeReader) CommitMessages(_ context.Context, messages ...kafkago.Message) error {
	r.commits = append(r.commits, messages...)
	return nil
}

func (r *fakeReader) Close() error { return nil }

func newTestConsumer(reader *fakeReader, publisher *fakePublisher) *Consumer {
	return &Consumer{
		reader:   reader,
		producer: publisher,
		log: logger.New(logger.Config{
			Level:       "disabled",
			ServiceName: "kafka-consumer-test",
		}),
		cfg: ConsumerConfig{
			MaxRetries:     3,
			RetryIntervals: []time.Duration{0, time.Millisecond, 2 * time.Millisecond},
		},
	}
}

func testMessage(t *testing.T) kafkago.Message {
	t.Helper()
	event := models.NewEvent("transfer.created", "transfer-1", "test", map[string]interface{}{"amount": "10.00"})
	value, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	return kafkago.Message{
		Topic:     models.TopicTransferCreated,
		Partition: 2,
		Offset:    42,
		Headers:   []kafkago.Header{{Key: "traceparent", Value: []byte("00-trace")}},
		Value:     value,
	}
}

func TestConsumerRetriesOnlyAfterDurablePublish(t *testing.T) {
	reader := &fakeReader{}
	publisher := &fakePublisher{}
	consumer := newTestConsumer(reader, publisher)

	consumer.processMessage(context.Background(), testMessage(t), func(context.Context, *models.Event) error {
		return errors.New("temporary failure")
	})

	if len(publisher.publishes) != 1 || publisher.publishes[0].topic != models.TopicTransferCreated {
		t.Fatalf("expected one durable retry on %q, got %#v", models.TopicTransferCreated, publisher.publishes)
	}
	if publisher.publishes[0].event.Metadata.RetryCount != 1 {
		t.Fatalf("expected retry count 1, got %d", publisher.publishes[0].event.Metadata.RetryCount)
	}
	if len(reader.commits) != 1 {
		t.Fatalf("expected source message commit after retry persistence, got %d commits", len(reader.commits))
	}
}

func TestConsumerDoesNotAcknowledgeWhenRetryPublishFails(t *testing.T) {
	reader := &fakeReader{}
	consumer := newTestConsumer(reader, &fakePublisher{err: errors.New("broker unavailable")})

	consumer.processMessage(context.Background(), testMessage(t), func(context.Context, *models.Event) error {
		return errors.New("temporary failure")
	})

	if len(reader.commits) != 0 {
		t.Fatalf("expected no commit after failed retry publish, got %d", len(reader.commits))
	}
}

func TestConsumerWritesCompleteDLQBeforeAcknowledging(t *testing.T) {
	reader := &fakeReader{}
	publisher := &fakePublisher{}
	consumer := newTestConsumer(reader, publisher)
	consumer.cfg.MaxRetries = 1
	message := testMessage(t)

	consumer.processMessage(context.Background(), message, func(context.Context, *models.Event) error {
		return errors.New("permanent failure")
	})

	if len(publisher.publishes) != 1 || publisher.publishes[0].topic != models.TopicTransferDLQ {
		t.Fatalf("expected one DLQ publish, got %#v", publisher.publishes)
	}
	payloadBytes, err := json.Marshal(publisher.publishes[0].event.Payload)
	if err != nil {
		t.Fatal(err)
	}
	var payload models.DLQPayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.OriginalEvent == nil || payload.OriginalEvent.EventID == "" {
		t.Fatal("expected replayable original event in DLQ payload")
	}
	if payload.OriginalTopic != message.Topic || payload.OriginalPartition != message.Partition || payload.OriginalOffset != message.Offset {
		t.Fatalf("expected source Kafka location in DLQ payload, got %#v", payload)
	}
	if payload.Headers["traceparent"] != "00-trace" || payload.FailureClass != "handler_error" || payload.RetryCount != 1 {
		t.Fatalf("expected failure metadata in DLQ payload, got %#v", payload)
	}
	if len(reader.commits) != 1 {
		t.Fatalf("expected source message commit after DLQ persistence, got %d", len(reader.commits))
	}
}

func TestConsumerRetryDelayUsesFirstConfiguredInterval(t *testing.T) {
	consumer := newTestConsumer(&fakeReader{}, &fakePublisher{})

	if got := consumer.retryDelay(1); got != 0 {
		t.Fatalf("expected first retry delay to be immediate, got %s", got)
	}
	if got := consumer.retryDelay(2); got != time.Millisecond {
		t.Fatalf("expected second retry delay to be 1ms, got %s", got)
	}
	if got := consumer.retryDelay(10); got != 2*time.Millisecond {
		t.Fatalf("expected retry delay to cap at final interval, got %s", got)
	}
}

func TestConsumerRetryDelayAppliesBoundedJitter(t *testing.T) {
	consumer := newTestConsumer(&fakeReader{}, &fakePublisher{})
	consumer.cfg.RetryIntervals = []time.Duration{100 * time.Millisecond}
	consumer.cfg.RetryJitter = 0.2

	for i := 0; i < 100; i++ {
		delay := consumer.retryDelay(1)
		if delay < 80*time.Millisecond || delay > 120*time.Millisecond {
			t.Fatalf("jittered delay %s is outside expected bounds", delay)
		}
	}
}
