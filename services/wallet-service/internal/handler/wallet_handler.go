// Package handler provides HTTP handlers for wallet service.
package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	apperrors "github.com/egesarisac/SagaWallet/pkg/errors"
	"github.com/egesarisac/SagaWallet/pkg/logger"
	"github.com/egesarisac/SagaWallet/pkg/middleware"
	db "github.com/egesarisac/SagaWallet/services/wallet-service/db/generated"
	"github.com/egesarisac/SagaWallet/services/wallet-service/internal/service"
)

// WalletHandler handles HTTP requests for wallet operations.
type WalletHandler struct {
	svc *service.WalletService
	log *logger.Logger
}

// NewWalletHandler creates a new wallet handler.
func NewWalletHandler(svc *service.WalletService, log *logger.Logger) *WalletHandler {
	return &WalletHandler{
		svc: svc,
		log: log,
	}
}

// RegisterRoutes registers wallet routes with the Gin router.
func (h *WalletHandler) RegisterRoutes(r *gin.RouterGroup) {
	wallets := r.Group("/wallets")
	{
		wallets.POST("", h.CreateWallet)
		wallets.GET("/:id", h.GetWallet)
		wallets.GET("/:id/balance", h.GetBalance)
		wallets.POST("/:id/credit", h.Credit)
		wallets.POST("/:id/debit", h.Debit)
		wallets.GET("/:id/transactions", h.GetTransactions)
		wallets.PUT("/:id/status", h.UpdateStatus)
		wallets.DELETE("/:id", h.DeleteWallet)
	}
}

// Helper functions for response formatting
func pgtypeToUUID(p pgtype.UUID) uuid.UUID {
	if !p.Valid {
		return uuid.Nil
	}
	return p.Bytes
}

func numericToString(n pgtype.Numeric) string {
	if !n.Valid {
		return "0.00"
	}
	val, _ := n.Value()
	if val == nil {
		return "0.00"
	}
	return val.(string)
}

// CreateWalletRequest is the request body for creating a wallet.
type CreateWalletRequest struct {
	UserID   string `json:"user_id" binding:"omitempty,uuid"`
	Currency string `json:"currency" binding:"omitempty,len=3"`
}

// WalletResponse is the response body for wallet operations.
type WalletResponse struct {
	ID        string `json:"id"`
	UserID    string `json:"user_id"`
	Balance   string `json:"balance"`
	Currency  string `json:"currency"`
	Status    string `json:"status"`
	Version   int64  `json:"version"`
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

// CreateWallet handles wallet creation.
func (h *WalletHandler) CreateWallet(c *gin.Context) {
	var req CreateWalletRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.respondError(c, apperrors.New(apperrors.CodeValidationFailed, err.Error()))
		return
	}

	userID, err := h.getAuthenticatedUserID(c)
	if err != nil {
		h.respondError(c, err)
		return
	}

	if req.UserID != "" && req.UserID != userID.String() {
		h.respondError(c, apperrors.New(apperrors.CodeForbidden, "user_id in request does not match authenticated user"))
		return
	}

	wallet, err := h.svc.CreateWallet(c.Request.Context(), service.CreateWalletInput{
		UserID:   userID,
		Currency: req.Currency,
	})
	if err != nil {
		h.respondError(c, err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"data": WalletResponse{
			ID:       pgtypeToUUID(wallet.ID).String(),
			UserID:   pgtypeToUUID(wallet.UserID).String(),
			Balance:  numericToString(wallet.Balance),
			Currency: wallet.Currency,
			Status:   wallet.Status,
			Version:  wallet.Version,
		},
	})
}

