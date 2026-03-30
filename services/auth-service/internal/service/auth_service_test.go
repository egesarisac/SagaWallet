package service

import (
	"context"
	"testing"
	"time"
)

func TestAuthLifecycle(t *testing.T) {
	svc := NewAuthService(Config{
		JWTSecret:       "test-secret",
		AccessTokenTTL:  10 * time.Minute,
		RefreshTokenTTL: time.Hour,
		Issuer:          "test-issuer",
	}, NewInMemoryStore())

	ctx := context.Background()
	_, err := svc.Register(ctx, "user@example.com", "strong-pass-123")
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	loginResult, err := svc.Login(ctx, "user@example.com", "strong-pass-123")
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}
	if loginResult.AccessToken == "" || loginResult.RefreshToken == "" {
		t.Fatal("expected both access and refresh tokens")
	}

	refreshResult, err := svc.Refresh(ctx, loginResult.RefreshToken)
	if err != nil {
		t.Fatalf("refresh failed: %v", err)
	}
	if refreshResult.AccessToken == "" || refreshResult.RefreshToken == "" {
		t.Fatal("expected refreshed tokens")
	}

	svc.Logout(ctx, refreshResult.RefreshToken)
	if _, err := svc.Refresh(ctx, refreshResult.RefreshToken); err == nil {
		t.Fatal("expected refresh to fail after logout")
	}
}

func TestDuplicateRegister(t *testing.T) {
	svc := NewAuthService(Config{JWTSecret: "test-secret"}, NewInMemoryStore())
	ctx := context.Background()

	if _, err := svc.Register(ctx, "dupe@example.com", "strong-pass-123"); err != nil {
		t.Fatalf("first register failed: %v", err)
	}
	if _, err := svc.Register(ctx, "dupe@example.com", "strong-pass-123"); err != ErrUserAlreadyExists {
		t.Fatalf("expected ErrUserAlreadyExists, got: %v", err)
	}
}

func TestInvalidLogin(t *testing.T) {
	svc := NewAuthService(Config{JWTSecret: "test-secret"}, NewInMemoryStore())
	ctx := context.Background()

	if _, err := svc.Register(ctx, "login@example.com", "strong-pass-123"); err != nil {
		t.Fatalf("register failed: %v", err)
	}
	if _, err := svc.Login(ctx, "login@example.com", "wrong-pass"); err != ErrInvalidCredentials {
		t.Fatalf("expected ErrInvalidCredentials, got: %v", err)
	}
}
