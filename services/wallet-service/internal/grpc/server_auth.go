package grpc

import (
	"context"
	"crypto/subtle"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/egesarisac/SagaWallet/pkg/logger"
)

const (
	serviceTokenHeader = "x-service-token"
	serviceNameHeader  = "x-service-name"
)

// ServiceAuthUnaryInterceptor enforces token-based auth for internal gRPC calls.
func ServiceAuthUnaryInterceptor(expectedToken string, log *logger.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if expectedToken == "" {
			log.Error().Str("grpc_method", info.FullMethod).Msg("gRPC service auth misconfigured: missing expected token")
			return nil, status.Error(codes.Unauthenticated, "service authentication is not configured")
		}

		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "missing service auth metadata")
		}

		tokens := md.Get(serviceTokenHeader)
		if len(tokens) == 0 || tokens[0] == "" {
			return nil, status.Error(codes.Unauthenticated, "missing service token")
		}

		if subtle.ConstantTimeCompare([]byte(tokens[0]), []byte(expectedToken)) != 1 {
			caller := "unknown"
			if names := md.Get(serviceNameHeader); len(names) > 0 && names[0] != "" {
				caller = names[0]
			}
			log.Warn().Str("grpc_method", info.FullMethod).Str("caller", caller).Msg("Rejected unauthorized gRPC caller")
			return nil, status.Error(codes.PermissionDenied, "invalid service token")
		}

		return handler(ctx, req)
	}
}
