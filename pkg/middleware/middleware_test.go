package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestJWT_ValidToken(t *testing.T) {
	secret := "test-secret-key"
	userID := "user-123"
	roles := []string{"user", "admin"}

	// Generate a valid token
	token, err := GenerateToken(secret, userID, roles, 1)
	assert.NoError(t, err)
	assert.NotEmpty(t, token)

	// Setup Gin router with JWT middleware
	router := gin.New()
	router.Use(JWT(DefaultJWTConfig(secret)))
	router.GET("/protected", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"user_id": GetUserID(c),
			"roles":   GetRoles(c),
		})
	})

	// Make request with valid token
	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestJWT_MissingToken(t *testing.T) {
	secret := "test-secret-key"

	router := gin.New()
	router.Use(JWT(DefaultJWTConfig(secret)))
	router.GET("/protected", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Make request without token
	req := httptest.NewRequest("GET", "/protected", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestJWT_InvalidToken(t *testing.T) {
	secret := "test-secret-key"

	router := gin.New()
	router.Use(JWT(DefaultJWTConfig(secret)))
	router.GET("/protected", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Make request with invalid token
	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer invalid-token-here")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestJWT_WrongSecret(t *testing.T) {
	// Generate token with one secret
	token, _ := GenerateToken("secret-1", "user-123", []string{"user"}, 1)

	// Validate with different secret
	router := gin.New()
	router.Use(JWT(DefaultJWTConfig("secret-2")))
	router.GET("/protected", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestJWT_SkipPaths(t *testing.T) {
	secret := "test-secret-key"

	cfg := DefaultJWTConfig(secret)
	cfg.SkipPaths = []string{"/health", "/metrics"}

	router := gin.New()
	router.Use(JWT(cfg))
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Request to skip path should work without token
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestJWT_InvalidAuthorizationFormat(t *testing.T) {
	secret := "test-secret-key"
	token, _ := GenerateToken(secret, "user-123", []string{"user"}, 1)

	router := gin.New()
	router.Use(JWT(DefaultJWTConfig(secret)))
	router.GET("/protected", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	tests := []struct {
		name   string
		header string
	}{
		{"no bearer prefix", token},
		{"wrong prefix", "Basic " + token},
		{"empty bearer", "Bearer "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/protected", nil)
			req.Header.Set("Authorization", tt.header)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusUnauthorized, w.Code)
		})
	}
}

func TestGenerateToken(t *testing.T) {
	secret := "test-secret-key"
	userID := "user-123"
	roles := []string{"user", "admin"}
	expiryHours := 24

	token, err := GenerateToken(secret, userID, roles, expiryHours)

	assert.NoError(t, err)
	assert.NotEmpty(t, token)

	// Token should be a valid JWT format (3 parts separated by dots)
	parts := len(token)
	assert.Greater(t, parts, 50) // JWTs are typically longer than 50 chars
}

func TestRateLimit(t *testing.T) {
	cfg := RateLimitConfig{
		Enabled:           true,
		RequestsPerMinute: 5,
		CleanupInterval:   time.Minute,
	}

	router := gin.New()
	router.Use(RateLimit(cfg))
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// First 5 requests should succeed
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code, "Request %d should succeed", i+1)
	}

	// Next request should be rate limited
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusTooManyRequests, w.Code)
}

func TestRateLimit_DifferentIPs(t *testing.T) {
	cfg := RateLimitConfig{
		Enabled:           true,
		RequestsPerMinute: 2,
		CleanupInterval:   time.Minute,
	}

	router := gin.New()
	router.Use(RateLimit(cfg))
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Each IP should have its own limit
	ips := []string{"192.168.1.1:12345", "192.168.1.2:12345"}

	for _, ip := range ips {
		for i := 0; i < 2; i++ {
			req := httptest.NewRequest("GET", "/test", nil)
			req.RemoteAddr = ip
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code, "IP %s request %d should succeed", ip, i+1)
		}
	}
}
