package evalharness

import (
	"context"
	"fmt"
	"sync"
)

type EvidenceStore interface {
	AddEvidence(ctx context.Context, evidence Evidence) (Evidence, error)
}

type MemoryEvidenceStore struct {
	mu       sync.Mutex
	nextID   int
	evidence []Evidence
}

func NewMemoryEvidenceStore() *MemoryEvidenceStore {
	return &MemoryEvidenceStore{}
}

func (s *MemoryEvidenceStore) AddEvidence(ctx context.Context, evidence Evidence) (Evidence, error) {
	if err := ctx.Err(); err != nil {
		return Evidence{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	stored := cloneEvidence(evidence)
	if stored.ID == "" {
		s.nextID++
		stored.ID = fmt.Sprintf("EV-%04d", s.nextID)
	}
	s.evidence = append(s.evidence, stored)
	return cloneEvidence(stored), nil
}

func (s *MemoryEvidenceStore) ListEvidence() []Evidence {
	s.mu.Lock()
	defer s.mu.Unlock()

	items := make([]Evidence, 0, len(s.evidence))
	for _, item := range s.evidence {
		items = append(items, cloneEvidence(item))
	}
	return items
}

func (s *MemoryEvidenceStore) EvidenceByScenario(scenarioID string) []Evidence {
	s.mu.Lock()
	defer s.mu.Unlock()

	items := make([]Evidence, 0)
	for _, item := range s.evidence {
		if item.ScenarioID == scenarioID {
			items = append(items, cloneEvidence(item))
		}
	}
	return items
}

func cloneEvidence(evidence Evidence) Evidence {
	cloned := evidence
	if evidence.Metadata != nil {
		cloned.Metadata = make(map[string]string, len(evidence.Metadata))
		for key, value := range evidence.Metadata {
			cloned.Metadata[key] = value
		}
	}
	return cloned
}
