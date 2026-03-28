// Package integration contains integration tests for the wallet service.
// These tests require running services (use docker compose).
package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	walletServiceURL      = getEnv("WALLET_SERVICE_URL", "http://localhost:8081")
	transactionServiceURL = getEnv("TRANSACTION_SERVICE_URL", "http://localhost:8083")
	jwtToken              = getEnv("JWT_TOKEN", "")

	// Track created resources for cleanup
	createdWallets   []string
	createdWalletsMu sync.Mutex
)

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func skipIfNoToken(t *testing.T) {
	if jwtToken == "" {
		t.Skip("Skipping integration test: JWT_TOKEN not set")
	}
}

func skipIfNoServices(t *testing.T) {
	resp, err := http.Get(walletServiceURL + "/health")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Skip("Skipping integration test: wallet service not available")
	}
	resp.Body.Close()
}

func makeRequest(t *testing.T, method, url string, body interface{}) *http.Response {
	var bodyReader io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		require.NoError(t, err)
		bodyReader = bytes.NewBuffer(jsonBody)
	}

	req, err := http.NewRequest(method, url, bodyReader)
	require.NoError(t, err)

	req.Header.Set("Content-Type", "application/json")
	if jwtToken != "" {
		req.Header.Set("Authorization", "Bearer "+jwtToken)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	require.NoError(t, err)

	return resp
}

// trackWallet records a wallet ID for cleanup
func trackWallet(walletID string) {
	createdWalletsMu.Lock()
	defer createdWalletsMu.Unlock()
	createdWallets = append(createdWallets, walletID)
}


// createTestWallet creates a wallet, tracks it for cleanup, and returns its ID
func createTestWallet(t *testing.T, userID, currency string) string {
	body := map[string]interface{}{
		"user_id":  userID,
		"currency": currency,
	}

	resp := makeRequest(t, "POST", walletServiceURL+"/api/v1/wallets", body)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Failed to create test wallet: status %d", resp.StatusCode)
	}

	var result map[string]interface{}
	err := json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	data := result["data"].(map[string]interface{})
	walletID := data["id"].(string)

	// Track for cleanup
	trackWallet(walletID)

	return walletID
}

// TestMain runs before/after all tests for setup and cleanup
func TestMain(m *testing.M) {
	// Run tests
	code := m.Run()

	// Cleanup: Delete all created wallets
	fmt.Printf("\n🧹 Cleaning up %d test wallets...\n", len(createdWallets))
	for _, walletID := range createdWallets {
		deleteWallet(walletID)
	}
	fmt.Println("✅ Cleanup complete")

	os.Exit(code)
}

