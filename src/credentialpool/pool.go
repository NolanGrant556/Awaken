package credentialpool

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

type Pool struct {
	mu                   sync.Mutex
	provider             string
	selector             Selector
	credentials          []credentialState
	roundRobinCursor     int
	stickyID             string
	unauthorizedCooldown time.Duration
	now                  func() time.Time
}

func NewPool(provider string, strategy Strategy, credentials []ProviderCredential) (*Pool, error) {
	return NewPoolWithOptions(provider, strategy, credentials, PoolOptions{})
}

type PoolOptions struct {
	UnauthorizedCooldown time.Duration
	Now                  func() time.Time
}

func NewPoolWithOptions(provider string, strategy Strategy, credentials []ProviderCredential, options PoolOptions) (*Pool, error) {
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return nil, ErrEmptyProvider
	}
	if len(credentials) == 0 {
		return nil, ErrNoCredentials
	}
	selector, err := NewSelector(strategy)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{}, len(credentials))
	states := make([]credentialState, 0, len(credentials))
	for _, credential := range credentials {
		credential.Provider = strings.TrimSpace(firstNonEmpty(credential.Provider, provider))
		credential.ID = strings.TrimSpace(credential.ID)
		if credential.ID == "" {
			credential.ID = stableCredentialID(credential)
		}
		if credential.Provider != provider {
			credential.Provider = provider
		}
		if _, ok := seen[credential.ID]; ok {
			return nil, fmt.Errorf("%w: %s", ErrDuplicateID, credential.ID)
		}
		seen[credential.ID] = struct{}{}
		states = append(states, credentialState{credential: cloneCredential(credential)})
	}

	cooldown := options.UnauthorizedCooldown
	if cooldown == 0 {
		cooldown = DefaultUnauthorizedCooldown
	}
	now := options.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}

	return &Pool{
		provider:             provider,
		selector:             selector,
		credentials:          states,
		unauthorizedCooldown: cooldown,
		now:                  now,
	}, nil
}

func Restore(snapshot PoolSnapshot, credentials []ProviderCredential) (*Pool, error) {
	return RestoreWithOptions(snapshot, credentials, PoolOptions{})
}

func RestoreWithOptions(snapshot PoolSnapshot, credentials []ProviderCredential, options PoolOptions) (*Pool, error) {
	pool, err := NewPoolWithOptions(snapshot.Provider, snapshot.Strategy, credentials, options)
	if err != nil {
		return nil, err
	}
	pool.restoreState(snapshot)
	return pool, nil
}

func (p *Pool) Provider() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.provider
}

func (p *Pool) Strategy() Strategy {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.selector.Strategy()
}

func (p *Pool) Select() (ProviderCredential, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	index, ok := p.selectIndexLocked()
	if !ok {
		return ProviderCredential{}, false
	}
	now := p.now()
	p.credentials[index].stats.Requests++
	p.credentials[index].stats.LastUsedAt = now
	return cloneCredential(p.credentials[index].credential), true
}

func (p *Pool) MarkSuccess(credentialID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	index := p.indexOfLocked(credentialID)
	if index < 0 {
		return ErrCredentialNotFound
	}
	now := p.now()
	state := &p.credentials[index]
	state.stats.Successes++
	state.stats.LastSuccessAt = now
	state.disabledUntil = time.Time{}
	state.lastFailureReason = ""
	state.lastFailureMessage = ""
	return nil
}

func (p *Pool) MarkFailure(credentialID string, reason FailureReason) (ProviderCredential, bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	index := p.indexOfLocked(credentialID)
	if index < 0 {
		return ProviderCredential{}, false, ErrCredentialNotFound
	}

	reason = NormalizeFailureReason(reason)
	now := p.now()
	state := &p.credentials[index]
	state.stats.Failures++
	state.stats.LastFailureAt = now
	state.stats.LastFailureReason = reason
	state.lastFailureReason = reason
	state.lastFailureMessage = ""

	switch reason {
	case FailureReasonUnauthorized:
		state.stats.UnauthorizedFailures++
		state.disabledUntil = now.Add(p.unauthorizedCooldown)
		p.clearStickyAfterFailureLocked(index)
		return p.selectFailoverLocked()
	case FailureReasonRateLimit:
		state.stats.RateLimitFailures++
		p.clearStickyAfterFailureLocked(index)
		return p.selectFailoverLocked()
	default:
		p.clearStickyAfterFailureLocked(index)
		return p.selectFailoverLocked()
	}
}

func (p *Pool) MarkHTTPFailure(credentialID string, status int) (ProviderCredential, bool, error) {
	return p.MarkFailure(credentialID, FailureReasonFromHTTPStatus(status))
}

func (p *Pool) Stats(credentialID string) (UsageStats, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	index := p.indexOfLocked(credentialID)
	if index < 0 {
		return UsageStats{}, false
	}
	return p.credentials[index].stats, true
}

func (p *Pool) Snapshot() PoolSnapshot {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.snapshotLocked()
}

func (p *Pool) String() string {
	snapshot := p.Snapshot()
	return fmt.Sprintf(
		"credentialpool.Pool{provider:%q strategy:%q credentials:%d}",
		snapshot.Provider,
		snapshot.Strategy,
		len(snapshot.Credentials),
	)
}

func (p *Pool) MarshalJSON() ([]byte, error) {
	return json.Marshal(p.Snapshot())
}

