package computer

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ResourceLease grants exclusive control of a resource to a single controller.
// It is separate from the existing Desktop/Core attachment lease in
// src/runtime/lease.go because the resource key, fencing semantics and lifetime
// differ.
type ResourceLease struct {
	ResourceKey  string     `json:"resource_key"`
	BindingID    BindingID  `json:"binding_id"`
	SessionID    SessionID  `json:"session_id"`
	FenceToken   FenceToken `json:"fence_token"`
	IssuedAt     time.Time  `json:"issued_at"`
	ExpiresAt    time.Time  `json:"expires_at"`
	ControllerID string     `json:"controller_id"`
}

// LeaseManager tracks active leases and enforces monotonic fences.
type LeaseManager struct {
	changed chan struct{}
	mu      sync.RWMutex
	leases  map[string]ResourceLease
	fences  map[string]FenceToken
}

// NewLeaseManager creates an empty lease manager.
func NewLeaseManager() *LeaseManager {
	return &LeaseManager{
		changed: make(chan struct{}),
		leases:  make(map[string]ResourceLease),
		fences:  make(map[string]FenceToken),
	}
}

// Child controllers wait on resource ownership, never on model polling. A
// cancellation releases the waiter immediately and does not acquire a lease.
func (m *LeaseManager) AcquireContext(ctx context.Context, key string, bind Binding, sessionID SessionID, controllerID string, ttl time.Duration) (ResourceLease, error) {
	if key == "" {
		return ResourceLease{}, fmt.Errorf("resource_key is required")
	}
	for {
		if err := ctx.Err(); err != nil {
			return ResourceLease{}, err
		}
		m.mu.Lock()
		if m.changed == nil {
			m.changed = make(chan struct{})
		}
		changed := m.changed
		m.mu.Unlock()
		lease, err := m.Acquire(key, bind, sessionID, controllerID, ttl)
		if err == nil {
			if err := ctx.Err(); err != nil {
				_ = m.Release(key, bind.BindingID)
				return ResourceLease{}, err
			}
			return lease, nil
		}
		select {
		case <-ctx.Done():
			return ResourceLease{}, ctx.Err()
		case <-changed:
		}
	}
}

// Acquire attempts to take exclusive control of a resource. It returns a new
// fence token. If the resource is already held by another binding, it fails.
func (m *LeaseManager) Acquire(resourceKey string, bind Binding, sessionID SessionID, controllerID string, ttl time.Duration) (ResourceLease, error) {
	if resourceKey == "" {
		return ResourceLease{}, fmt.Errorf("resource_key is required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	existing, held := m.leases[resourceKey]
	if held && existing.BindingID != bind.BindingID {
		return ResourceLease{}, fmt.Errorf("resource %s is already leased by %s", resourceKey, existing.BindingID)
	}
	fence := NewFenceToken(m.fences[resourceKey])
	m.fences[resourceKey] = fence
	now := time.Now().UTC()
	lease := ResourceLease{
		ResourceKey:  resourceKey,
		BindingID:    bind.BindingID,
		SessionID:    sessionID,
		FenceToken:   fence,
		IssuedAt:     now,
		ExpiresAt:    now.Add(ttl),
		ControllerID: controllerID,
	}
	m.leases[resourceKey] = lease
	return lease, nil
}

// Release revokes a lease. It does not decrease the fence; the fence stays at
// its highest value so stale authorizations are permanently rejected.
func (m *LeaseManager) Release(resourceKey string, bindingID BindingID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	lease, ok := m.leases[resourceKey]
	if !ok {
		return nil
	}
	if lease.BindingID != bindingID {
		return fmt.Errorf("binding %s does not hold lease for %s", bindingID, resourceKey)
	}
	delete(m.leases, resourceKey)
	if m.changed != nil {
		close(m.changed)
	}
	m.changed = make(chan struct{})
	return nil
}

// Current returns the active lease for a resource, if any.
func (m *LeaseManager) Current(resourceKey string) (ResourceLease, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	lease, ok := m.leases[resourceKey]
	return lease, ok
}

// ValidateFence rejects any token lower than the highest fence seen for the
// resource.
func (m *LeaseManager) ValidateFence(resourceKey string, token FenceToken) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	highest, ok := m.fences[resourceKey]
	if !ok {
		return fmt.Errorf("no fence history for resource %s", resourceKey)
	}
	if token < highest {
		return fmt.Errorf("stale fence token %s for resource %s (highest %s)", token, resourceKey, highest)
	}
	return nil
}

// HighestFence returns the highest fence token ever issued for a resource.
func (m *LeaseManager) HighestFence(resourceKey string) (FenceToken, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	f, ok := m.fences[resourceKey]
	return f, ok
}