// GetWallet handles retrieving a wallet by ID.
func (h *WalletHandler) GetWallet(c *gin.Context) {
	walletID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		h.respondError(c, apperrors.New(apperrors.CodeValidationFailed, "Invalid wallet ID"))
		return
	}

	wallet, ok := h.getOwnedWallet(c, walletID)
	if !ok {
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": WalletResponse{
			ID:       pgtypeToUUID(wallet.ID).String(),
			UserID:   pgtypeToUUID(wallet.UserID).String(),
			Balance:  numericToString(wallet.Balance),
			Currency: wallet.Currency,
			Status:   wallet.Status,
			Version:  wallet.Version,
		},
	})
}

// GetBalance handles retrieving wallet balance.
func (h *WalletHandler) GetBalance(c *gin.Context) {
	walletID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		h.respondError(c, apperrors.New(apperrors.CodeValidationFailed, "Invalid wallet ID"))
		return
	}

	wallet, ok := h.getOwnedWallet(c, walletID)
	if !ok {
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"wallet_id": walletID.String(),
			"balance":   numericToString(wallet.Balance),
			"currency":  wallet.Currency,
		},
	})
}

// CreditRequest is the request body for crediting a wallet.
type CreditRequest struct {
	Amount      string `json:"amount" binding:"required"`
	ReferenceID string `json:"reference_id" binding:"required,uuid"`
	Description string `json:"description"`
}

// Credit handles adding funds to a wallet.
func (h *WalletHandler) Credit(c *gin.Context) {
	walletID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		h.respondError(c, apperrors.New(apperrors.CodeValidationFailed, "Invalid wallet ID"))
		return
	}

	if _, ok := h.getOwnedWallet(c, walletID); !ok {
		return
	}

	var req CreditRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.respondError(c, apperrors.New(apperrors.CodeValidationFailed, err.Error()))
		return
	}

	referenceID, _ := uuid.Parse(req.ReferenceID)
	wallet, err := h.svc.Credit(c.Request.Context(), service.CreditInput{
		WalletID:    walletID,
		Amount:      req.Amount,
		ReferenceID: referenceID,
		Description: req.Description,
	})
	if err != nil {
		h.respondError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"wallet_id":   pgtypeToUUID(wallet.ID).String(),
			"new_balance": numericToString(wallet.Balance),
			"currency":    wallet.Currency,
		},
	})
}

// DebitRequest is the request body for debiting a wallet.
type DebitRequest struct {
	Amount      string `json:"amount" binding:"required"`
	ReferenceID string `json:"reference_id" binding:"required,uuid"`
	Description string `json:"description"`
}

// Debit handles withdrawing funds from a wallet.
func (h *WalletHandler) Debit(c *gin.Context) {
	walletID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		h.respondError(c, apperrors.New(apperrors.CodeValidationFailed, "Invalid wallet ID"))
		return
	}

	if _, ok := h.getOwnedWallet(c, walletID); !ok {
		return
	}

	var req DebitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.respondError(c, apperrors.New(apperrors.CodeValidationFailed, err.Error()))
		return
	}

	referenceID, _ := uuid.Parse(req.ReferenceID)
	wallet, err := h.svc.Debit(c.Request.Context(), service.DebitInput{
		WalletID:    walletID,
		Amount:      req.Amount,
		ReferenceID: referenceID,
		Description: req.Description,
	})
	if err != nil {
		h.respondError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"wallet_id":   pgtypeToUUID(wallet.ID).String(),
			"new_balance": numericToString(wallet.Balance),
			"currency":    wallet.Currency,
		},
	})
}

// GetTransactions handles retrieving wallet transactions.
func (h *WalletHandler) GetTransactions(c *gin.Context) {
	walletID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		h.respondError(c, apperrors.New(apperrors.CodeValidationFailed, "Invalid wallet ID"))
		return
	}

	if _, ok := h.getOwnedWallet(c, walletID); !ok {
		return
	}

	// Parse pagination (default: 20 items, offset 0)
	limit := int32(20)
	offset := int32(0)

	transactions, err := h.svc.GetTransactions(c.Request.Context(), walletID, limit, offset)
	if err != nil {
		h.respondError(c, err)
		return
	}

	// Convert transactions to response format
	var response []gin.H
	for _, tx := range transactions {
		response = append(response, gin.H{
			"id":            pgtypeToUUID(tx.ID).String(),
			"wallet_id":     pgtypeToUUID(tx.WalletID).String(),
			"amount":        numericToString(tx.Amount),
			"type":          tx.Type,
			"reference_id":  pgtypeToUUID(tx.ReferenceID).String(),
			"description":   tx.Description.String,
			"balance_after": numericToString(tx.BalanceAfter),
			"created_at":    tx.CreatedAt.Time.String(),
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"data": response,
	})
}

