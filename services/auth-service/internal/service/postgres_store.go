package service

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStore persists users and refresh sessions in PostgreSQL.
type PostgresStore struct {
	db *pgxpool.Pool
}

// NewPostgresStore creates a PostgreSQL-backed auth store.
func NewPostgresStore(db *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{db: db}
}

func (s *PostgresStore) EnsureSchema(ctx context.Context) error {
	_, err := s.db.Exec(ctx, `
CREATE TABLE IF NOT EXISTS auth_users (
  id UUID PRIMARY KEY,
  email TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  roles TEXT[] NOT NULL,
  created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS auth_refresh_sessions (
  token_hash TEXT PRIMARY KEY,
  user_id UUID NOT NULL REFERENCES auth_users(id) ON DELETE CASCADE,
  expires_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_auth_refresh_sessions_user_id ON auth_refresh_sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_auth_refresh_sessions_expires_at ON auth_refresh_sessions(expires_at);
`)
	return err
}

func (s *PostgresStore) CreateUser(ctx context.Context, user User, passwordHash string) error {
	_, err := s.db.Exec(ctx,
		`INSERT INTO auth_users (id, email, password_hash, roles, created_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		user.ID, user.Email, passwordHash, user.Roles, user.CreatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrUserAlreadyExists
		}
		return err
	}
	return nil
}

func (s *PostgresStore) GetUserByEmail(ctx context.Context, email string) (userRecord, error) {
	var rec userRecord
	var roles []string

	err := s.db.QueryRow(ctx,
		`SELECT id, email, password_hash, roles, created_at
		 FROM auth_users
		 WHERE email = $1`,
		email,
	).Scan(&rec.ID, &rec.Email, &rec.PasswordHash, &roles, &rec.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return userRecord{}, ErrNotFound
		}
		return userRecord{}, err
	}

	rec.Roles = roles
	return rec, nil
}

func (s *PostgresStore) GetUserByID(ctx context.Context, userID string) (userRecord, error) {
	var rec userRecord
	var roles []string

	err := s.db.QueryRow(ctx,
		`SELECT id, email, password_hash, roles, created_at
		 FROM auth_users
		 WHERE id = $1`,
		userID,
	).Scan(&rec.ID, &rec.Email, &rec.PasswordHash, &roles, &rec.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return userRecord{}, ErrNotFound
		}
		return userRecord{}, err
	}

	rec.Roles = roles
	return rec, nil
}

func (s *PostgresStore) UpsertRefreshSession(ctx context.Context, tokenHash, userID string, expiresAt time.Time) error {
	_, err := s.db.Exec(ctx,
		`INSERT INTO auth_refresh_sessions (token_hash, user_id, expires_at, created_at)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (token_hash)
		 DO UPDATE SET user_id = EXCLUDED.user_id, expires_at = EXCLUDED.expires_at`,
		tokenHash, userID, expiresAt, time.Now().UTC(),
	)
	return err
}

func (s *PostgresStore) GetRefreshSession(ctx context.Context, tokenHash string) (refreshSession, error) {
	var session refreshSession
	err := s.db.QueryRow(ctx,
		`SELECT user_id, expires_at
		 FROM auth_refresh_sessions
		 WHERE token_hash = $1`,
		tokenHash,
	).Scan(&session.UserID, &session.ExpiresAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return refreshSession{}, ErrNotFound
		}
		return refreshSession{}, err
	}

	return session, nil
}

func (s *PostgresStore) DeleteRefreshSession(ctx context.Context, tokenHash string) error {
	res, err := s.db.Exec(ctx,
		`DELETE FROM auth_refresh_sessions
		 WHERE token_hash = $1`,
		tokenHash,
	)
	if err != nil {
		return err
	}

	if res.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
