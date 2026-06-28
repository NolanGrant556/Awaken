package settingssync

import (
	"context"
	"fmt"
	"sync"
)

type MigrationState interface {
	HasRun(id string) bool
	MarkRun(id string) error
}

type MemoryMigrationState struct {
	mu  sync.Mutex
	run map[string]bool
}

func NewMemoryMigrationState(completed ...string) *MemoryMigrationState {
	state := &MemoryMigrationState{run: map[string]bool{}}
	for _, id := range completed {
		state.run[id] = true
	}
	return state
}

func (s *MemoryMigrationState) HasRun(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.run[id]
}

func (s *MemoryMigrationState) MarkRun(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.run[id] = true
	return nil
}

type Migration struct {
	ID    string
	Plan  func(context.Context) (MigrationPlan, error)
	Apply func(context.Context) (MigrationApplyResult, error)
}

type MigrationChange struct {
	Key       string
	OldValue  string
	NewValue  string
	Sensitive bool
}

func (c MigrationChange) String() string {
	sensitive := c.Sensitive || IsSensitiveKey(c.Key) || LooksSensitiveValue(c.OldValue) || LooksSensitiveValue(c.NewValue)
	return fmt.Sprintf(
		"settingssync.MigrationChange{key:%q old:%q new:%q}",
		c.Key,
		RedactValue(c.Key, c.OldValue, sensitive),
		RedactValue(c.Key, c.NewValue, sensitive),
	)
}

type MigrationPlan struct {
	ID          string
	Description string
	Changes     []MigrationChange
	DryRun      bool
}

func (p MigrationPlan) Summary() string {
	return fmt.Sprintf("settingssync.MigrationPlan{id:%q changes:%d dryRun:%t}", p.ID, len(p.Changes), p.DryRun)
}

type MigrationApplyResult struct {
	ID      string
	Applied bool
	Changes []MigrationChange
	Message string
}

func (r MigrationApplyResult) Summary() string {
	return fmt.Sprintf("settingssync.MigrationApplyResult{id:%q applied:%t changes:%d message:%q}", r.ID, r.Applied, len(r.Changes), r.Message)
}

type MigrationStatus string

const (
	MigrationStatusPlanned MigrationStatus = "planned"
	MigrationStatusApplied MigrationStatus = "applied"
	MigrationStatusSkipped MigrationStatus = "skipped"
	MigrationStatusFailed  MigrationStatus = "failed"
)

type MigrationResult struct {
	ID     string
	Status MigrationStatus
	Plan   MigrationPlan
	Apply  MigrationApplyResult
	Error  string
}

type MigrationRunOptions struct {
	DryRun bool
}

type MigrationRunResult struct {
	Results []MigrationResult
}

func (r MigrationRunResult) Success() bool {
	for _, result := range r.Results {
		if result.Status == MigrationStatusFailed {
			return false
		}
	}
	return true
}

func RunMigrations(ctx context.Context, migrations []Migration, state MigrationState, opts MigrationRunOptions) MigrationRunResult {
	if state == nil {
		state = NewMemoryMigrationState()
	}

	result := MigrationRunResult{Results: make([]MigrationResult, 0, len(migrations))}
	for _, migration := range migrations {
		result.Results = append(result.Results, runMigration(ctx, migration, state, opts))
	}
	return result
}

func runMigration(ctx context.Context, migration Migration, state MigrationState, opts MigrationRunOptions) (result MigrationResult) {
	result.ID = migration.ID
	defer func() {
		if recovered := recover(); recovered != nil {
			result.Status = MigrationStatusFailed
			result.Error = fmt.Sprintf("migration panic: %v", recovered)
		}
	}()

	if migration.ID == "" {
		result.Status = MigrationStatusFailed
		result.Error = "migration id is required"
		return result
	}
	if state.HasRun(migration.ID) {
		result.Status = MigrationStatusSkipped
		return result
	}

	if migration.Plan != nil {
		plan, err := migration.Plan(ctx)
		if err != nil {
			result.Status = MigrationStatusFailed
			result.Error = err.Error()
			return result
		}
		if plan.ID == "" {
			plan.ID = migration.ID
		}
		plan.DryRun = opts.DryRun
		result.Plan = plan
	}

	if opts.DryRun {
		result.Status = MigrationStatusPlanned
		return result
	}

	if migration.Apply != nil {
		applyResult, err := migration.Apply(ctx)
		if err != nil {
			result.Status = MigrationStatusFailed
			result.Error = err.Error()
			return result
		}
		if applyResult.ID == "" {
			applyResult.ID = migration.ID
		}
		result.Apply = applyResult
	} else {
		result.Apply = MigrationApplyResult{ID: migration.ID, Applied: true}
	}

	if err := state.MarkRun(migration.ID); err != nil {
		result.Status = MigrationStatusFailed
		result.Error = err.Error()
		return result
	}
	result.Status = MigrationStatusApplied
	return result
}
