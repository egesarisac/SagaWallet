// Command outbox-retry makes a failed transaction outbox record immediately retryable.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/egesarisac/SagaWallet/pkg/config"
	"github.com/egesarisac/SagaWallet/services/transaction-service/internal/repository"
)

func main() {
	idText := flag.String("id", "", "outbox event UUID")
	reason := flag.String("reason", "", "operator reason for retrying the event")
	actor := flag.String("actor", defaultActor(), "operator identity recorded in the audit log")
	flag.Parse()

	eventID, err := uuid.Parse(strings.TrimSpace(*idText))
	if err != nil {
		fatal(fmt.Errorf("-id must be a valid outbox event UUID: %w", err))
	}
	if strings.TrimSpace(*reason) == "" {
		fatal(fmt.Errorf("-reason is required"))
	}
	if strings.TrimSpace(*actor) == "" {
		fatal(fmt.Errorf("-actor is required"))
	}

	cfg, err := config.Load("transaction-service")
	if err != nil {
		fatal(fmt.Errorf("load transaction service configuration: %w", err))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, cfg.GetDSN())
	if err != nil {
		fatal(fmt.Errorf("connect to transaction database: %w", err))
	}
	defer pool.Close()

	retried, err := repository.NewTransferRepository(pool).RetryOutbox(ctx, eventID, strings.TrimSpace(*actor), strings.TrimSpace(*reason))
	if err != nil {
		fatal(err)
	}
	if !retried {
		fatal(fmt.Errorf("outbox event %s does not exist or is already published", eventID))
	}
	fmt.Printf("scheduled outbox event %s for retry\n", eventID)
}

func defaultActor() string {
	if actor := strings.TrimSpace(os.Getenv("USER")); actor != "" {
		return actor
	}
	return "operator"
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
