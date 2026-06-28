package sessionmemory

import (
	"errors"
	"sort"
	"strings"
	"time"
)

type MemoryScope string

const (
	MemoryScopeUser    MemoryScope = "user"
	MemoryScopeProject MemoryScope = "project"
	MemoryScopeLocal   MemoryScope = "local"
	MemoryScopeSession MemoryScope = "session"
	MemoryScopeAgent   MemoryScope = "agent"
)

type UpdatePolicy string

const (
	UpdatePolicyReplace      UpdatePolicy = "replace"
	UpdatePolicyMerge        UpdatePolicy = "merge"
	UpdatePolicyKeepExisting UpdatePolicy = "keep_existing"
)

var (
	ErrInvalidScope        = errors.New("invalid memory scope")
	ErrInvalidUpdatePolicy = errors.New("invalid update policy")
	ErrInvalidMemoryEntry  = errors.New("invalid memory entry")
	ErrMemoryNotFound      = errors.New("memory entry not found")
)

type MemoryEntry struct {
	Key             string            `json:"key"`
	Scope           MemoryScope       `json:"scope"`
	Content         string            `json:"content"`
	Source          string            `json:"source,omitempty"`
	SessionID       string            `json:"session_id,omitempty"`
	AgentID         string            `json:"agent_id,omitempty"`
	Tags            []string          `json:"tags,omitempty"`
	Metadata        map[string]string `json:"metadata,omitempty"`
	SensitiveFields map[string]string `json:"-"`
	Sensitive       bool              `json:"sensitive,omitempty"`
	CreatedAt       time.Time         `json:"created_at,omitempty"`
	UpdatedAt       time.Time         `json:"updated_at,omitempty"`
}

type MemoryRef struct {
	Key       string
	Scope     MemoryScope
	SessionID string
	AgentID   string
}

type MemoryFilter struct {
	Scopes    []MemoryScope
	SessionID string
	AgentID   string
	Query     string
	Limit     int
}

type MemorySnapshot struct {
	Entries   []MemoryEntry
	CreatedAt time.Time
}

type ExtractionCandidate struct {
	Key       string
	Scope     MemoryScope
	SessionID string
	AgentID   string
	Reason    string
	Stats     TranscriptStats
}

type TranscriptStats struct {
	MessageCount                 int
	TokenCount                   int
	TokensAtLastExtraction       int
	ToolCallsSinceLastExtraction int
	LastAssistantHasToolCalls    bool
	LastMessageID                string
	MemoryInitialized            bool
}

type ExtractionThresholds struct {
	MinimumTokensToInit        int
	MinimumTokensBetweenUpdate int
	ToolCallsBetweenUpdates    int
}

type ExtractionState struct {
	MemoryInitialized      bool
	TokensAtLastExtraction int
	LastExtractedMessageID string
}

type ExtractionPlan struct {
	ShouldExtract bool
	Candidate     ExtractionCandidate
	Reason        string
	Stats         TranscriptStats
	Thresholds    ExtractionThresholds
	NextState     ExtractionState
}

type PromptInjection struct {
	Section    string
	Injected   bool
	EntryCount int
	Truncated  bool
	Sources    []string
}

func NormalizeScope(scope MemoryScope) MemoryScope {
	return MemoryScope(strings.ToLower(strings.TrimSpace(string(scope))))
}

func (scope MemoryScope) IsValid() bool {
	switch NormalizeScope(scope) {
	case MemoryScopeUser, MemoryScopeProject, MemoryScopeLocal, MemoryScopeSession, MemoryScopeAgent:
		return true
	default:
		return false
	}
}

func ValidateEntry(entry MemoryEntry) error {
	_, err := NormalizeEntry(entry)
	return err
}

