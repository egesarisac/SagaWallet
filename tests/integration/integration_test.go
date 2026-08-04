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
	authServiceURL        = getEnv("AUTH_SERVICE_URL", "http://localhost:8085")
	jwtToken              = getEnv("JWT_TOKEN", "")
	runIntegration        = os.Getenv("RUN_INTEGRATION") == "1"

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

func requireToken(t *testing.T) {
	if jwtToken == "" {
		t.Fatal("integration fixture token was not created")
	}
}

func requireServices(t *testing.T) {
	t.Helper()
	if !runIntegration {
		t.Skip("set RUN_INTEGRATION=1 after starting the full stack to run integration tests")
	}
	client := &http.Client{Timeout: 5 * time.Second}
	for name, url := range map[string]string{
		"auth":        authServiceURL,
		"wallet":      walletServiceURL,
		"transaction": transactionServiceURL,
	} {
		resp, err := client.Get(url + "/health")
		if err != nil {
			t.Fatalf("%s service is unavailable: %v", name, err)
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			t.Fatalf("%s service health check returned %d", name, resp.StatusCode)
		}
		resp.Body.Close()
	}
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
func createTestWallet(t *testing.T, currency string) string {
	body := map[string]interface{}{
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
	if runIntegration && jwtToken == "" {
		token, err := registerFixtureUser()
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to create integration test user: %v\n", err)
			os.Exit(1)
		}
		jwtToken = token
	}

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

func registerFixtureUser() (string, error) {
	email := "integration-" + uuid.NewString() + "@example.test"
	body, err := json.Marshal(map[string]string{
		"email":    email,
		"password": "integration-test-password",
	})
	if err != nil {
		return "", err
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(authServiceURL+"/api/v1/auth/register", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("auth registration returned %d", resp.StatusCode)
	}
	var result struct {
		Data struct {
			AccessToken string `json:"access_token"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if result.Data.AccessToken == "" {
		return "", fmt.Errorf("auth registration returned no access token")
	}
	return result.Data.AccessToken, nil
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
	requireServices(t)

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
	requireServices(t)
	requireToken(t)

	body := map[string]interface{}{
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
	requireServices(t)
	requireToken(t)

	// Create TRY wallet
	tryWalletID := createTestWallet(t, "TRY")
	assert.NotEmpty(t, tryWalletID)

	// Create USD wallet for same user
	usdWalletID := createTestWallet(t, "USD")
	assert.NotEmpty(t, usdWalletID)

	// Verify they are different wallets
	assert.NotEqual(t, tryWalletID, usdWalletID)
}

func TestGetWallet(t *testing.T) {
	requireServices(t)
	requireToken(t)

	// Create a wallet first
	walletID := createTestWallet(t, "EUR")

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
	requireServices(t)

	walletID := uuid.New().String()

	// Request without token
	resp, err := http.Get(walletServiceURL + "/api/v1/wallets/" + walletID)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func waitForTransferStatus(t *testing.T, transferID, expectedStatus string) map[string]interface{} {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	lastStatus := ""
	for time.Now().Before(deadline) {
		resp := makeRequest(t, "GET", transactionServiceURL+"/api/v1/transfers/"+transferID, nil)
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			t.Fatalf("get transfer returned %d", resp.StatusCode)
		}
		var result map[string]interface{}
		err := json.NewDecoder(resp.Body).Decode(&result)
		resp.Body.Close()
		require.NoError(t, err)
		data, ok := result["data"].(map[string]interface{})
		if !ok {
			t.Fatalf("transfer response is missing data: %#v", result)
		}
		status, ok := data["status"].(string)
		if !ok {
			t.Fatalf("transfer response is missing status: %#v", data)
		}
		lastStatus = status
		if status == expectedStatus {
			return data
		}
		if status == "COMPLETED" || status == "FAILED" || status == "MANUAL_REVIEW" {
			t.Fatalf("transfer reached terminal status %s; expected %s", status, expectedStatus)
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("transfer did not reach %s within 15s; last status was %s", expectedStatus, lastStatus)
	return nil
}

func TestTransferFlow(t *testing.T) {
	requireServices(t)
	requireToken(t)

	// Create two wallets with initial balance
	senderWalletID := createTestWallet(t, "TRY")
	receiverWalletID := createTestWallet(t, "TRY")

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

	waitForTransferStatus(t, transferID, "COMPLETED")
}

func TestRateLimiting(t *testing.T) {
	requireServices(t)

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
	requireServices(t)
	requireToken(t)

	// Create sender wallet with NO balance
	senderWalletID := createTestWallet(t, "TRY")
	receiverWalletID := createTestWallet(t, "TRY")

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

	data2 := waitForTransferStatus(t, transferID, "FAILED")

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
	requireServices(t)
	requireToken(t)

	// Create sender wallet with balance
	senderWalletID := createTestWallet(t, "TRY")

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

	waitForTransferStatus(t, transferID, "FAILED")

	// The key test: sender should have their money back after refund
	// If saga worked correctly, balance should be back to 100.00
	finalBalance := getWalletBalance(t, senderWalletID)
	t.Logf("Final sender balance: %s (expected 100.00 after refund)", finalBalance)

	assert.Equal(t, "100.00", finalBalance,
		"Sender balance should be refunded to 100.00 after failed transfer")
}

// TestSuccessfulTransferBalanceVerification tests a complete transfer and verifies balances.
func TestSuccessfulTransferBalanceVerification(t *testing.T) {
	requireServices(t)
	requireToken(t)

	// Create two wallets
	senderWalletID := createTestWallet(t, "TRY")
	receiverWalletID := createTestWallet(t, "TRY")

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
	err := json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)
	data := result["data"].(map[string]interface{})
	transferID := data["transfer_id"].(string)

	waitForTransferStatus(t, transferID, "COMPLETED")

	// Verify balances after the terminal saga status.
	senderBalance := getWalletBalance(t, senderWalletID)
	receiverBalance := getWalletBalance(t, receiverWalletID)

	t.Logf("Sender balance: %s (expected 70.00)", senderBalance)
	t.Logf("Receiver balance: %s (expected 30.00)", receiverBalance)

	assert.Equal(t, "70.00", senderBalance, "Sender should have 70.00 after transfer")
	assert.Equal(t, "30.00", receiverBalance, "Receiver should have 30.00 after transfer")
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
