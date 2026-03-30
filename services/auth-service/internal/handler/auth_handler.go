package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	apperrors "github.com/egesarisac/SagaWallet/pkg/errors"
	"github.com/egesarisac/SagaWallet/pkg/logger"
	"github.com/egesarisac/SagaWallet/services/auth-service/internal/service"
)

// AuthHandler handles auth endpoints.
type AuthHandler struct {
	svc *service.AuthService
	log *logger.Logger
}

// NewAuthHandler creates an auth handler.
func NewAuthHandler(svc *service.AuthService, log *logger.Logger) *AuthHandler {
	return &AuthHandler{svc: svc, log: log}
}

// RegisterRoutes registers auth API routes.
func (h *AuthHandler) RegisterRoutes(r *gin.RouterGroup) {
	auth := r.Group("/auth")
	{
		auth.POST("/register", h.Register)
		auth.POST("/login", h.Login)
		auth.POST("/refresh", h.Refresh)
		auth.POST("/logout", h.Logout)
		auth.GET("/oauth/:provider/start", h.SocialLoginStart)
		auth.POST("/oauth/:provider/callback", h.SocialLoginCallback)
	}
}

// RegisterRequest is the body for user registration.
type RegisterRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8"`
}

// LoginRequest is the body for user login.
type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

// RefreshRequest is the body for token refresh.
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// LogoutRequest is the body for logout.
type LogoutRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// SocialCallbackRequest carries provider callback payload.
type SocialCallbackRequest struct {
	Code  string `json:"code" binding:"required"`
	State string `json:"state"`
}

func (h *AuthHandler) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.respondError(c, apperrors.New(apperrors.CodeValidationFailed, err.Error()))
		return
	}

	_, err := h.svc.Register(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		h.respondServiceError(c, err)
		return
	}

	result, err := h.svc.Login(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		h.respondServiceError(c, err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": result})
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.respondError(c, apperrors.New(apperrors.CodeValidationFailed, err.Error()))
		return
	}

	result, err := h.svc.Login(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		h.respondServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": result})
}

func (h *AuthHandler) Refresh(c *gin.Context) {
	var req RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.respondError(c, apperrors.New(apperrors.CodeValidationFailed, err.Error()))
		return
	}

	result, err := h.svc.Refresh(c.Request.Context(), req.RefreshToken)
	if err != nil {
		h.respondServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": result})
}

func (h *AuthHandler) Logout(c *gin.Context) {
	var req LogoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.respondError(c, apperrors.New(apperrors.CodeValidationFailed, err.Error()))
		return
	}

	h.svc.Logout(c.Request.Context(), req.RefreshToken)
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"message": "logged out"}})
}

func (h *AuthHandler) SocialLoginStart(c *gin.Context) {
	provider := strings.ToLower(strings.TrimSpace(c.Param("provider")))
	if err := h.svc.SocialLoginStart(provider); err != nil {
		h.respondServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"provider": provider,
			"status":   "not_implemented",
			"message":  "OAuth start endpoint is scaffolded. Next step is provider configuration + PKCE flow.",
		},
	})
}

func (h *AuthHandler) SocialLoginCallback(c *gin.Context) {
	provider := strings.ToLower(strings.TrimSpace(c.Param("provider")))
	if err := h.svc.SocialLoginCallback(provider); err != nil {
		h.respondServiceError(c, err)
		return
	}

	var req SocialCallbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.respondError(c, apperrors.New(apperrors.CodeValidationFailed, err.Error()))
		return
	}

	c.JSON(http.StatusNotImplemented, gin.H{
		"error": apperrors.New(apperrors.CodeServiceUnavailable,
			"OAuth callback is scaffolded but not implemented yet"),
	})
}

func (h *AuthHandler) respondServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrUserAlreadyExists):
		h.respondError(c, apperrors.New(apperrors.CodeConflict, err.Error()))
	case errors.Is(err, service.ErrInvalidCredentials):
		h.respondError(c, apperrors.New(apperrors.CodeUnauthorized, err.Error()))
	case errors.Is(err, service.ErrWeakPassword):
		h.respondError(c, apperrors.New(apperrors.CodeValidationFailed, err.Error()))
	case errors.Is(err, service.ErrInvalidRefreshToken):
		h.respondError(c, apperrors.New(apperrors.CodeUnauthorized, err.Error()))
	case errors.Is(err, service.ErrUnsupportedProvider):
		h.respondError(c, apperrors.New(apperrors.CodeValidationFailed, err.Error()))
	default:
		h.log.WithError(err).Error().Msg("auth handler error")
		h.respondError(c, apperrors.InternalError(err))
	}
}

func (h *AuthHandler) respondError(c *gin.Context, err *apperrors.AppError) {
	c.JSON(err.HTTPStatus(), apperrors.ErrorResponse{Error: err})
}
