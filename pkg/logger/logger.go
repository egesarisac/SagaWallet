// Package logger provides structured logging using zerolog.
package logger

import (
	"io"
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Logger wraps zerolog.Logger with additional context.
type Logger struct {
	zerolog.Logger
}

// Config holds logger configuration.
type Config struct {
	Level       string // debug, info, warn, error
	Format      string // json, console
	ServiceName string
}

// New creates a new logger instance.
func New(cfg Config) *Logger {
	// Set log level
	level, err := zerolog.ParseLevel(cfg.Level)
	if err != nil {
		level = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(level)

	// Set output format
	var output io.Writer = os.Stdout
	if cfg.Format == "console" {
		output = zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: time.RFC3339,
		}
	}

	// Create logger with service context
	logger := zerolog.New(output).
		With().
		Timestamp().
		Str("service", cfg.ServiceName).
		Logger()

	// Set as global logger
	log.Logger = logger

	return &Logger{Logger: logger}
}

// WithTraceID returns a new logger with trace ID.
func (l *Logger) WithTraceID(traceID string) *Logger {
	return &Logger{
		Logger: l.With().Str("trace_id", traceID).Logger(),
	}
}

// WithSpanID returns a new logger with span ID.
func (l *Logger) WithSpanID(spanID string) *Logger {
	return &Logger{
		Logger: l.With().Str("span_id", spanID).Logger(),
	}
}

// WithWalletID returns a new logger with wallet ID.
func (l *Logger) WithWalletID(walletID string) *Logger {
	return &Logger{
		Logger: l.With().Str("wallet_id", walletID).Logger(),
	}
}

// WithTransferID returns a new logger with transfer ID.
func (l *Logger) WithTransferID(transferID string) *Logger {
	return &Logger{
		Logger: l.With().Str("transfer_id", transferID).Logger(),
	}
}

// WithUserID returns a new logger with user ID.
func (l *Logger) WithUserID(userID string) *Logger {
	return &Logger{
		Logger: l.With().Str("user_id", userID).Logger(),
	}
}

// WithAmount returns a new logger with amount.
func (l *Logger) WithAmount(amount string) *Logger {
	return &Logger{
		Logger: l.With().Str("amount", amount).Logger(),
	}
}

// WithDuration returns a new logger with duration in milliseconds.
func (l *Logger) WithDuration(d time.Duration) *Logger {
	return &Logger{
		Logger: l.With().Int64("duration_ms", d.Milliseconds()).Logger(),
	}
}

// WithError returns a new logger with error.
func (l *Logger) WithError(err error) *Logger {
	return &Logger{
		Logger: l.With().Err(err).Logger(),
	}
}

// WithField returns a new logger with a custom field.
func (l *Logger) WithField(key string, value interface{}) *Logger {
	return &Logger{
		Logger: l.With().Interface(key, value).Logger(),
	}
}

// Global returns the global logger instance.
func Global() *Logger {
	return &Logger{Logger: log.Logger}
}
