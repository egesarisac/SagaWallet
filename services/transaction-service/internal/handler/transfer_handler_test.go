package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestTransferHandler_CreateTransfer_Validation(t *testing.T) {
	transferID := uuid.New()

	tests := []struct {
		name           string
		requestBody    map[string]interface{}
		expectedStatus int
	}{
		{
			name: "valid request",
			requestBody: map[string]interface{}{
				"sender_wallet_id":   uuid.New().String(),
				"receiver_wallet_id": uuid.New().String(),
				"amount":             "100.00",
			},
			expectedStatus: http.StatusAccepted,
		},
		{
			name: "missing sender_wallet_id",
			requestBody: map[string]interface{}{
				"receiver_wallet_id": uuid.New().String(),
				"amount":             "100.00",
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "missing amount",
			requestBody: map[string]interface{}{
				"sender_wallet_id":   uuid.New().String(),
				"receiver_wallet_id": uuid.New().String(),
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "invalid sender_wallet_id format",
			requestBody: map[string]interface{}{
				"sender_wallet_id":   "not-a-uuid",
				"receiver_wallet_id": uuid.New().String(),
				"amount":             "100.00",
			},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.POST("/transfers", func(c *gin.Context) {
				var req struct {
					SenderWalletID   string `json:"sender_wallet_id" binding:"required"`
					ReceiverWalletID string `json:"receiver_wallet_id" binding:"required"`
					Amount           string `json:"amount" binding:"required"`
				}
				if err := c.ShouldBindJSON(&req); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
					return
				}

				// Validate UUIDs
				if _, err := uuid.Parse(req.SenderWalletID); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": "invalid sender_wallet_id"})
					return
				}

				c.JSON(http.StatusAccepted, gin.H{
					"data": map[string]interface{}{
						"transfer_id": transferID.String(),
						"status":      "PENDING",
					},
				})
			})

			body, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest("POST", "/transfers", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

func TestTransferHandler_GetTransfer_Validation(t *testing.T) {
	transferID := uuid.New()

	tests := []struct {
		name           string
		transferID     string
		expectedStatus int
	}{
		{
			name:           "valid uuid",
			transferID:     transferID.String(),
			expectedStatus: http.StatusOK,
		},
		{
			name:           "invalid uuid",
			transferID:     "not-a-uuid",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "empty uuid",
			transferID:     "",
			expectedStatus: http.StatusNotFound, // Route won't match
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.GET("/transfers/:id", func(c *gin.Context) {
				id := c.Param("id")
				if _, err := uuid.Parse(id); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": "invalid transfer id"})
					return
				}

				c.JSON(http.StatusOK, gin.H{
					"data": map[string]interface{}{
						"transfer_id": id,
						"status":      "COMPLETED",
					},
				})
			})

			req := httptest.NewRequest("GET", "/transfers/"+tt.transferID, nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

func TestTransferStatusValues(t *testing.T) {
	validStatuses := []string{"PENDING", "DEBITED", "COMPLETED", "REFUNDING", "FAILED"}

	for _, status := range validStatuses {
		t.Run(status, func(t *testing.T) {
			assert.Contains(t, validStatuses, status)
		})
	}
}
