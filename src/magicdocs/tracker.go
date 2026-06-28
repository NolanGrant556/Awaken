package magicdocs

import (
	"fmt"

	convcontext "awaken/src/context"
)

const QuerySourceReplMainThread = "repl_main_thread"

type PlanSkipReason string

const (
	PlanSkipNone                   PlanSkipReason = ""
	PlanSkipNonMainQuery           PlanSkipReason = "non_main_query"
	PlanSkipLastAssistantToolCalls PlanSkipReason = "last_assistant_turn_has_tool_calls"
	PlanSkipNoTrackedDocs          PlanSkipReason = "no_tracked_docs"
)

type ReadStatus string

const (
	ReadStatusOK           ReadStatus = "ok"
	ReadStatusDeleted      ReadStatus = "deleted"
	ReadStatusInaccessible ReadStatus = "inaccessible"
	ReadStatusError        ReadStatus = "error"
)

type ReadResult struct {
	Content string
	Status  ReadStatus
	Err     error
}

type TrackedDoc struct {
	Path string
}

type Tracker struct {
	docs  map[string]TrackedDoc
	order []string
}

type UpdateContext struct {
	QuerySource string
	Messages    []convcontext.Message
	Latest      map[string]ReadResult
}

type UpdatePlan struct {
	Sequential bool
	SkipReason PlanSkipReason
	Updates    []UpdateTask
}

type UpdateTask struct {
	Path      string
	Content   string
	Detection Detection
}

func NewTracker() *Tracker {
	return &Tracker{
		docs: make(map[string]TrackedDoc),
	}
}

func (t *Tracker) Register(path string) bool {
	if t == nil || path == "" {
		return false
	}
	if t.docs == nil {
		t.docs = make(map[string]TrackedDoc)
	}
	if _, exists := t.docs[path]; exists {
		return false
	}
	t.docs[path] = TrackedDoc{Path: path}
	t.order = append(t.order, path)
	return true
}

func (t *Tracker) RegisterIfMagic(path string, content string) (Detection, bool) {
	detection, ok := DetectMagicDocHeader(content)
	if !ok {
		return Detection{}, false
	}
	t.Register(path)
	return detection, true
}

func (t *Tracker) Remove(path string) {
	if t == nil {
		return
	}
	if _, exists := t.docs[path]; !exists {
		return
	}
	delete(t.docs, path)
	for i, candidate := range t.order {
		if candidate == path {
			t.order = append(t.order[:i], t.order[i+1:]...)
			return
		}
	}
}

func (t *Tracker) Len() int {
	if t == nil {
		return 0
	}
	return len(t.docs)
}

func (t *Tracker) IsTracked(path string) bool {
	if t == nil {
		return false
	}
	_, ok := t.docs[path]
	return ok
}

func (t *Tracker) Paths() []string {
	if t == nil {
		return nil
	}
	paths := make([]string, 0, len(t.order))
	for _, path := range t.order {
		if _, ok := t.docs[path]; ok {
			paths = append(paths, path)
		}
	}
	return paths
}

func (t *Tracker) PlanUpdates(ctx UpdateContext) (UpdatePlan, error) {
	plan := UpdatePlan{Sequential: true}
	if ctx.QuerySource != QuerySourceReplMainThread {
		plan.SkipReason = PlanSkipNonMainQuery
		return plan, nil
	}
	if LastAssistantTurnHasToolCalls(ctx.Messages) {
		plan.SkipReason = PlanSkipLastAssistantToolCalls
		return plan, nil
	}
	if t == nil || t.Len() == 0 {
		plan.SkipReason = PlanSkipNoTrackedDocs
		return plan, nil
	}

	for _, path := range t.Paths() {
		result, ok := ctx.Latest[path]
		if !ok {
			return plan, fmt.Errorf("missing latest read result for tracked magic doc %s", path)
		}

		switch result.normalizedStatus() {
		case ReadStatusDeleted, ReadStatusInaccessible:
			t.Remove(path)
			continue
		case ReadStatusError:
			return plan, readFailureError(path, result)
		case ReadStatusOK:
		default:
			return plan, fmt.Errorf("unknown read status %q for tracked magic doc %s", result.Status, path)
		}

		detection, detected := DetectMagicDocHeader(result.Content)
		if !detected {
			t.Remove(path)
			continue
		}
		plan.Updates = append(plan.Updates, UpdateTask{
			Path:      path,
			Content:   result.Content,
			Detection: detection,
		})
	}

	return plan, nil
}

func LastAssistantTurnHasToolCalls(messages []convcontext.Message) bool {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == convcontext.RoleAssistant {
			return len(messages[i].ToolCalls) > 0
		}
	}
	return false
}

func SuccessfulRead(content string) ReadResult {
	return ReadResult{Content: content, Status: ReadStatusOK}
}

func DeletedRead(err error) ReadResult {
	return ReadResult{Status: ReadStatusDeleted, Err: err}
}

func InaccessibleRead(err error) ReadResult {
	return ReadResult{Status: ReadStatusInaccessible, Err: err}
}

func FailedRead(err error) ReadResult {
	return ReadResult{Status: ReadStatusError, Err: err}
}

func (r ReadResult) normalizedStatus() ReadStatus {
	if r.Status != "" {
		return r.Status
	}
	if r.Err != nil {
		return ReadStatusError
	}
	return ReadStatusOK
}

func readFailureError(path string, result ReadResult) error {
	if result.Err != nil {
		return fmt.Errorf("read magic doc %s failed: %w", path, result.Err)
	}
	return fmt.Errorf("read magic doc %s failed", path)
}
