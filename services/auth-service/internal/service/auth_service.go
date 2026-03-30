package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/argon2"

	"github.com/egesarisac/SagaWallet/pkg/middleware"
)

var (
	ErrUserAlreadyExists   = errors.New("user already exists")
	ErrInvalidCredentials  = errors.New("invalid credentials")
	ErrWeakPassword        = errors.New("password must be at least 8 characters")
	ErrInvalidRefreshToken = errors.New("invalid refresh token")
	ErrUnsupportedProvider = errors.New("unsupported provider")
)

// User holds public user information.
type User struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Roles     []string  `json:"roles"`
	CreatedAt time.Time `json:"created_at"`
}

// AuthResult is returned after login and refresh operations.
type AuthResult struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	User         User   `json:"user"`
}

// Config configures auth service behavior.
type Config struct {
	JWTSecret       string
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
	Issuer          string
}

type userRecord struct {
	User
	PasswordHash string
}

type refreshSession struct {
	UserID    string
	ExpiresAt time.Time
}

// AuthService provides auth business logic.
type AuthService struct {
	cfg   Config
	store AuthStore
}

// NewAuthService creates an auth service instance.
func NewAuthService(cfg Config, store AuthStore) *AuthService {
	if cfg.AccessTokenTTL <= 0 {
		cfg.AccessTokenTTL = 15 * time.Minute
	}
	if cfg.RefreshTokenTTL <= 0 {
		cfg.RefreshTokenTTL = 7 * 24 * time.Hour
	}
	if cfg.Issuer == "" {
		cfg.Issuer = "sagawallet-auth"
	}
	if store == nil {
		store = NewInMemoryStore()
	}

	return &AuthService{
		cfg:   cfg,
		store: store,
	}
}

// Register creates a user with local email/password credentials.
func (s *AuthService) Register(ctx context.Context, email, password string) (User, error) {
	normalizedEmail := normalizeEmail(email)
	if normalizedEmail == "" || !strings.Contains(normalizedEmail, "@") {
		return User{}, ErrInvalidCredentials
	}
	if len(password) < 8 {
		return User{}, ErrWeakPassword
	}

	hash, err := hashPassword(password)
	if err != nil {
		return User{}, err
	}

	if _, err := s.store.GetUserByEmail(ctx, normalizedEmail); err == nil {
		return User{}, ErrUserAlreadyExists
	} else if !errors.Is(err, ErrNotFound) {
		return User{}, err
	}

	now := time.Now().UTC()
	newUser := User{
		ID:        uuid.NewString(),
		Email:     normalizedEmail,
		Roles:     []string{"user"},
		CreatedAt: now,
	}
	if err := s.store.CreateUser(ctx, newUser, hash); err != nil {
		if errors.Is(err, ErrUserAlreadyExists) {
			return User{}, ErrUserAlreadyExists
		}
		return User{}, err
	}

	return newUser, nil
}

// Login validates local credentials and returns access+refresh tokens.
func (s *AuthService) Login(ctx context.Context, email, password string) (AuthResult, error) {
	normalizedEmail := normalizeEmail(email)

	record, err := s.store.GetUserByEmail(ctx, normalizedEmail)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return AuthResult{}, ErrInvalidCredentials
		}
		return AuthResult{}, err
	}

	if record.Email == "" {
		return AuthResult{}, ErrInvalidCredentials
	}
	if !verifyPassword(record.PasswordHash, password) {
		return AuthResult{}, ErrInvalidCredentials
	}

	return s.issueTokens(ctx, record.User)
}

