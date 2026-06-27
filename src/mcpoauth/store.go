package mcpoauth

import (
	"context"
	"sync"
)

type PendingSessionStore interface {
	Save(ctx context.Context, session AuthSession) error
	Consume(ctx context.Context, state string) (AuthSession, error)
	Delete(ctx context.Context, state string) error
}

type InMemoryPendingSessionStore struct {
	mu       sync.Mutex
	sessions map[string]AuthSession
	clock    Clock
}

func NewInMemoryPendingSessionStore(clock Clock) *InMemoryPendingSessionStore {
	return &InMemoryPendingSessionStore{
		sessions: make(map[string]AuthSession),
		clock:    clock,
	}
}

func (s *InMemoryPendingSessionStore) Save(ctx context.Context, session AuthSession) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := session.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.sessions[session.State]; exists {
		return ErrSessionAlreadyExists
	}
	s.sessions[session.State] = session
	return nil
}

func (s *InMemoryPendingSessionStore) Consume(ctx context.Context, state string) (AuthSession, error) {
	if err := ctx.Err(); err != nil {
		return AuthSession{}, err
	}
	if state == "" {
		return AuthSession{}, ErrMissingCallbackState
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[state]
	if !ok {
		return AuthSession{}, ErrStateMismatch
	}
	delete(s.sessions, state)
	if !session.ExpiresAt.IsZero() && !s.clock.Now().Before(session.ExpiresAt) {
		return AuthSession{}, ErrSessionExpired
	}
	return session, nil
}

func (s *InMemoryPendingSessionStore) Delete(ctx context.Context, state string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, state)
	return nil
}
