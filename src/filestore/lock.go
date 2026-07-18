package filestore

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"
)

// FileLock is an OS-backed cross-process exclusive lock built on O_EXCL lock
// files with owner metadata and stale-owner recovery. Lock acquisition never
// degrades: callers must treat acquisition failure as a hard stop, not fall
// back to lock-free writes.
type FileLock struct {
	path    string
	ownerID string
	pid     int
	ttl     time.Duration
	held    bool
	mu      sync.Mutex
}

type lockOwner struct {
	PID         int       `json:"pid"`
	AcquiredAt  time.Time `json:"acquired_at"`
	ExpiresAt   time.Time `json:"expires_at"`
	OwnerID     string    `json:"owner_id,omitempty"`
	HeartbeatAt time.Time `json:"heartbeat_at,omitempty"`
}

// AcquireLock attempts to take the exclusive lock at path with the given
// time-to-live. A live lock returns ErrLockHeld. A lock whose owner process
// is provably dead, or whose TTL has expired, is reclaimed atomically.
func AcquireLock(path, ownerID string, ttl time.Duration) (*FileLock, error) {
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	for attempt := 0; attempt < 2; attempt++ {
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, FilePerm)
		if err == nil {
			now := time.Now().UTC()
			owner := lockOwner{
				PID:         os.Getpid(),
				AcquiredAt:  now,
				HeartbeatAt: now,
				ExpiresAt:   now.Add(ttl),
				OwnerID:     ownerID,
			}
			encoded, _ := json.Marshal(owner)
			if _, writeErr := file.Write(encoded); writeErr != nil {
				file.Close()
				os.Remove(path)
				return nil, fmt.Errorf("filestore: write lock owner: %w", writeErr)
			}
			if syncErr := file.Sync(); syncErr != nil {
				file.Close()
				os.Remove(path)
				return nil, fmt.Errorf("filestore: fsync lock file: %w", syncErr)
			}
			if closeErr := file.Close(); closeErr != nil {
				os.Remove(path)
				return nil, closeErr
			}
			return &FileLock{path: path, ownerID: ownerID, pid: os.Getpid(), ttl: ttl, held: true}, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("filestore: create lock file: %w", err)
		}
		stale, observed, staleErr := lockIsStale(path)
		if staleErr != nil {
			return nil, staleErr
		}
		if !stale {
			return nil, ErrLockHeld
		}
		current, readErr := os.ReadFile(path)
		if readErr == nil && !bytes.Equal(current, observed) {
			// The owner refreshed or another contender replaced the lock after
			// our stale check. Re-evaluate instead of deleting the new lock.
			continue
		}
		if readErr != nil && !os.IsNotExist(readErr) {
			return nil, fmt.Errorf("filestore: re-read stale lock: %w", readErr)
		}
		// Reclaim: remove the stale lock and retry the exclusive create once.
		// A concurrent reclaimer may win the retry; that is correct.
		if removeErr := os.Remove(path); removeErr != nil && !os.IsNotExist(removeErr) {
			return nil, fmt.Errorf("filestore: reclaim stale lock: %w", removeErr)
		}
	}
	return nil, ErrLockHeld
}

