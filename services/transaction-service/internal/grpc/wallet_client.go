package grpc

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	pb "github.com/egesarisac/SagaWallet/api/gen/wallet"
	"github.com/egesarisac/SagaWallet/pkg/circuitbreaker"
	"github.com/egesarisac/SagaWallet/pkg/logger"
)

// WalletClient is the gRPC client for interacting with the Wallet Service.
type WalletClient struct {
	client       pb.WalletServiceClient
	conn         *grpc.ClientConn
	log          *logger.Logger
	cb           *circuitbreaker.CircuitBreaker
	serviceToken string
}

// NewWalletClient creates a new Wallet gRPC client with a circuit breaker.
func NewWalletClient(address, serviceToken string, log *logger.Logger) (*WalletClient, error) {
	if strings.TrimSpace(serviceToken) == "" {
		return nil, fmt.Errorf("wallet gRPC service token is required")
	}

	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to create gRPC client for wallet service at %s: %w", address, err)
	}

	client := pb.NewWalletServiceClient(conn)
	cb := circuitbreaker.New(circuitbreaker.DefaultSettings("wallet-grpc"))

	return &WalletClient{
		client:       client,
		conn:         conn,
		log:          log,
		cb:           cb,
		serviceToken: serviceToken,
	}, nil
}

func (c *WalletClient) withAuthContext(ctx context.Context) (context.Context, context.CancelFunc) {
	rpcCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	md := metadata.Pairs(
		"x-service-token", c.serviceToken,
		"x-service-name", "transaction-service",
	)
	return metadata.NewOutgoingContext(rpcCtx, md), cancel
}

// Close closes the underlying gRPC connection.
func (c *WalletClient) Close() error {
	return c.conn.Close()
}

// GetBalance gets the balance of a specific wallet, protected by a circuit breaker.
func (c *WalletClient) GetBalance(ctx context.Context, walletID uuid.UUID) (string, string, error) {
	result, err := c.cb.Execute(func() (interface{}, error) {
		rpcCtx, cancel := c.withAuthContext(ctx)
		defer cancel()

		resp, err := c.client.GetBalance(rpcCtx, &pb.GetBalanceRequest{
			WalletId: walletID.String(),
		})
		if err != nil {
			c.log.WithError(err).WithField("wallet_id", walletID.String()).Error().Msg("gRPC GetBalance failed")
			return nil, err
		}
		return resp, nil
	})
	if err != nil {
		return "", "", err
	}

	resp := result.(*pb.GetBalanceResponse)
	return resp.GetBalance(), resp.GetCurrency(), nil
}

// GetWallet gets the complete wallet details, protected by a circuit breaker.
func (c *WalletClient) GetWallet(ctx context.Context, walletID uuid.UUID) (*pb.Wallet, error) {
	result, err := c.cb.Execute(func() (interface{}, error) {
		rpcCtx, cancel := c.withAuthContext(ctx)
		defer cancel()

		resp, err := c.client.GetWallet(rpcCtx, &pb.GetWalletRequest{
			WalletId: walletID.String(),
		})
		if err != nil {
			c.log.WithError(err).WithField("wallet_id", walletID.String()).Error().Msg("gRPC GetWallet failed")
			return nil, err
		}
		return resp, nil
	})
	if err != nil {
		return nil, err
	}

	return result.(*pb.GetWalletResponse).GetWallet(), nil
}
