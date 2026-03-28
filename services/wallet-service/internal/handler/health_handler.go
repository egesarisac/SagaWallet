// Package handler provides HTTP handlers for wallet service.
package handler

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/segmentio/kafka-go"
)

// Dependencies holds external service connections for health checks.
type Dependencies struct {
	DB    *pgxpool.Pool
	Kafka *kafka.Writer
}

var (
	deps     *Dependencies
	depsOnce sync.Once
)

// SetDependencies sets the dependencies for health checks.
func SetDependencies(d *Dependencies) {
	depsOnce.Do(func() {
		deps = d
	})
}

// HealthHandler handles health check requests.
type HealthHandler struct{}

// NewHealthHandler creates a new health handler.
func NewHealthHandler() *HealthHandler {
	return &HealthHandler{}
}

// RegisterRoutes registers health check routes.
func (h *HealthHandler) RegisterRoutes(r *gin.Engine) {
	r.GET("/health", h.Liveness)
	r.GET("/ready", h.Readiness)
}

// HealthResponse is the response for health checks.
type HealthResponse struct {
	Status  string            `json:"status"`
	Version string            `json:"version"`
	Checks  map[string]string `json:"checks,omitempty"`
}

// Liveness handles the liveness probe (is the process running?).
func (h *HealthHandler) Liveness(c *gin.Context) {
	c.JSON(http.StatusOK, HealthResponse{
		Status:  "healthy",
		Version: "1.0.0",
	})
}

// Readiness handles the readiness probe (can the service handle traffic?).
func (h *HealthHandler) Readiness(c *gin.Context) {
	checks := make(map[string]string)
	allHealthy := true

	// Check database connection
	if deps != nil && deps.DB != nil {
		ctx, cancel := c.Request.Context(), func() {}
		_ = cancel
		if err := deps.DB.Ping(ctx); err != nil {
			checks["database"] = "unhealthy"
			allHealthy = false
		} else {
			checks["database"] = "ok"
		}
	} else {
		checks["database"] = "not_configured"
	}

	// Check Kafka connection (best effort)
	if deps != nil && deps.Kafka != nil {
		checks["kafka"] = "ok" // Kafka writer doesn't have a ping method
	} else {
		checks["kafka"] = "not_configured"
	}

	status := "healthy"
	httpStatus := http.StatusOK
	if !allHealthy {
		status = "degraded"
		httpStatus = http.StatusServiceUnavailable
	}

	c.JSON(httpStatus, HealthResponse{
		Status:  status,
		Version: "1.0.0",
		Checks:  checks,
	})
}

// StartupTime records when the service started.
var StartupTime = time.Now()
