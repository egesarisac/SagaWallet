// Package middleware provides HTTP middleware for Gin.
package middleware

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// httpRequestsTotal counts HTTP requests by method, path, and status.
	httpRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "path", "status"},
	)

	// httpRequestDuration tracks request duration in seconds.
	httpRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)

	// httpRequestsInFlight tracks current in-flight requests.
	httpRequestsInFlight = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "http_requests_in_flight",
			Help: "Current number of HTTP requests being processed",
		},
	)

	// kafkaEventsProcessed counts Kafka events processed.
	kafkaEventsProcessed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kafka_events_processed_total",
			Help: "Total number of Kafka events processed",
		},
		[]string{"topic", "status"},
	)

	// transfersTotal counts transfers by status.
	transfersTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "transfers_total",
			Help: "Total number of transfers by final status",
		},
		[]string{"status"},
	)
)

// Metrics returns a Prometheus metrics middleware.
func Metrics() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip metrics endpoint itself
		if c.Request.URL.Path == "/metrics" {
			c.Next()
			return
		}

		start := time.Now()
		httpRequestsInFlight.Inc()

		c.Next()

		httpRequestsInFlight.Dec()
		duration := time.Since(start).Seconds()
		status := strconv.Itoa(c.Writer.Status())
		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}

		httpRequestsTotal.WithLabelValues(c.Request.Method, path, status).Inc()
		httpRequestDuration.WithLabelValues(c.Request.Method, path).Observe(duration)
	}
}

// MetricsHandler returns the Prometheus metrics HTTP handler.
func MetricsHandler() gin.HandlerFunc {
	h := promhttp.Handler()
	return func(c *gin.Context) {
		h.ServeHTTP(c.Writer, c.Request)
	}
}

// RecordKafkaEvent records a Kafka event processing result.
func RecordKafkaEvent(topic, status string) {
	kafkaEventsProcessed.WithLabelValues(topic, status).Inc()
}

// RecordTransfer records a transfer result.
func RecordTransfer(status string) {
	transfersTotal.WithLabelValues(status).Inc()
}
