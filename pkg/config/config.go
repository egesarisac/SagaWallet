// Package config provides configuration management using Viper.
package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config holds all configuration for the application.
type Config struct {
	// Service identification
	ServiceName string `mapstructure:"SERVICE_NAME"`

	// HTTP Server
	HTTPPort int `mapstructure:"HTTP_PORT"`

	// Database DSN (full connection string)
	DBDSN string `mapstructure:"DB_DSN"`

	// gRPC Server
	GRPCPort int `mapstructure:"GRPC_PORT"`

	// Database
	DB DatabaseConfig

	// Kafka
	Kafka KafkaConfig

	// JWT
	JWT JWTConfig

	// Logging
	Log LogConfig
}

// DatabaseConfig holds database connection settings.
type DatabaseConfig struct {
	Host     string `mapstructure:"DB_HOST"`
	Port     int    `mapstructure:"DB_PORT"`
	User     string `mapstructure:"DB_USER"`
	Password string `mapstructure:"DB_PASSWORD"`
	Name     string `mapstructure:"DB_NAME"`
	SSLMode  string `mapstructure:"DB_SSLMODE"`
}

// DSN returns the database connection string.
func (d DatabaseConfig) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		d.Host, d.Port, d.User, d.Password, d.Name, d.SSLMode,
	)
}

// GetDSN returns the connection string, prioritizing the full DSN if provided.
func (c *Config) GetDSN() string {
	if c.DBDSN != "" {
		return c.DBDSN
	}
	return c.DB.DSN()
}

// KafkaConfig holds Kafka connection settings.
type KafkaConfig struct {
	Brokers       []string      `mapstructure:"KAFKA_BROKERS"`
	GroupID       string        `mapstructure:"KAFKA_GROUP_ID"`
	Username      string        `mapstructure:"KAFKA_USERNAME"`
	Password      string        `mapstructure:"KAFKA_PASSWORD"`
	TLS           bool          `mapstructure:"KAFKA_TLS"`
	RetryAttempts int           `mapstructure:"KAFKA_RETRY_ATTEMPTS"`
	RetryInterval time.Duration `mapstructure:"KAFKA_RETRY_INTERVAL"`
}

// JWTConfig holds JWT settings.
type JWTConfig struct {
	Secret      string        `mapstructure:"JWT_SECRET"`
	ExpiryHours time.Duration `mapstructure:"JWT_EXPIRY_HOURS"`
}

// LogConfig holds logging settings.
type LogConfig struct {
	Level  string `mapstructure:"LOG_LEVEL"`
	Format string `mapstructure:"LOG_FORMAT"`
}

// Load reads configuration from environment variables and config files.
func Load(serviceName string) (*Config, error) {
	v := viper.New()

	// Set defaults
	v.SetDefault("SERVICE_NAME", serviceName)
	v.SetDefault("HTTP_PORT", 8080)
	v.SetDefault("PORT", 8080) // Cloud Run convention
	v.SetDefault("GRPC_PORT", 9090)
	v.SetDefault("DB_HOST", "localhost")
	v.SetDefault("DB_PORT", 5432)
	v.SetDefault("DB_SSLMODE", "disable")
	v.SetDefault("KAFKA_BROKERS", "localhost:9092")
	v.SetDefault("KAFKA_GROUP_ID", "go-fintech")
	v.SetDefault("KAFKA_TLS", false)
	v.SetDefault("KAFKA_RETRY_ATTEMPTS", 5)
	v.SetDefault("KAFKA_RETRY_INTERVAL", "1s")
	v.SetDefault("JWT_EXPIRY_HOURS", 24)
	v.SetDefault("LOG_LEVEL", "info")
	v.SetDefault("LOG_FORMAT", "json")

	// Alias PORT to HTTP_PORT for Cloud Run compatibility
	if err := v.BindEnv("HTTP_PORT", "PORT"); err != nil {
		return nil, err
	}
	// Alias DATABASE_URL to DB_DSN
	if err := v.BindEnv("DB_DSN", "DATABASE_URL"); err != nil {
		return nil, err
	}
	if err := v.BindEnv("DB_DSN", "DB_DSN"); err != nil {
		return nil, err
	}

	// Read from environment
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Try to read config file (optional)
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath("./config")
	v.AddConfigPath(".")
	_ = v.ReadInConfig() // Ignore error if config file doesn't exist

	cfg := &Config{
		ServiceName: v.GetString("SERVICE_NAME"),
		HTTPPort:    v.GetInt("HTTP_PORT"),
		DBDSN:       v.GetString("DB_DSN"),
		GRPCPort:    v.GetInt("GRPC_PORT"),
		DB: DatabaseConfig{
			Host:     v.GetString("DB_HOST"),
			Port:     v.GetInt("DB_PORT"),
			User:     v.GetString("DB_USER"),
			Password: v.GetString("DB_PASSWORD"),
			Name:     v.GetString("DB_NAME"),
			SSLMode:  v.GetString("DB_SSLMODE"),
		},
		Kafka: KafkaConfig{
			Brokers:       strings.Split(v.GetString("KAFKA_BROKERS"), ","),
			GroupID:       v.GetString("KAFKA_GROUP_ID"),
			Username:      v.GetString("KAFKA_USERNAME"),
			Password:      v.GetString("KAFKA_PASSWORD"),
			TLS:           v.GetBool("KAFKA_TLS"),
			RetryAttempts: v.GetInt("KAFKA_RETRY_ATTEMPTS"),
			RetryInterval: v.GetDuration("KAFKA_RETRY_INTERVAL"),
		},
		JWT: JWTConfig{
			Secret:      v.GetString("JWT_SECRET"),
			ExpiryHours: time.Duration(v.GetInt("JWT_EXPIRY_HOURS")) * time.Hour,
		},
		Log: LogConfig{
			Level:  v.GetString("LOG_LEVEL"),
			Format: v.GetString("LOG_FORMAT"),
		},
	}

	return cfg, nil
}
