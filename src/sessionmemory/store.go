package sessionmemory

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type Store struct {
	mu      sync.RWMutex
	entries map[string]MemoryEntry
	now     func() time.Time
}

func NewStore() *Store {
	return &Store{
		entries: make(map[string]MemoryEntry),
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
}

func (store *Store) Add(entry MemoryEntry, policy UpdatePolicy) (MemoryEntry, error) {
	normalized, err := NormalizeEntry(entry)
	if err != nil {
		return MemoryEntry{}, err
	}
	normalizedPolicy, err := NormalizeUpdatePolicy(policy)
	if err != nil {
		return MemoryEntry{}, err
	}

	now := store.now()
	if normalized.CreatedAt.IsZero() {
		normalized.CreatedAt = now
	}
	if normalized.UpdatedAt.IsZero() {
		normalized.UpdatedAt = normalized.CreatedAt
	}

	store.mu.Lock()
	defer store.mu.Unlock()

	id := memoryIdentity(normalized.Ref())
	if existing, ok := store.entries[id]; ok {
		updated := applyUpdatePolicy(existing, normalized, normalizedPolicy, now)
		store.entries[id] = copyEntry(updated)
		return copyEntry(updated), nil
	}

	store.entries[id] = copyEntry(normalized)
	return copyEntry(normalized), nil
}

func (store *Store) Update(entry MemoryEntry, policy UpdatePolicy) (MemoryEntry, error) {
	normalized, err := NormalizeEntry(entry)
	if err != nil {
		return MemoryEntry{}, err
	}
	normalizedPolicy, err := NormalizeUpdatePolicy(policy)
	if err != nil {
		return MemoryEntry{}, err
	}

	store.mu.Lock()
	defer store.mu.Unlock()

	id := memoryIdentity(normalized.Ref())
	existing, ok := store.entries[id]
	if !ok {
		return MemoryEntry{}, ErrMemoryNotFound
	}

	now := store.now()
	if normalized.CreatedAt.IsZero() {
		normalized.CreatedAt = existing.CreatedAt
	}
	if normalized.UpdatedAt.IsZero() {
		normalized.UpdatedAt = now
	}

	updated := applyUpdatePolicy(existing, normalized, normalizedPolicy, now)
	store.entries[id] = copyEntry(updated)
	return copyEntry(updated), nil
}

func (store *Store) Delete(ref MemoryRef) error {
	normalized, err := NormalizeRef(ref)
	if err != nil {
		return err
	}

	store.mu.Lock()
	defer store.mu.Unlock()

	id := memoryIdentity(normalized)
	if _, ok := store.entries[id]; !ok {
		return ErrMemoryNotFound
	}
	delete(store.entries, id)
	return nil
}

func (store *Store) List(filter MemoryFilter) ([]MemoryEntry, error) {
	normalizedFilter, err := normalizeFilter(filter)
	if err != nil {
		return nil, err
	}

	store.mu.RLock()
	defer store.mu.RUnlock()

	entries := make([]MemoryEntry, 0, len(store.entries))
	for _, entry := range store.entries {
		if matchesFilter(entry, normalizedFilter) {
			entries = append(entries, copyEntry(entry))
		}
	}
	sortMemoryEntries(entries)
	if normalizedFilter.Limit > 0 && len(entries) > normalizedFilter.Limit {
		entries = entries[:normalizedFilter.Limit]
	}
	return entries, nil
}

func (store *Store) SearchByScope(scope MemoryScope, query string, filter MemoryFilter) ([]MemoryEntry, error) {
	normalizedScope := NormalizeScope(scope)
	if !normalizedScope.IsValid() {
		return nil, ErrInvalidScope
	}
	filter.Scopes = []MemoryScope{normalizedScope}
	filter.Query = query
	return store.List(filter)
}

func (store *Store) Snapshot(filter MemoryFilter) (MemorySnapshot, error) {
	entries, err := store.List(filter)
	if err != nil {
		return MemorySnapshot{}, err
	}
	return MemorySnapshot{
		Entries:   entries,
		CreatedAt: store.now(),
	}, nil
}

func applyUpdatePolicy(existing, next MemoryEntry, policy UpdatePolicy, now time.Time) MemoryEntry {
	switch policy {
	case UpdatePolicyKeepExisting:
		return existing
	case UpdatePolicyMerge:
		merged := existing
		merged.Content = fitContentLimit(mergeContent(existing.Content, next.Content))
		merged.Source = mergeLabels(existing.Source, next.Source)
		merged.Tags = mergeTags(existing.Tags, next.Tags)
		merged.Metadata = mergeStringMaps(existing.Metadata, next.Metadata)
		merged.SensitiveFields = mergeStringMaps(existing.SensitiveFields, next.SensitiveFields)
		merged.Sensitive = false
		merged.UpdatedAt = now
		return merged
	default:
		replacement := next
		if replacement.CreatedAt.IsZero() {
			replacement.CreatedAt = existing.CreatedAt
		}
		if replacement.UpdatedAt.IsZero() {
			replacement.UpdatedAt = now
		}
		return replacement
	}
}

func normalizeFilter(filter MemoryFilter) (MemoryFilter, error) {
	normalized := filter
	normalized.SessionID = strings.TrimSpace(normalized.SessionID)
	normalized.AgentID = strings.TrimSpace(normalized.AgentID)
	normalized.Query = strings.ToLower(strings.TrimSpace(normalized.Query))
	if normalized.Limit < 0 {
		normalized.Limit = 0
	}
	if len(normalized.Scopes) == 0 {
		return normalized, nil
	}

	scopes := make([]MemoryScope, 0, len(normalized.Scopes))
	seen := make(map[MemoryScope]struct{}, len(normalized.Scopes))
	for _, scope := range normalized.Scopes {
		normalizedScope := NormalizeScope(scope)
		if !normalizedScope.IsValid() {
			return MemoryFilter{}, ErrInvalidScope
		}
		if _, ok := seen[normalizedScope]; ok {
			continue
		}
		seen[normalizedScope] = struct{}{}
		scopes = append(scopes, normalizedScope)
	}
	normalized.Scopes = scopes
	return normalized, nil
}

func matchesFilter(entry MemoryEntry, filter MemoryFilter) bool {
	if len(filter.Scopes) > 0 && !containsScope(filter.Scopes, entry.Scope) {
		return false
	}
	if entry.Scope == MemoryScopeSession && filter.SessionID != "" && entry.SessionID != filter.SessionID {
		return false
	}
	if entry.Scope == MemoryScopeAgent {
		if filter.AgentID != "" && entry.AgentID != filter.AgentID {
			return false
		}
		if filter.SessionID != "" && entry.SessionID != "" && entry.SessionID != filter.SessionID {
			return false
		}
	}
	if filter.Query == "" {
		return true
	}
	return entryContainsQuery(entry, filter.Query)
}

func containsScope(scopes []MemoryScope, target MemoryScope) bool {
	for _, scope := range scopes {
		if scope == target {
			return true
		}
	}
	return false
}

func entryContainsQuery(entry MemoryEntry, query string) bool {
	fields := []string{
		entry.Key,
		entry.Content,
		entry.Source,
		strings.Join(entry.Tags, " "),
	}
	return strings.Contains(strings.ToLower(strings.Join(fields, " ")), query)
}

func memoryIdentity(ref MemoryRef) string {
	normalized, err := NormalizeRef(ref)
	if err != nil {
		return ""
	}
	parts := []string{
		string(normalized.Scope),
		normalized.SessionID,
		normalized.AgentID,
		normalized.Key,
	}
	return strings.Join(parts, "\x00")
}

func sortMemoryEntries(entries []MemoryEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		left := entries[i]
		right := entries[j]
		if scopeRank(left.Scope) != scopeRank(right.Scope) {
			return scopeRank(left.Scope) < scopeRank(right.Scope)
		}
		if !left.CreatedAt.Equal(right.CreatedAt) {
			return left.CreatedAt.Before(right.CreatedAt)
		}
		if left.Key != right.Key {
			return left.Key < right.Key
		}
		if left.SessionID != right.SessionID {
			return left.SessionID < right.SessionID
		}
		return left.AgentID < right.AgentID
	})
}

