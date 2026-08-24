package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	resilienceKafkaBroker = getEnv("KAFKA_BROKER", "localhost:9092")
	resilienceComposeFile = getEnv("SAGAWALLET_COMPOSE_FILE", "../../docker-compose.full.yml")
	runResilience         = os.Getenv("RUN_RESILIENCE") == "1"
)

const (
	topicTransferCreated       = "transfer.created"
	topicTransferDebitSuccess  = "transfer.debit.success"
	topicTransferCreditSuccess = "transfer.credit.success"
	topicTransferDLQ           = "transfer.dlq"
)

type integrationEvent struct {
	EventID       string                   `json:"event_id"`
	EventType     string                   `json:"event_type"`
	Timestamp     time.Time                `json:"timestamp"`
	Version       string                   `json:"version"`
	CorrelationID string                   `json:"correlation_id"`
	Payload       map[string]interface{}   `json:"payload"`
	Metadata      integrationEventMetadata `json:"metadata"`
}

type integrationEventMetadata struct {
	Source     string `json:"source"`
	RetryCount int    `json:"retry_count"`
}

type deadLetterPayload struct {
	OriginalTopic     string            `json:"original_topic"`
	OriginalEvent     *integrationEvent `json:"original_event"`
	OriginalPartition int               `json:"original_partition"`
	OriginalOffset    int64             `json:"original_offset"`
	Headers           map[string]string `json:"headers"`
	FailureClass      string            `json:"failure_class"`
	Error             string            `json:"error"`
	RetryCount        int               `json:"retry_count"`
}

type deadLetterEnvelope struct {
	EventID   string            `json:"event_id"`
	EventType string            `json:"event_type"`
	Payload   deadLetterPayload `json:"payload"`
}

type replayAuditRecord struct {
	ReplayID        string `json:"replay_id"`
	DLQEventID      string `json:"dlq_event_id"`
	OriginalEventID string `json:"original_event_id"`
	OriginalTopic   string `json:"original_topic"`
	TargetTopic     string `json:"target_topic"`
	Actor           string `json:"actor"`
	Reason          string `json:"reason"`
	Status          string `json:"status"`
}

type walletTransaction struct {
	Type        string `json:"type"`
	ReferenceID string `json:"reference_id"`
}

func requireResilience(t *testing.T) {
	t.Helper()
	requireServices(t)
	if !runResilience {
		t.Skip("set RUN_RESILIENCE=1 with the provisioned Compose stack to run fault-injection tests")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Fatalf("docker is required for resilience tests: %v", err)
	}
	compose(t, "start", "redpanda", "wallet-worker", "transaction-worker")
	waitForKafka(t, 30*time.Second)
}

func composeOutput(timeout time.Duration, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	commandArgs := append([]string{"compose", "-f", resilienceComposeFile}, args...)
	// The arguments are fixed by this integration harness or generated UUID fixtures.
	cmd := exec.CommandContext(ctx, "docker", commandArgs...) //nolint:gosec
	output, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(output)), err
}

func compose(t *testing.T, args ...string) string {
	t.Helper()
	output, err := composeOutput(60*time.Second, args...)
	require.NoErrorf(t, err, "docker compose %s failed:\n%s", strings.Join(args, " "), output)
	return output
}

func restoreService(t *testing.T, service string) {
	t.Helper()
	t.Cleanup(func() {
		if output, err := composeOutput(60*time.Second, "start", service); err != nil {
			t.Logf("failed to restore %s: %v\n%s", service, err, output)
		}
	})
}

