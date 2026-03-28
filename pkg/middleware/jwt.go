// Package middleware provides HTTP middleware for Gin.
package middleware

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	apperrors "github.com/egesarisac/SagaWallet/pkg/errors"
)

// JWTConfig holds JWT middleware configuration.
type JWTConfig struct {
	Secret          string
	SkipPaths       []string // Paths to skip authentication (e.g., /health, /ready)
	ContextUserKey  string   // Key to store user ID in context
	ContextRolesKey string   // Key to store roles in context
}

// JWTClaims represents the JWT token claims.
type JWTClaims struct {
	UserID string   `json:"user_id"`
	Roles  []string `json:"roles"`
	jwt.RegisteredClaims
}

// DefaultJWTConfig returns a default JWT configuration.
func DefaultJWTConfig(secret string) JWTConfig {
	return JWTConfig{
		Secret:          secret,
		SkipPaths:       []string{"/health", "/ready", "/metrics"},
		ContextUserKey:  "user_id",
		ContextRolesKey: "roles",
	}
}

// JWT returns a JWT authentication middleware.
func JWT(cfg JWTConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip authentication for certain paths
		for _, path := range cfg.SkipPaths {
			if strings.HasPrefix(c.Request.URL.Path, path) {
				c.Next()
				return
			}
		}

		// Get token from Authorization header
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, apperrors.ErrorResponse{
				Error: apperrors.New(apperrors.CodeUnauthorized, "missing authorization header"),
			})
			return
		}

		// Extract token from "Bearer <token>"
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, apperrors.ErrorResponse{
				Error: apperrors.New(apperrors.CodeUnauthorized, "invalid authorization format"),
			})
			return
		}

		tokenString := parts[1]

		// Parse and validate token
		token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
			// Validate signing method
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return []byte(cfg.Secret), nil
		})

		if err != nil || !token.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, apperrors.ErrorResponse{
				Error: apperrors.New(apperrors.CodeUnauthorized, "invalid or expired token"),
			})
			return
		}

		// Extract claims
		claims, ok := token.Claims.(*JWTClaims)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, apperrors.ErrorResponse{
				Error: apperrors.New(apperrors.CodeUnauthorized, "invalid token claims"),
			})
			return
		}

		// Set user info in context
		c.Set(cfg.ContextUserKey, claims.UserID)
		c.Set(cfg.ContextRolesKey, claims.Roles)

		c.Next()
	}
}

// GenerateToken creates a new JWT token (for testing/admin purposes).
func GenerateToken(secret, userID string, roles []string, expiryHours int) (string, error) {
	claims := JWTClaims{
		UserID: userID,
		Roles:  roles,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(expiryHours) * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "go-fintech-microservices",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// GetUserID extracts user ID from Gin context.
func GetUserID(c *gin.Context) string {
	if userID, exists := c.Get("user_id"); exists {
		if id, ok := userID.(string); ok {
			return id
		}
	}
	return ""
}

// GetRoles extracts roles from Gin context.
func GetRoles(c *gin.Context) []string {
	if roles, exists := c.Get("roles"); exists {
		if r, ok := roles.([]string); ok {
			return r
		}
	}
	return nil
}
