package agentcore

import (
	stdcontext "context"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"awaken/src/tool"
)

// Run-plan validation bounds (contract [校验规则](docs/02_主智能体闭环/运行计划/实现方案.md#校验规则)).
const (
	runPlanMinItems          = 1
	runPlanMaxItems          = 12
	runPlanMaxTextRunes      = 160
	runPlanContextBlockRunes = 4000
)

// RunPlanCounts derives the display counters of one snapshot: the current
// step is completed+1 and equals the total when every item is completed.
type RunPlanCounts struct {
	CompletedCount int
	TotalCount     int
	CurrentStep    int
	AllCompleted   bool
}

func DeriveRunPlanCounts(items []tool.RunPlanItem) RunPlanCounts {
	counts := RunPlanCounts{TotalCount: len(items)}
	for _, item := range items {
		if item.Status == tool.RunPlanCompleted {
			counts.CompletedCount++
		}
	}
	counts.AllCompleted = counts.TotalCount > 0 && counts.CompletedCount == counts.TotalCount
	counts.CurrentStep = counts.CompletedCount + 1
	if counts.CurrentStep > counts.TotalCount {
		counts.CurrentStep = counts.TotalCount
	}
	return counts
}

// RunPlanPersistFunc durably appends AND flushes one plan_updated semantic
// record. It is the journal-first gate: ApplyUpdate must not touch in-memory
// state when it fails.
type RunPlanPersistFunc func(ctx stdcontext.Context, record SemanticRecord) error

// RunPlanServiceConfig binds the run-scoped service to its host identity.
type RunPlanServiceConfig struct {
	ThreadID  string
	RunID     string
	SessionID string
	Persist   RunPlanPersistFunc
	// Now and NewPlanID are test seams; nil uses wall clock and newAwakenID.
	Now       func() time.Time
	NewPlanID func() string
}

type runPlanService struct {
	mu       sync.Mutex
	config   RunPlanServiceConfig
	snapshot *tool.RunPlanSnapshot
}

// NewRunPlanService creates the run-scoped RunPlanService for one root run.
// Child agents must never receive one (contract [工具定义](docs/02_主智能体闭环/运行计划/实现方案.md#工具定义): root-only tool).
func NewRunPlanService(config RunPlanServiceConfig) tool.RunPlanService {
	if config.Now == nil {
		config.Now = func() time.Time { return time.Now().UTC() }
	}
	if config.NewPlanID == nil {
		config.NewPlanID = func() string { return newAwakenID("plan") }
	}
	return &runPlanService{config: config}
}

func (s *runPlanService) Snapshot() (tool.RunPlanSnapshot, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.snapshot == nil {
		return tool.RunPlanSnapshot{}, false
	}
	return cloneRunPlanSnapshot(*s.snapshot), true
}

