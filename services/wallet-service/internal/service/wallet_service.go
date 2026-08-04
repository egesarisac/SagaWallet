// Package service provides business logic for wallet operations.
package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	apperrors "github.com/egesarisac/SagaWallet/pkg/errors"
	"github.com/egesarisac/SagaWallet/pkg/logger"
	db "github.com/egesarisac/SagaWallet/services/wallet-service/db/generated"
	"github.com/egesarisac/SagaWallet/services/wallet-service/internal/repository"
)

// WalletService handles wallet business logic.
type WalletService struct {
	repo *repository.WalletRepository
	log  *logger.Logger
}

// NewWalletService creates a new wallet service.
func NewWalletService(repo *repository.WalletRepository, log *logger.Logger) *WalletService {
	return &WalletService{
		repo: repo,
		log:  log,
	}
}

// Helper to extract balance as string from pgtype.Numeric
func numericToString(n pgtype.Numeric) string {
	if !n.Valid {
		return "0.00"
	}
	val, _ := n.Value()
	if val == nil {
		return "0.00"
	}
	return val.(string)
}

// Helper to extract UUID from pgtype.UUID
func pgtypeToUUID(p pgtype.UUID) uuid.UUID {
	if !p.Valid {
		return uuid.Nil
	}
	return p.Bytes
}

// CreateWalletInput is the input for creating a wallet.
type CreateWalletInput struct {
	UserID   uuid.UUID
	Currency string
}

// CreateWallet creates a new wallet for a user.
func (s *WalletService) CreateWallet(ctx context.Context, input CreateWalletInput) (*db.Wallet, error) {
	if input.Currency == "" {
		input.Currency = "TRY"
	}

	wallet, err := s.repo.CreateWallet(ctx, input.UserID, input.Currency)
	if err != nil {
		s.log.WithUserID(input.UserID.String()).
			WithError(err).
			Error().Msg("Failed to create wallet")
		return nil, err
	}

	s.log.WithUserID(input.UserID.String()).
		WithWalletID(pgtypeToUUID(wallet.ID).String()).
		Info().Msg("Wallet created")

	return wallet, nil
}

// GetWallet retrieves a wallet by ID.
func (s *WalletService) GetWallet(ctx context.Context, walletID uuid.UUID) (*db.Wallet, error) {
	return s.repo.GetWalletByID(ctx, walletID)
}

// GetWalletByUser retrieves a wallet by user ID.
func (s *WalletService) GetWalletByUser(ctx context.Context, userID uuid.UUID) (*db.Wallet, error) {
	return s.repo.GetWalletByUserID(ctx, userID)
}

// GetBalance retrieves the current balance of a wallet.
func (s *WalletService) GetBalance(ctx context.Context, walletID uuid.UUID) (string, string, error) {
	wallet, err := s.repo.GetWalletByID(ctx, walletID)
	if err != nil {
		return "", "", err
	}
	return numericToString(wallet.Balance), wallet.Currency, nil
}

// CreditInput is the input for crediting a wallet.
type CreditInput struct {
	WalletID    uuid.UUID
	Amount      string
	ReferenceID uuid.UUID
	Description string
}

// Credit adds funds to a wallet with audit trail.
func (s *WalletService) Credit(ctx context.Context, input CreditInput) (*db.Wallet, error) {
	wallet, _, err := s.applyCredit(ctx, uuid.Nil, "", input)
	return wallet, err
}

// DebitInput is the input for debiting a wallet.
type DebitInput struct {
	WalletID    uuid.UUID
	Amount      string
	ReferenceID uuid.UUID
	Description string
}

// Debit removes funds from a wallet with balance validation and audit trail.
func (s *WalletService) Debit(ctx context.Context, input DebitInput) (*db.Wallet, error) {
	wallet, _, err := s.applyDebit(ctx, uuid.Nil, "", input)
	return wallet, err
}