func (p *Pool) restoreState(snapshot PoolSnapshot) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.roundRobinCursor = normalizeCursor(snapshot.RoundRobinCursor, len(p.credentials))
	p.stickyID = snapshot.StickyID

	snapshotsByID := make(map[string]CredentialSnapshot, len(snapshot.Credentials))
	for _, credential := range snapshot.Credentials {
		snapshotsByID[credential.ID] = credential
	}
	for index := range p.credentials {
		state := &p.credentials[index]
		snap, ok := snapshotsByID[state.credential.ID]
		if !ok {
			continue
		}
		state.stats = snap.Stats
		state.disabledUntil = snap.DisabledUntil
		state.lastFailureReason = snap.LastFailureReason
		state.lastFailureMessage = snap.LastFailureMessage
	}
}

func (p *Pool) snapshotLocked() PoolSnapshot {
	credentials := make([]CredentialSnapshot, 0, len(p.credentials))
	for _, state := range p.credentials {
		metadata := cloneMetadata(state.credential.Metadata)
		credentials = append(credentials, CredentialSnapshot{
			ID:                 state.credential.ID,
			Provider:           state.credential.Provider,
			Label:              state.credential.Label,
			BaseURL:            state.credential.BaseURL,
			Metadata:           metadata,
			Stats:              state.stats,
			DisabledUntil:      state.disabledUntil,
			LastFailureReason:  state.lastFailureReason,
			LastFailureMessage: state.lastFailureMessage,
		})
	}
	return PoolSnapshot{
		Provider:         p.provider,
		Strategy:         p.selector.Strategy(),
		RoundRobinCursor: p.roundRobinCursor,
		StickyID:         p.stickyID,
		Credentials:      credentials,
	}
}

func (p *Pool) selectFailoverLocked() (ProviderCredential, bool, error) {
	index, ok := p.selectIndexLocked()
	if !ok {
		return ProviderCredential{}, false, nil
	}
	now := p.now()
	p.credentials[index].stats.Requests++
	p.credentials[index].stats.LastUsedAt = now
	return cloneCredential(p.credentials[index].credential), true, nil
}

func (p *Pool) selectIndexLocked() (int, bool) {
	available := p.availableIndexesLocked()
	if len(available) == 0 {
		p.stickyID = ""
		return -1, false
	}

	switch p.selector.Strategy() {
	case StrategyRoundRobin:
		return p.selectRoundRobinLocked(available)
	case StrategyStickyUntilFailure:
		return p.selectStickyLocked(available)
	default:
		return p.selectLeastUsedLocked(available)
	}
}

func (p *Pool) selectLeastUsedLocked(available []int) (int, bool) {
	sort.SliceStable(available, func(i, j int) bool {
		left := p.credentials[available[i]].stats
		right := p.credentials[available[j]].stats
		if left.Requests != right.Requests {
			return left.Requests < right.Requests
		}
		return available[i] < available[j]
	})
	return available[0], true
}

func (p *Pool) selectRoundRobinLocked(available []int) (int, bool) {
	for offset := 0; offset < len(p.credentials); offset++ {
		index := (p.roundRobinCursor + offset) % len(p.credentials)
		if containsIndex(available, index) {
			p.roundRobinCursor = (index + 1) % len(p.credentials)
			return index, true
		}
	}
	return available[0], true
}

func (p *Pool) selectStickyLocked(available []int) (int, bool) {
	if p.stickyID != "" {
		index := p.indexOfLocked(p.stickyID)
		if index >= 0 && containsIndex(available, index) {
			return index, true
		}
	}
	index, ok := p.selectRoundRobinLocked(available)
	if ok {
		p.stickyID = p.credentials[index].credential.ID
	}
	return index, ok
}

func (p *Pool) availableIndexesLocked() []int {
	now := p.now()
	available := make([]int, 0, len(p.credentials))
	for index := range p.credentials {
		state := &p.credentials[index]
		if !state.disabledUntil.IsZero() {
			if now.Before(state.disabledUntil) {
				continue
			}
			state.disabledUntil = time.Time{}
		}
		available = append(available, index)
	}
	return available
}

func (p *Pool) clearStickyAfterFailureLocked(failedIndex int) {
	if failedIndex >= 0 && failedIndex < len(p.credentials) {
		failedID := p.credentials[failedIndex].credential.ID
		if p.stickyID == failedID {
			p.stickyID = ""
			p.roundRobinCursor = (failedIndex + 1) % len(p.credentials)
		}
	}
}

func (p *Pool) indexOfLocked(credentialID string) int {
	for index, state := range p.credentials {
		if state.credential.ID == credentialID {
			return index
		}
	}
	return -1
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func stableCredentialID(credential ProviderCredential) string {
	if credential.Label != "" {
		return credential.Label
	}
	if credential.Secret != "" {
		sum := sha256.Sum256([]byte(credential.Provider + "\x00" + credential.Secret))
		return fmt.Sprintf("%s:%s", credential.Provider, hex.EncodeToString(sum[:])[:12])
	}
	return fmt.Sprintf("%s:%s", credential.Provider, credential.BaseURL)
}

func cloneCredential(credential ProviderCredential) ProviderCredential {
	credential.Metadata = cloneMetadata(credential.Metadata)
	return credential
}

func cloneMetadata(metadata map[string]string) map[string]string {
	if len(metadata) == 0 {
		return nil
	}
	clone := make(map[string]string, len(metadata))
	for key, value := range metadata {
		clone[key] = value
	}
	return clone
}

func normalizeCursor(cursor int, size int) int {
	if size <= 0 {
		return 0
	}
	if cursor < 0 {
		cursor = 0
	}
	return cursor % size
}

func containsIndex(indexes []int, target int) bool {
	for _, index := range indexes {
		if index == target {
			return true
		}
	}
	return false
}
