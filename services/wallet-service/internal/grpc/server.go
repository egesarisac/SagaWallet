package grpc

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/egesarisac/SagaWallet/api/gen/wallet"
	"github.com/egesarisac/SagaWallet/pkg/logger"
	"github.com/egesarisac/SagaWallet/services/wallet-service/internal/service"
)

// Server implements the WalletService gRPC server
type Server struct {
	pb.UnimplementedWalletServiceServer
	walletService *service.WalletService
	log           *logger.Logger
}

// NewServer creates a new gRPC server implementation for WalletService
func NewServer(walletService *service.WalletService, log *logger.Logger) *Server {
	return &Server{
		walletService: walletService,
		log:           log,
	}
}

// numericToString converts pgtype.Numeric to string
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

// GetBalance returns the current balance of a wallet.
func (s *Server) GetBalance(ctx context.Context, req *pb.GetBalanceRequest) (*pb.GetBalanceResponse, error) {
	walletID, err := uuid.Parse(req.GetWalletId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid wallet ID: %v", err)
	}

	balance, currency, err := s.walletService.GetBalance(ctx, walletID)
	if err != nil {
		s.log.WithError(err).Error().Msg("Failed to get balance via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to get balance: %v", err)
	}

	return &pb.GetBalanceResponse{
		WalletId: req.GetWalletId(),
		Balance:  balance,
		Currency: currency,
	}, nil
}

// Credit adds funds to a wallet.
func (s *Server) Credit(ctx context.Context, req *pb.CreditRequest) (*pb.CreditResponse, error) {
	walletID, err := uuid.Parse(req.GetWalletId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid wallet ID: %v", err)
	}

	var referenceID uuid.UUID
	if req.GetReferenceId() != "" {
		referenceID, err = uuid.Parse(req.GetReferenceId())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid reference ID: %v", err)
		}
	} else {
		referenceID = uuid.Nil
	}

	wallet, err := s.walletService.Credit(ctx, service.CreditInput{
		WalletID:    walletID,
		Amount:      req.GetAmount(),
		ReferenceID: referenceID,
		Description: req.GetDescription(),
	})
	if err != nil {
		s.log.WithError(err).Error().Msg("Failed to credit wallet via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to credit wallet: %v", err)
	}

	return &pb.CreditResponse{
		WalletId:      req.GetWalletId(),
		NewBalance:    numericToString(wallet.Balance),
		TransactionId: "", // Not explicitly returned by business layer currently
	}, nil
}

// Debit removes funds from a wallet.
func (s *Server) Debit(ctx context.Context, req *pb.DebitRequest) (*pb.DebitResponse, error) {
	walletID, err := uuid.Parse(req.GetWalletId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid wallet ID: %v", err)
	}

	var referenceID uuid.UUID
	if req.GetReferenceId() != "" {
		referenceID, err = uuid.Parse(req.GetReferenceId())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid reference ID: %v", err)
		}
	} else {
		referenceID = uuid.Nil
	}

	wallet, err := s.walletService.Debit(ctx, service.DebitInput{
		WalletID:    walletID,
		Amount:      req.GetAmount(),
		ReferenceID: referenceID,
		Description: req.GetDescription(),
	})
	if err != nil {
		s.log.WithError(err).Error().Msg("Failed to debit wallet via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to debit wallet: %v", err)
	}

	return &pb.DebitResponse{
		WalletId:      req.GetWalletId(),
		NewBalance:    numericToString(wallet.Balance),
		TransactionId: "", // Not explicitly returned by business layer currently
	}, nil
}

// GetWallet returns wallet details.
func (s *Server) GetWallet(ctx context.Context, req *pb.GetWalletRequest) (*pb.GetWalletResponse, error) {
	walletID, err := uuid.Parse(req.GetWalletId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid wallet ID: %v", err)
	}

	wallet, err := s.walletService.GetWallet(ctx, walletID)
	if err != nil {
		s.log.WithError(err).Error().Msg("Failed to get wallet via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to get wallet: %v", err)
	}

	var userID string
	if wallet.UserID.Valid {
		// Convert the [16]byte to a string representation of the UUID
		uid := uuid.UUID(wallet.UserID.Bytes)
		userID = uid.String()
	}

	var createdAt, updatedAt *timestamppb.Timestamp
	if wallet.CreatedAt.Valid {
		createdAt = timestamppb.New(wallet.CreatedAt.Time)
	}
	if wallet.UpdatedAt.Valid {
		updatedAt = timestamppb.New(wallet.UpdatedAt.Time)
	}

	return &pb.GetWalletResponse{
		Wallet: &pb.Wallet{
			Id:        req.GetWalletId(),
			UserId:    userID,
			Balance:   numericToString(wallet.Balance),
			Currency:  wallet.Currency,
			Status:    wallet.Status,
			Version:   int64(wallet.Version),
			CreatedAt: createdAt,
			UpdatedAt: updatedAt,
		},
	}, nil
}
