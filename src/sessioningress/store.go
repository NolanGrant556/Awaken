package sessioningress

import (
	"context"
	"sync"
)

// HeadStore persists the local append-chain head for each remote session.
type HeadStore interface {
	LastUUID(ctx context.Context, sessionID string) (string, bool, error)
	SetLastUUID(ctx context.Context, sessionID string, uuid string) error
}

type MemoryHeadStore struct {
	mu    sync.Mutex
	heads map[string]string
}

func NewMemoryHeadStore() *MemoryHeadStore {
	return &MemoryHeadStore{
		heads: make(map[string]string),
	}
}

func (s *MemoryHeadStore) LastUUID(ctx context.Context, sessionID string) (string, bool, error) {
	if err := ctx.Err(); err != nil {
		return "", false, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	uuid, ok := s.heads[sessionID]
	return uuid, ok, nil
}

func (s *MemoryHeadStore) SetLastUUID(ctx context.Context, sessionID string, uuid string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if uuid == "" {
		delete(s.heads, sessionID)
		return nil
	}
	s.heads[sessionID] = uuid
	return nil
}
