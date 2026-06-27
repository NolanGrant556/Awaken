package bridgetoken

import (
	"sync"
	"time"
)

const (
	defaultRefreshBuffer    = 5 * time.Minute
	defaultFallbackInterval = 30 * time.Minute
	defaultRetryDelay       = time.Minute
	defaultMaxFailures      = 3
)

type Clock interface {
	Now() time.Time
	AfterFunc(time.Duration, func()) Timer
}

type Timer interface {
	Stop() bool
}

type SchedulerConfig struct {
	GetAccessToken   func() (string, error)
	OnRefresh        func(sessionID string, token string)
	RefreshBuffer    time.Duration
	FallbackInterval time.Duration
	RetryDelay       time.Duration
	MaxFailures      int
	Clock            Clock
}

type Scheduler struct {
	getAccessToken   func() (string, error)
	onRefresh        func(sessionID string, token string)
	refreshBuffer    time.Duration
	fallbackInterval time.Duration
	retryDelay       time.Duration
	maxFailures      int
	clock            Clock

	mu          sync.Mutex
	timers      map[string]Timer
	failures    map[string]int
	generations map[string]uint64
}

func NewScheduler(config SchedulerConfig) *Scheduler {
	getAccessToken := config.GetAccessToken
	if getAccessToken == nil {
		getAccessToken = func() (string, error) { return "", nil }
	}
	onRefresh := config.OnRefresh
	if onRefresh == nil {
		onRefresh = func(string, string) {}
	}

	refreshBuffer := config.RefreshBuffer
	if refreshBuffer <= 0 {
		refreshBuffer = defaultRefreshBuffer
	}
	fallbackInterval := config.FallbackInterval
	if fallbackInterval <= 0 {
		fallbackInterval = defaultFallbackInterval
	}
	retryDelay := config.RetryDelay
	if retryDelay <= 0 {
		retryDelay = defaultRetryDelay
	}
	maxFailures := config.MaxFailures
	if maxFailures <= 0 {
		maxFailures = defaultMaxFailures
	}
	clock := config.Clock
	if clock == nil {
		clock = realClock{}
	}

	return &Scheduler{
		getAccessToken:   getAccessToken,
		onRefresh:        onRefresh,
		refreshBuffer:    refreshBuffer,
		fallbackInterval: fallbackInterval,
		retryDelay:       retryDelay,
		maxFailures:      maxFailures,
		clock:            clock,
		timers:           make(map[string]Timer),
		failures:         make(map[string]int),
		generations:      make(map[string]uint64),
	}
}

func (s *Scheduler) Schedule(sessionID string, token string) bool {
	expiry, ok := DecodeJWTExpiry(token)
	if !ok {
		return false
	}
	delay := time.Unix(expiry, 0).Sub(s.clock.Now()) - s.refreshBuffer
	s.scheduleAfter(sessionID, delay)
	return true
}

func (s *Scheduler) ScheduleFromExpiresIn(sessionID string, expiresInSeconds int64) bool {
	if expiresInSeconds < 0 {
		expiresInSeconds = 0
	}
	delay := time.Duration(expiresInSeconds)*time.Second - s.refreshBuffer
	s.scheduleAfter(sessionID, delay)
	return true
}

func (s *Scheduler) Cancel(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.nextGenerationLocked(sessionID)
	if timer := s.timers[sessionID]; timer != nil {
		timer.Stop()
		delete(s.timers, sessionID)
	}
	delete(s.failures, sessionID)
}

func (s *Scheduler) CancelAll() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for sessionID := range s.generations {
		s.nextGenerationLocked(sessionID)
	}
	for sessionID, timer := range s.timers {
		if timer != nil {
			timer.Stop()
		}
		delete(s.timers, sessionID)
	}
	clear(s.failures)
}

func (s *Scheduler) scheduleAfter(sessionID string, delay time.Duration) {
	s.mu.Lock()
	if timer := s.timers[sessionID]; timer != nil {
		timer.Stop()
		delete(s.timers, sessionID)
	}
	generation := s.nextGenerationLocked(sessionID)
	if delay <= 0 {
		s.mu.Unlock()
		s.startRefresh(sessionID, generation)
		return
	}

	timer := s.clock.AfterFunc(delay, func() {
		s.startRefresh(sessionID, generation)
	})
	s.timers[sessionID] = timer
	s.mu.Unlock()
}

func (s *Scheduler) startRefresh(sessionID string, generation uint64) {
	go s.doRefresh(sessionID, generation)
}

func (s *Scheduler) doRefresh(sessionID string, generation uint64) {
	token, err := s.getAccessToken()
	if !s.isCurrent(sessionID, generation) {
		return
	}
	if err != nil || token == "" {
		s.recordFailure(sessionID, generation)
		return
	}

	s.mu.Lock()
	if s.generations[sessionID] != generation {
		s.mu.Unlock()
		return
	}
	delete(s.failures, sessionID)
	s.mu.Unlock()

	s.onRefresh(sessionID, token)
	s.scheduleFollowUp(sessionID, generation)
}

func (s *Scheduler) recordFailure(sessionID string, generation uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.generations[sessionID] != generation {
		return
	}
	failures := s.failures[sessionID] + 1
	s.failures[sessionID] = failures
	if failures >= s.maxFailures {
		return
	}

	timer := s.clock.AfterFunc(s.retryDelay, func() {
		s.startRefresh(sessionID, generation)
	})
	s.timers[sessionID] = timer
}

func (s *Scheduler) scheduleFollowUp(sessionID string, generation uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.generations[sessionID] != generation {
		return
	}
	timer := s.clock.AfterFunc(s.fallbackInterval, func() {
		s.startRefresh(sessionID, generation)
	})
	s.timers[sessionID] = timer
}

func (s *Scheduler) isCurrent(sessionID string, generation uint64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.generations[sessionID] == generation
}

func (s *Scheduler) nextGenerationLocked(sessionID string) uint64 {
	generation := s.generations[sessionID] + 1
	s.generations[sessionID] = generation
	return generation
}

type realClock struct{}

func (realClock) Now() time.Time {
	return time.Now()
}

func (realClock) AfterFunc(delay time.Duration, fn func()) Timer {
	return time.AfterFunc(delay, fn)
}