// Refresh rotates refresh token and returns a new access+refresh pair.
func (s *AuthService) Refresh(ctx context.Context, refreshToken string) (AuthResult, error) {
	incomingHash := hashRefreshToken(refreshToken)

	session, err := s.store.GetRefreshSession(ctx, incomingHash)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return AuthResult{}, ErrInvalidRefreshToken
		}
		return AuthResult{}, err
	}

	if session.UserID == "" {
		return AuthResult{}, ErrInvalidRefreshToken
	}
	if session.ExpiresAt.Before(time.Now().UTC()) {
		_ = s.store.DeleteRefreshSession(ctx, incomingHash)
		return AuthResult{}, ErrInvalidRefreshToken
	}

	record, err := s.store.GetUserByID(ctx, session.UserID)
	if err != nil {
		_ = s.store.DeleteRefreshSession(ctx, incomingHash)
		if errors.Is(err, ErrNotFound) {
			return AuthResult{}, ErrInvalidRefreshToken
		}
		return AuthResult{}, err
	}

	if record.ID == "" {
		return AuthResult{}, ErrInvalidRefreshToken
	}

	if err := s.store.DeleteRefreshSession(ctx, incomingHash); err != nil && !errors.Is(err, ErrNotFound) {
		return AuthResult{}, err
	}

	accessToken, expiresIn, err := s.generateAccessToken(record.User)
	if err != nil {
		return AuthResult{}, err
	}

	newRefresh, newRefreshHash, err := generateRefreshToken()
	if err != nil {
		return AuthResult{}, err
	}

	if err := s.store.UpsertRefreshSession(
		ctx,
		newRefreshHash,
		record.ID,
		time.Now().UTC().Add(s.cfg.RefreshTokenTTL),
	); err != nil {
		return AuthResult{}, err
	}

	return AuthResult{
		AccessToken:  accessToken,
		RefreshToken: newRefresh,
		TokenType:    "Bearer",
		ExpiresIn:    expiresIn,
		User:         record.User,
	}, nil
}

// Logout revokes a refresh token.
func (s *AuthService) Logout(ctx context.Context, refreshToken string) {
	err := s.store.DeleteRefreshSession(ctx, hashRefreshToken(refreshToken))
	if err != nil && !errors.Is(err, ErrNotFound) {
		return
	}
}

// SocialLoginStart validates provider support and returns start metadata.
func (s *AuthService) SocialLoginStart(provider string) error {
	if provider != "google" && provider != "apple" {
		return ErrUnsupportedProvider
	}
	return nil
}

// SocialLoginCallback validates provider support for callback path.
func (s *AuthService) SocialLoginCallback(provider string) error {
	if provider != "google" && provider != "apple" {
		return ErrUnsupportedProvider
	}
	return nil
}

func (s *AuthService) issueTokens(ctx context.Context, user User) (AuthResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	accessToken, expiresIn, err := s.generateAccessToken(user)
	if err != nil {
		return AuthResult{}, err
	}

	refreshToken, refreshHash, err := generateRefreshToken()
	if err != nil {
		return AuthResult{}, err
	}

	if err := s.store.UpsertRefreshSession(
		ctx,
		refreshHash,
		user.ID,
		time.Now().UTC().Add(s.cfg.RefreshTokenTTL),
	); err != nil {
		return AuthResult{}, err
	}

	return AuthResult{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    expiresIn,
		User:         user,
	}, nil
}

func (s *AuthService) generateAccessToken(user User) (string, int64, error) {
	now := time.Now().UTC()
	expiresAt := now.Add(s.cfg.AccessTokenTTL)

	claims := middleware.JWTClaims{
		UserID: user.ID,
		Roles:  user.Roles,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.ID,
			Issuer:    s.cfg.Issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(s.cfg.JWTSecret))
	if err != nil {
		return "", 0, err
	}

	return tokenString, int64(s.cfg.AccessTokenTTL.Seconds()), nil
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func generateRefreshToken() (token string, tokenHash string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}

	token = base64.RawURLEncoding.EncodeToString(buf)
	return token, hashRefreshToken(token), nil
}

func hashRefreshToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func hashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}

	memory := uint32(64 * 1024)
	iterations := uint32(3)
	parallelism := uint8(2)
	keyLen := uint32(32)

	hash := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, keyLen)

	encodedSalt := base64.RawStdEncoding.EncodeToString(salt)
	encodedHash := base64.RawStdEncoding.EncodeToString(hash)

	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", memory, iterations, parallelism, encodedSalt, encodedHash), nil
}

func verifyPassword(encodedHash string, password string) bool {
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}

	var memory uint32
	var iterations uint32
	var parallelism uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &parallelism); err != nil {
		return false
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	decodedHash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false
	}

	candidate := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, uint32(len(decodedHash)))
	return subtle.ConstantTimeCompare(decodedHash, candidate) == 1
}
