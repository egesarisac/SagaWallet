package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"

	"github.com/egesarisac/SagaWallet/pkg/config"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("expected 'up' or 'down' argument")
	}
	cmd := os.Args[1]

	cfg, err := config.Load("transaction-service")
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	dbURL := cfg.GetDSN()
	// golang-migrate expects a URL (e.g. postgres://...). Our config DSN() is a libpq
	// string (host=... port=...); convert it unless the user already provided a URL.
	if !strings.HasPrefix(dbURL, "postgres://") && !strings.HasPrefix(dbURL, "postgresql://") {
		dbURL = fmt.Sprintf(
			"postgres://%s:%s@%s:%d/%s?sslmode=%s",
			cfg.DB.User,
			cfg.DB.Password,
			cfg.DB.Host,
			cfg.DB.Port,
			cfg.DB.Name,
			cfg.DB.SSLMode,
		)
	}

	m, err := migrate.New(
		"file://db/migrations",
		dbURL,
	)
	if err != nil {
		log.Fatalf("failed to create migrate instance: %v", err)
	}

	if cmd == "up" {
		if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			log.Fatalf("failed to run migrate up: %v", err)
		}
		fmt.Println("Transaction migrations applied successfully!")
	} else if cmd == "down" {
		if err := m.Down(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			log.Fatalf("failed to run migrate down: %v", err)
		}
		fmt.Println("Transaction migrations rolled back successfully!")
	} else {
		log.Fatalf("unknown command: %s", cmd)
	}
}
