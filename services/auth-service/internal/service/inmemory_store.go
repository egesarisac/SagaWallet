package service

import (
	"context"
	"sync"
	"time"
)

// InMemoryStore provides non-persistent storage, mainly for tests.
type InMemoryStore struct {
	mu              sync.RWMutex
	usersByEmail    map[string]userRecord
	usersByID       map[string]userRecord
	refreshSessions map[string]refreshSession
}

// NewInMemoryStore creates a fresh in-memory store.
func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		usersByEmail:    make(map[string]userRecord),
		usersByID:       make(map[string]userRecord),
		refreshSessions: make(map[string]refreshSession),
	}
}

func (s *InMemoryStore) EnsureSchema(_ context.Context) error {
	return nil
}

func (s *InMemoryStore) CreateUser(_ context.Context, user User, passwordHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.usersByEmail[user.Email]; exists {
		return ErrUserAlreadyExists
	}

	rec := userRecord{User: user, PasswordHash: passwordHash}
	s.usersByEmail[user.Email] = rec
	s.usersByID[user.ID] = rec

	return nil
}

func (s *InMemoryStore) GetUserByEmail(_ context.Context, email string) (userRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rec, ok := s.usersByEmail[email]
	if !ok {
		return userRecord{}, ErrNotFound
	}
	return rec, nil
}

func (s *InMemoryStore) GetUserByID(_ context.Context, userID string) (userRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rec, ok := s.usersByID[userID]
	if !ok {
		return userRecord{}, ErrNotFound
	}
	return rec, nil
}

func (s *InMemoryStore) UpsertRefreshSession(_ context.Context, tokenHash, userID string, expiresAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.refreshSessions[tokenHash] = refreshSession{
		UserID:    userID,
		ExpiresAt: expiresAt,
	}
	return nil
}

func (s *InMemoryStore) GetRefreshSession(_ context.Context, tokenHash string) (refreshSession, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, ok := s.refreshSessions[tokenHash]
	if !ok {
		return refreshSession{}, ErrNotFound
	}
	return session, nil
}

func (s *InMemoryStore) DeleteRefreshSession(_ context.Context, tokenHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.refreshSessions[tokenHash]; !exists {
		return ErrNotFound
	}
	delete(s.refreshSessions, tokenHash)
	return nil
}
