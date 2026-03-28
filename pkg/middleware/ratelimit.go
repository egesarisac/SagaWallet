// Package middleware provides HTTP middleware for Gin.
package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	apperrors "github.com/egesarisac/SagaWallet/pkg/errors"
)

// RateLimitConfig holds rate limiter configuration.
type RateLimitConfig struct {
	Enabled           bool          // Whether rate limiting is enabled
	RequestsPerMinute int           // Max requests per minute per IP
	CleanupInterval   time.Duration // How often to clean up old entries
}

// DefaultRateLimitConfig returns default rate limit configuration.
func DefaultRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		Enabled:           true,
		RequestsPerMinute: 100,
		CleanupInterval:   5 * time.Minute,
	}
}

// tokenBucket tracks rate limit state for a single IP.
type tokenBucket struct {
	tokens    int
	lastCheck time.Time
}

// rateLimiter manages rate limiting state.
type rateLimiter struct {
	buckets map[string]*tokenBucket
	mu      sync.RWMutex
	cfg     RateLimitConfig
}

// newRateLimiter creates a new rate limiter.
func newRateLimiter(cfg RateLimitConfig) *rateLimiter {
	rl := &rateLimiter{
		buckets: make(map[string]*tokenBucket),
		cfg:     cfg,
	}

	// Start cleanup goroutine
	go rl.cleanup()

	return rl
}

// allow checks if a request is allowed for the given IP.
func (rl *rateLimiter) allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	bucket, exists := rl.buckets[ip]

	if !exists {
		// New IP, create bucket with full tokens
		rl.buckets[ip] = &tokenBucket{
			tokens:    rl.cfg.RequestsPerMinute - 1,
			lastCheck: now,
		}
		return true
	}

	// Refill tokens based on time elapsed
	elapsed := now.Sub(bucket.lastCheck)
	tokensToAdd := int(elapsed.Minutes() * float64(rl.cfg.RequestsPerMinute))
	bucket.tokens += tokensToAdd
	if bucket.tokens > rl.cfg.RequestsPerMinute {
		bucket.tokens = rl.cfg.RequestsPerMinute
	}
	bucket.lastCheck = now

	// Check if request is allowed
	if bucket.tokens > 0 {
		bucket.tokens--
		return true
	}

	return false
}

// cleanup removes old entries periodically.
func (rl *rateLimiter) cleanup() {
	ticker := time.NewTicker(rl.cfg.CleanupInterval)
	for range ticker.C {
		rl.mu.Lock()
		cutoff := time.Now().Add(-rl.cfg.CleanupInterval)
		for ip, bucket := range rl.buckets {
			if bucket.lastCheck.Before(cutoff) {
				delete(rl.buckets, ip)
			}
		}
		rl.mu.Unlock()
	}
}

// RateLimit returns a rate limiting middleware.
func RateLimit(cfg RateLimitConfig) gin.HandlerFunc {
	// If rate limiting is disabled, return a no-op middleware
	if !cfg.Enabled {
		return func(c *gin.Context) {
			c.Next()
		}
	}

	limiter := newRateLimiter(cfg)

	return func(c *gin.Context) {
		// Get client IP
		ip := c.ClientIP()

		if !limiter.allow(ip) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, apperrors.ErrorResponse{
				Error: apperrors.New(apperrors.CodeRateLimitExceeded, "rate limit exceeded, please try again later"),
			})
			return
		}

		c.Next()
	}
}
