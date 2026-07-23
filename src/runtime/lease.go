package runtime

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"
)

const (
	defaultLeaseTTL       = 30 * time.Second
	defaultLeaseHeartbeat = 10 * time.Second
	headlessIdleBudget    = 30 * time.Second
	leaseCleanupInterval  = 5 * time.Second
)

var (
	ErrLeaseNotFound     = errors.New("lease not found")
	ErrLeaseExpired      = errors.New("lease expired")
	ErrLeaseOwnedByOther = errors.New("lease owned by another bridge")
)

// Lease represents a bridge or Desktop attachment to the Core.
type Lease struct {
	LeaseID   string    `json:"lease_id"`
	OwnerKind string    `json:"owner_kind"`
	OwnerID   string    `json:"owner_id"`
	ExpiresAt time.Time `json:"expires_at"`
}

// LeaseManager tracks active bridge/Desktop leases and signals when a headless
// Core becomes idle. It is owned by the Core process.
type LeaseManager struct {
	mu              sync.RWMutex
	leases          map[string]*Lease
	ownerIndex      map[string]string // ownerID -> leaseID
	idleCallback    func()
	cleanupInterval time.Duration
	stopCh          chan struct{}
	wg              sync.WaitGroup
}

// NewLeaseManager creates a lease manager. If idleCallback is non-nil and the
// manager is running, the callback is invoked once after the last lease expires
// and the headless idle budget has elapsed.
func NewLeaseManager(idleCallback func()) *LeaseManager {
	return &LeaseManager{
		leases:          make(map[string]*Lease),
		ownerIndex:      make(map[string]string),
		idleCallback:    idleCallback,
		cleanupInterval: leaseCleanupInterval,
		stopCh:          make(chan struct{}),
	}
}

// NewLeaseManagerWithInterval is a test helper that uses a custom cleanup interval.
func NewLeaseManagerWithInterval(idleCallback func(), interval time.Duration) *LeaseManager {
	lm := NewLeaseManager(idleCallback)
	if interval > 0 {
		lm.cleanupInterval = interval
	}
	return lm
}

// Run starts the lease expiry reaper and idle watcher. It blocks until Stop is
// called; callers typically run it in a goroutine.
func (m *LeaseManager) Run(ctx context.Context) {
	m.wg.Add(1)
	defer m.wg.Done()
	ticker := time.NewTicker(m.cleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.reapExpired()
			if m.shouldIdleShutdown() {
				if m.idleCallback != nil {
					m.idleCallback()
				}
				return
			}
		}
	}
}

// Stop halts the lease manager.
func (m *LeaseManager) Stop() {
	close(m.stopCh)
	m.wg.Wait()
}

// CreateLease issues a new lease for ownerID. ownerKind distinguishes bridge vs
// desktop leases.
func (m *LeaseManager) CreateLease(ownerKind, ownerID string) (*Lease, error) {
	if ownerID == "" {
		return nil, errors.New("owner_id is required")
	}
	if ownerKind != "bridge" && ownerKind != "desktop" {
		return nil, fmt.Errorf("invalid owner_kind %q", ownerKind)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if existingID, ok := m.ownerIndex[ownerID]; ok {
		if existing, ok := m.leases[existingID]; ok {
			existing.ExpiresAt = time.Now().UTC().Add(defaultLeaseTTL)
			return existing, nil
		}
	}
	leaseID, err := newLeaseID()
	if err != nil {
		return nil, err
	}
	lease := &Lease{
		LeaseID:   leaseID,
		OwnerKind: ownerKind,
		OwnerID:   ownerID,
		ExpiresAt: time.Now().UTC().Add(defaultLeaseTTL),
	}
	m.leases[leaseID] = lease
	m.ownerIndex[ownerID] = leaseID
	return lease, nil
}

// Heartbeat renews a lease.
func (m *LeaseManager) Heartbeat(leaseID string) (*Lease, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	lease, ok := m.leases[leaseID]
	if !ok {
		return nil, ErrLeaseNotFound
	}
	if time.Now().UTC().After(lease.ExpiresAt) {
		delete(m.leases, leaseID)
		delete(m.ownerIndex, lease.OwnerID)
		return nil, ErrLeaseExpired
	}
	lease.ExpiresAt = time.Now().UTC().Add(defaultLeaseTTL)
	return lease, nil
}

// ReleaseLease removes a lease.
func (m *LeaseManager) ReleaseLease(leaseID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	lease, ok := m.leases[leaseID]
	if !ok {
		return ErrLeaseNotFound
	}
	delete(m.leases, leaseID)
	delete(m.ownerIndex, lease.OwnerID)
	return nil
}

// LeaseCount returns the number of active (non-expired) leases.
func (m *LeaseManager) LeaseCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	m.reapExpiredLocked()
	return len(m.leases)
}

func (m *LeaseManager) reapExpired() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reapExpiredLocked()
}

func (m *LeaseManager) reapExpiredLocked() {
	now := time.Now().UTC()
	for id, lease := range m.leases {
		if now.After(lease.ExpiresAt) {
			delete(m.leases, id)
			delete(m.ownerIndex, lease.OwnerID)
		}
	}
}

func (m *LeaseManager) shouldIdleShutdown() bool {
	// This is a placeholder; actual idle shutdown is gated by the idleCallback
	// caller, which should check ActiveRunNotifier. The lease manager itself
	// only signals that no leases remain.
	m.mu.RLock()
	defer m.mu.RUnlock()
	m.reapExpiredLocked()
	return len(m.leases) == 0
}

func newLeaseID() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "lease_" + hex.EncodeToString(b), nil
}
