package memory

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const EnvStorePath = "AWAKEN_MEMORY_PATH"

var (
	ErrEmptyText    = errors.New("memory text is required")
	ErrEmptyScope   = errors.New("memory scope is required")
	ErrInvalidLimit = errors.New("memory limit must be positive")

	// Only redact formats with a low false-positive rate. Broader filtering
	// should live closer to the eventual write API and prompt/runtime boundary.
	highConfidenceSecretPatterns = []*regexp.Regexp{
		regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{12,}\b`),
		regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
	}
)

type MemoryRecord struct {
	ID        string    `json:"id"`
	Scope     string    `json:"scope"`
	Text      string    `json:"text"`
	Tags      []string  `json:"tags,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Source    string    `json:"source,omitempty"`
}

type AddRequest struct {
	Scope  string
	Text   string
	Tags   []string
	Source string
}

type SearchResult struct {
	MemoryRecord
	Score int `json:"score"`
}

type JSONLStore struct {
	mu      sync.RWMutex
	path    string
	records []MemoryRecord
}

func DefaultStorePath() (string, error) {
	path := strings.TrimSpace(os.Getenv(EnvStorePath))
	if path == "" {
		return "", nil
	}
	path = filepath.Clean(path)
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("%s must be an absolute path", EnvStorePath)
	}
	return path, nil
}

func NewDefaultStore() (*JSONLStore, error) {
	path, err := DefaultStorePath()
	if err != nil {
		return nil, err
	}
	if path == "" {
		return nil, nil
	}
	return NewJSONLStore(path)
}

func NewJSONLStore(path string) (*JSONLStore, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("memory store path is required")
	}

	store := &JSONLStore{path: filepath.Clean(path)}
	if err := store.load(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *JSONLStore) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

func (s *JSONLStore) Add(req AddRequest) (MemoryRecord, error) {
	if s == nil {
		return MemoryRecord{}, errors.New("memory store is nil")
	}

	scope, err := normalizeScope(req.Scope)
	if err != nil {
		return MemoryRecord{}, err
	}
	text := strings.TrimSpace(req.Text)
	if text == "" {
		return MemoryRecord{}, ErrEmptyText
	}

	now := time.Now().UTC()
	record := MemoryRecord{
		ID:        newMemoryID(now),
		Scope:     scope,
		Text:      redactHighConfidenceSecrets(text),
		Tags:      normalizeTags(redactHighConfidenceSecretTags(req.Tags)),
		CreatedAt: now,
		UpdatedAt: now,
		Source:    redactHighConfidenceSecrets(strings.TrimSpace(req.Source)),
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return MemoryRecord{}, err
	}
	file, err := os.OpenFile(s.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return MemoryRecord{}, err
	}
	defer file.Close()

	if err := json.NewEncoder(file).Encode(record); err != nil {
		return MemoryRecord{}, err
	}
	s.records = append(s.records, record)
	return record, nil
}

func (s *JSONLStore) List(scope string, limit int) ([]MemoryRecord, error) {
	if s == nil {
		return nil, errors.New("memory store is nil")
	}
	scope, err := normalizeScope(scope)
	if err != nil {
		return nil, err
	}
	limit, err = normalizeLimit(limit)
	if err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	var records []MemoryRecord
	for _, record := range s.records {
		if record.Scope != scope {
			continue
		}
		records = append(records, cloneRecord(record))
		if len(records) >= limit {
			break
		}
	}
	return records, nil
}

func (s *JSONLStore) Search(scope, query string, limit int) ([]SearchResult, error) {
	if s == nil {
		return nil, errors.New("memory store is nil")
	}
	scope, err := normalizeScope(scope)
	if err != nil {
		return nil, err
	}
	limit, err = normalizeLimit(limit)
	if err != nil {
		return nil, err
	}

	terms := tokenize(query)
	if len(terms) == 0 {
		return nil, ErrEmptyText
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []SearchResult
	for _, record := range s.records {
		if record.Scope != scope {
			continue
		}
		score := scoreRecord(record, terms)
		if score <= 0 {
			continue
		}
		results = append(results, SearchResult{
			MemoryRecord: cloneRecord(record),
			Score:        score,
		})
	}
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		if !results[i].UpdatedAt.Equal(results[j].UpdatedAt) {
			return results[i].UpdatedAt.After(results[j].UpdatedAt)
		}
		return results[i].ID < results[j].ID
	})
	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

func (s *JSONLStore) load() error {
	file, err := os.Open(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var record MemoryRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			continue
		}
		if record.ID == "" || strings.TrimSpace(record.Text) == "" || strings.TrimSpace(record.Scope) == "" {
			continue
		}
		record.Scope, err = normalizeScope(record.Scope)
		if err != nil {
			continue
		}
		record.Text = strings.TrimSpace(record.Text)
		record.Tags = normalizeTags(record.Tags)
		record.Source = strings.TrimSpace(record.Source)
		s.records = append(s.records, record)
	}
	return scanner.Err()
}

func normalizeScope(scope string) (string, error) {
	scope = strings.TrimSpace(scope)
	if scope == "" {
		return "", ErrEmptyScope
	}
	if filepath.IsAbs(scope) {
		return filepath.Clean(scope), nil
	}
	return scope, nil
}

func normalizeLimit(limit int) (int, error) {
	if limit == 0 {
		return 100, nil
	}
	if limit < 0 {
		return 0, ErrInvalidLimit
	}
	return limit, nil
}

func normalizeTags(tags []string) []string {
	seen := make(map[string]bool, len(tags))
	normalized := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = redactHighConfidenceSecrets(strings.TrimSpace(tag))
		if tag == "" || seen[tag] {
			continue
		}
		seen[tag] = true
		normalized = append(normalized, tag)
	}
	return normalized
}

func tokenize(query string) []string {
	fields := strings.Fields(strings.ToLower(strings.TrimSpace(query)))
	seen := make(map[string]bool, len(fields))
	terms := make([]string, 0, len(fields))
	for _, field := range fields {
		if seen[field] {
			continue
		}
		seen[field] = true
		terms = append(terms, field)
	}
	return terms
}

func scoreRecord(record MemoryRecord, terms []string) int {
	text := strings.ToLower(record.Text)
	tags := strings.ToLower(strings.Join(record.Tags, " "))
	source := strings.ToLower(record.Source)
	score := 0
	for _, term := range terms {
		if strings.Contains(text, term) {
			score += 10
		}
		if strings.Contains(tags, term) {
			score += 4
		}
		if strings.Contains(source, term) {
			score += 2
		}
	}
	return score
}

func cloneRecord(record MemoryRecord) MemoryRecord {
	record.Tags = append([]string(nil), record.Tags...)
	return record
}

func newMemoryID(now time.Time) string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("mem_%d", now.UnixNano())
	}
	return fmt.Sprintf("mem_%d_%s", now.UnixNano(), hex.EncodeToString(b[:]))
}

func redactHighConfidenceSecretTags(tags []string) []string {
	redacted := make([]string, 0, len(tags))
	for _, tag := range tags {
		redacted = append(redacted, redactHighConfidenceSecrets(tag))
	}
	return redacted
}

func redactHighConfidenceSecrets(value string) string {
	redacted := value
	for _, pattern := range highConfidenceSecretPatterns {
		redacted = pattern.ReplaceAllString(redacted, "[REDACTED_SECRET]")
	}
	return redacted
}
