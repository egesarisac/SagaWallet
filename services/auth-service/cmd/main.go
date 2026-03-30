package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/egesarisac/SagaWallet/pkg/config"
	"github.com/egesarisac/SagaWallet/pkg/logger"
	"github.com/egesarisac/SagaWallet/pkg/middleware"
	"github.com/egesarisac/SagaWallet/services/auth-service/internal/handler"
	"github.com/egesarisac/SagaWallet/services/auth-service/internal/service"
)

func main() {
	cfg, err := config.Load("auth-service")
	if err != nil {
		fmt.Printf("failed to load config: %v\n", err)
		os.Exit(1)
	}
	if cfg.JWT.Secret == "" {
		fmt.Println("missing JWT_SECRET for auth-service")
		os.Exit(1)
	}

	log := logger.New(logger.Config{
		Level:       cfg.Log.Level,
		Format:      cfg.Log.Format,
		ServiceName: cfg.ServiceName,
	})
	log.Info().Msg("Starting auth service...")

	dbPool, err := connectDB(cfg)
	if err != nil {
		log.WithError(err).Fatal().Msg("Failed to connect to database")
	}
	defer dbPool.Close()

	store := service.NewPostgresStore(dbPool)
	initCtx, initCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer initCancel()
	if err := store.EnsureSchema(initCtx); err != nil {
		log.WithError(err).Fatal().Msg("Failed to initialize auth schema")
	}

	authSvc := service.NewAuthService(service.Config{
		JWTSecret:       cfg.JWT.Secret,
		AccessTokenTTL:  time.Duration(envInt("ACCESS_TOKEN_EXPIRY_MINUTES", 15)) * time.Minute,
		RefreshTokenTTL: time.Duration(envInt("REFRESH_TOKEN_EXPIRY_HOURS", 24*7)) * time.Hour,
		Issuer:          envString("JWT_ISSUER", "sagawallet-auth"),
	}, store)
	authHandler := handler.NewAuthHandler(authSvc, log)

	if cfg.Log.Level != "debug" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(requestLogger(log))
	router.Use(middleware.CORS(middleware.DefaultCORSConfig()))
	router.Use(middleware.Metrics())

	rateLimitCfg := middleware.DefaultRateLimitConfig()
	if os.Getenv("DISABLE_RATE_LIMIT") == "true" {
		rateLimitCfg.Enabled = false
	}
	router.Use(middleware.RateLimit(rateLimitCfg))

	router.GET("/metrics", middleware.MetricsHandler())
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "healthy",
			"service": "auth-service",
		})
	})
	router.GET("/ready", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ready"})
	})

	api := router.Group("/api/v1")
	authHandler.RegisterRoutes(api)

	httpAddr := fmt.Sprintf(":%d", cfg.HTTPPort)
	httpServer := &http.Server{
		Addr:    httpAddr,
		Handler: router,
	}

	go func() {
		log.Info().Str("addr", httpAddr).Msg("HTTP server starting")
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.WithError(err).Fatal().Msg("HTTP server error")
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info().Msg("Shutting down auth service...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		log.WithError(err).Error().Msg("Auth service shutdown error")
	}

	log.Info().Msg("Auth service stopped")
}

func envInt(key string, defaultValue int) int {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return defaultValue
	}
	return parsed
}

func connectDB(cfg *config.Config) (*pgxpool.Pool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, cfg.GetDSN())
	if err != nil {
		return nil, err
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}

	return pool, nil
}

func envString(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
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
