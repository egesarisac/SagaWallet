// Package repository provides database access layer for wallet service.
package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	apperrors "github.com/egesarisac/SagaWallet/pkg/errors"
	db "github.com/egesarisac/SagaWallet/services/wallet-service/db/generated"
)

// WalletRepository handles wallet database operations.
type WalletRepository struct {
	pool    *pgxpool.Pool
	queries *db.Queries
}

// NewWalletRepository creates a new wallet repository.
func NewWalletRepository(pool *pgxpool.Pool) *WalletRepository {
	return &WalletRepository{
		pool:    pool,
		queries: db.New(pool),
	}
}

// Helper functions for type conversion
func uuidToPgtype(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

func stringToNumeric(s string) pgtype.Numeric {
	var n pgtype.Numeric
	_ = n.Scan(s)
	return n
}

func numericToString(n pgtype.Numeric) string {
	if !n.Valid {
		return "0.00"
	}
	// Format the numeric value
	val, _ := n.Value()
	if val == nil {
		return "0.00"
	}
	return val.(string)
}

func pgtypeToUUID(p pgtype.UUID) uuid.UUID {
	if !p.Valid {
		return uuid.Nil
	}
	return p.Bytes
}

// CreateWallet creates a new wallet for a user.
func (r *WalletRepository) CreateWallet(ctx context.Context, userID uuid.UUID, currency string) (*db.Wallet, error) {
	wallet, err := r.queries.CreateWallet(ctx, db.CreateWalletParams{
		UserID:   uuidToPgtype(userID),
		Balance:  stringToNumeric("0.00"),
		Currency: currency,
		Status:   "ACTIVE",
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, apperrors.WalletAlreadyExists(fmt.Sprintf("wallet for user %s already exists", userID))
		}
		return nil, apperrors.Wrap(apperrors.CodeDatabaseError, "failed to create wallet", err)
	}
	return &wallet, nil
}

// GetWalletByID retrieves a wallet by its ID.
func (r *WalletRepository) GetWalletByID(ctx context.Context, walletID uuid.UUID) (*db.Wallet, error) {
	wallet, err := r.queries.GetWalletByID(ctx, uuidToPgtype(walletID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperrors.WalletNotFound(walletID.String())
		}
		return nil, apperrors.Wrap(apperrors.CodeDatabaseError, "failed to get wallet", err)
	}
	return &wallet, nil
}

// GetWalletByUserID retrieves a wallet by user ID.
func (r *WalletRepository) GetWalletByUserID(ctx context.Context, userID uuid.UUID) (*db.Wallet, error) {
	wallet, err := r.queries.GetWalletByUserID(ctx, uuidToPgtype(userID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperrors.WalletNotFound("user:" + userID.String())
		}
		return nil, apperrors.Wrap(apperrors.CodeDatabaseError, "failed to get wallet by user", err)
	}
	return &wallet, nil
}

// CreditWallet adds funds to a wallet with optimistic locking.
func (r *WalletRepository) CreditWallet(ctx context.Context, walletID uuid.UUID, amount string, version int64) (*db.Wallet, error) {
	wallet, err := r.queries.CreditWallet(ctx, db.CreditWalletParams{
		ID:      uuidToPgtype(walletID),
		Balance: stringToNumeric(amount),
		Version: version,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperrors.ConcurrentModification("wallet")
		}
		return nil, apperrors.Wrap(apperrors.CodeDatabaseError, "failed to credit wallet", err)
	}
	return &wallet, nil
}

// DebitWallet removes funds from a wallet with balance check and optimistic locking.
func (r *WalletRepository) DebitWallet(ctx context.Context, walletID uuid.UUID, amount string, version int64) (*db.Wallet, error) {
	wallet, err := r.queries.DebitWallet(ctx, db.DebitWalletParams{
		ID:      uuidToPgtype(walletID),
		Balance: stringToNumeric(amount),
		Version: version,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Determine the actual reason for failure
			existingWallet, getErr := r.queries.GetWalletByID(ctx, uuidToPgtype(walletID))
			if getErr != nil {
				if errors.Is(getErr, pgx.ErrNoRows) {
					return nil, apperrors.WalletNotFound(walletID.String())
				}
				return nil, apperrors.Wrap(apperrors.CodeDatabaseError, "failed to check wallet", getErr)
			}

			if existingWallet.Status != "ACTIVE" {
				return nil, apperrors.WalletFrozen(walletID.String())
			}
			if existingWallet.Version != version {
				return nil, apperrors.ConcurrentModification("wallet")
			}
			// Must be insufficient funds
			return nil, apperrors.InsufficientFunds(walletID.String(), amount, numericToString(existingWallet.Balance))
		}
		return nil, apperrors.Wrap(apperrors.CodeDatabaseError, "failed to debit wallet", err)
	}
	return &wallet, nil
}

// ApplyBalanceChange atomically mutates a balance and records its immutable ledger entry.
// When eventID is supplied, the inbox row and ledger mutation share the same transaction.
func (r *WalletRepository) ApplyBalanceChange(
	ctx context.Context,
	eventID uuid.UUID,
	topic string,
	walletID uuid.UUID,
	amount string,
	txType string,
	referenceID uuid.UUID,
	description string,
) (*db.Wallet, bool, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, false, apperrors.Wrap(apperrors.CodeDatabaseError, "failed to begin wallet transaction", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if eventID != uuid.Nil {
		var accepted uuid.UUID
		err := tx.QueryRow(ctx, `
			INSERT INTO processed_events (event_id, topic)
			VALUES ($1, $2)
			ON CONFLICT (event_id) DO NOTHING
			RETURNING event_id`, uuidToPgtype(eventID), topic).Scan(&accepted)
		if errors.Is(err, pgx.ErrNoRows) {
			wallet, getErr := getWalletForUpdate(ctx, tx, walletID)
			if getErr != nil {
				return nil, false, getErr
			}
			if err := tx.Commit(ctx); err != nil {
				return nil, false, apperrors.Wrap(apperrors.CodeDatabaseError, "failed to commit duplicate event", err)
			}
			return wallet, true, nil
		}
		if err != nil {
			return nil, false, apperrors.Wrap(apperrors.CodeDatabaseError, "failed to record processed event", err)
		}
	}

	wallet, err := getWalletForUpdate(ctx, tx, walletID)
	if err != nil {
		return nil, false, err
	}
	var ledgerExists bool
	if err := tx.QueryRow(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM wallet_transactions
				WHERE wallet_id = $1 AND reference_id = $2 AND type = $3
			)`, uuidToPgtype(walletID), uuidToPgtype(referenceID), txType).Scan(&ledgerExists); err != nil {
		return nil, false, apperrors.Wrap(apperrors.CodeDatabaseError, "failed to check wallet ledger idempotency", err)
	}
	if ledgerExists {
		if err := tx.Commit(ctx); err != nil {
			return nil, false, apperrors.Wrap(apperrors.CodeDatabaseError, "failed to commit duplicate wallet command", err)
		}
		return wallet, true, nil
	}
	if wallet.Status != "ACTIVE" {
		return nil, false, apperrors.WalletFrozen(walletID.String())
	}

	operator := "+"
	if txType == "DEBIT" {
		operator = "-"
	}
	query := `UPDATE wallets SET balance = balance ` + operator + ` $2, version = version + 1, updated_at = NOW() WHERE id = $1`
	if txType == "DEBIT" {
		query += ` AND balance >= $2`
	}
	query += ` RETURNING id, user_id, balance, currency, status, version, created_at, updated_at`
	updated, err := scanWallet(tx.QueryRow(ctx, query, uuidToPgtype(walletID), stringToNumeric(amount)))
	if errors.Is(err, pgx.ErrNoRows) && txType == "DEBIT" {
		return nil, false, apperrors.InsufficientFunds(walletID.String(), amount, numericToString(wallet.Balance))
	}
	if err != nil {
		return nil, false, apperrors.Wrap(apperrors.CodeDatabaseError, "failed to update wallet balance", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO wallet_transactions (wallet_id, amount, type, reference_id, description, balance_after)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		uuidToPgtype(walletID), stringToNumeric(amount), txType, uuidToPgtype(referenceID), description, updated.Balance); err != nil {
		return nil, false, apperrors.Wrap(apperrors.CodeDatabaseError, "failed to create wallet ledger entry", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, false, apperrors.Wrap(apperrors.CodeDatabaseError, "failed to commit wallet transaction", err)
	}
	return updated, false, nil
}

type walletRow interface {
	Scan(dest ...any) error
}

func getWalletForUpdate(ctx context.Context, tx pgx.Tx, walletID uuid.UUID) (*db.Wallet, error) {
	wallet, err := scanWallet(tx.QueryRow(ctx, `
		SELECT id, user_id, balance, currency, status, version, created_at, updated_at
		FROM wallets WHERE id = $1 FOR UPDATE`, uuidToPgtype(walletID)))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperrors.WalletNotFound(walletID.String())
	}
	if err != nil {
		return nil, apperrors.Wrap(apperrors.CodeDatabaseError, "failed to lock wallet", err)
	}
	return wallet, nil
}

func scanWallet(row walletRow) (*db.Wallet, error) {
	var wallet db.Wallet
	err := row.Scan(&wallet.ID, &wallet.UserID, &wallet.Balance, &wallet.Currency, &wallet.Status, &wallet.Version, &wallet.CreatedAt, &wallet.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &wallet, nil
}

// UpdateWalletStatus updates the status of a wallet.
func (r *WalletRepository) UpdateWalletStatus(ctx context.Context, walletID uuid.UUID, status string) (*db.Wallet, error) {
	wallet, err := r.queries.UpdateWalletStatus(ctx, db.UpdateWalletStatusParams{
		ID:     uuidToPgtype(walletID),
		Status: status,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperrors.WalletNotFound(walletID.String())
		}
		return nil, apperrors.Wrap(apperrors.CodeDatabaseError, "failed to update wallet status", err)
	}
	return &wallet, nil
}

// CreateWalletTransaction records an audit entry for a balance change.
func (r *WalletRepository) CreateWalletTransaction(
	ctx context.Context,
	walletID uuid.UUID,
	amount string,
	txType string,
	referenceID uuid.UUID,
	description string,
	balanceAfter string,
) (*db.WalletTransaction, error) {
	tx, err := r.queries.CreateWalletTransaction(ctx, db.CreateWalletTransactionParams{
		WalletID:     uuidToPgtype(walletID),
		Amount:       stringToNumeric(amount),
		Type:         txType,
		ReferenceID:  uuidToPgtype(referenceID),
		Description:  pgtype.Text{String: description, Valid: true},
		BalanceAfter: stringToNumeric(balanceAfter),
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			// This error typically indicates a unique constraint violation.
			// For wallet transactions, this might mean a duplicate reference ID for a specific wallet,
			// or another unique constraint.
			// The specific error message should reflect the context of a transaction.
			return nil, apperrors.Conflict(fmt.Sprintf("wallet transaction with reference ID %s already exists or another unique constraint violated", referenceID))
		}
		return nil, apperrors.Wrap(apperrors.CodeDatabaseError, "failed to create wallet transaction", err)
	}
	return &tx, nil
}

// ListWalletTransactions retrieves paginated transactions for a wallet.
func (r *WalletRepository) ListWalletTransactions(ctx context.Context, walletID uuid.UUID, limit, offset int32) ([]db.WalletTransaction, error) {
	transactions, err := r.queries.ListWalletTransactions(ctx, db.ListWalletTransactionsParams{
		WalletID: uuidToPgtype(walletID),
		Limit:    limit,
		Offset:   offset,
	})
	if err != nil {
		return nil, apperrors.Wrap(apperrors.CodeDatabaseError, "failed to list wallet transactions", err)
	}
	return transactions, nil
}

// GetWalletID extracts UUID from pgtype.UUID
func GetWalletID(wallet *db.Wallet) uuid.UUID {
	return pgtypeToUUID(wallet.ID)
}

// GetWalletBalance extracts balance as string
func GetWalletBalance(wallet *db.Wallet) string {
	return numericToString(wallet.Balance)
}

// DeleteWallet deletes a wallet by ID.
func (r *WalletRepository) DeleteWallet(ctx context.Context, walletID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, "DELETE FROM wallets WHERE id = $1", uuidToPgtype(walletID))
	if err != nil {
		return apperrors.Wrap(apperrors.CodeDatabaseError, "failed to delete wallet", err)
	}
	return nil
}

// DeleteWalletTransactions deletes all transactions for a wallet.
func (r *WalletRepository) DeleteWalletTransactions(ctx context.Context, walletID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, "DELETE FROM wallet_transactions WHERE wallet_id = $1", uuidToPgtype(walletID))
	if err != nil {
		return apperrors.Wrap(apperrors.CodeDatabaseError, "failed to delete wallet transactions", err)
	}
	return nil
}