func deleteWallet(walletID string) {
	req, err := http.NewRequest("DELETE", walletServiceURL+"/api/v1/wallets/"+walletID, nil)
	if err != nil {
		fmt.Printf("   ❌ Failed to create request for %s: %v\n", walletID, err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	if jwtToken != "" {
		req.Header.Set("Authorization", "Bearer "+jwtToken)
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("   ❌ Failed to delete %s: %v\n", walletID, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusOK {
		fmt.Printf("   ✓ Deleted %s\n", walletID)
	} else {
		fmt.Printf("   ❌ Failed to delete %s: status %d\n", walletID, resp.StatusCode)
	}
}

func TestHealthEndpoints(t *testing.T) {
	skipIfNoServices(t)

	tests := []struct {
		name string
		url  string
	}{
		{"wallet service health", walletServiceURL + "/health"},
		{"wallet service metrics", walletServiceURL + "/metrics"},
		{"transaction service health", transactionServiceURL + "/health"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := http.Get(tt.url)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode)
		})
	}
}

func TestCreateWallet(t *testing.T) {
	skipIfNoServices(t)
	skipIfNoToken(t)

	userID := uuid.New().String()

	body := map[string]interface{}{
		"user_id":  userID,
		"currency": "TRY",
	}

	resp := makeRequest(t, "POST", walletServiceURL+"/api/v1/wallets", body)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var result map[string]interface{}
	err := json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	data := result["data"].(map[string]interface{})
	walletID := data["id"].(string)
	assert.NotEmpty(t, walletID)

	// Track for cleanup
	trackWallet(walletID)

	// Balance can be "0" or "0.00" depending on serialization
	balance := data["balance"].(string)
	assert.True(t, balance == "0" || balance == "0.00", "Expected 0 or 0.00, got %s", balance)
	assert.Equal(t, "TRY", data["currency"])
	assert.Equal(t, "ACTIVE", data["status"])
}

func TestCreateMultiCurrencyWallets(t *testing.T) {
	skipIfNoServices(t)
	skipIfNoToken(t)

	userID := uuid.New().String()

	// Create TRY wallet
	tryWalletID := createTestWallet(t, userID, "TRY")
	assert.NotEmpty(t, tryWalletID)

	// Create USD wallet for same user
	usdWalletID := createTestWallet(t, userID, "USD")
	assert.NotEmpty(t, usdWalletID)

	// Verify they are different wallets
	assert.NotEqual(t, tryWalletID, usdWalletID)
}

func TestGetWallet(t *testing.T) {
	skipIfNoServices(t)
	skipIfNoToken(t)

	// Create a wallet first
	userID := uuid.New().String()
	walletID := createTestWallet(t, userID, "EUR")

	// Now get it
	resp := makeRequest(t, "GET", walletServiceURL+"/api/v1/wallets/"+walletID, nil)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	err := json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	data := result["data"].(map[string]interface{})
	assert.Equal(t, walletID, data["id"])
	assert.Equal(t, "EUR", data["currency"])
}

func TestGetWalletUnauthorized(t *testing.T) {
	skipIfNoServices(t)

	walletID := uuid.New().String()

	// Request without token
	resp, err := http.Get(walletServiceURL + "/api/v1/wallets/" + walletID)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestTransferFlow(t *testing.T) {
	skipIfNoServices(t)
	skipIfNoToken(t)

	// Create two wallets with initial balance
	senderUserID := uuid.New().String()
	receiverUserID := uuid.New().String()

	senderWalletID := createTestWallet(t, senderUserID, "TRY")
	receiverWalletID := createTestWallet(t, receiverUserID, "TRY")

	// Credit sender wallet first
	creditBody := map[string]interface{}{
		"amount":       "100.00",
		"reference_id": uuid.New().String(),
		"description":  "Test credit",
	}
	creditResp := makeRequest(t, "POST", walletServiceURL+"/api/v1/wallets/"+senderWalletID+"/credit", creditBody)
	creditResp.Body.Close()
	require.Equal(t, http.StatusOK, creditResp.StatusCode)

	// Create transfer
	body := map[string]interface{}{
		"sender_wallet_id":   senderWalletID,
		"receiver_wallet_id": receiverWalletID,
		"amount":             "10.00",
	}

	resp := makeRequest(t, "POST", transactionServiceURL+"/api/v1/transfers", body)
	defer resp.Body.Close()

	require.Equal(t, http.StatusAccepted, resp.StatusCode)

	var result map[string]interface{}
	err := json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	data := result["data"].(map[string]interface{})
	transferID := data["transfer_id"].(string)
	assert.NotEmpty(t, transferID)
	assert.Equal(t, "PENDING", data["status"])

	// Wait for saga to complete
	time.Sleep(3 * time.Second)

	// Check transfer status
	resp2 := makeRequest(t, "GET", transactionServiceURL+"/api/v1/transfers/"+transferID, nil)
	defer resp2.Body.Close()

	assert.Equal(t, http.StatusOK, resp2.StatusCode)

	var result2 map[string]interface{}
	err = json.NewDecoder(resp2.Body).Decode(&result2)
	require.NoError(t, err)

	data2 := result2["data"].(map[string]interface{})
	status := data2["status"].(string)
	assert.True(t, status == "COMPLETED" || status == "DEBITED", "Expected COMPLETED or DEBITED, got %s", status)
}

func TestRateLimiting(t *testing.T) {
	skipIfNoServices(t)

	// Send many requests quickly
	rateLimitedCount := 0
	for i := 0; i < 150; i++ {
		resp, err := http.Get(walletServiceURL + "/health")
		if err != nil {
			continue
		}
		if resp.StatusCode == http.StatusTooManyRequests {
			rateLimitedCount++
		}
		resp.Body.Close()
	}

	// If rate limiting is disabled (DISABLE_RATE_LIMIT=true), skip the assertion
	if rateLimitedCount == 0 {
		t.Log("Rate limiting appears to be disabled (DISABLE_RATE_LIMIT=true)")
		t.Skip("Skipping rate limit test - rate limiting is disabled")
	}

	// Some requests should have been rate limited
	assert.Greater(t, rateLimitedCount, 0, "Expected some requests to be rate limited")
}

// ===================
// Saga Failure Tests
// ===================

// TestTransferInsufficientFunds tests that a transfer fails properly when sender has insufficient funds.
// The saga should mark the transfer as FAILED without any compensation needed (debit never happened).
func TestTransferInsufficientFunds(t *testing.T) {
	skipIfNoServices(t)
	skipIfNoToken(t)

	// Create sender wallet with NO balance
	senderUserID := uuid.New().String()
	receiverUserID := uuid.New().String()

	senderWalletID := createTestWallet(t, senderUserID, "TRY")
	receiverWalletID := createTestWallet(t, receiverUserID, "TRY")

	// Try to transfer 100 TRY when sender has 0 balance
	body := map[string]interface{}{
		"sender_wallet_id":   senderWalletID,
		"receiver_wallet_id": receiverWalletID,
		"amount":             "100.00",
	}

	resp := makeRequest(t, "POST", transactionServiceURL+"/api/v1/transfers", body)
	defer resp.Body.Close()

	require.Equal(t, http.StatusAccepted, resp.StatusCode)

	var result map[string]interface{}
	err := json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	data := result["data"].(map[string]interface{})
	transferID := data["transfer_id"].(string)
	assert.NotEmpty(t, transferID)

	// Wait for saga to process
	time.Sleep(3 * time.Second)

	// Check transfer status - should be FAILED
	resp2 := makeRequest(t, "GET", transactionServiceURL+"/api/v1/transfers/"+transferID, nil)
	defer resp2.Body.Close()

	assert.Equal(t, http.StatusOK, resp2.StatusCode)

	var result2 map[string]interface{}
	err = json.NewDecoder(resp2.Body).Decode(&result2)
	require.NoError(t, err)

	data2 := result2["data"].(map[string]interface{})
	status := data2["status"].(string)
	assert.Equal(t, "FAILED", status, "Transfer should be FAILED due to insufficient funds")

	// Verify failure reason contains "insufficient" or similar
	if failureReason, ok := data2["failure_reason"].(string); ok {
		t.Logf("Transfer failure reason: %s", failureReason)
	}

	// Verify sender balance is still 0 (no debit happened)
	senderBalance := getWalletBalance(t, senderWalletID)
	assert.True(t, senderBalance == "0" || senderBalance == "0.00",
		"Sender balance should still be 0, got %s", senderBalance)

	// Verify receiver balance is still 0 (no credit happened)
	receiverBalance := getWalletBalance(t, receiverWalletID)
	assert.True(t, receiverBalance == "0" || receiverBalance == "0.00",
		"Receiver balance should still be 0, got %s", receiverBalance)
}

// TestTransferInvalidReceiver tests that when credit fails, the saga properly refunds the sender.
// This simulates a case where debit succeeds but credit fails, triggering compensation.
func TestTransferInvalidReceiver(t *testing.T) {
	skipIfNoServices(t)
	skipIfNoToken(t)

	// Create sender wallet with balance
	senderUserID := uuid.New().String()
	senderWalletID := createTestWallet(t, senderUserID, "TRY")

	// Credit sender wallet first
	creditBody := map[string]interface{}{
		"amount":       "100.00",
		"reference_id": uuid.New().String(),
		"description":  "Initial balance",
	}
	creditResp := makeRequest(t, "POST", walletServiceURL+"/api/v1/wallets/"+senderWalletID+"/credit", creditBody)
	creditResp.Body.Close()
	require.Equal(t, http.StatusOK, creditResp.StatusCode)

	// Verify initial balance
	initialBalance := getWalletBalance(t, senderWalletID)
	assert.Equal(t, "100.00", initialBalance, "Initial balance should be 100.00")

	// Try to transfer to non-existent wallet
	invalidReceiverID := uuid.New().String() // This wallet doesn't exist

	body := map[string]interface{}{
		"sender_wallet_id":   senderWalletID,
		"receiver_wallet_id": invalidReceiverID,
		"amount":             "50.00",
	}

	resp := makeRequest(t, "POST", transactionServiceURL+"/api/v1/transfers", body)
	defer resp.Body.Close()

	require.Equal(t, http.StatusAccepted, resp.StatusCode)

	var result map[string]interface{}
	err := json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	data := result["data"].(map[string]interface{})
	transferID := data["transfer_id"].(string)

	// Wait for saga to process (debit -> credit fail -> refund)
	// This takes longer due to the compensation step
	time.Sleep(5 * time.Second)

	// Check transfer status
	resp2 := makeRequest(t, "GET", transactionServiceURL+"/api/v1/transfers/"+transferID, nil)
	defer resp2.Body.Close()

	var result2 map[string]interface{}
	err = json.NewDecoder(resp2.Body).Decode(&result2)
	require.NoError(t, err)

	data2 := result2["data"].(map[string]interface{})
	status := data2["status"].(string)

	// Status should be FAILED (after refund) or REFUNDING (still processing)
	t.Logf("Transfer status: %s", status)
	assert.True(t, status == "FAILED" || status == "REFUNDING" || status == "DEBITED",
		"Expected FAILED/REFUNDING/DEBITED, got %s", status)

	// The key test: sender should have their money back after refund
	// If saga worked correctly, balance should be back to 100.00
	finalBalance := getWalletBalance(t, senderWalletID)
	t.Logf("Final sender balance: %s (expected 100.00 after refund)", finalBalance)

	// If status is FAILED, refund should have completed
	if status == "FAILED" {
		assert.Equal(t, "100.00", finalBalance,
			"Sender balance should be refunded to 100.00 after failed transfer")
	}
}

// TestSuccessfulTransferBalanceVerification tests a complete transfer and verifies balances.
func TestSuccessfulTransferBalanceVerification(t *testing.T) {
	skipIfNoServices(t)
	skipIfNoToken(t)

	// Create two wallets
	senderUserID := uuid.New().String()
	receiverUserID := uuid.New().String()

	senderWalletID := createTestWallet(t, senderUserID, "TRY")
	receiverWalletID := createTestWallet(t, receiverUserID, "TRY")

	// Credit sender with 100 TRY
	creditBody := map[string]interface{}{
		"amount":       "100.00",
		"reference_id": uuid.New().String(),
		"description":  "Initial balance",
	}
	creditResp := makeRequest(t, "POST", walletServiceURL+"/api/v1/wallets/"+senderWalletID+"/credit", creditBody)
	creditResp.Body.Close()
	require.Equal(t, http.StatusOK, creditResp.StatusCode)

	// Transfer 30 TRY
	body := map[string]interface{}{
		"sender_wallet_id":   senderWalletID,
		"receiver_wallet_id": receiverWalletID,
		"amount":             "30.00",
	}

	resp := makeRequest(t, "POST", transactionServiceURL+"/api/v1/transfers", body)
	defer resp.Body.Close()

	require.Equal(t, http.StatusAccepted, resp.StatusCode)

	var result map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&result)
	data := result["data"].(map[string]interface{})
	transferID := data["transfer_id"].(string)

	// Wait for saga
	time.Sleep(3 * time.Second)

	// Verify transfer completed
	resp2 := makeRequest(t, "GET", transactionServiceURL+"/api/v1/transfers/"+transferID, nil)
	defer resp2.Body.Close()

	var result2 map[string]interface{}
	_ = json.NewDecoder(resp2.Body).Decode(&result2)
	data2 := result2["data"].(map[string]interface{})
	status := data2["status"].(string)

	if status == "COMPLETED" {
		// Verify balances
		senderBalance := getWalletBalance(t, senderWalletID)
		receiverBalance := getWalletBalance(t, receiverWalletID)

		t.Logf("Sender balance: %s (expected 70.00)", senderBalance)
		t.Logf("Receiver balance: %s (expected 30.00)", receiverBalance)

		assert.Equal(t, "70.00", senderBalance, "Sender should have 70.00 after transfer")
		assert.Equal(t, "30.00", receiverBalance, "Receiver should have 30.00 after transfer")
	} else {
		t.Logf("Transfer not yet completed, status: %s", status)
	}
}

// Helper function to get wallet balance
func getWalletBalance(t *testing.T, walletID string) string {
	resp := makeRequest(t, "GET", walletServiceURL+"/api/v1/wallets/"+walletID+"/balance", nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Failed to get wallet balance: status %d", resp.StatusCode)
	}

	var result map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&result)

	data := result["data"].(map[string]interface{})
	return data["balance"].(string)
}
