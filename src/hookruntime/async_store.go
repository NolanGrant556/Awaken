package hookruntime

import (
	"strings"
	"sync"
)

const (
	asyncCompletionThreadCapacity = 32
	asyncCompletionContextBudget  = 32 * 1024
)

// AsyncCompletionStore is a process-local, thread-scoped handoff for late Hook
// context. Hook receipts remain the execution audit surface; this store only
// carries bounded model-visible data into the next run of the same thread.
type AsyncCompletionStore struct {
	mu      sync.Mutex
	threads map[string][]AsyncCompletion
}

func NewAsyncCompletionStore() *AsyncCompletionStore {
	return &AsyncCompletionStore{threads: map[string][]AsyncCompletion{}}
}

func (s *AsyncCompletionStore) RecordAsyncHookCompletion(threadID string, completion AsyncCompletion) {
	if s == nil {
		return
	}
	threadID = strings.TrimSpace(threadID)
	completion.AdditionalContext = strings.TrimSpace(completion.AdditionalContext)
	if threadID == "" || completion.AdditionalContext == "" {
		return
	}
	if len(completion.AdditionalContext) > maxAdditionalContextPerHook {
		completion.AdditionalContext = completion.AdditionalContext[:maxAdditionalContextPerHook]
	}
	s.mu.Lock()
	items := append(s.threads[threadID], completion)
	if len(items) > asyncCompletionThreadCapacity {
		items = append([]AsyncCompletion(nil), items[len(items)-asyncCompletionThreadCapacity:]...)
	}
	for asyncCompletionBytes(items) > asyncCompletionContextBudget && len(items) > 1 {
		items = items[1:]
	}
	s.threads[threadID] = items
	s.mu.Unlock()
}

func (s *AsyncCompletionStore) HasAsyncHookCompletions(threadID string) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	has := len(s.threads[strings.TrimSpace(threadID)]) > 0
	s.mu.Unlock()
	return has
}

func (s *AsyncCompletionStore) DrainAsyncHookCompletions(threadID string) []AsyncCompletion {
	if s == nil {
		return nil
	}
	threadID = strings.TrimSpace(threadID)
	s.mu.Lock()
	items := append([]AsyncCompletion(nil), s.threads[threadID]...)
	delete(s.threads, threadID)
	s.mu.Unlock()
	return items
}

func asyncCompletionBytes(items []AsyncCompletion) int {
	total := 0
	for _, item := range items {
		total += len(item.AdditionalContext)
	}
	return total
}