func NormalizeEntry(entry MemoryEntry) (MemoryEntry, error) {
	normalized := entry
	normalized.Scope = NormalizeScope(normalized.Scope)
	if !normalized.Scope.IsValid() {
		return MemoryEntry{}, ErrInvalidScope
	}

	normalized.Key = normalizeKey(normalized.Key)
	if normalized.Key == "" {
		return MemoryEntry{}, errors.Join(ErrInvalidMemoryEntry, errors.New("memory key is required"))
	}

	normalized.Content = strings.TrimSpace(normalized.Content)
	if normalized.Content == "" {
		return MemoryEntry{}, errors.Join(ErrInvalidMemoryEntry, errors.New("memory content is required"))
	}

	normalized.Source = normalizeLabel(normalized.Source)
	normalized.SessionID = strings.TrimSpace(normalized.SessionID)
	normalized.AgentID = strings.TrimSpace(normalized.AgentID)
	switch normalized.Scope {
	case MemoryScopeSession:
		if normalized.SessionID == "" {
			return MemoryEntry{}, errors.Join(ErrInvalidMemoryEntry, errors.New("session memory requires a session id"))
		}
		normalized.AgentID = ""
	case MemoryScopeAgent:
		if normalized.AgentID == "" {
			return MemoryEntry{}, errors.Join(ErrInvalidMemoryEntry, errors.New("agent memory requires an agent id"))
		}
	default:
		normalized.SessionID = ""
		normalized.AgentID = ""
	}

	normalized.Tags = normalizeTags(normalized.Tags)
	normalized.Metadata = normalizeMap(normalized.Metadata)
	normalized.SensitiveFields = normalizeMap(normalized.SensitiveFields)
	return normalized, nil
}

func NormalizeRef(ref MemoryRef) (MemoryRef, error) {
	normalized := ref
	normalized.Scope = NormalizeScope(normalized.Scope)
	if !normalized.Scope.IsValid() {
		return MemoryRef{}, ErrInvalidScope
	}
	normalized.Key = normalizeKey(normalized.Key)
	if normalized.Key == "" {
		return MemoryRef{}, errors.Join(ErrInvalidMemoryEntry, errors.New("memory key is required"))
	}
	normalized.SessionID = strings.TrimSpace(normalized.SessionID)
	normalized.AgentID = strings.TrimSpace(normalized.AgentID)
	switch normalized.Scope {
	case MemoryScopeSession:
		if normalized.SessionID == "" {
			return MemoryRef{}, errors.Join(ErrInvalidMemoryEntry, errors.New("session memory requires a session id"))
		}
		normalized.AgentID = ""
	case MemoryScopeAgent:
		if normalized.AgentID == "" {
			return MemoryRef{}, errors.Join(ErrInvalidMemoryEntry, errors.New("agent memory requires an agent id"))
		}
	default:
		normalized.SessionID = ""
		normalized.AgentID = ""
	}
	return normalized, nil
}

func NormalizeUpdatePolicy(policy UpdatePolicy) (UpdatePolicy, error) {
	if strings.TrimSpace(string(policy)) == "" {
		return UpdatePolicyReplace, nil
	}
	normalized := UpdatePolicy(strings.ToLower(strings.TrimSpace(string(policy))))
	switch normalized {
	case UpdatePolicyReplace, UpdatePolicyMerge, UpdatePolicyKeepExisting:
		return normalized, nil
	default:
		return "", ErrInvalidUpdatePolicy
	}
}

func (entry MemoryEntry) Ref() MemoryRef {
	return MemoryRef{
		Key:       entry.Key,
		Scope:     entry.Scope,
		SessionID: entry.SessionID,
		AgentID:   entry.AgentID,
	}
}

func normalizeKey(key string) string {
	return strings.ToLower(strings.TrimSpace(key))
}

func normalizeLabel(label string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(label)), " ")
}

func normalizeTags(tags []string) []string {
	if len(tags) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(tags))
	result := make([]string, 0, len(tags))
	for _, tag := range tags {
		normalized := strings.ToLower(strings.TrimSpace(tag))
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	sort.Strings(result)
	return result
}

func normalizeMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	result := make(map[string]string, len(values))
	for key, value := range values {
		normalizedKey := strings.TrimSpace(key)
		if normalizedKey == "" {
			continue
		}
		result[normalizedKey] = strings.TrimSpace(value)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func copyEntry(entry MemoryEntry) MemoryEntry {
	copied := entry
	copied.Tags = append([]string(nil), entry.Tags...)
	copied.Metadata = copyMap(entry.Metadata)
	copied.SensitiveFields = copyMap(entry.SensitiveFields)
	return copied
}

func copyMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	copied := make(map[string]string, len(values))
	for key, value := range values {
		copied[key] = value
	}
	return copied
}