func scopeRank(scope MemoryScope) int {
	switch scope {
	case MemoryScopeUser:
		return 0
	case MemoryScopeProject:
		return 1
	case MemoryScopeLocal:
		return 2
	case MemoryScopeSession:
		return 3
	case MemoryScopeAgent:
		return 4
	default:
		return 99
	}
}

func mergeContent(existing, next string) string {
	existing = strings.TrimSpace(existing)
	next = strings.TrimSpace(next)
	if existing == "" {
		return fitContentLimit(next)
	}
	if next == "" || existing == next {
		return fitContentLimit(existing)
	}
	return fitContentLimit(existing + "\n\n" + next)
}

func mergeLabels(existing, next string) string {
	existing = normalizeLabel(existing)
	next = normalizeLabel(next)
	if existing == "" {
		return next
	}
	if next == "" || existing == next {
		return existing
	}
	parts := strings.Split(existing, ", ")
	for _, part := range parts {
		if part == next {
			return existing
		}
	}
	return existing + ", " + next
}

func mergeTags(left, right []string) []string {
	return normalizeTags(append(append([]string(nil), left...), right...))
}

func mergeStringMaps(left, right map[string]string) map[string]string {
	if len(left) == 0 && len(right) == 0 {
		return nil
	}
	merged := make(map[string]string, len(left)+len(right))
	for key, value := range left {
		merged[key] = value
	}
	for key, value := range right {
		merged[key] = value
	}
	return merged
}

func IsMemoryNotFound(err error) bool {
	return errors.Is(err, ErrMemoryNotFound)
}

func fitContentLimit(content string) string {
	content = strings.TrimSpace(content)
	if runeLen(content) <= MaxMemoryContentChars {
		return content
	}
	return truncateRunes(content, MaxMemoryContentChars, "\n[truncated]")
}