func (s *runPlanService) ApplyUpdate(ctx stdcontext.Context, identity tool.ToolExecutionIdentity, update tool.RunPlanUpdate) (tool.RunPlanResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	normalized, err := validateRunPlanUpdate(update)
	if err != nil {
		return tool.RunPlanResult{}, err
	}
	switch {
	case s.snapshot == nil:
		if update.BaseVersion != 0 {
			return tool.RunPlanResult{}, &tool.RunPlanError{
				Code:      tool.RunPlanErrorVersionConflict,
				Message:   fmt.Sprintf("first update of a run requires base_version 0, got %d", update.BaseVersion),
				Retryable: true,
			}
		}
	case update.BaseVersion != s.snapshot.Version:
		latest := cloneRunPlanSnapshot(*s.snapshot)
		return tool.RunPlanResult{}, &tool.RunPlanError{
			Code:           tool.RunPlanErrorVersionConflict,
			Message:        fmt.Sprintf("base_version %d does not match current version %d", update.BaseVersion, s.snapshot.Version),
			Retryable:      true,
			LatestVersion:  s.snapshot.Version,
			LatestSnapshot: &latest,
		}
	}
	if s.snapshot != nil && runPlanCompletedRegression(*s.snapshot, normalized.Items) && strings.TrimSpace(update.Explanation) == "" {
		return tool.RunPlanResult{}, &tool.RunPlanError{
			Code:      tool.RunPlanErrorInvalidInput,
			Message:   "reverting or deleting a completed step requires a non-empty explanation",
			Retryable: false,
		}
	}

	planID := ""
	version := uint64(1)
	if s.snapshot != nil {
		planID = s.snapshot.PlanID
		version = s.snapshot.Version + 1
	} else {
		planID = s.config.NewPlanID()
	}
	next := tool.RunPlanSnapshot{
		PlanID:      planID,
		Version:     version,
		Explanation: strings.TrimSpace(update.Explanation),
		Items:       normalized.Items,
		UpdatedAt:   s.config.Now(),
	}
	counts := DeriveRunPlanCounts(next.Items)

	// Journal-first causal order (contract [SemanticJournal](docs/02_主智能体闭环/运行计划/实现方案.md#semanticjournal)): the durable plan_updated
	// record must be appended and flushed before the in-memory snapshot moves.
	if s.config.Persist == nil {
		return tool.RunPlanResult{}, &tool.RunPlanError{
			Code:      tool.RunPlanErrorPersistenceFailed,
			Message:   string(ErrPersistenceUnavailable.Error()),
			Retryable: true,
		}
	}
	record := PlanUpdatedSemanticRecord(
		s.config.ThreadID, s.config.RunID, identity.TurnID, s.config.SessionID,
		identity.ToolCallID, next, counts,
	)
	if err := s.config.Persist(ctx, record); err != nil {
		return tool.RunPlanResult{}, &tool.RunPlanError{
			Code:      tool.RunPlanErrorPersistenceFailed,
			Message:   err.Error(),
			Retryable: true,
		}
	}
	s.snapshot = &next

	return tool.RunPlanResult{
		Snapshot:       cloneRunPlanSnapshot(next),
		CompletedCount: counts.CompletedCount,
		TotalCount:     counts.TotalCount,
		CurrentStep:    counts.CurrentStep,
		AllCompleted:   counts.AllCompleted,
	}, nil
}

func (s *runPlanService) ContextBlock() (string, error) {
	snapshot, ok := s.Snapshot()
	if !ok {
		return "", nil
	}
	counts := DeriveRunPlanCounts(snapshot.Items)
	var builder strings.Builder
	builder.WriteString("<awaken_run_plan>\n")
	builder.WriteString("This block is generated by the Awaken host for the current run. It is the latest durable run plan snapshot, not a user message. Keep it in sync only through the update_plan tool.\n\n")
	builder.WriteString(fmt.Sprintf("plan_id: %s\n", snapshot.PlanID))
	builder.WriteString(fmt.Sprintf("version: %d\n", snapshot.Version))
	builder.WriteString(fmt.Sprintf("progress: %d/%d completed (current step %d)\n", counts.CompletedCount, counts.TotalCount, counts.CurrentStep))
	for index, item := range snapshot.Items {
		builder.WriteString(fmt.Sprintf("%d. [%s] %s", index+1, item.Status, item.Step))
		if item.Status == tool.RunPlanInProgress && strings.TrimSpace(item.ActiveForm) != "" && item.ActiveForm != item.Step {
			builder.WriteString(fmt.Sprintf(" — %s", item.ActiveForm))
		}
		builder.WriteString("\n")
	}
	builder.WriteString("</awaken_run_plan>")
	content := builder.String()
	if utf8.RuneCountInString(content) > runPlanContextBlockRunes {
		runes := []rune(content)
		content = string(runes[:runPlanContextBlockRunes]) + "\n... (run plan block truncated)\n</awaken_run_plan>"
	}
	return content, nil
}

type runPlanValidatedUpdate struct {
	Items []tool.RunPlanItem
}

