// Transaction Service - Main Entry Point
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/egesarisac/SagaWallet/pkg/config"
	"github.com/egesarisac/SagaWallet/pkg/kafka"
	"github.com/egesarisac/SagaWallet/pkg/logger"
	"github.com/egesarisac/SagaWallet/pkg/middleware"
	"github.com/egesarisac/SagaWallet/pkg/models"
	"github.com/egesarisac/SagaWallet/pkg/tracing"
	"github.com/egesarisac/SagaWallet/services/transaction-service/internal/consumer"
	grpc_client "github.com/egesarisac/SagaWallet/services/transaction-service/internal/grpc"
	"github.com/egesarisac/SagaWallet/services/transaction-service/internal/handler"
	"github.com/egesarisac/SagaWallet/services/transaction-service/internal/repository"
	"github.com/egesarisac/SagaWallet/services/transaction-service/internal/service"
)

func main() {
	// Load configuration
	cfg, err := config.Load("transaction-service")
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

	log.Info().Msg("Starting transaction service...")

	// Setup Gin router early so Cloud Run sees the container listening on PORT.
	if cfg.Log.Level != "debug" {
		gin.SetMode(gin.ReleaseMode)
	}
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(requestLogger(log))

	// CORS for Swagger UI
	router.Use(middleware.CORS(middleware.DefaultCORSConfig()))

	// Apply Phase 5 middleware
	router.Use(middleware.Metrics())
	// Rate limiting - can be disabled via DISABLE_RATE_LIMIT env var
	rateLimitCfg := middleware.DefaultRateLimitConfig()
	if os.Getenv("DISABLE_RATE_LIMIT") == "true" {
		rateLimitCfg.Enabled = false
	}
	router.Use(middleware.RateLimit(rateLimitCfg))

	// Metrics endpoint
	router.GET("/metrics", middleware.MetricsHandler())

	// Register routes (health endpoints are public)
	healthHandler := handler.NewHealthHandler()
	healthHandler.RegisterRoutes(router)

	// Start HTTP server before connecting to external deps.
	httpAddr := fmt.Sprintf(":%d", cfg.HTTPPort)
	server := &http.Server{Addr: httpAddr, Handler: router}
	go func() {
		log.Info().Str("addr", httpAddr).Msg("HTTP server starting")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.WithError(err).Fatal().Msg("HTTP server error")
		}
	}()

	// Initialize OpenTelemetry tracer
	shutdownTracer, err := tracing.InitTracer(cfg.ServiceName)
	if err != nil {
		log.WithError(err).Warn().Msg("Failed to initialize tracer (non-fatal)")
	} else {
		defer shutdownTracer(context.Background()) //nolint:errcheck
		log.Info().Msg("OpenTelemetry tracer initialized")
	}

	// Connect to database
	dbPool, err := connectDB(cfg)
	if err != nil {
		log.WithError(err).Fatal().Msg("Failed to connect to database")
	}
	defer dbPool.Close()
	log.Info().Msg("Connected to database")

	// Create Kafka producer
	producer := kafka.NewProducer(kafka.ProducerConfig{
		Brokers:  cfg.Kafka.Brokers,
		Username: cfg.Kafka.Username,
		Password: cfg.Kafka.Password,
		TLS:      cfg.Kafka.TLS,
	}, log)
	defer producer.Close()
	log.Info().Msg("Kafka producer initialized")

	// Initialize Wallet gRPC Client
	walletGrpcAddr := "localhost:9081"
	if os.Getenv("WALLET_GRPC_ADDR") != "" {
		walletGrpcAddr = os.Getenv("WALLET_GRPC_ADDR")
	}

	walletGrpcToken := os.Getenv("WALLET_GRPC_TOKEN")
	if strings.TrimSpace(walletGrpcToken) == "" {
		log.Fatal().Msg("WALLET_GRPC_TOKEN is required for wallet gRPC internal service authentication")
	}

	walletClient, err := grpc_client.NewWalletClient(walletGrpcAddr, walletGrpcToken, log)
	if err != nil {
		log.WithError(err).Warn().Msg("Failed to connect to Wallet gRPC service (non-fatal)")
	} else {
		defer walletClient.Close()
		log.Info().Str("addr", walletGrpcAddr).Msg("Wallet gRPC client initialized")
	}

	// Initialize layers
	transferRepo := repository.NewTransferRepository(dbPool)
	transferService := service.NewTransferService(transferRepo, producer, walletClient, log)
	transferHandler := handler.NewTransferHandler(transferService, log)

	// Set dependencies for health checks
	handler.SetDependencies(&handler.Dependencies{DB: dbPool})

	// Create Kafka consumer (raw) - listens to saga events
	kafkaConsumer := kafka.NewConsumer(kafka.ConsumerConfig{
		Brokers:  cfg.Kafka.Brokers,
		Username: cfg.Kafka.Username,
		Password: cfg.Kafka.Password,
		TLS:      cfg.Kafka.TLS,
		GroupID:  "transaction-service",
		Topics: []string{
			models.TopicTransferDebitSuccess,
			models.TopicTransferDebitFailed,
			models.TopicTransferCreditSuccess,
			models.TopicTransferCreditFailed,
			models.TopicTransferRefundSuccess,
		},
	}, producer, log)

	// Create Transfer Consumer handler
	transferConsumer := consumer.NewTransferConsumer(kafkaConsumer, producer, transferService, log)

	// Start Kafka consumer in background
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		log.Info().Msg("Starting Transfer Consumer (Saga Observer)...")
		if err := transferConsumer.Start(ctx); err != nil {
			log.WithError(err).Error().Msg("Transfer consumer error")
		}
	}()

	// Start Timeout Worker in background
	timeoutWorker := service.NewTimeoutWorker(transferRepo, transferService, producer, log)
	go func() {
		timeoutWorker.Start(ctx, 10*time.Second)
	}()

	// Start DLQ Worker in background
	dlqWorker := service.NewDLQWorker(cfg.Kafka.Brokers, cfg.Kafka.GroupID, log)
	go func() {
		log.Info().Msg("Starting DLQ worker...")
		dlqWorker.Run(ctx)
	}()

	// Protected API routes (JWT auth)
	api := router.Group("/api/v1")
	if cfg.JWT.Secret != "" {
		api.Use(middleware.JWT(middleware.DefaultJWTConfig(cfg.JWT.Secret)))
	}
	transferHandler.RegisterRoutes(api)

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
	log.Info().Msg("Transaction service stopped")
}

func connectDB(cfg *config.Config) (*pgxpool.Pool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, cfg.GetDSN())
	if err != nil {
		return nil, err
	}

	if err := pool.Ping(ctx); err != nil {
		return nil, err
	}

	return pool, nil
}

func requestLogger(log *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path

		c.Next()

		log.WithField("method", c.Request.Method).
			WithField("path", path).
			WithField("status", c.Writer.Status()).
			WithDuration(time.Since(start)).
			Info().Msg("HTTP request")
	}
}
