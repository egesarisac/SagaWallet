// Package handler provides HTTP handlers for transaction service.
package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	apperrors "github.com/egesarisac/SagaWallet/pkg/errors"
	"github.com/egesarisac/SagaWallet/pkg/logger"
	"github.com/egesarisac/SagaWallet/pkg/middleware"
	"github.com/egesarisac/SagaWallet/services/transaction-service/internal/service"
)

// TransferHandler handles HTTP requests for transfers.
type TransferHandler struct {
	svc *service.TransferService
	log *logger.Logger
}

// NewTransferHandler creates a new transfer handler.
func NewTransferHandler(svc *service.TransferService, log *logger.Logger) *TransferHandler {
	return &TransferHandler{
		svc: svc,
		log: log,
	}
}

// RegisterRoutes registers the transfer routes.
func (h *TransferHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/transfers", h.CreateTransfer)
	rg.GET("/transfers/:id", h.GetTransfer)
}

// CreateTransferRequest represents the request body for creating a transfer.
type CreateTransferRequest struct {
	SenderWalletID   string `json:"sender_wallet_id" binding:"required"`
	ReceiverWalletID string `json:"receiver_wallet_id" binding:"required"`
	Amount           string `json:"amount" binding:"required"`
	Currency         string `json:"currency"`
	IdempotencyKey   string `json:"idempotency_key"`
}

// CreateTransfer handles POST /transfers
func (h *TransferHandler) CreateTransfer(c *gin.Context) {
	var req CreateTransferRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, apperrors.ErrorResponse{
			Error: apperrors.New(apperrors.CodeValidationFailed, err.Error()),
		})
		return
	}

	requestUserID, err := uuid.Parse(middleware.GetUserID(c))
	if err != nil {
		c.JSON(http.StatusUnauthorized, apperrors.ErrorResponse{
			Error: apperrors.New(apperrors.CodeUnauthorized, "invalid or missing user context"),
		})
		return
	}

	senderID, err := uuid.Parse(req.SenderWalletID)
	if err != nil {
		c.JSON(http.StatusBadRequest, apperrors.ErrorResponse{
			Error: apperrors.New(apperrors.CodeValidationFailed, "invalid sender_wallet_id"),
		})
		return
	}

	receiverID, err := uuid.Parse(req.ReceiverWalletID)
	if err != nil {
		c.JSON(http.StatusBadRequest, apperrors.ErrorResponse{
			Error: apperrors.New(apperrors.CodeValidationFailed, "invalid receiver_wallet_id"),
		})
		return
	}

	var idempotencyKey uuid.UUID
	if req.IdempotencyKey != "" {
		idempotencyKey, err = uuid.Parse(req.IdempotencyKey)
		if err != nil {
			c.JSON(http.StatusBadRequest, apperrors.ErrorResponse{
				Error: apperrors.New(apperrors.CodeValidationFailed, "invalid idempotency_key"),
			})
			return
		}
	} else {
		idempotencyKey = uuid.New()
	}

	currency := req.Currency
	if currency == "" {
		currency = "TRY"
	}

	result, err := h.svc.CreateTransfer(c.Request.Context(), service.CreateTransferInput{
		RequestUserID:    requestUserID,
		SenderWalletID:   senderID,
		ReceiverWalletID: receiverID,
		Amount:           req.Amount,
		Currency:         currency,
		IdempotencyKey:   idempotencyKey,
	})

	if err != nil {
		if appErr, ok := err.(*apperrors.AppError); ok {
			c.JSON(appErr.HTTPStatus(), apperrors.ErrorResponse{Error: appErr})
			return
		}
		c.JSON(http.StatusInternalServerError, apperrors.ErrorResponse{
			Error: apperrors.InternalError(err),
		})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"data": result,
	})
}

// GetTransfer handles GET /transfers/:id
func (h *TransferHandler) GetTransfer(c *gin.Context) {
	idStr := c.Param("id")
	transferID, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, apperrors.ErrorResponse{
			Error: apperrors.New(apperrors.CodeValidationFailed, "invalid transfer id"),
		})
		return
	}

	requestUserID, err := uuid.Parse(middleware.GetUserID(c))
	if err != nil {
		c.JSON(http.StatusUnauthorized, apperrors.ErrorResponse{
			Error: apperrors.New(apperrors.CodeUnauthorized, "invalid or missing user context"),
		})
		return
	}

	result, err := h.svc.GetTransfer(c.Request.Context(), transferID, requestUserID)
	if err != nil {
		if appErr, ok := err.(*apperrors.AppError); ok {
			c.JSON(appErr.HTTPStatus(), apperrors.ErrorResponse{Error: appErr})
			return
		}
		c.JSON(http.StatusInternalServerError, apperrors.ErrorResponse{
			Error: apperrors.InternalError(err),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": result,
	})
}