// Refresh renews a held lock after verifying that the lock file still belongs
// to this exact owner. Callers should refresh well before the TTL expires.
func (l *FileLock) Refresh() error {
	if l == nil {
		return fmt.Errorf("filestore: refresh nil lock")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.held {
		return fmt.Errorf("filestore: refresh released lock")
	}
	data, err := os.ReadFile(l.path)
	if err != nil {
		return fmt.Errorf("filestore: read lock for refresh: %w", err)
	}
	var owner lockOwner
	if err := json.Unmarshal(data, &owner); err != nil {
		return fmt.Errorf("filestore: decode lock for refresh: %w", err)
	}
	if owner.PID != l.pid || owner.OwnerID != l.ownerID {
		return fmt.Errorf("%w: lock owner changed", ErrLockHeld)
	}
	now := time.Now().UTC()
	owner.HeartbeatAt = now
	owner.ExpiresAt = now.Add(l.ttl)
	encoded, _ := json.Marshal(owner)
	if err := WriteFileAtomic(l.path, append(encoded, '\n')); err != nil {
		return fmt.Errorf("filestore: refresh lock: %w", err)
	}
	return nil
}

func lockIsStale(path string) (bool, []byte, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return true, nil, nil
	}
	if err != nil {
		return false, nil, fmt.Errorf("filestore: read lock owner: %w", err)
	}
	var owner lockOwner
	if err := json.Unmarshal(data, &owner); err != nil {
		// An unreadable lock file cannot prove a live owner, but reclaiming a
		// lock we cannot understand is unsafe. Fail closed.
		return false, data, fmt.Errorf("filestore: unreadable lock owner at %s: %w", path, err)
	}
	if !owner.ExpiresAt.IsZero() && time.Now().UTC().After(owner.ExpiresAt) {
		return true, data, nil
	}
	if owner.PID > 0 && !processAlive(owner.PID) {
		return true, data, nil
	}
	return false, data, nil
}

// Release removes the lock file. Releasing twice is a no-op.
func (l *FileLock) Release() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.held {
		return nil
	}
	l.held = false
	data, err := os.ReadFile(l.path)
	if err == nil {
		var owner lockOwner
		if json.Unmarshal(data, &owner) == nil && (owner.PID != l.pid || owner.OwnerID != l.ownerID) {
			return fmt.Errorf("%w: refusing to release a lock owned by another process", ErrLockHeld)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("filestore: read lock for release: %w", err)
	}
	if err := os.Remove(l.path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("filestore: release lock: %w", err)
	}
	return nil
}

// Lease is a fencing-token writer lease (for example one per active session).
// Tokens increase monotonically per lease path so a revived stale owner can
// never commit over a newer claim.
type Lease struct {
	Owner        string    `json:"owner"`
	FencingToken uint64    `json:"fencing_token"`
	IssuedAt     time.Time `json:"issued_at"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// ClaimLease claims or renews the lease at path for owner. A live lease held
// by a different owner returns ErrLockHeld. Clock rollback relative to the
// stored lease fails closed.
func ClaimLease(path, guardPath, owner string, ttl time.Duration) (Lease, error) {
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	guard, err := AcquireLock(guardPath, owner, 10*time.Second)
	if err != nil {
		return Lease{}, err
	}
	defer guard.Release()

	var current Lease
	err = ReadJSON(path, &current)
	now := time.Now().UTC()
	switch {
	case err == nil:
		if now.Before(current.IssuedAt.Add(-1 * time.Minute)) {
			return Lease{}, fmt.Errorf("filestore: clock moved backwards relative to lease %s; failing closed", path)
		}
		if current.Owner != owner && now.Before(current.ExpiresAt) {
			return Lease{}, fmt.Errorf("%w: lease owned by %s until %s", ErrLockHeld, current.Owner, current.ExpiresAt.Format(time.RFC3339))
		}
	case os.IsNotExist(err):
		// First claim.
	default:
		return Lease{}, err
	}
	next := Lease{
		Owner:        owner,
		FencingToken: current.FencingToken + 1,
		IssuedAt:     now,
		ExpiresAt:    now.Add(ttl),
	}
	encoded, _ := json.Marshal(next)
	if err := WriteFileAtomic(path, append(encoded, '\n')); err != nil {
		return Lease{}, err
	}
	return next, nil
}

// ValidateLease re-checks that the caller still holds the newest fencing
// token before a destructive mutation or completion.
func ValidateLease(path string, held Lease) error {
	var current Lease
	if err := ReadJSON(path, &current); err != nil {
		return fmt.Errorf("filestore: read lease for validation: %w", err)
	}
	if current.Owner != held.Owner || current.FencingToken != held.FencingToken {
		return fmt.Errorf("%w: fencing token superseded (held %d, current %d)", ErrLockHeld, held.FencingToken, current.FencingToken)
	}
	return nil
}