func validateRunPlanUpdate(update tool.RunPlanUpdate) (runPlanValidatedUpdate, error) {
	invalid := func(format string, args ...any) (runPlanValidatedUpdate, error) {
		return runPlanValidatedUpdate{}, &tool.RunPlanError{
			Code:      tool.RunPlanErrorInvalidInput,
			Message:   fmt.Sprintf(format, args...),
			Retryable: false,
		}
	}
	if len(update.Items) < runPlanMinItems || len(update.Items) > runPlanMaxItems {
		return invalid("plan must contain %d-%d items, got %d", runPlanMinItems, runPlanMaxItems, len(update.Items))
	}
	items := make([]tool.RunPlanItem, len(update.Items))
	seenSteps := map[string]int{}
	inProgressCount := 0
	for index, item := range update.Items {
		step := strings.TrimSpace(item.Step)
		activeForm := strings.TrimSpace(item.ActiveForm)
		if length := utf8.RuneCountInString(step); length < 1 || length > runPlanMaxTextRunes {
			return invalid("item %d step must be 1-%d characters after trimming, got %d", index+1, runPlanMaxTextRunes, length)
		}
		if length := utf8.RuneCountInString(activeForm); length < 1 || length > runPlanMaxTextRunes {
			return invalid("item %d active_form must be 1-%d characters after trimming, got %d", index+1, runPlanMaxTextRunes, length)
		}
		if first, duplicate := seenSteps[step]; duplicate {
			return invalid("item %d step duplicates item %d after normalization", index+1, first+1)
		}
		seenSteps[step] = index
		switch item.Status {
		case tool.RunPlanPending, tool.RunPlanInProgress, tool.RunPlanCompleted:
		default:
			return invalid("item %d status %q is not pending, in_progress or completed", index+1, item.Status)
		}
		if item.Status == tool.RunPlanInProgress {
			inProgressCount++
		}
		items[index] = tool.RunPlanItem{Step: step, ActiveForm: activeForm, Status: item.Status}
	}
	completedCount := 0
	for _, item := range items {
		if item.Status == tool.RunPlanCompleted {
			completedCount++
		}
	}
	allCompleted := completedCount == len(items)
	if allCompleted {
		if inProgressCount != 0 {
			return invalid("an all-completed plan must not contain in_progress items")
		}
	} else if inProgressCount != 1 {
		return invalid("a plan that is not fully completed must contain exactly one in_progress item, got %d", inProgressCount)
	}
	// Order: completed* + optional single in_progress + pending*.
	phase := 0
	for index, item := range items {
		switch item.Status {
		case tool.RunPlanCompleted:
			if phase != 0 {
				return invalid("item %d is completed but appears after an in_progress or pending item", index+1)
			}
		case tool.RunPlanInProgress:
			if phase > 1 {
				return invalid("item %d is in_progress but appears after a pending item", index+1)
			}
			phase = 1
		case tool.RunPlanPending:
			phase = 2
		}
	}
	return runPlanValidatedUpdate{Items: items}, nil
}

// runPlanCompletedRegression reports whether any step that was completed in
// the previous snapshot was reverted to a non-completed status or removed.
// Steps are matched by their normalized text; snapshots carry no item IDs.
func runPlanCompletedRegression(previous tool.RunPlanSnapshot, next []tool.RunPlanItem) bool {
	nextStatus := make(map[string]tool.RunPlanStatus, len(next))
	for _, item := range next {
		nextStatus[item.Step] = item.Status
	}
	for _, old := range previous.Items {
		if old.Status != tool.RunPlanCompleted {
			continue
		}
		status, kept := nextStatus[old.Step]
		if !kept || status != tool.RunPlanCompleted {
			return true
		}
	}
	return false
}

func cloneRunPlanSnapshot(snapshot tool.RunPlanSnapshot) tool.RunPlanSnapshot {
	snapshot.Items = append([]tool.RunPlanItem(nil), snapshot.Items...)
	return snapshot
}
