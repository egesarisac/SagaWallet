// Wallet Service - Main Entry Point
package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"

	pb "github.com/egesarisac/SagaWallet/api/gen/wallet"

	"github.com/egesarisac/SagaWallet/pkg/config"
	"github.com/egesarisac/SagaWallet/pkg/kafka"
	"github.com/egesarisac/SagaWallet/pkg/logger"
	"github.com/egesarisac/SagaWallet/pkg/middleware"
	"github.com/egesarisac/SagaWallet/pkg/models"
	"github.com/egesarisac/SagaWallet/pkg/tracing"

	"github.com/egesarisac/SagaWallet/services/wallet-service/internal/consumer"
	grpc_internal "github.com/egesarisac/SagaWallet/services/wallet-service/internal/grpc"
	"github.com/egesarisac/SagaWallet/services/wallet-service/internal/handler"
	"github.com/egesarisac/SagaWallet/services/wallet-service/internal/repository"
	"github.com/egesarisac/SagaWallet/services/wallet-service/internal/service"
)

func main() {
	// Load configuration
	cfg, err := config.Load("wallet-service")
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

	log.Info().Msg("Starting wallet service...")

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

	// Initialize layers
	walletRepo := repository.NewWalletRepository(dbPool)
	walletService := service.NewWalletService(walletRepo, log)
	walletHandler := handler.NewWalletHandler(walletService, log)
	healthHandler := handler.NewHealthHandler()

	// Set dependencies for health checks
	handler.SetDependencies(&handler.Dependencies{
		DB: dbPool,
	})

	// Create Kafka consumer (raw)
	kafkaConsumer := kafka.NewConsumer(kafka.ConsumerConfig{
		Brokers:  cfg.Kafka.Brokers,
		Username: cfg.Kafka.Username,
		Password: cfg.Kafka.Password,
		TLS:      cfg.Kafka.TLS,
		GroupID:  "wallet-service",
		Topics: []string{
			models.TopicTransferCreated,
			models.TopicTransferDebitSuccess,
			models.TopicTransferCreditFailed,
		},
	}, producer, log)

	// Create Wallet Consumer handler
	walletConsumer := consumer.NewWalletConsumer(kafkaConsumer, producer, walletService, log)

	// Start Kafka consumer in background
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		log.Info().Msg("Starting Wallet Consumer...")
		if err := walletConsumer.Start(ctx); err != nil {
			log.WithError(err).Error().Msg("Wallet consumer error")
		}
	}()

	// Setup Gin router
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
	healthHandler.RegisterRoutes(router)

	// Protected API routes (JWT auth)
	api := router.Group("/api/v1")
	if cfg.JWT.Secret != "" {
		api.Use(middleware.JWT(middleware.DefaultJWTConfig(cfg.JWT.Secret)))
	}
	walletHandler.RegisterRoutes(api)

	// Start gRPC server
	grpcAddr := fmt.Sprintf(":%d", cfg.GRPCPort)
	grpcListener, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		log.WithError(err).Fatal().Msg("Failed to listen for gRPC")
	}

	walletGrpcToken := os.Getenv("WALLET_GRPC_TOKEN")
	if strings.TrimSpace(walletGrpcToken) == "" {
		log.Fatal().Msg("WALLET_GRPC_TOKEN is required for wallet gRPC internal service authentication")
	}

	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(grpc_internal.ServiceAuthUnaryInterceptor(walletGrpcToken, log)),
	)
	pb.RegisterWalletServiceServer(grpcServer, grpc_internal.NewServer(walletService, log))

	go func() {
		log.Info().Str("addr", grpcAddr).Msg("gRPC server starting")
		if err := grpcServer.Serve(grpcListener); err != nil {
			log.WithError(err).Fatal().Msg("gRPC server error")
		}
	}()

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

	log.Info().Msg("Stopping gRPC server...")
	grpcServer.GracefulStop()

	kafkaConsumer.Close()
	log.Info().Msg("Wallet service stopped")
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