// CreditForEvent applies a consumer side effect once for the supplied event ID.
func (s *WalletService) CreditForEvent(ctx context.Context, eventID uuid.UUID, topic string, input CreditInput) (*db.Wallet, bool, error) {
	return s.applyCredit(ctx, eventID, topic, input)
}

// DebitForEvent applies a consumer side effect once for the supplied event ID.
func (s *WalletService) DebitForEvent(ctx context.Context, eventID uuid.UUID, topic string, input DebitInput) (*db.Wallet, bool, error) {
	return s.applyDebit(ctx, eventID, topic, input)
}

func (s *WalletService) applyCredit(ctx context.Context, eventID uuid.UUID, topic string, input CreditInput) (*db.Wallet, bool, error) {
	if input.Amount == "" || input.Amount == "0" || input.Amount == "0.00" {
		return nil, false, apperrors.InvalidAmount(input.Amount)
	}
	wallet, duplicate, err := s.repo.ApplyBalanceChange(ctx, eventID, topic, input.WalletID, input.Amount, "CREDIT", input.ReferenceID, input.Description)
	if err != nil {
		s.log.WithWalletID(input.WalletID.String()).WithAmount(input.Amount).WithError(err).Error().Msg("Failed to credit wallet")
		return nil, false, err
	}
	return wallet, duplicate, nil
}

func (s *WalletService) applyDebit(ctx context.Context, eventID uuid.UUID, topic string, input DebitInput) (*db.Wallet, bool, error) {
	if input.Amount == "" || input.Amount == "0" || input.Amount == "0.00" {
		return nil, false, apperrors.InvalidAmount(input.Amount)
	}
	wallet, duplicate, err := s.repo.ApplyBalanceChange(ctx, eventID, topic, input.WalletID, input.Amount, "DEBIT", input.ReferenceID, input.Description)
	if err != nil {
		s.log.WithWalletID(input.WalletID.String()).WithAmount(input.Amount).WithError(err).Error().Msg("Failed to debit wallet")
		return nil, false, err
	}
	return wallet, duplicate, nil
}

// GetTransactions retrieves paginated transactions for a wallet.
func (s *WalletService) GetTransactions(ctx context.Context, walletID uuid.UUID, limit, offset int32) ([]db.WalletTransaction, error) {
	// Verify wallet exists
	_, err := s.repo.GetWalletByID(ctx, walletID)
	if err != nil {
		return nil, err
	}

	return s.repo.ListWalletTransactions(ctx, walletID, limit, offset)
}

// UpdateStatus updates the status of a wallet.
func (s *WalletService) UpdateStatus(ctx context.Context, walletID uuid.UUID, status string) (*db.Wallet, error) {
	// Validate status
	validStatuses := map[string]bool{
		"ACTIVE":    true,
		"SUSPENDED": true,
		"FROZEN":    true,
		"CLOSED":    true,
	}
	if !validStatuses[status] {
		return nil, apperrors.New(apperrors.CodeValidationFailed, "Invalid status")
	}

	wallet, err := s.repo.UpdateWalletStatus(ctx, walletID, status)
	if err != nil {
		return nil, err
	}

	s.log.WithWalletID(walletID.String()).
		WithField("status", status).
		Info().Msg("Wallet status updated")

	return wallet, nil
}

// DeleteWallet deletes a wallet and its transactions.
func (s *WalletService) DeleteWallet(ctx context.Context, walletID uuid.UUID) error {
	// Delete transactions first (foreign key constraint)
	if err := s.repo.DeleteWalletTransactions(ctx, walletID); err != nil {
		s.log.WithWalletID(walletID.String()).
			WithError(err).
			Warn().Msg("Failed to delete wallet transactions")
	}

	// Delete the wallet
	if err := s.repo.DeleteWallet(ctx, walletID); err != nil {
		s.log.WithWalletID(walletID.String()).
			WithError(err).
			Error().Msg("Failed to delete wallet")
		return err
	}

	s.log.WithWalletID(walletID.String()).
		Info().Msg("Wallet deleted")

	return nil
}