func waitForComposeLog(t *testing.T, service string, timeout time.Duration, fragments ...string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		output, err := composeOutput(15*time.Second, "logs", "--no-color", "--since", "2m", service)
		matched := err == nil
		for _, fragment := range fragments {
			matched = matched && strings.Contains(output, fragment)
		}
		if matched {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("%s logs did not contain %q within %s", service, fragments, timeout)
}

func waitForKafka(t *testing.T, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		conn, err := kafka.DialContext(ctx, "tcp", resilienceKafkaBroker)
		cancel()
		if err == nil {
			_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
			_, err = conn.ReadPartitions()
			_ = conn.Close()
		}
		if err == nil {
			return
		}
		lastErr = err
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("Kafka at %s did not become ready within %s: %v", resilienceKafkaBroker, timeout, lastErr)
}

func newIntegrationEvent(eventType, correlationID string, payload map[string]interface{}) integrationEvent {
	return integrationEvent{
		EventID:       uuid.NewString(),
		EventType:     eventType,
		Timestamp:     time.Now().UTC(),
		Version:       "1.0",
		CorrelationID: correlationID,
		Payload:       payload,
		Metadata: integrationEventMetadata{
			Source: "integration-test",
		},
	}
}

func publishIntegrationEvent(t *testing.T, topic string, event integrationEvent) {
	t.Helper()
	value, err := json.Marshal(event)
	require.NoError(t, err)

	writer := &kafka.Writer{
		Addr:         kafka.TCP(resilienceKafkaBroker),
		Balancer:     &kafka.Hash{},
		RequiredAcks: kafka.RequireAll,
		BatchSize:    1,
		WriteTimeout: 10 * time.Second,
	}
	defer func() { _ = writer.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	require.NoError(t, writer.WriteMessages(ctx, kafka.Message{
		Topic: topic,
		Key:   []byte(event.CorrelationID),
		Value: value,
	}))
}

func createResilienceTransfer(t *testing.T, senderWalletID, receiverWalletID, amount string) (string, string) {
	t.Helper()
	response := makeRequest(t, http.MethodPost, transactionServiceURL+"/api/v1/transfers", map[string]interface{}{
		"sender_wallet_id":   senderWalletID,
		"receiver_wallet_id": receiverWalletID,
		"amount":             amount,
		"idempotency_key":    uuid.NewString(),
	})
	defer func() { _ = response.Body.Close() }()
	require.Equal(t, http.StatusAccepted, response.StatusCode)

	var result struct {
		Data struct {
			TransferID string `json:"transfer_id"`
			Status     string `json:"status"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(response.Body).Decode(&result))
	require.NotEmpty(t, result.Data.TransferID)
	return result.Data.TransferID, result.Data.Status
}

func getTransferStatus(t *testing.T, transferID string) string {
	t.Helper()
	response := makeRequest(t, http.MethodGet, transactionServiceURL+"/api/v1/transfers/"+transferID, nil)
	defer func() { _ = response.Body.Close() }()
	require.Equal(t, http.StatusOK, response.StatusCode)

	var result struct {
		Data struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(response.Body).Decode(&result))
	return result.Data.Status
}

func waitForTransferStatusWithin(t *testing.T, transferID, expectedStatus string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	lastStatus := ""
	for time.Now().Before(deadline) {
		lastStatus = getTransferStatus(t, transferID)
		if lastStatus == expectedStatus {
			return
		}
		if lastStatus == "COMPLETED" || lastStatus == "FAILED" || lastStatus == "MANUAL_REVIEW" {
			t.Fatalf("transfer reached terminal status %s; expected %s", lastStatus, expectedStatus)
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("transfer did not reach %s within %s; last status was %s", expectedStatus, timeout, lastStatus)
}

func applyWalletCommand(t *testing.T, walletID, action, amount, referenceID string) {
	t.Helper()
	response := makeRequest(t, http.MethodPost, walletServiceURL+"/api/v1/wallets/"+walletID+"/"+action, map[string]interface{}{
		"amount":       amount,
		"reference_id": referenceID,
		"description":  "P0 resilience test",
	})
	defer func() { _ = response.Body.Close() }()
	require.Equal(t, http.StatusOK, response.StatusCode)
}

func waitForWalletBalance(t *testing.T, walletID, expected string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	lastBalance := ""
	for time.Now().Before(deadline) {
		lastBalance = getWalletBalance(t, walletID)
		if lastBalance == expected {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("wallet %s did not reach balance %s within %s; last balance was %s", walletID, expected, timeout, lastBalance)
}

func getWalletTransactions(t *testing.T, walletID string) []walletTransaction {
	t.Helper()
	response := makeRequest(t, http.MethodGet, walletServiceURL+"/api/v1/wallets/"+walletID+"/transactions", nil)
	defer func() { _ = response.Body.Close() }()
	require.Equal(t, http.StatusOK, response.StatusCode)

	var result struct {
		Data []walletTransaction `json:"data"`
	}
	require.NoError(t, json.NewDecoder(response.Body).Decode(&result))
	return result.Data
}

func ledgerEntryCount(transactions []walletTransaction, referenceID, transactionType string) int {
	count := 0
	for _, transaction := range transactions {
		if transaction.ReferenceID == referenceID && transaction.Type == transactionType {
			count++
		}
	}
	return count
}

func waitForLedgerEntry(t *testing.T, walletID, referenceID, transactionType string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if ledgerEntryCount(getWalletTransactions(t, walletID), referenceID, transactionType) == 1 {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("wallet %s did not record one %s ledger entry for %s", walletID, transactionType, referenceID)
}

func initialOutboxState(t *testing.T, transferID string) string {
	t.Helper()
	require.NoError(t, uuid.Validate(transferID))
	query := fmt.Sprintf(
		"SELECT CASE WHEN published_at IS NULL THEN 'pending' ELSE 'published' END FROM outbox_events WHERE aggregate_id = '%s' AND topic = 'transfer.created'",
		transferID,
	)
	output := compose(t, "exec", "-T", "transaction-db", "psql", "-U", "transaction_user", "-d", "transaction_db", "-v", "ON_ERROR_STOP=1", "-Atc", query)
	lines := strings.Fields(output)
	require.NotEmpty(t, lines)
	state := lines[len(lines)-1]
	require.Contains(t, []string{"pending", "published"}, state)
	return state
}

func suppressInitialOutbox(t *testing.T, transferID string) {
	t.Helper()
	require.NoError(t, uuid.Validate(transferID))
	query := fmt.Sprintf(
		"UPDATE outbox_events SET published_at = NOW(), locked_until = NULL, locked_by = NULL WHERE aggregate_id = '%s' AND topic = 'transfer.created' AND published_at IS NULL",
		transferID,
	)
	assert.Contains(t, compose(t, "exec", "-T", "transaction-db", "psql", "-U", "transaction_user", "-d", "transaction_db", "-v", "ON_ERROR_STOP=1", "-Atc", query), "UPDATE 1")
}

func transitionEventID(t *testing.T, transferID, sourceTopic string) string {
	t.Helper()
	require.NoError(t, uuid.Validate(transferID))
	require.Contains(t, []string{topicTransferDebitSuccess, topicTransferCreditSuccess}, sourceTopic)
	query := fmt.Sprintf(
		"SELECT payload->>'event_id' FROM saga_events WHERE transfer_id = '%s' AND event_type = 'STATUS_TRANSITION' AND payload->>'source_topic' = '%s' ORDER BY created_at LIMIT 1",
		transferID,
		sourceTopic,
	)
	output := compose(t, "exec", "-T", "transaction-db", "psql", "-U", "transaction_user", "-d", "transaction_db", "-v", "ON_ERROR_STOP=1", "-Atc", query)
	fields := strings.Fields(output)
	require.NotEmpty(t, fields)
	eventID := fields[len(fields)-1]
	require.NoError(t, uuid.Validate(eventID))
	return eventID
}

func waitForDeadLetter(t *testing.T, originalEventID string, timeout time.Duration) ([]byte, deadLetterEnvelope) {
	t.Helper()
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:     []string{resilienceKafkaBroker},
		GroupID:     "integration-dlq-" + uuid.NewString(),
		Topic:       topicTransferDLQ,
		StartOffset: kafka.FirstOffset,
		MinBytes:    1,
		MaxBytes:    10e6,
		MaxWait:     500 * time.Millisecond,
	})
	defer func() { _ = reader.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	for {
		message, err := reader.ReadMessage(ctx)
		if err != nil {
			t.Fatalf("waiting for DLQ event %s: %v", originalEventID, err)
		}
		var envelope deadLetterEnvelope
		if err := json.Unmarshal(message.Value, &envelope); err != nil {
			continue
		}
		if envelope.Payload.OriginalEvent != nil && envelope.Payload.OriginalEvent.EventID == originalEventID {
			return append([]byte(nil), message.Value...), envelope
		}
	}
}

func TestOutboxRecoversAfterKafkaOutage(t *testing.T) {
	requireResilience(t)
	requireToken(t)

	senderWalletID := createTestWallet(t, "TRY")
	receiverWalletID := createTestWallet(t, "TRY")
	applyWalletCommand(t, senderWalletID, "credit", "100.00", uuid.NewString())

	restoreService(t, "redpanda")
	compose(t, "stop", "redpanda")

	transferID, status := createResilienceTransfer(t, senderWalletID, receiverWalletID, "25.00")
	require.Equal(t, "PENDING", status)
	require.Equal(t, "PENDING", getTransferStatus(t, transferID))
	require.Equal(t, "pending", initialOutboxState(t, transferID))

	compose(t, "start", "redpanda")
	waitForKafka(t, 30*time.Second)
	waitForTransferStatusWithin(t, transferID, "COMPLETED", 90*time.Second)
	require.Equal(t, "published", initialOutboxState(t, transferID))
	waitForWalletBalance(t, senderWalletID, "75.00", 15*time.Second)
	waitForWalletBalance(t, receiverWalletID, "25.00", 15*time.Second)
}

func TestConsumerRestartRedeliveryIsIdempotent(t *testing.T) {
	requireResilience(t)
	requireToken(t)

	senderWalletID := createTestWallet(t, "TRY")
	receiverWalletID := createTestWallet(t, "TRY")
	applyWalletCommand(t, senderWalletID, "credit", "100.00", uuid.NewString())

	restoreService(t, "transaction-worker")
	compose(t, "stop", "transaction-worker")
	transferID, status := createResilienceTransfer(t, senderWalletID, receiverWalletID, "10.00")
	require.Equal(t, "PENDING", status)
	suppressInitialOutbox(t, transferID)

	createdEvent := newIntegrationEvent(topicTransferCreated, transferID, map[string]interface{}{
		"transfer_id":        transferID,
		"sender_wallet_id":   senderWalletID,
		"receiver_wallet_id": receiverWalletID,
		"amount":             "10.00",
		"currency":           "TRY",
	})
	publishIntegrationEvent(t, topicTransferCreated, createdEvent)
	waitForWalletBalance(t, senderWalletID, "90.00", 20*time.Second)
	waitForWalletBalance(t, receiverWalletID, "10.00", 20*time.Second)
	waitForLedgerEntry(t, senderWalletID, transferID, "DEBIT", 10*time.Second)
	waitForLedgerEntry(t, receiverWalletID, transferID, "CREDIT", 10*time.Second)

	restoreService(t, "wallet-worker")
	compose(t, "kill", "wallet-worker")
	compose(t, "start", "wallet-worker")
	publishIntegrationEvent(t, topicTransferCreated, createdEvent)
	waitForComposeLog(t, "wallet-worker", 20*time.Second, createdEvent.EventID, "Re-emitting debit result for an already applied command")

	require.Equal(t, "90.00", getWalletBalance(t, senderWalletID))
	require.Equal(t, "10.00", getWalletBalance(t, receiverWalletID))
	require.Equal(t, 1, ledgerEntryCount(getWalletTransactions(t, senderWalletID), transferID, "DEBIT"))
	require.Equal(t, 1, ledgerEntryCount(getWalletTransactions(t, receiverWalletID), transferID, "CREDIT"))

	compose(t, "start", "transaction-worker")
	waitForTransferStatusWithin(t, transferID, "COMPLETED", 45*time.Second)
}

func TestReorderedAndDuplicateSagaEventsAreGuarded(t *testing.T) {
	requireResilience(t)
	requireToken(t)

	senderWalletID := createTestWallet(t, "TRY")
	receiverWalletID := createTestWallet(t, "TRY")
	applyWalletCommand(t, senderWalletID, "credit", "50.00", uuid.NewString())
	transferID, _ := createResilienceTransfer(t, senderWalletID, receiverWalletID, "5.00")
	waitForTransferStatusWithin(t, transferID, "COMPLETED", 30*time.Second)
	waitForWalletBalance(t, senderWalletID, "45.00", 10*time.Second)
	waitForWalletBalance(t, receiverWalletID, "5.00", 10*time.Second)

	creditSuccess := newIntegrationEvent(topicTransferCreditSuccess, transferID, map[string]interface{}{
		"transfer_id":      transferID,
		"wallet_id":        receiverWalletID,
		"sender_wallet_id": senderWalletID,
		"amount":           "5.00",
	})
	creditSuccess.EventID = transitionEventID(t, transferID, topicTransferCreditSuccess)

	debitSuccess := newIntegrationEvent(topicTransferDebitSuccess, transferID, map[string]interface{}{
		"transfer_id":        transferID,
		"wallet_id":          senderWalletID,
		"sender_wallet_id":   senderWalletID,
		"receiver_wallet_id": receiverWalletID,
		"amount":             "5.00",
	})
	debitSuccess.EventID = transitionEventID(t, transferID, topicTransferDebitSuccess)

	// Replay the completed saga's historical events in reverse order.
	publishIntegrationEvent(t, topicTransferCreditSuccess, creditSuccess)
	publishIntegrationEvent(t, topicTransferDebitSuccess, debitSuccess)
	waitForComposeLog(t, "wallet-worker", 15*time.Second, debitSuccess.EventID, "Re-emitting credit result for an already applied command")

	require.Equal(t, "COMPLETED", getTransferStatus(t, transferID))
	require.Equal(t, "45.00", getWalletBalance(t, senderWalletID))
	require.Equal(t, "5.00", getWalletBalance(t, receiverWalletID))
	require.Equal(t, 1, ledgerEntryCount(getWalletTransactions(t, senderWalletID), transferID, "DEBIT"))
	require.Equal(t, 1, ledgerEntryCount(getWalletTransactions(t, receiverWalletID), transferID, "CREDIT"))
}

func TestDeadLetterCanBeAuditedAndReplayed(t *testing.T) {
	requireResilience(t)

	poisonEvent := newIntegrationEvent(topicTransferCreated, uuid.NewString(), map[string]interface{}{
		"transfer_id":        uuid.NewString(),
		"sender_wallet_id":   "not-a-uuid",
		"receiver_wallet_id": uuid.NewString(),
		"amount":             "1.00",
		"currency":           "TRY",
	})
	poisonEvent.Metadata.RetryCount = 4
	publishIntegrationEvent(t, topicTransferCreated, poisonEvent)

	rawEnvelope, envelope := waitForDeadLetter(t, poisonEvent.EventID, 30*time.Second)
	require.Equal(t, "dlq.entry", envelope.EventType)
	require.Equal(t, topicTransferCreated, envelope.Payload.OriginalTopic)
	require.Equal(t, "handler_error", envelope.Payload.FailureClass)
	require.Equal(t, 5, envelope.Payload.RetryCount)
	require.NotEmpty(t, envelope.Payload.Error)
	require.GreaterOrEqual(t, envelope.Payload.OriginalPartition, 0)
	require.GreaterOrEqual(t, envelope.Payload.OriginalOffset, int64(0))

	tempDir := t.TempDir()
	dlqPath := filepath.Join(tempDir, "dead-letter.json")
	auditPath := filepath.Join(tempDir, "replay-audit.jsonl")
	require.NoError(t, os.WriteFile(dlqPath, rawEnvelope, 0o600))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	// The command and all paths are controlled by this integration test.
	cmd := exec.CommandContext(ctx, "go", "run", ".", //nolint:gosec
		"-file", dlqPath,
		"-brokers", resilienceKafkaBroker,
		"-actor", "integration-test",
		"-reason", "verify audited P0 replay",
		"-audit-file", auditPath,
	)
	cmd.Dir = filepath.Clean("../../tools/eventreplay")
	cmd.Env = append(os.Environ(), "GOWORK=off")
	output, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "event replay failed:\n%s", output)
	assert.Contains(t, string(output), "replayed event "+poisonEvent.EventID)

	auditData, err := os.ReadFile(auditPath)
	require.NoError(t, err)
	lines := bytes.Split(bytes.TrimSpace(auditData), []byte("\n"))
	require.Len(t, lines, 2)
	records := make([]replayAuditRecord, 0, len(lines))
	for _, line := range lines {
		var record replayAuditRecord
		require.NoError(t, json.Unmarshal(line, &record))
		records = append(records, record)
	}
	require.Equal(t, records[0].ReplayID, records[1].ReplayID)
	require.Equal(t, []string{"requested", "published"}, []string{records[0].Status, records[1].Status})
	for _, record := range records {
		require.Equal(t, envelope.EventID, record.DLQEventID)
		require.Equal(t, poisonEvent.EventID, record.OriginalEventID)
		require.Equal(t, topicTransferCreated, record.OriginalTopic)
		require.Equal(t, topicTransferCreated, record.TargetTopic)
		require.Equal(t, "integration-test", record.Actor)
		require.Equal(t, "verify audited P0 replay", record.Reason)
	}
}

func TestConcurrentWalletCommandsPreserveBalanceAndLedger(t *testing.T) {
	requireResilience(t)
	requireToken(t)

	walletID := createTestWallet(t, "TRY")
	initialReference := uuid.NewString()
	applyWalletCommand(t, walletID, "credit", "100.00", initialReference)

	type commandResult struct {
		action      string
		referenceID string
		statusCode  int
		body        string
		err         error
	}

	const commandsPerType = 8
	results := make(chan commandResult, commandsPerType*2)
	start := make(chan struct{})
	var waitGroup sync.WaitGroup
	references := map[string]string{initialReference: "CREDIT"}

	launch := func(action string) {
		referenceID := uuid.NewString()
		references[referenceID] = strings.ToUpper(action)
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			<-start
			response, err := doJSONRequest(http.MethodPost, walletServiceURL+"/api/v1/wallets/"+walletID+"/"+action, map[string]interface{}{
				"amount":       "1.00",
				"reference_id": referenceID,
				"description":  "Concurrent P0 command",
			})
			if err != nil {
				results <- commandResult{action: action, referenceID: referenceID, err: err}
				return
			}
			defer func() { _ = response.Body.Close() }()
			responseBody, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
			results <- commandResult{
				action:      action,
				referenceID: referenceID,
				statusCode:  response.StatusCode,
				body:        string(responseBody),
			}
		}()
	}

	for range commandsPerType {
		launch("credit")
		launch("debit")
	}
	close(start)
	waitGroup.Wait()
	close(results)

	for result := range results {
		require.NoErrorf(t, result.err, "%s command %s failed", result.action, result.referenceID)
		require.Equalf(t, http.StatusOK, result.statusCode, "%s command %s returned: %s", result.action, result.referenceID, result.body)
	}

	require.Equal(t, "100.00", getWalletBalance(t, walletID))
	transactions := getWalletTransactions(t, walletID)
	require.Len(t, transactions, 1+(commandsPerType*2))
	for referenceID, transactionType := range references {
		require.Equalf(t, 1, ledgerEntryCount(transactions, referenceID, transactionType), "ledger entry %s/%s", referenceID, transactionType)
	}
}
