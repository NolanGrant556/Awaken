package runtime

import (
	"sort"
	"strings"
	"time"
)

type ScheduledRunStatus string

const (
	ScheduledRunStatusScheduled  ScheduledRunStatus = "scheduled"
	ScheduledRunStatusTriggering ScheduledRunStatus = "triggering"
	ScheduledRunStatusTriggered  ScheduledRunStatus = "triggered"
	ScheduledRunStatusCancelled  ScheduledRunStatus = "cancelled"
	ScheduledRunStatusFailed     ScheduledRunStatus = "failed"
)

type CreateScheduledRunRequest struct {
	Input     string          `json:"input"`
	Workspace WorkspaceConfig `json:"workspace"`
	RunAt     time.Time       `json:"run_at"`
}

type ScheduledRun struct {
	ID        string             `json:"id"`
	Input     string             `json:"input"`
	Workspace WorkspaceConfig    `json:"workspace"`
	RunAt     time.Time          `json:"run_at"`
	Status    ScheduledRunStatus `json:"status"`
	RunID     string             `json:"run_id,omitempty"`
	Error     string             `json:"error,omitempty"`
	CreatedAt time.Time          `json:"created_at"`
	UpdatedAt time.Time          `json:"updated_at"`
}

type scheduledRunState struct {
	run   ScheduledRun
	timer scheduledTimer
}

type scheduleClock interface {
	Now() time.Time
	AfterFunc(d time.Duration, f func()) scheduledTimer
}

type scheduledTimer interface {
	Stop() bool
}

type realScheduleClock struct{}

func (realScheduleClock) Now() time.Time {
	return time.Now().UTC()
}

func (realScheduleClock) AfterFunc(d time.Duration, f func()) scheduledTimer {
	return time.AfterFunc(d, f)
}

func (s *RunService) CreateScheduledRun(req CreateScheduledRunRequest) (ScheduledRun, error) {
	input := strings.TrimSpace(req.Input)
	if input == "" {
		return ScheduledRun{}, ErrInvalidInput
	}
	if req.RunAt.IsZero() {
		return ScheduledRun{}, ErrInvalidRunAt
	}

	workspace, err := normalizeWorkspace(req.Workspace)
	if err != nil {
		return ScheduledRun{}, err
	}

	clock := s.schedulerClock()
	now := clock.Now().UTC()
	runAt := req.RunAt.UTC()
	id := newRuntimeID("scheduled_run")
	state := &scheduledRunState{
		run: ScheduledRun{
			ID:        id,
			Input:     input,
			Workspace: workspace,
			RunAt:     runAt,
			Status:    ScheduledRunStatusScheduled,
			CreatedAt: now,
			UpdatedAt: now,
		},
	}

	s.mu.Lock()
	s.scheduledRuns[id] = state
	s.mu.Unlock()

	delay := runAt.Sub(now)
	if delay < 0 {
		delay = 0
	}
	timer := clock.AfterFunc(delay, func() {
		s.triggerScheduledRun(id)
	})

	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.scheduledRuns[id]
	if !ok {
		if timer != nil {
			timer.Stop()
		}
		return ScheduledRun{}, ErrScheduledRunNotFound
	}
	if current.run.Status == ScheduledRunStatusScheduled {
		current.timer = timer
	} else if timer != nil {
		timer.Stop()
	}
	return snapshotScheduledRun(current.run), nil
}

func (s *RunService) ListScheduledRuns() []ScheduledRun {
	s.mu.RLock()
	defer s.mu.RUnlock()

	runs := make([]ScheduledRun, 0, len(s.scheduledRuns))
	for _, state := range s.scheduledRuns {
		runs = append(runs, snapshotScheduledRun(state.run))
	}
	sort.Slice(runs, func(i, j int) bool {
		return runs[i].CreatedAt.Before(runs[j].CreatedAt)
	})
	return runs
}

func (s *RunService) CancelScheduledRun(id string) (ScheduledRun, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return ScheduledRun{}, ErrScheduledRunNotFound
	}

	clock := s.schedulerClock()
	now := clock.Now().UTC()

	s.mu.Lock()
	defer s.mu.Unlock()

	state, ok := s.scheduledRuns[id]
	if !ok {
		return ScheduledRun{}, ErrScheduledRunNotFound
	}

	switch state.run.Status {
	case ScheduledRunStatusScheduled:
		if state.timer != nil {
			state.timer.Stop()
			state.timer = nil
		}
		state.run.Status = ScheduledRunStatusCancelled
		state.run.UpdatedAt = now
		return snapshotScheduledRun(state.run), nil
	case ScheduledRunStatusCancelled:
		return snapshotScheduledRun(state.run), nil
	default:
		return ScheduledRun{}, ErrScheduledRunNotCancelable
	}
}

func (s *RunService) triggerScheduledRun(id string) {
	clock := s.schedulerClock()
	now := clock.Now().UTC()

	s.mu.Lock()
	state, ok := s.scheduledRuns[id]
	if !ok || state.run.Status != ScheduledRunStatusScheduled {
		s.mu.Unlock()
		return
	}
	state.run.Status = ScheduledRunStatusTriggering
	state.run.UpdatedAt = now
	state.timer = nil
	req := StartRunRequest{
		Input:     state.run.Input,
		Workspace: state.run.Workspace,
	}
	s.mu.Unlock()

	run, err := s.StartRun(req)

	now = clock.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok = s.scheduledRuns[id]
	if !ok || state.run.Status != ScheduledRunStatusTriggering {
		return
	}
	state.run.UpdatedAt = now
	if err != nil {
		state.run.Status = ScheduledRunStatusFailed
		state.run.Error = err.Error()
		return
	}
	state.run.Status = ScheduledRunStatusTriggered
	state.run.RunID = run.RunID
}

func (s *RunService) schedulerClock() scheduleClock {
	if s.scheduleClock == nil {
		return realScheduleClock{}
	}
	return s.scheduleClock
}

func snapshotScheduledRun(run ScheduledRun) ScheduledRun {
	run.Workspace.WritableRoots = append([]string(nil), run.Workspace.WritableRoots...)
	return run
}
