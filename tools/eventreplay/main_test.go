package main

import (
	"bufio"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egesarisac/SagaWallet/pkg/models"
)

func TestValidateDeadLetter(t *testing.T) {
	original := models.NewEvent(models.TopicTransferCreated, "transfer-1", "test", map[string]interface{}{})
	envelope := models.NewEvent("dlq.entry", "", "test", map[string]interface{}{})
	deadLetter := &models.DLQPayload{OriginalTopic: models.TopicTransferCreated, OriginalEvent: original}

	if err := validateDeadLetter(envelope, deadLetter); err != nil {
		t.Fatalf("valid dead letter rejected: %v", err)
	}

	envelope.EventType = models.TopicTransferCreated
	if err := validateDeadLetter(envelope, deadLetter); err == nil {
		t.Fatal("expected non-DLQ envelope to be rejected")
	}
}

func TestAppendAuditIsAppendOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	record := replayAuditRecord{
		ReplayID:        "replay-1",
		OriginalEventID: "event-1",
		Status:          "requested",
		RecordedAt:      time.Now().UTC(),
	}
	if err := appendAudit(path, record); err != nil {
		t.Fatal(err)
	}
	record.Status = "published"
	if err := appendAudit(path, record); err != nil {
		t.Fatal(err)
	}

	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	scanner := bufio.NewScanner(file)
	lines := 0
	for scanner.Scan() {
		lines++
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if lines != 2 {
		t.Fatalf("expected two audit records, got %d", lines)
	}
}
