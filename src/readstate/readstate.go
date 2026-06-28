package readstate

import (
	"path"
	"strings"
	"time"
)

const DefaultMaxEntries = 100

type Entry struct {
	Content       string
	Timestamp     time.Time
	Offset        int
	Limit         int
	IsPartialView bool
}

type Decision string

const (
	DecisionAllow Decision = "allow"
	DecisionDeny  Decision = "deny"
)

type ErrorCode string

const (
	ErrorNone        ErrorCode = ""
	ErrorUnread      ErrorCode = "unread"
	ErrorPartialView ErrorCode = "partial_view"
	ErrorStale       ErrorCode = "stale"
)

type Cache struct {
	maxEntries int
	entries    map[string]Entry
	order      []string
}

type CacheEntry struct {
	Path  string
	Entry Entry
}

func NewCache(maxEntries int) *Cache {
	if maxEntries <= 0 {
		maxEntries = DefaultMaxEntries
	}
	return &Cache{
		maxEntries: maxEntries,
		entries:    make(map[string]Entry),
		order:      make([]string, 0, maxEntries),
	}
}

func (c *Cache) Set(filePath string, entry Entry) {
	normalized := NormalizePath(filePath)
	if _, ok := c.entries[normalized]; !ok {
		c.order = append(c.order, normalized)
	}
	c.entries[normalized] = entry
	c.evictOverflow()
}

func (c *Cache) Get(filePath string) (Entry, bool) {
	entry, ok := c.entries[NormalizePath(filePath)]
	return entry, ok
}

func (c *Cache) Delete(filePath string) bool {
	normalized := NormalizePath(filePath)
	if _, ok := c.entries[normalized]; !ok {
		return false
	}
	delete(c.entries, normalized)
	c.removeFromOrder(normalized)
	return true
}

func (c *Cache) Clear() {
	c.entries = make(map[string]Entry)
	c.order = c.order[:0]
}

func (c *Cache) Keys() []string {
	keys := make([]string, len(c.order))
	copy(keys, c.order)
	return keys
}

func (c *Cache) Entries() []CacheEntry {
	entries := make([]CacheEntry, 0, len(c.order))
	for _, key := range c.order {
		entry, ok := c.entries[key]
		if !ok {
			continue
		}
		entries = append(entries, CacheEntry{
			Path:  key,
			Entry: entry,
		})
	}
	return entries
}

// ValidateWrite checks the read-before-write guard for an existing file.
// A zero currentMTime means the caller observed that the file does not exist.
func (c *Cache) ValidateWrite(filePath string, currentMTime time.Time) (Decision, ErrorCode) {
	if IsUNCPath(filePath) {
		return DecisionAllow, ErrorNone
	}
	if currentMTime.IsZero() {
		return DecisionAllow, ErrorNone
	}

	entry, ok := c.Get(filePath)
	if !ok {
		return DecisionDeny, ErrorUnread
	}
	if entry.IsPartialView {
		return DecisionDeny, ErrorPartialView
	}
	if currentMTime.After(entry.Timestamp) {
		return DecisionDeny, ErrorStale
	}
	return DecisionAllow, ErrorNone
}

// Merge returns a new cache with entries from both caches. When both caches
// contain the same path, the entry with the newer timestamp wins.
func Merge(first, second *Cache) *Cache {
	merged := NewCache(first.maxEntries)
	for _, cacheEntry := range first.Entries() {
		merged.Set(cacheEntry.Path, cacheEntry.Entry)
	}
	for _, cacheEntry := range second.Entries() {
		existing, ok := merged.Get(cacheEntry.Path)
		if !ok || cacheEntry.Entry.Timestamp.After(existing.Timestamp) {
			merged.Set(cacheEntry.Path, cacheEntry.Entry)
		}
	}
	return merged
}

func NormalizePath(filePath string) string {
	normalized := strings.ReplaceAll(filePath, "\\", "/")
	if strings.HasPrefix(normalized, "//") {
		cleaned := path.Clean(strings.TrimPrefix(normalized, "//"))
		if cleaned == "." {
			return "//"
		}
		return "//" + cleaned
	}
	return path.Clean(normalized)
}

func IsUNCPath(filePath string) bool {
	return strings.HasPrefix(filePath, `\\`) || strings.HasPrefix(filePath, "//")
}

func (c *Cache) evictOverflow() {
	for len(c.entries) > c.maxEntries && len(c.order) > 0 {
		oldest := c.order[0]
		c.order = c.order[1:]
		delete(c.entries, oldest)
	}
}

func (c *Cache) removeFromOrder(filePath string) {
	for i, key := range c.order {
		if key == filePath {
			c.order = append(c.order[:i], c.order[i+1:]...)
			return
		}
	}
}
