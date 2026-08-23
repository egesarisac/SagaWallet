// Command eventreplay safely republishes the original event from a DLQ envelope.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/egesarisac/SagaWallet/pkg/kafka"
	"github.com/egesarisac/SagaWallet/pkg/logger"
	"github.com/egesarisac/SagaWallet/pkg/models"
)

func main() {
	file := flag.String("file", "", "DLQ event JSON file")
	topic := flag.String("topic", "", "target topic; defaults to the original topic")
	allowTopicOverride := flag.Bool("allow-topic-override", false, "allow replaying to a topic other than the recorded source topic")
	brokers := flag.String("brokers", os.Getenv("KAFKA_BROKERS"), "comma-separated Kafka brokers")
	actor := flag.String("actor", defaultActor(), "operator identity written to the replay audit")
	reason := flag.String("reason", "", "operator reason for replaying the dead letter")
	auditFile := flag.String("audit-file", "event-replay-audit.jsonl", "append-only JSONL replay audit file")
	flag.Parse()
	if *file == "" || *brokers == "" {
		fmt.Fprintln(os.Stderr, "-file and -brokers (or KAFKA_BROKERS) are required")
		os.Exit(2)
	}
	if strings.TrimSpace(*reason) == "" || strings.TrimSpace(*actor) == "" {
		fatal(fmt.Errorf("-reason and -actor are required"))
	}

	data, err := os.ReadFile(*file)
	if err != nil {
		fatal(err)
	}
	var envelope models.Event
	if err := json.Unmarshal(data, &envelope); err != nil {
		fatal(fmt.Errorf("decode DLQ envelope: %w", err))
	}
	payload, err := json.Marshal(envelope.Payload)
	if err != nil {
		fatal(err)
	}
	var deadLetter models.DLQPayload
	if err := json.Unmarshal(payload, &deadLetter); err != nil {
		fatal(fmt.Errorf("decode DLQ payload: %w", err))
	}
	if err := validateDeadLetter(&envelope, &deadLetter); err != nil {
		fatal(err)
	}
	target := *topic
	if target == "" {
		target = deadLetter.OriginalTopic
	}
	if target != deadLetter.OriginalTopic && !*allowTopicOverride {
		fatal(fmt.Errorf("target topic differs from the recorded source; pass -allow-topic-override to confirm"))
	}
	if target == models.TopicTransferDLQ {
		fatal(fmt.Errorf("refusing to replay an event back to the DLQ topic"))
	}

	replayID := uuid.NewString()
	audit := replayAuditRecord{
		ReplayID:          replayID,
		DLQEventID:        envelope.EventID,
		OriginalEventID:   deadLetter.OriginalEvent.EventID,
		OriginalTopic:     deadLetter.OriginalTopic,
		OriginalPartition: deadLetter.OriginalPartition,
		OriginalOffset:    deadLetter.OriginalOffset,
		TargetTopic:       target,
		Actor:             strings.TrimSpace(*actor),
		Reason:            strings.TrimSpace(*reason),
		InputSHA256:       fmt.Sprintf("%x", sha256.Sum256(data)),
		RecordedAt:        time.Now().UTC(),
		Status:            "requested",
	}
	if err := appendAudit(*auditFile, audit); err != nil {
		fatal(fmt.Errorf("record replay request: %w", err))
	}

	log := logger.New(logger.Config{Level: "info", Format: "console", ServiceName: "eventreplay"})
	producer := kafka.NewProducer(kafka.ProducerConfig{
		Brokers:  strings.Split(*brokers, ","),
		Username: os.Getenv("KAFKA_USERNAME"),
		Password: os.Getenv("KAFKA_PASSWORD"),
		TLS:      os.Getenv("KAFKA_TLS") == "true",
	}, log)
	defer func() {
		if err := producer.Close(); err != nil {
			log.WithError(err).Error().Msg("Failed to close Kafka producer")
		}
	}()

	if err := producer.Publish(context.Background(), target, deadLetter.OriginalEvent); err != nil {
		fatal(err)
	}
	audit.Status = "published"
	audit.RecordedAt = time.Now().UTC()
	if err := appendAudit(*auditFile, audit); err != nil {
		fatal(fmt.Errorf("record successful replay: %w", err))
	}
	fmt.Printf("replayed event %s to %s (audit %s)\n", deadLetter.OriginalEvent.EventID, target, replayID)
}

type replayAuditRecord struct {
	ReplayID          string    `json:"replay_id"`
	DLQEventID        string    `json:"dlq_event_id"`
	OriginalEventID   string    `json:"original_event_id"`
	OriginalTopic     string    `json:"original_topic"`
	OriginalPartition int       `json:"original_partition"`
	OriginalOffset    int64     `json:"original_offset"`
	TargetTopic       string    `json:"target_topic"`
	Actor             string    `json:"actor"`
	Reason            string    `json:"reason"`
	InputSHA256       string    `json:"input_sha256"`
	Status            string    `json:"status"`
	RecordedAt        time.Time `json:"recorded_at"`
}

func validateDeadLetter(envelope *models.Event, deadLetter *models.DLQPayload) error {
	if envelope.EventType != "dlq.entry" {
		return fmt.Errorf("input event is %q, not a DLQ entry", envelope.EventType)
	}
	if deadLetter.OriginalEvent == nil {
		return fmt.Errorf("DLQ entry does not contain a replayable original event")
	}
	if _, err := uuid.Parse(deadLetter.OriginalEvent.EventID); err != nil {
		return fmt.Errorf("original event ID is invalid: %w", err)
	}
	if strings.TrimSpace(deadLetter.OriginalEvent.EventType) == "" || strings.TrimSpace(deadLetter.OriginalEvent.CorrelationID) == "" {
		return fmt.Errorf("original event is missing its type or correlation ID")
	}
	if strings.TrimSpace(deadLetter.OriginalTopic) == "" {
		return fmt.Errorf("DLQ entry does not contain an original topic")
	}
	return nil
}

func appendAudit(path string, record replayAuditRecord) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	if err := json.NewEncoder(file).Encode(record); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func defaultActor() string {
	if actor := strings.TrimSpace(os.Getenv("USER")); actor != "" {
		return actor
	}
	return "operator"
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
