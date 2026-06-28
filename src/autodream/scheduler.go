package autodream

import (
	"errors"
	"strings"
	"time"
)

func Evaluate(inputs Inputs, deps Dependencies) Decision {
	cfg := inputs.Config.normalized()
	now := inputs.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}

	decision := Decision{
		Forced:       inputs.Forced,
		MinHours:     cfg.MinHours,
		MinSessions:  cfg.MinSessions,
		ScanThrottle: cfg.ScanThrottle,
		NextState:    inputs.State,
	}

	if !inputs.Forced {
		closed := inputs.Gates.ClosedReasons()
		if len(closed) > 0 {
			decision.Status = StatusDisabled
			decision.ClosedGates = closed
			decision.Reason = strings.Join(closed, ", ")
			return decision
		}
	}

	lastConsolidatedAt, err := callReadLast(deps)
	if err != nil {
		decision.Status = StatusReadLastError
		decision.Reason = err.Error()
		return decision
	}
	decision.LastConsolidatedAt = lastConsolidatedAt
	decision.HoursSince = now.Sub(lastConsolidatedAt).Hours()

	if !inputs.Forced && decision.HoursSince < cfg.MinHours {
		decision.Status = StatusTimeGateClosed
		decision.Reason = "minimum hours since last consolidation not reached"
		return decision
	}

	if !inputs.Forced && !inputs.State.LastSessionScanAt.IsZero() && now.Sub(inputs.State.LastSessionScanAt) < cfg.ScanThrottle {
		decision.Status = StatusScanThrottled
		decision.Reason = "session scan throttle active"
		return decision
	}

	decision.NextState.LastSessionScanAt = now
	sessionIDs, err := callListSessions(deps, lastConsolidatedAt)
	if err != nil {
		decision.Status = StatusListSessionsError
		decision.Reason = err.Error()
		return decision
	}
	decision.SessionIDs = excludeCurrentSession(sessionIDs, inputs.CurrentSessionID)
	decision.SessionsCount = len(decision.SessionIDs)

	if !inputs.Forced && decision.SessionsCount < cfg.MinSessions {
		decision.Status = StatusNotEnoughSessions
		decision.Reason = "minimum sessions since last consolidation not reached"
		return decision
	}

	lock, err := callAcquireLock(deps)
	if err != nil {
		decision.Status = StatusLockAcquireError
		decision.Reason = err.Error()
		return decision
	}
	if !lock.Acquired {
		decision.Status = StatusLockBusy
		decision.Reason = "consolidation lock is held"
		return decision
	}

	prompt := BuildConsolidationPrompt(inputs.MemoryRoot, inputs.TranscriptDir, decision.SessionIDs)
	plan := BuildActionPlan(inputs.MemoryRoot, decision.SessionIDs, lock.PriorMtime, prompt)

	decision.Status = StatusFire
	decision.PriorMtime = lock.PriorMtime
	decision.Prompt = prompt
	decision.Plan = &plan
	return decision
}

func callReadLast(deps Dependencies) (time.Time, error) {
	if deps.ReadLast == nil {
		return time.Time{}, errors.New("readLast dependency is nil")
	}
	return deps.ReadLast()
}

func callListSessions(deps Dependencies, since time.Time) ([]string, error) {
	if deps.ListSessions == nil {
		return nil, errors.New("listSessions dependency is nil")
	}
	return deps.ListSessions(since)
}

func callAcquireLock(deps Dependencies) (LockAcquireResult, error) {
	if deps.AcquireLock == nil {
		return LockAcquireResult{}, errors.New("lockAcquire dependency is nil")
	}
	return deps.AcquireLock()
}

func excludeCurrentSession(sessionIDs []string, currentSessionID string) []string {
	filtered := make([]string, 0, len(sessionIDs))
	for _, id := range sessionIDs {
		if id == currentSessionID {
			continue
		}
		filtered = append(filtered, id)
	}
	return filtered
}
