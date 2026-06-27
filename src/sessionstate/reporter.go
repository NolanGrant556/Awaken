package sessionstate

import "sync"

type State string

const (
	StateIdle           State = "idle"
	StateRunning        State = "running"
	StateRequiresAction State = "requires_action"
)

type RequiresActionDetails struct {
	ToolName          string
	ActionDescription string
	ToolUseID         string
	RequestID         string
	Input             map[string]any
}

type Metadata struct {
	PendingAction        *RequiresActionDetails
	PendingActionChanged bool
	TaskSummary          *string
	TaskSummaryChanged   bool
}

type StateListener func(State, *RequiresActionDetails)
type MetadataListener func(Metadata)

type Reporter struct {
	mu               sync.Mutex
	state            State
	hasPendingAction bool
	stateListener    StateListener
	metadataListener MetadataListener
}

func NewReporter() *Reporter {
	return &Reporter{state: StateIdle}
}

func (r *Reporter) State() State {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.state
}

func (r *Reporter) SetStateListener(listener StateListener) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stateListener = listener
}

func (r *Reporter) SetMetadataListener(listener MetadataListener) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.metadataListener = listener
}

func (r *Reporter) Transition(state State, details *RequiresActionDetails) {
	stateDetails := cloneDetails(details)
	pendingDetails := cloneDetails(details)

	var metadataEvents []Metadata

	r.mu.Lock()
	r.state = state
	stateListener := r.stateListener
	metadataListener := r.metadataListener

	if state == StateRequiresAction && pendingDetails != nil {
		r.hasPendingAction = true
		metadataEvents = append(metadataEvents, Metadata{
			PendingAction:        pendingDetails,
			PendingActionChanged: true,
		})
	} else if r.hasPendingAction {
		r.hasPendingAction = false
		metadataEvents = append(metadataEvents, Metadata{
			PendingActionChanged: true,
		})
	}

	if state == StateIdle {
		metadataEvents = append(metadataEvents, Metadata{
			TaskSummaryChanged: true,
		})
	}
	r.mu.Unlock()

	if stateListener != nil {
		stateListener(state, stateDetails)
	}
	if metadataListener != nil {
		for _, metadata := range metadataEvents {
			metadataListener(cloneMetadata(metadata))
		}
	}
}

func (r *Reporter) NotifyMetadata(metadata Metadata) {
	r.mu.Lock()
	listener := r.metadataListener
	r.mu.Unlock()

	if listener != nil {
		listener(cloneMetadata(metadata))
	}
}

func cloneMetadata(metadata Metadata) Metadata {
	clone := metadata
	clone.PendingAction = cloneDetails(metadata.PendingAction)
	if metadata.TaskSummary != nil {
		taskSummary := *metadata.TaskSummary
		clone.TaskSummary = &taskSummary
	}
	return clone
}

func cloneDetails(details *RequiresActionDetails) *RequiresActionDetails {
	if details == nil {
		return nil
	}
	return &RequiresActionDetails{
		ToolName:          details.ToolName,
		ActionDescription: details.ActionDescription,
		ToolUseID:         details.ToolUseID,
		RequestID:         details.RequestID,
		Input:             cloneInput(details.Input),
	}
}

func cloneInput(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}

	clone := make(map[string]any, len(input))
	for key, value := range input {
		clone[key] = cloneValue(value)
	}
	return clone
}

func cloneValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneInput(typed)
	case []any:
		clone := make([]any, len(typed))
		for i, item := range typed {
			clone[i] = cloneValue(item)
		}
		return clone
	default:
		return value
	}
}
