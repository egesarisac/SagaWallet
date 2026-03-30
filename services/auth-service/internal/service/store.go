package service

import (
	"context"
	"errors"
	"time"
)

var ErrNotFound = errors.New("record not found")

// AuthStore defines persistence operations for auth entities.
type AuthStore interface {
	EnsureSchema(ctx context.Context) error
	CreateUser(ctx context.Context, user User, passwordHash string) error
	GetUserByEmail(ctx context.Context, email string) (userRecord, error)
	GetUserByID(ctx context.Context, userID string) (userRecord, error)
	UpsertRefreshSession(ctx context.Context, tokenHash, userID string, expiresAt time.Time) error
	GetRefreshSession(ctx context.Context, tokenHash string) (refreshSession, error)
	DeleteRefreshSession(ctx context.Context, tokenHash string) error
}
