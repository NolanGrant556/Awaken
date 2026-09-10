package llm

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

var ErrRequestQueueFull = errors.New("model request admission queue is full")

// RequestScheduler admits only transport work. Callers must prepare requests,
// run Hooks and compact their contexts before acquiring a permit. No permit is
// held during retry backoff. Three priority admissions then one ordinary
// admission bound starvation while keeping otherwise idle capacity usable.
type RequestScheduler struct {
	mu                                        sync.Mutex
	limit, queueLimit, active, priorityStreak int
	queue                                     []*requestWaiter
	accountLimit                              int
	accounts                                  map[string]int
	lastServed                                map[string]uint64
	groupActive                               map[string]int
	sequence                                  uint64
}

type requestWaiter struct {
	ready    chan struct{}
	priority bool
	granted  bool
	account  string
	group    string
}

func NewRequestScheduler(limit, queueLimit int) *RequestScheduler {
	if limit <= 0 {
		limit = 8
	}
	if queueLimit <= 0 {
		queueLimit = 128
	}
	return &RequestScheduler{limit: limit, queueLimit: queueLimit, accounts: map[string]int{}, lastServed: map[string]uint64{}, groupActive: map[string]int{}}
}

var hostRequestScheduler = NewRequestScheduler(8, 128)

// ConfigureRequestAdmission is called by the Host at startup; live queued work
// stays owned by the same scheduler if the limits are changed.
func ConfigureRequestAdmission(limit, queueLimit int) {
	hostRequestScheduler.mu.Lock()
	defer hostRequestScheduler.mu.Unlock()
	if limit > 0 {
		hostRequestScheduler.limit = limit
	}
	if queueLimit > 0 {
		hostRequestScheduler.queueLimit = queueLimit
	}
	hostRequestScheduler.admitLocked()
}

func (s *RequestScheduler) Acquire(ctx context.Context, purpose RequestPurpose) (func(), error) {
	return s.AcquireScoped(ctx, purpose, "", "")
}

// SetAccountLimit is an explicit Host limit, not a guessed provider RPM/TPM.
// Zero inherits the global transport limit.
func (s *RequestScheduler) SetAccountLimit(limit int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.accountLimit = max(0, limit)
	s.admitLocked()
}

func ConfigureAccountRequestAdmission(limit int) { hostRequestScheduler.SetAccountLimit(limit) }

func (s *RequestScheduler) AcquireScoped(ctx context.Context, purpose RequestPurpose, account, group string) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	w := &requestWaiter{ready: make(chan struct{}), priority: purpose == RequestPurposeCompactSummary || purpose == RequestPurposeNativeCompaction, account: account, group: group}
	s.mu.Lock()
	if len(s.queue) >= s.queueLimit {
		s.mu.Unlock()
		return nil, ErrRequestQueueFull
	}
	s.queue = append(s.queue, w)
	s.admitLocked()
	s.mu.Unlock()
	select {
	case <-w.ready:
	case <-ctx.Done():
	}
	s.mu.Lock()
	if err := ctx.Err(); err != nil {
		if w.granted {
			s.active--
			s.accounts[w.account]--
			s.groupActive[w.group]--
		} else {
			for i, pending := range s.queue {
				if pending == w {
					s.queue = append(s.queue[:i], s.queue[i+1:]...)
					break
				}
			}
		}
		s.admitLocked()
		s.releaseIdleKeys(w.account, w.group)
		s.mu.Unlock()
		return nil, err
	}
	s.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			s.mu.Lock()
			s.active--
			s.accounts[w.account]--
			s.groupActive[w.group]--
			s.admitLocked()
			s.releaseIdleKeys(w.account, w.group)
			s.mu.Unlock()
		})
	}, nil
}

func (s *RequestScheduler) admitLocked() {
	for s.active < s.limit && len(s.queue) > 0 {
		index := -1
		wantPriority := s.priorityStreak < 3
		for pass := 0; pass < 2 && index < 0; pass++ {
			for i, w := range s.queue {
				if s.accountLimit > 0 && s.accounts[w.account] >= s.accountLimit {
					continue
				}
				if pass == 0 && w.priority != wantPriority {
					continue
				}
				if index < 0 || s.lastServed[w.group] < s.lastServed[s.queue[index].group] {
					index = i
				}
			}
		}
		if index < 0 {
			return
		}
		w := s.queue[index]
		s.queue = append(s.queue[:index], s.queue[index+1:]...)
		if w.priority {
			s.priorityStreak = min(3, s.priorityStreak+1)
		} else {
			s.priorityStreak = 0
		}
		s.active++
		s.accounts[w.account]++
		s.groupActive[w.group]++
		s.sequence++
		s.lastServed[w.group] = s.sequence
		w.granted = true
		close(w.ready)
	}
}

func (s *RequestScheduler) releaseIdleKeys(account, group string) {
	accountPending, groupPending := s.accounts[account] > 0, s.groupActive[group] > 0
	for _, waiter := range s.queue {
		accountPending = accountPending || waiter.account == account
		groupPending = groupPending || waiter.group == group
	}
	if !accountPending {
		delete(s.accounts, account)
	}
	if !groupPending {
		delete(s.groupActive, group)
		delete(s.lastServed, group)
	}
}

func acquireModelRequest(ctx context.Context, cfg Config, prepared PreparedRequest) (func(), error) {
	scheduler := cfg.RequestScheduler
	if scheduler == nil {
		scheduler = hostRequestScheduler
	}
	account := cfg.AccountScopeID
	if account == "" {
		// Only an in-memory grouping key; never log credentials or this digest.
		account = fmt.Sprintf("%s:%s:%x", cfg.ProviderID, cfg.BaseURL, sha256.Sum256([]byte(cfg.APIKey)))
	}
	queuedAt := time.Now()
	release, err := scheduler.AcquireScoped(ctx, RequestPurpose(strings.TrimSpace(string(prepared.Purpose))), account, strings.Split(prepared.AffinityScopeID, ":agent:")[0])
	if prepared.Timing != nil {
		prepared.Timing.QueueWaitMS = time.Since(queuedAt).Milliseconds()
	}
	if err != nil {
		return nil, err
	}
	started := time.Now()
	if prepared.Timing != nil {
		prepared.Timing.TransportStarted = started
	}
	return func() {
		if prepared.Timing != nil {
			prepared.Timing.TransportMS = time.Since(started).Milliseconds()
		}
		release()
	}, nil
}
