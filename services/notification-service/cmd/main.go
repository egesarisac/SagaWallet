// Notification Service - Main Entry Point
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/egesarisac/SagaWallet/pkg/config"
	"github.com/egesarisac/SagaWallet/pkg/kafka"
	"github.com/egesarisac/SagaWallet/pkg/logger"
	"github.com/egesarisac/SagaWallet/pkg/models"
	"github.com/egesarisac/SagaWallet/services/notification-service/internal/consumer"
	"github.com/egesarisac/SagaWallet/services/notification-service/internal/notifier"
)

func main() {
	// Load configuration
	cfg, err := config.Load("notification-service")
	if err != nil {
		fmt.Printf("Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	log := logger.New(logger.Config{
		Level:       cfg.Log.Level,
		Format:      cfg.Log.Format,
		ServiceName: cfg.ServiceName,
	})

	log.Info().Msg("Starting notification service...")

	// Create Kafka producer (for DLQ)
	producer := kafka.NewProducer(kafka.ProducerConfig{
		Brokers:  cfg.Kafka.Brokers,
		Username: cfg.Kafka.Username,
		Password: cfg.Kafka.Password,
		TLS:      cfg.Kafka.TLS,
	}, log)
	defer producer.Close()
	log.Info().Msg("Kafka producer initialized")

	// Create notifier
	notif := notifier.NewNotifier(log)

	// Create Kafka consumer
	kafkaConsumer := kafka.NewConsumer(kafka.ConsumerConfig{
		Brokers:  cfg.Kafka.Brokers,
		Username: cfg.Kafka.Username,
		Password: cfg.Kafka.Password,
		TLS:      cfg.Kafka.TLS,
		GroupID:  "notification-service",
		Topics: []string{
			models.TopicTransferCompleted,
			models.TopicTransferFailed,
		},
	}, producer, log)

	// Create Notification Consumer handler
	notificationConsumer := consumer.NewNotificationConsumer(kafkaConsumer, notif, log)

	// Start Kafka consumer in background
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		log.Info().Msg("Starting Notification Consumer...")
		if err := notificationConsumer.Start(ctx); err != nil {
			log.WithError(err).Error().Msg("Notification consumer error")
		}
	}()

	// Setup Gin router (health checks only)
	if cfg.Log.Level != "debug" {
		gin.SetMode(gin.ReleaseMode)
	}
	router := gin.New()
	router.Use(gin.Recovery())

	// Health check endpoints
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "healthy",
			"service": "notification-service",
		})
	})
	router.GET("/ready", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "ready",
		})
	})

	// Start HTTP server
	httpAddr := fmt.Sprintf(":%d", cfg.HTTPPort)
	server := &http.Server{
		Addr:    httpAddr,
		Handler: router,
	}

	go func() {
		log.Info().Str("addr", httpAddr).Msg("HTTP server starting")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.WithError(err).Fatal().Msg("HTTP server error")
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info().Msg("Shutting down...")
	cancel() // Stop Kafka consumer

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.WithError(err).Error().Msg("Server shutdown error")
	}

	kafkaConsumer.Close()
	log.Info().Msg("Notification service stopped")
}
