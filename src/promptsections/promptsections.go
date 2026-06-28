package promptsections

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
)

var ErrMissingUncachedReason = errors.New("promptsections: uncached section requires reason")

type ComputeFunc func(context.Context) (string, bool, error)

type Section struct {
	name       string
	reason     string
	cacheBreak bool
	compute    ComputeFunc
}

type ResolvedSection struct {
	Name       string
	Content    string
	Present    bool
	CacheBreak bool
	Reason     string
}

type Cache struct {
	mu      sync.Mutex
	entries map[string]cacheEntry
}

type Resolver struct {
	cache *Cache
}

type cacheEntry struct {
	content string
	present bool
}

func NewCache() *Cache {
	return &Cache{
		entries: make(map[string]cacheEntry),
	}
}

func NewResolver(cache *Cache) *Resolver {
	if cache == nil {
		cache = NewCache()
	}
	return &Resolver{cache: cache}
}

func (c *Cache) Section(name string, compute ComputeFunc) Section {
	return Section{
		name:    name,
		compute: compute,
	}
}

func (c *Cache) UncachedSection(name, reason string, compute ComputeFunc) Section {
	return Section{
		name:       name,
		reason:     reason,
		cacheBreak: true,
		compute:    compute,
	}
}

func (c *Cache) Resolve(ctx context.Context, sections []Section) ([]ResolvedSection, error) {
	return NewResolver(c).Resolve(ctx, sections)
}

func (c *Cache) Clear() {
	if c == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	clear(c.entries)
}

func (r *Resolver) Resolve(ctx context.Context, sections []Section) ([]ResolvedSection, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	resolved := make([]ResolvedSection, 0, len(sections))
	for _, section := range sections {
		item, err := r.resolveSection(ctx, section)
		if err != nil {
			return nil, err
		}
		resolved = append(resolved, item)
	}
	return resolved, nil
}

func (r *Resolver) Clear() {
	if r == nil {
		return
	}
	r.cache.Clear()
}

func (s Section) Name() string {
	return s.name
}

func (s Section) CacheBreak() bool {
	return s.cacheBreak
}

func (s Section) Reason() string {
	return s.reason
}

func (r *Resolver) resolveSection(ctx context.Context, section Section) (ResolvedSection, error) {
	if section.isZero() {
		return section.resolved("", false), nil
	}

	if section.cacheBreak {
		if strings.TrimSpace(section.reason) == "" {
			return ResolvedSection{}, fmt.Errorf("%w: %q", ErrMissingUncachedReason, section.name)
		}
		return section.computeResolved(ctx)
	}

	if entry, ok := r.cache.get(section.name); ok {
		return section.resolved(entry.content, entry.present), nil
	}

	resolved, err := section.computeResolved(ctx)
	if err != nil {
		return ResolvedSection{}, err
	}
	r.cache.set(section.name, cacheEntry{content: resolved.Content, present: resolved.Present})
	return resolved, nil
}

func (s Section) computeResolved(ctx context.Context) (ResolvedSection, error) {
	if s.compute == nil {
		return s.resolved("", false), nil
	}

	content, present, err := s.compute(ctx)
	if err != nil {
		return ResolvedSection{}, fmt.Errorf("resolve system prompt section %q: %w", s.name, err)
	}
	return s.resolved(content, present), nil
}

func (s Section) resolved(content string, present bool) ResolvedSection {
	return ResolvedSection{
		Name:       s.name,
		Content:    content,
		Present:    present,
		CacheBreak: s.cacheBreak,
		Reason:     s.reason,
	}
}

func (s Section) isZero() bool {
	return s.name == "" && s.reason == "" && !s.cacheBreak && s.compute == nil
}

func (c *Cache) get(name string) (cacheEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensureEntriesLocked()
	entry, ok := c.entries[name]
	return entry, ok
}

func (c *Cache) set(name string, entry cacheEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensureEntriesLocked()
	c.entries[name] = entry
}

func (c *Cache) ensureEntriesLocked() {
	if c.entries == nil {
		c.entries = make(map[string]cacheEntry)
	}
}
