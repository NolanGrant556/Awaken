package hookruntime

import (
	"sort"
	"strings"
	"sync"
	"time"
)

const receiptCapacity = 512

type ReceiptStore struct {
	mu       sync.RWMutex
	receipts []Receipt
	retryOf  map[string]string
}

func NewReceiptStore() *ReceiptStore { return &ReceiptStore{retryOf: map[string]string{}} }

func (s *ReceiptStore) MarkRetry(invocationID, originalID string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.retryOf[invocationID] = originalID
	s.mu.Unlock()
}

func (s *ReceiptStore) LinkRetry(invocationID, originalID string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	for index := len(s.receipts) - 1; index >= 0; index-- {
		if s.receipts[index].ID == invocationID {
			s.receipts[index].RetryOf = originalID
			break
		}
	}
	s.mu.Unlock()
}

func (s *ReceiptStore) RecordHookReceipt(outcome Outcome) {
	if s == nil || outcome.SourceKind != SourcePlugin || strings.TrimSpace(outcome.PluginName) == "" {
		return
	}
	s.mu.Lock()
	receipt := Receipt{
		ID: outcome.InvocationID, PluginName: outcome.PluginName, PluginDigest: outcome.PluginDigest,
		LogicalID: outcome.LogicalID, RevisionID: outcome.RevisionID, Event: outcome.Event,
		Status: outcome.Status, Action: outcome.Action, ReasonCode: outcome.ReasonCode,
		DurationMS: outcome.DurationMS, CreatedAt: time.Now().UTC(), RetryOf: s.retryOf[outcome.InvocationID],
	}
	delete(s.retryOf, outcome.InvocationID)
	s.receipts = append(s.receipts, receipt)
	if len(s.receipts) > receiptCapacity {
		s.receipts = append([]Receipt(nil), s.receipts[len(s.receipts)-receiptCapacity:]...)
	}
	s.mu.Unlock()
}

func (s *ReceiptStore) Get(id string) (Receipt, bool) {
	if s == nil {
		return Receipt{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for index := len(s.receipts) - 1; index >= 0; index-- {
		if s.receipts[index].ID == id {
			return s.receipts[index], true
		}
	}
	return Receipt{}, false
}

func (s *ReceiptStore) List(pluginName, logicalID, revisionID string, offset, limit int) ([]Receipt, int) {
	if s == nil {
		return []Receipt{}, 0
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	s.mu.RLock()
	var filtered []Receipt
	for _, receipt := range s.receipts {
		if receipt.PluginName == pluginName && receipt.LogicalID == logicalID && (revisionID == "" || receipt.RevisionID == revisionID) {
			filtered = append(filtered, receipt)
		}
	}
	s.mu.RUnlock()
	sort.SliceStable(filtered, func(i, j int) bool { return filtered[i].CreatedAt.After(filtered[j].CreatedAt) })
	if offset < 0 {
		offset = 0
	}
	if offset >= len(filtered) {
		return []Receipt{}, 0
	}
	end := offset + limit
	next := 0
	if end < len(filtered) {
		next = end
	} else {
		end = len(filtered)
	}
	return append([]Receipt(nil), filtered[offset:end]...), next
}
