package assistantcore

import (
	"errors"
	"fmt"
	"sort"
	"sync"
)

var (
	ErrAssistantNotFound      = errors.New("assistant not found")
	ErrAssistantAlreadyExists = errors.New("assistant already exists")
	ErrAssistantReadOnly      = errors.New("assistant is read-only")
	ErrInvalidReorder         = errors.New("invalid assistant reorder")
)

type MemoryStore struct {
	mu         sync.RWMutex
	assistants map[string]Assistant
}

func NewMemoryStore(initial ...Assistant) (*MemoryStore, error) {
	store := &MemoryStore{
		assistants: make(map[string]Assistant, len(initial)),
	}
	for _, assistant := range initial {
		if _, err := store.Create(assistant); err != nil {
			return nil, err
		}
	}
	return store, nil
}

func (s *MemoryStore) Create(assistant Assistant) (Assistant, error) {
	normalized, err := NormalizeAssistant(assistant)
	if err != nil {
		return Assistant{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.assistants[normalized.ID]; ok {
		return Assistant{}, fmt.Errorf("%w: %s", ErrAssistantAlreadyExists, normalized.ID)
	}
	s.assistants[normalized.ID] = normalized
	return cloneAssistant(normalized), nil
}

func (s *MemoryStore) Update(assistant Assistant) (Assistant, error) {
	normalized, err := NormalizeAssistant(assistant)
	if err != nil {
		return Assistant{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.assistants[normalized.ID]
	if !ok {
		return Assistant{}, fmt.Errorf("%w: %s", ErrAssistantNotFound, normalized.ID)
	}
	if err := ValidateAssistantUpdate(existing, normalized); err != nil {
		return Assistant{}, err
	}
	s.assistants[normalized.ID] = normalized
	return cloneAssistant(normalized), nil
}

func (s *MemoryStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.assistants[id]
	if !ok {
		return fmt.Errorf("%w: %s", ErrAssistantNotFound, id)
	}
	if existing.Source != AssistantSourceUser {
		return fmt.Errorf("%w: %s", ErrAssistantReadOnly, id)
	}
	delete(s.assistants, id)
	return nil
}

func (s *MemoryStore) Get(id string) (Assistant, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	assistant, ok := s.assistants[id]
	if !ok {
		return Assistant{}, false
	}
	return cloneAssistant(assistant), true
}

func (s *MemoryStore) List() []Assistant {
	s.mu.RLock()
	defer s.mu.RUnlock()
	assistants := make([]Assistant, 0, len(s.assistants))
	for _, assistant := range s.assistants {
		assistants = append(assistants, cloneAssistant(assistant))
	}
	sortAssistants(assistants)
	return assistants
}

func (s *MemoryStore) Reorder(ids []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(ids) != len(s.assistants) {
		return fmt.Errorf("%w: expected %d ids, got %d", ErrInvalidReorder, len(s.assistants), len(ids))
	}

	seen := make(map[string]struct{}, len(ids))
	for index, id := range ids {
		assistant, ok := s.assistants[id]
		if !ok {
			return fmt.Errorf("%w: unknown id %s", ErrInvalidReorder, id)
		}
		if _, ok := seen[id]; ok {
			return fmt.Errorf("%w: duplicate id %s", ErrInvalidReorder, id)
		}
		seen[id] = struct{}{}
		assistant.State.SortOrder = index
		s.assistants[id] = assistant
	}
	return nil
}

func (s *MemoryStore) EnableDisable(id string, enabled bool) (Assistant, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	assistant, ok := s.assistants[id]
	if !ok {
		return Assistant{}, fmt.Errorf("%w: %s", ErrAssistantNotFound, id)
	}
	assistant.State.Enabled = enabled
	s.assistants[id] = assistant
	return cloneAssistant(assistant), nil
}

func sortAssistants(assistants []Assistant) {
	sort.SliceStable(assistants, func(i, j int) bool {
		left := assistants[i]
		right := assistants[j]
		if left.State.SortOrder != right.State.SortOrder {
			return left.State.SortOrder < right.State.SortOrder
		}
		if left.Profile.Name != right.Profile.Name {
			return left.Profile.Name < right.Profile.Name
		}
		return left.ID < right.ID
	})
}
