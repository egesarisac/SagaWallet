// Command eventreplay safely republishes the original event from a DLQ envelope.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/egesarisac/SagaWallet/pkg/kafka"
	"github.com/egesarisac/SagaWallet/pkg/logger"
	"github.com/egesarisac/SagaWallet/pkg/models"
)

func main() {
	file := flag.String("file", "", "DLQ event JSON file")
	topic := flag.String("topic", "", "target topic; defaults to the original topic")
	brokers := flag.String("brokers", os.Getenv("KAFKA_BROKERS"), "comma-separated Kafka brokers")
	flag.Parse()
	if *file == "" || *brokers == "" {
		fmt.Fprintln(os.Stderr, "-file and -brokers (or KAFKA_BROKERS) are required")
		os.Exit(2)
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
	if deadLetter.OriginalEvent == nil || deadLetter.OriginalEvent.EventID == "" {
		fatal(fmt.Errorf("DLQ entry does not contain a replayable original event"))
	}
	target := *topic
	if target == "" {
		target = deadLetter.OriginalTopic
	}
	if target == "" {
		fatal(fmt.Errorf("DLQ entry does not contain an original topic"))
	}

	log := logger.New(logger.Config{Level: "info", Format: "console", ServiceName: "eventreplay"})
	producer := kafka.NewProducer(kafka.ProducerConfig{
		Brokers:  strings.Split(*brokers, ","),
		Username: os.Getenv("KAFKA_USERNAME"),
		Password: os.Getenv("KAFKA_PASSWORD"),
		TLS:      os.Getenv("KAFKA_TLS") == "true",
	}, log)
	defer producer.Close()

	if err := producer.Publish(context.Background(), target, deadLetter.OriginalEvent); err != nil {
		fatal(err)
	}
	fmt.Printf("replayed event %s to %s\n", deadLetter.OriginalEvent.EventID, target)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
