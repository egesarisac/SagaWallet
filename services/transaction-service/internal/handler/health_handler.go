// Package handler provides HTTP handlers for transaction service.
package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Dependencies holds shared dependencies for handlers.
type Dependencies struct {
	DB *pgxpool.Pool
}

var deps *Dependencies

// SetDependencies sets the shared dependencies.
func SetDependencies(d *Dependencies) {
	deps = d
}

// HealthHandler handles health check endpoints.
type HealthHandler struct{}

// NewHealthHandler creates a new health handler.
func NewHealthHandler() *HealthHandler {
	return &HealthHandler{}
}

// RegisterRoutes registers health check routes.
func (h *HealthHandler) RegisterRoutes(r *gin.Engine) {
	r.GET("/health", h.Health)
	r.GET("/ready", h.Ready)
}

// Health handles GET /health (liveness check)
func (h *HealthHandler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "healthy",
		"service": "transaction-service",
	})
}

// Ready handles GET /ready (readiness check)
func (h *HealthHandler) Ready(c *gin.Context) {
	checks := make(map[string]string)

	// Check database
	if deps != nil && deps.DB != nil {
		if err := deps.DB.Ping(c.Request.Context()); err != nil {
			checks["database"] = "error: " + err.Error()
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "unhealthy",
				"checks": checks,
			})
			return
		}
		checks["database"] = "ok"
	} else {
		checks["database"] = "not configured"
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "ready",
		"checks": checks,
	})
}