// UpdateStatusRequest is the request body for updating wallet status.
type UpdateStatusRequest struct {
	Status string `json:"status" binding:"required,oneof=ACTIVE SUSPENDED FROZEN CLOSED"`
}

// UpdateStatus handles updating wallet status.
func (h *WalletHandler) UpdateStatus(c *gin.Context) {
	walletID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		h.respondError(c, apperrors.New(apperrors.CodeValidationFailed, "Invalid wallet ID"))
		return
	}

	if _, ok := h.getOwnedWallet(c, walletID); !ok {
		return
	}

	var req UpdateStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.respondError(c, apperrors.New(apperrors.CodeValidationFailed, err.Error()))
		return
	}

	wallet, err := h.svc.UpdateStatus(c.Request.Context(), walletID, req.Status)
	if err != nil {
		h.respondError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": WalletResponse{
			ID:       pgtypeToUUID(wallet.ID).String(),
			UserID:   pgtypeToUUID(wallet.UserID).String(),
			Balance:  numericToString(wallet.Balance),
			Currency: wallet.Currency,
			Status:   wallet.Status,
			Version:  wallet.Version,
		},
	})
}

// respondError sends a standardized error response.
func (h *WalletHandler) respondError(c *gin.Context, err error) {
	if appErr, ok := err.(*apperrors.AppError); ok {
		c.JSON(appErr.HTTPStatus(), apperrors.ErrorResponse{Error: appErr})
		return
	}

	// Unknown error
	c.JSON(http.StatusInternalServerError, apperrors.ErrorResponse{
		Error: apperrors.InternalError(err),
	})
}

func (h *WalletHandler) getAuthenticatedUserID(c *gin.Context) (uuid.UUID, error) {
	authUserID := middleware.GetUserID(c)
	if authUserID == "" {
		return uuid.Nil, apperrors.New(apperrors.CodeUnauthorized, "missing authenticated user context")
	}

	userID, err := uuid.Parse(authUserID)
	if err != nil {
		return uuid.Nil, apperrors.New(apperrors.CodeUnauthorized, "invalid authenticated user context")
	}

	return userID, nil
}

func (h *WalletHandler) getOwnedWallet(c *gin.Context, walletID uuid.UUID) (*db.Wallet, bool) {
	authUserID, err := h.getAuthenticatedUserID(c)
	if err != nil {
		h.respondError(c, err)
		return nil, false
	}

	wallet, err := h.svc.GetWallet(c.Request.Context(), walletID)
	if err != nil {
		h.respondError(c, err)
		return nil, false
	}

	if pgtypeToUUID(wallet.UserID) != authUserID {
		h.respondError(c, apperrors.New(apperrors.CodeForbidden, "wallet does not belong to authenticated user"))
		return nil, false
	}

	return wallet, true
}

// DeleteWallet handles wallet deletion (for testing cleanup).
func (h *WalletHandler) DeleteWallet(c *gin.Context) {
	walletID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		h.respondError(c, apperrors.New(apperrors.CodeValidationFailed, "Invalid wallet ID"))
		return
	}

	if _, ok := h.getOwnedWallet(c, walletID); !ok {
		return
	}

	err = h.svc.DeleteWallet(c.Request.Context(), walletID)
	if err != nil {
		h.respondError(c, err)
		return
	}

	c.JSON(http.StatusNoContent, nil)
}
