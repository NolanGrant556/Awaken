package agenttask

import (
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

var (
	ErrInvalidTaskID     = errors.New("agenttask: invalid task id")
	ErrInvalidAgent      = errors.New("agenttask: invalid agent")
	ErrInvalidWorkspace  = errors.New("agenttask: invalid workspace")
	ErrInvalidMode       = errors.New("agenttask: invalid mode")
	ErrInvalidTranscript = errors.New("agenttask: invalid transcript path")
	ErrInvalidMessage    = errors.New("agenttask: invalid message")
	ErrInvalidLimit      = errors.New("agenttask: invalid limit")
	ErrInvalidStopReason = errors.New("agenttask: invalid stop reason")
	ErrTaskExists        = errors.New("agenttask: task already exists")
	ErrTaskNotFound      = errors.New("agenttask: task not found")
	ErrTaskTerminal      = errors.New("agenttask: task is terminal")
	ErrInvalidTransition = errors.New("agenttask: invalid status transition")
)

type Manager struct {
	mu     sync.RWMutex
	tasks  map[string]*taskState
	order  []string
	nextID uint64
	now    func() time.Time
}

type taskState struct {
	task             Task
	events           []TaskEvent
	nextEventSeq     uint64
	nextMessageSeq   uint64
	nextContinuation uint64
}

func NewManager() *Manager {
	return &Manager{
		tasks: make(map[string]*taskState),
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
}

func (m *Manager) Start(options StartOptions) (Task, error) {
	if m == nil {
		return Task{}, errors.New("agenttask: nil manager")
	}

	options.ID = strings.TrimSpace(options.ID)
	options.ParentRunID = strings.TrimSpace(options.ParentRunID)
	options.Workspace = strings.TrimSpace(options.Workspace)
	options.TranscriptPath = strings.TrimSpace(options.TranscriptPath)
	options.Agent = normalizeAgent(options.Agent)
	options.Mode = normalizeMode(options.Mode)

	m.mu.Lock()
	defer m.mu.Unlock()

	if options.ID == "" {
		m.nextID++
		options.ID = "agenttask-" + strconv.FormatUint(m.nextID, 10)
	}
	if err := validateTaskID(options.ID); err != nil {
		return Task{}, err
	}
	if options.ParentRunID != "" {
		if err := validateTaskID(options.ParentRunID); err != nil {
			return Task{}, fmt.Errorf("%w: parent run id %q", ErrInvalidTaskID, options.ParentRunID)
		}
	}
	if err := validateAgent(options.Agent); err != nil {
		return Task{}, err
	}
	if err := validateWorkspace(options.Workspace); err != nil {
		return Task{}, err
	}
	if err := validateMode(options.Mode); err != nil {
		return Task{}, err
	}
	if err := validateTranscriptPath(options.TranscriptPath); err != nil {
		return Task{}, err
	}
	if _, exists := m.tasks[options.ID]; exists {
		return Task{}, fmt.Errorf("%w: %s", ErrTaskExists, options.ID)
	}

	now := m.now().UTC()
	state := &taskState{
		task: Task{
			ID:             options.ID,
			ParentRunID:    options.ParentRunID,
			Description:    strings.TrimSpace(options.Description),
			Prompt:         strings.TrimSpace(options.Prompt),
			Mode:           options.Mode,
			Agent:          options.Agent,
			Workspace:      options.Workspace,
			TranscriptPath: options.TranscriptPath,
			Status:         TaskStatusRunning,
			CreatedAt:      now,
			UpdatedAt:      now,
			LastActivityAt: now,
		},
	}
	m.appendEventLocked(state, TaskEvent{
		Type:      TaskEventStarted,
		Timestamp: now,
		Summary:   state.task.Description,
	})

	m.tasks[state.task.ID] = state
	m.order = append(m.order, state.task.ID)

	return cloneTask(state.task), nil
}

func (m *Manager) RegisterEvent(taskID string, event TaskEvent) (TaskEvent, error) {
	taskID = strings.TrimSpace(taskID)
	if err := validateTaskID(taskID); err != nil {
		return TaskEvent{}, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	state, err := m.requireTaskLocked(taskID)
	if err != nil {
		return TaskEvent{}, err
	}
	if IsTerminalStatus(state.task.Status) {
		return TaskEvent{}, fmt.Errorf("%w: %s", ErrTaskTerminal, taskID)
	}

	if event.Type == "" {
		event.Type = TaskEventOutput
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = m.now().UTC()
	} else {
		event.Timestamp = event.Timestamp.UTC()
	}

	appended := m.appendEventLocked(state, event)
	state.task.UpdatedAt = appended.Timestamp
	state.task.LastActivityAt = appended.Timestamp

	if event.Activity != "" {
		state.task.Progress.LastActivity = event.Activity
	}
	if event.InputTokenDelta != 0 {
		state.task.Progress.InputTokenCount += event.InputTokenDelta
	}
	if event.OutputTokenDelta != 0 {
		state.task.Progress.OutputTokenCount += event.OutputTokenDelta
	}
	tokenDelta := event.TokenDelta
	if tokenDelta == 0 {
		tokenDelta = event.InputTokenDelta + event.OutputTokenDelta
	}
	state.task.Progress.TokenCount += tokenDelta
	if event.ToolUseDelta != 0 {
		state.task.Progress.ToolUseCount += event.ToolUseDelta
	} else if event.Type == TaskEventToolUse {
		state.task.Progress.ToolUseCount++
	}

	return appended, nil
}

func (m *Manager) Get(taskID string) (Task, error) {
	taskID = strings.TrimSpace(taskID)
	if err := validateTaskID(taskID); err != nil {
		return Task{}, err
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	state, err := m.requireTaskLocked(taskID)
	if err != nil {
		return Task{}, err
	}
	return cloneTask(state.task), nil
}

func (m *Manager) List() []Task {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tasks := make([]Task, 0, len(m.order))
	for _, id := range m.order {
		state := m.tasks[id]
		if state == nil {
			continue
		}
		tasks = append(tasks, cloneTask(state.task))
	}
	return tasks
}

func (m *Manager) ReadOutput(taskID string, afterSeq uint64, limit int) (OutputPage, error) {
	taskID = strings.TrimSpace(taskID)
	if err := validateTaskID(taskID); err != nil {
		return OutputPage{}, err
	}
	if limit < 0 {
		return OutputPage{}, fmt.Errorf("%w: %d", ErrInvalidLimit, limit)
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	state, err := m.requireTaskLocked(taskID)
	if err != nil {
		return OutputPage{}, err
	}

	events := make([]TaskEvent, 0)
	for _, event := range state.events {
		if event.Seq <= afterSeq {
			continue
		}
		if limit > 0 && len(events) == limit {
			break
		}
		events = append(events, event)
	}

	lastSeq := uint64(0)
	if len(state.events) > 0 {
		lastSeq = state.events[len(state.events)-1].Seq
	}
	nextAfterSeq := afterSeq
	if len(events) > 0 {
		nextAfterSeq = events[len(events)-1].Seq
	}

	hasMore := false
	if len(events) > 0 {
		for _, event := range state.events {
			if event.Seq > nextAfterSeq {
				hasMore = true
				break
			}
		}
	} else if afterSeq < lastSeq {
		hasMore = true
	}

	return OutputPage{
		TaskID: taskID,
		Events: append([]TaskEvent(nil), events...),
		Cursor: OutputCursor{
			AfterSeq:     afterSeq,
			NextAfterSeq: nextAfterSeq,
			LastSeq:      lastSeq,
			Limit:        limit,
			HasMore:      hasMore,
		},
	}, nil
}

func (m *Manager) SendMessage(taskID string, request ContinuationRequest) (ContinuationRequest, error) {
	taskID = strings.TrimSpace(taskID)
	if err := validateTaskID(taskID); err != nil {
		return ContinuationRequest{}, err
	}
	if request.TaskID != "" && request.TaskID != taskID {
		return ContinuationRequest{}, fmt.Errorf("%w: continuation task id %q does not match %q", ErrInvalidTaskID, request.TaskID, taskID)
	}
	request.Content = strings.TrimSpace(request.Content)
	request.Summary = strings.TrimSpace(request.Summary)
	request.Sender = strings.TrimSpace(request.Sender)
	if request.Content == "" || hasControlRune(request.Content) {
		return ContinuationRequest{}, ErrInvalidMessage
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	state, err := m.requireTaskLocked(taskID)
	if err != nil {
		return ContinuationRequest{}, err
	}
	if IsTerminalStatus(state.task.Status) {
		return ContinuationRequest{}, fmt.Errorf("%w: %s", ErrTaskTerminal, taskID)
	}

	now := m.now().UTC()
	state.nextMessageSeq++
	message := TaskMessage{
		ID:        "msg-" + strconv.FormatUint(state.nextMessageSeq, 10),
		TaskID:    taskID,
		Seq:       state.nextMessageSeq,
		Role:      MessageRoleUser,
		Sender:    request.Sender,
		Summary:   request.Summary,
		Content:   request.Content,
		CreatedAt: now,
	}
	state.task.Messages = append(state.task.Messages, message)

	state.nextContinuation++
	request.ID = "cont-" + strconv.FormatUint(state.nextContinuation, 10)
	request.TaskID = taskID
	request.MessageID = message.ID
	request.ParentRunID = state.task.ParentRunID
	request.CreatedAt = now
	state.task.Continuations = append(state.task.Continuations, request)
	state.task.Progress.MessageCount++
	state.task.UpdatedAt = now
	state.task.LastActivityAt = now
	state.task.Progress.LastActivity = "message_sent"

	m.appendEventLocked(state, TaskEvent{
		Type:      TaskEventMessageSent,
		Timestamp: now,
		MessageID: message.ID,
		Role:      MessageRoleUser,
		Content:   request.Content,
		Summary:   request.Summary,
	})

	return request, nil
}

func (m *Manager) RequestStop(taskID string, reason StopReason, message string) (Task, error) {
	taskID = strings.TrimSpace(taskID)
	if err := validateTaskID(taskID); err != nil {
		return Task{}, err
	}
	reason = normalizeStopReason(reason)
	if err := validateStopReason(reason); err != nil {
		return Task{}, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	state, err := m.requireTaskLocked(taskID)
	if err != nil {
		return Task{}, err
	}
	if err := validateTransition(state.task.Status, TaskStatusStopped); err != nil {
		return Task{}, err
	}

	now := m.now().UTC()
	state.task.Status = TaskStatusStopped
	state.task.CancelRequested = true
	state.task.StopRequested = true
	state.task.StopRequestedAt = &now
	state.task.StopReason = reason
	state.task.StopMessage = strings.TrimSpace(message)
	state.task.UpdatedAt = now
	state.task.LastActivityAt = now
	state.task.FinishedAt = &now
	state.task.Progress.LastActivity = "stop_requested"
	m.appendEventLocked(state, TaskEvent{
		Type:      TaskEventStopRequested,
		Timestamp: now,
		Summary:   state.task.StopMessage,
	})

	return cloneTask(state.task), nil
}

func (m *Manager) Complete(taskID string, result string) (Task, error) {
	taskID = strings.TrimSpace(taskID)
	if err := validateTaskID(taskID); err != nil {
		return Task{}, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	state, err := m.requireTaskLocked(taskID)
	if err != nil {
		return Task{}, err
	}
	if err := validateTransition(state.task.Status, TaskStatusCompleted); err != nil {
		return Task{}, err
	}

	now := m.now().UTC()
	state.task.Status = TaskStatusCompleted
	state.task.Result = strings.TrimSpace(result)
	state.task.UpdatedAt = now
	state.task.LastActivityAt = now
	state.task.FinishedAt = &now
	state.task.Progress.LastActivity = "completed"
	m.appendEventLocked(state, TaskEvent{
		Type:      TaskEventCompleted,
		Timestamp: now,
		Content:   state.task.Result,
	})

	return cloneTask(state.task), nil
}

func (m *Manager) Fail(taskID string, message string) (Task, error) {
	taskID = strings.TrimSpace(taskID)
	if err := validateTaskID(taskID); err != nil {
		return Task{}, err
	}
	message = strings.TrimSpace(message)
	if message == "" {
		message = "task failed"
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	state, err := m.requireTaskLocked(taskID)
	if err != nil {
		return Task{}, err
	}
	if err := validateTransition(state.task.Status, TaskStatusFailed); err != nil {
		return Task{}, err
	}

	now := m.now().UTC()
	state.task.Status = TaskStatusFailed
	state.task.Error = message
	state.task.UpdatedAt = now
	state.task.LastActivityAt = now
	state.task.FinishedAt = &now
	state.task.Progress.LastActivity = "failed"
	m.appendEventLocked(state, TaskEvent{
		Type:      TaskEventFailed,
		Timestamp: now,
		Content:   message,
	})

	return cloneTask(state.task), nil
}

func (m *Manager) requireTaskLocked(taskID string) (*taskState, error) {
	state := m.tasks[taskID]
	if state == nil {
		return nil, fmt.Errorf("%w: %s", ErrTaskNotFound, taskID)
	}
	return state, nil
}

func (m *Manager) appendEventLocked(state *taskState, event TaskEvent) TaskEvent {
	state.nextEventSeq++
	event.TaskID = state.task.ID
	event.Seq = state.nextEventSeq
	if event.Timestamp.IsZero() {
		event.Timestamp = m.now().UTC()
	} else {
		event.Timestamp = event.Timestamp.UTC()
	}
	state.events = append(state.events, event)
	state.task.Progress.OutputEventCount++
	return event
}

func validateTransition(from, to TaskStatus) error {
	if IsTerminalStatus(from) {
		return fmt.Errorf("%w: %s -> %s", ErrTaskTerminal, from, to)
	}
	if from != TaskStatusRunning {
		return fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, from, to)
	}
	switch to {
	case TaskStatusCompleted, TaskStatusFailed, TaskStatusStopped:
		return nil
	default:
		return fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, from, to)
	}
}

func cloneTask(task Task) Task {
	if task.FinishedAt != nil {
		finishedAt := *task.FinishedAt
		task.FinishedAt = &finishedAt
	}
	if task.StopRequestedAt != nil {
		stopRequestedAt := *task.StopRequestedAt
		task.StopRequestedAt = &stopRequestedAt
	}
	task.Messages = append([]TaskMessage(nil), task.Messages...)
	task.Continuations = append([]ContinuationRequest(nil), task.Continuations...)
	return task
}

func normalizeAgent(agent TaskAgent) TaskAgent {
	return TaskAgent{
		Type:  strings.TrimSpace(agent.Type),
		Name:  strings.TrimSpace(agent.Name),
		Model: strings.TrimSpace(agent.Model),
	}
}

func normalizeMode(mode TaskMode) TaskMode {
	if mode == "" {
		return TaskModeBackground
	}
	return mode
}

func normalizeStopReason(reason StopReason) StopReason {
	if reason == "" {
		return StopReasonUser
	}
	return reason
}

func validateTaskID(id string) error {
	if id == "" || len(id) > 128 {
		return fmt.Errorf("%w: %q", ErrInvalidTaskID, id)
	}
	for i, r := range id {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-' || r == '_' || r == '.':
			if i == 0 {
				return fmt.Errorf("%w: %q", ErrInvalidTaskID, id)
			}
		default:
			return fmt.Errorf("%w: %q", ErrInvalidTaskID, id)
		}
	}
	return nil
}

func validateAgent(agent TaskAgent) error {
	if agent.Type == "" {
		return fmt.Errorf("%w: missing type", ErrInvalidAgent)
	}
	if err := validateTaskID(agent.Type); err != nil {
		return fmt.Errorf("%w: invalid type %q", ErrInvalidAgent, agent.Type)
	}
	if agent.Name != "" && hasControlRune(agent.Name) {
		return fmt.Errorf("%w: invalid name", ErrInvalidAgent)
	}
	if agent.Model != "" && hasControlRune(agent.Model) {
		return fmt.Errorf("%w: invalid model", ErrInvalidAgent)
	}
	return nil
}

func validateWorkspace(workspace string) error {
	if workspace == "" || strings.ContainsRune(workspace, '\x00') || !filepath.IsAbs(workspace) {
		return fmt.Errorf("%w: %q", ErrInvalidWorkspace, workspace)
	}
	return nil
}

func validateMode(mode TaskMode) error {
	if mode != TaskModeBackground && mode != TaskModeForeground {
		return fmt.Errorf("%w: %q", ErrInvalidMode, mode)
	}
	return nil
}

func validateTranscriptPath(path string) error {
	if path == "" {
		return nil
	}
	if strings.ContainsRune(path, '\x00') || !filepath.IsAbs(path) {
		return fmt.Errorf("%w: %q", ErrInvalidTranscript, path)
	}
	return nil
}

func validateStopReason(reason StopReason) error {
	switch reason {
	case StopReasonUser, StopReasonParentCancelled, StopReasonTimeout, StopReasonRuntimeShutdown, StopReasonOther:
		return nil
	default:
		return fmt.Errorf("%w: %q", ErrInvalidStopReason, reason)
	}
}

func hasControlRune(value string) bool {
	for _, r := range value {
		if unicode.IsControl(r) && r != '\n' && r != '\t' && r != '\r' {
			return true
		}
	}
	return false
}
