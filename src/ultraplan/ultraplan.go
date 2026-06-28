package ultraplan

import (
	"fmt"
	"strings"
	"time"
)

const (
	CanonicalRemoteModelID = "claude-opus-4-6"
	DefaultTimeout         = 30 * time.Minute
	TermsURL               = "https://code.claude.com/docs/en/claude-code-on-the-web"

	RemoteTaskTypeUltraplan = "ultraplan"
	PermissionModePlan      = "plan"
	NotificationModeTask    = "task-notification"
)

const DefaultInstructions = `<system-reminder>
You are running advanced remote plan mode. Explore the problem deeply, ask for browser input when blocked, and only surface the final plan once it is ready for explicit approval.
</system-reminder>`

type PromptInput struct {
	Blurb           string
	SeedPlan        string
	Instructions    string
	SystemReminder  string
	HideScaffolding bool
}

func BuildPrompt(input PromptInput) string {
	parts := make([]string, 0, 5)
	if strings.TrimSpace(input.SeedPlan) != "" {
		parts = append(parts, "Here is a draft plan to refine:", "", strings.TrimRight(input.SeedPlan, "\n"), "")
	}

	instructions := strings.TrimRight(input.Instructions, "\n")
	if reminder := strings.TrimSpace(input.SystemReminder); reminder != "" {
		instructions = wrapSystemReminder(reminder)
	} else if instructions == "" {
		instructions = DefaultInstructions
	}
	parts = append(parts, instructions)

	if strings.TrimSpace(input.Blurb) != "" {
		parts = append(parts, "", strings.TrimRight(input.Blurb, "\n"))
	}
	return strings.Join(parts, "\n")
}

func wrapSystemReminder(text string) string {
	trimmed := strings.TrimRight(text, "\n")
	if strings.Contains(trimmed, "<system-reminder>") {
		return trimmed
	}
	return "<system-reminder>\n" + trimmed + "\n</system-reminder>"
}

type PreconditionType string

const (
	PreconditionUnknown               PreconditionType = "unknown"
	PreconditionNotLoggedIn           PreconditionType = "not_logged_in"
	PreconditionNoRemoteEnvironment   PreconditionType = "no_remote_environment"
	PreconditionNotInGitRepo          PreconditionType = "not_in_git_repo"
	PreconditionNoGitRemote           PreconditionType = "no_git_remote"
	PreconditionGitHubAppNotInstalled PreconditionType = "github_app_not_installed"
	PreconditionPolicyBlocked         PreconditionType = "policy_blocked"
)

type PreconditionError struct {
	Type    PreconditionType
	Message string
}

type Eligibility struct {
	Eligible bool
	Errors   []PreconditionError
}

func FormatPreconditionError(err PreconditionError) string {
	if strings.TrimSpace(err.Message) != "" {
		return err.Message
	}
	switch err.Type {
	case PreconditionNotLoggedIn:
		return "Please sign in before launching a remote planning session."
	case PreconditionNoRemoteEnvironment:
		return "No cloud environment is available for remote planning."
	case PreconditionNotInGitRepo:
		return "Remote planning requires a git repository."
	case PreconditionNoGitRemote:
		return "Remote planning requires a GitHub remote."
	case PreconditionGitHubAppNotInstalled:
		return "The GitHub app must be installed on this repository first."
	case PreconditionPolicyBlocked:
		return "Remote sessions are disabled by policy."
	default:
		return "Remote planning preconditions are not satisfied."
	}
}

type LaunchInput struct {
	Blurb           string
	SeedPlan        string
	Instructions    string
	SystemReminder  string
	HideScaffolding bool

	ActiveURL        string
	ActiveSessionURL string
	Launching        bool

	Preconditions      []PreconditionError
	PreconditionErrors []PreconditionError
	Eligibility        Eligibility
	EligibilityChecked bool

	TaskID             string
	SessionID          string
	SessionURL         string
	Session            RemoteSession
	SessionIngressURL  string
	ModelID            string
	RemoteModelID      string
	ProviderLocalModel string
	Now                time.Time
}

type LaunchStatus string

const (
	LaunchUsage            LaunchStatus = "usage"
	LaunchBlocked          LaunchStatus = "blocked"
	LaunchAlreadyActive    LaunchStatus = "already_active"
	LaunchAlreadyLaunching LaunchStatus = "already_launching"
	LaunchStarted          LaunchStatus = "started"
)

type PlanResult struct {
	Status             LaunchStatus
	Message            string
	Prompt             string
	Task               TaskState
	RemoteTask         RemoteTask
	Actions            []Action
	Notifications      []Notification
	ActiveURL          string
	Launching          bool
	PreconditionErrors []PreconditionError
	ModelID            string
	Timeout            time.Duration
	TimeoutMS          int64
}

func PlanLaunch(input LaunchInput) PlanResult {
	activeURL := firstNonEmpty(input.ActiveURL, input.ActiveSessionURL)
	if activeURL != "" {
		return PlanResult{
			Status:    LaunchAlreadyActive,
			Message:   buildAlreadyActiveMessage(activeURL),
			ActiveURL: activeURL,
			ModelID:   remoteModelID(input),
			Timeout:   DefaultTimeout,
			TimeoutMS: int64(DefaultTimeout / time.Millisecond),
		}
	}
	if input.Launching {
		return PlanResult{
			Status:    LaunchAlreadyLaunching,
			Message:   buildAlreadyActiveMessage(""),
			Launching: true,
			ModelID:   remoteModelID(input),
			Timeout:   DefaultTimeout,
			TimeoutMS: int64(DefaultTimeout / time.Millisecond),
		}
	}
	if strings.TrimSpace(input.Blurb) == "" && strings.TrimSpace(input.SeedPlan) == "" {
		return PlanResult{
			Status:    LaunchUsage,
			Message:   usageMessage(),
			ModelID:   remoteModelID(input),
			Timeout:   DefaultTimeout,
			TimeoutMS: int64(DefaultTimeout / time.Millisecond),
		}
	}

	if errors := launchPreconditionErrors(input); len(errors) > 0 {
		message := "ultraplan: cannot launch remote session -\n" + formatPreconditionErrors(errors)
		notification := Notification{Message: message, Mode: NotificationModeTask}
		return PlanResult{
			Status:             LaunchBlocked,
			Message:            message,
			Notifications:      []Notification{notification},
			Actions:            []Action{notificationAction(notification)},
			PreconditionErrors: append([]PreconditionError(nil), errors...),
			ModelID:            remoteModelID(input),
			Timeout:            DefaultTimeout,
			TimeoutMS:          int64(DefaultTimeout / time.Millisecond),
		}
	}

	now := normalizedNow(input.Now)
	modelID := remoteModelID(input)
	taskID := firstNonEmpty(input.TaskID, "ultraplan-task")
	sessionID := firstNonEmpty(input.SessionID, input.Session.ID)
	sessionURL := firstNonEmpty(input.SessionURL, input.Session.URL, RemoteSessionURL(sessionID, input.SessionIngressURL))
	prompt := BuildPrompt(PromptInput{
		Blurb:           input.Blurb,
		SeedPlan:        input.SeedPlan,
		Instructions:    input.Instructions,
		SystemReminder:  input.SystemReminder,
		HideScaffolding: input.HideScaffolding,
	})
	title := firstNonEmpty(input.Blurb, "Ultraplan")
	description := firstNonEmpty(input.Blurb, "Refine local plan")
	remoteTask := RemoteTask{
		TaskID:                taskID,
		SessionID:             sessionID,
		SessionURL:            sessionURL,
		Title:                 title,
		Command:               input.Blurb,
		Prompt:                prompt,
		ModelID:               modelID,
		Timeout:               DefaultTimeout,
		TimeoutMS:             int64(DefaultTimeout / time.Millisecond),
		PermissionMode:        PermissionModePlan,
		RemoteTaskType:        RemoteTaskTypeUltraplan,
		UseDefaultEnvironment: true,
		Status:                TaskRunning,
		CreatedAt:             now,
	}
	task := TaskState{
		ID:             taskID,
		TaskID:         taskID,
		SessionID:      sessionID,
		SessionURL:     sessionURL,
		ActiveURL:      sessionURL,
		Status:         TaskRunning,
		Phase:          PhaseRunning,
		RemoteTaskType: RemoteTaskTypeUltraplan,
		Title:          title,
		Command:        input.Blurb,
		Prompt:         prompt,
		ModelID:        modelID,
		Timeout:        DefaultTimeout,
		TimeoutMS:      int64(DefaultTimeout / time.Millisecond),
		StartTime:      now,
	}
	actions := []Action{
		{
			Kind:                  ActionCreateRemoteSession,
			TaskID:                taskID,
			SessionID:             sessionID,
			SessionURL:            sessionURL,
			Prompt:                prompt,
			Description:           description,
			ModelID:               modelID,
			Timeout:               DefaultTimeout,
			TimeoutMS:             int64(DefaultTimeout / time.Millisecond),
			PermissionMode:        PermissionModePlan,
			RemoteTaskType:        RemoteTaskTypeUltraplan,
			UseDefaultEnvironment: true,
		},
		{
			Kind:           ActionRegisterRemoteTask,
			TaskID:         taskID,
			SessionID:      sessionID,
			SessionURL:     sessionURL,
			RemoteTaskType: RemoteTaskTypeUltraplan,
		},
		{
			Kind:           ActionStartDetachedPoll,
			TaskID:         taskID,
			SessionID:      sessionID,
			SessionURL:     sessionURL,
			Timeout:        DefaultTimeout,
			TimeoutMS:      int64(DefaultTimeout / time.Millisecond),
			RemoteTaskType: RemoteTaskTypeUltraplan,
		},
	}
	task.Actions = append([]Action(nil), actions...)

	return PlanResult{
		Status:     LaunchStarted,
		Message:    "ultraplan: starting Claude Code on the web...",
		Prompt:     prompt,
		Task:       task,
		RemoteTask: remoteTask,
		Actions:    actions,
		ActiveURL:  sessionURL,
		ModelID:    modelID,
		Timeout:    DefaultTimeout,
		TimeoutMS:  int64(DefaultTimeout / time.Millisecond),
	}
}

func remoteModelID(input LaunchInput) string {
	candidate := strings.TrimSpace(firstNonEmpty(input.RemoteModelID, input.ModelID))
	if candidate != "" && !looksProviderLocalModel(candidate) {
		return candidate
	}
	return CanonicalRemoteModelID
}

func looksProviderLocalModel(modelID string) bool {
	return strings.HasPrefix(modelID, "us.anthropic.") ||
		strings.HasPrefix(modelID, "anthropic.") ||
		strings.Contains(modelID, ":") ||
		strings.Contains(modelID, "@")
}

func launchPreconditionErrors(input LaunchInput) []PreconditionError {
	if len(input.Preconditions) > 0 {
		return input.Preconditions
	}
	if len(input.PreconditionErrors) > 0 {
		return input.PreconditionErrors
	}
	if len(input.Eligibility.Errors) > 0 {
		return input.Eligibility.Errors
	}
	if input.EligibilityChecked && !input.Eligibility.Eligible {
		return []PreconditionError{{Type: PreconditionUnknown}}
	}
	return nil
}

func formatPreconditionErrors(errors []PreconditionError) string {
	lines := make([]string, 0, len(errors))
	for _, err := range errors {
		lines = append(lines, FormatPreconditionError(err))
	}
	return strings.Join(lines, "\n")
}

func buildAlreadyActiveMessage(url string) string {
	if url != "" {
		return fmt.Sprintf("ultraplan: already polling. Open %s to check status, or wait for the plan to land here.", url)
	}
	return "ultraplan: already launching. Please wait for the session to start."
}

func usageMessage() string {
	return strings.Join([]string{
		"Usage: /ultraplan <prompt>, or include \"ultraplan\" anywhere in your prompt",
		"",
		"Advanced multi-agent plan mode with a canonical remote model. Runs in Claude Code on the web.",
		"When the plan is ready, you can execute it in the web session or send it back here.",
		"Terminal stays free while the remote plans.",
		"",
		"Terms: " + TermsURL,
	}, "\n")
}

type RemoteTask struct {
	TaskID                string
	SessionID             string
	SessionURL            string
	Title                 string
	Command               string
	Prompt                string
	Description           string
	ModelID               string
	Timeout               time.Duration
	TimeoutMS             int64
	PermissionMode        string
	RemoteTaskType        string
	UseDefaultEnvironment bool
	Status                TaskStatus
	CreatedAt             time.Time
}

type RemoteSession struct {
	ID  string
	URL string
}

type TaskStatus string

const (
	TaskIdle      TaskStatus = ""
	TaskRunning   TaskStatus = "running"
	TaskCompleted TaskStatus = "completed"
	TaskFailed    TaskStatus = "failed"
	TaskStopped   TaskStatus = "stopped"
)

type PollPhase string

const (
	PhaseRunning    PollPhase = "running"
	PhaseNeedsInput PollPhase = "needs_input"
	PhasePlanReady  PollPhase = "plan_ready"
	PhaseApproved   PollPhase = "approved"
	PhaseFailed     PollPhase = "failed"
	PhaseStopped    PollPhase = "stopped"
)

type ExecutionTarget string

const (
	ExecutionTargetRemote ExecutionTarget = "remote"
	ExecutionTargetLocal  ExecutionTarget = "local"
)

type TaskState struct {
	ID             string
	TaskID         string
	SessionID      string
	SessionURL     string
	ActiveURL      string
	Status         TaskStatus
	Phase          PollPhase
	RemoteTaskType string
	Title          string
	Command        string
	Prompt         string
	ModelID        string
	Timeout        time.Duration
	TimeoutMS      int64
	Plan           string
	ApprovedPlan   string
	PendingChoice  *PendingChoice
	RejectCount    int
	Error          string
	Actions        []Action
	Notifications  []Notification
	StartTime      time.Time
	EndTime        time.Time
}

type PendingChoice struct {
	Plan      string
	SessionID string
	TaskID    string
}

type PollEventKind string

const (
	PollEventRunning    PollEventKind = "running"
	PollEventNeedsInput PollEventKind = "needs_input"
	PollEventPlanReady  PollEventKind = "plan_ready"
	PollEventApproved   PollEventKind = "approved"
	PollEventFailed     PollEventKind = "failed"
	PollEventStopped    PollEventKind = "stopped"
)

type PollFailureReason string

const (
	PollFailureTerminated           PollFailureReason = "terminated"
	PollFailureTimeoutPending       PollFailureReason = "timeout_pending"
	PollFailureTimeoutNoPlan        PollFailureReason = "timeout_no_plan"
	PollFailureExtractMarkerMissing PollFailureReason = "extract_marker_missing"
	PollFailureNetworkOrUnknown     PollFailureReason = "network_or_unknown"
	PollFailureStopped              PollFailureReason = "stopped"
)

type PollEvent struct {
	Kind            PollEventKind
	Phase           PollPhase
	ExecutionTarget ExecutionTarget
	Target          ExecutionTarget
	Plan            string
	RejectCount     int
	Error           string
	Message         string
	FailureReason   PollFailureReason
	SessionID       string
	SessionURL      string
	Now             time.Time
}

func PollTransition(current TaskState, event PollEvent) TaskState {
	next := cloneTaskState(current)
	if next.Status != TaskRunning {
		return next
	}

	switch pollKind(event) {
	case PollEventRunning:
		next.Phase = PhaseRunning
	case PollEventNeedsInput:
		next.Phase = PhaseNeedsInput
	case PollEventPlanReady:
		next.Phase = PhasePlanReady
	case PollEventApproved:
		next = transitionApproved(next, event)
	case PollEventFailed:
		next = transitionFailed(next, event)
	case PollEventStopped:
		next = transitionStopped(next, event)
	}
	return next
}

func transitionApproved(next TaskState, event PollEvent) TaskState {
	now := normalizedNow(event.Now)
	next.Phase = PhaseApproved
	next.Plan = event.Plan
	next.ApprovedPlan = event.Plan
	next.RejectCount = event.RejectCount
	target := event.ExecutionTarget
	if target == "" {
		target = event.Target
	}
	if target == "" {
		target = ExecutionTargetRemote
	}

	if target == ExecutionTargetLocal {
		next.PendingChoice = &PendingChoice{
			Plan:      event.Plan,
			SessionID: firstNonEmpty(event.SessionID, next.SessionID),
			TaskID:    firstNonEmpty(next.TaskID, next.ID),
		}
		return next
	}

	next.Status = TaskCompleted
	next.EndTime = now
	if shouldClearActiveURL(next.ActiveURL, event.SessionURL) {
		next.ActiveURL = ""
		appendTaskAction(&next, Action{Kind: ActionClearActiveURL, TaskID: firstNonEmpty(next.TaskID, next.ID), SessionID: firstNonEmpty(event.SessionID, next.SessionID), SessionURL: event.SessionURL})
	}
	notification := Notification{
		Message: fmt.Sprintf("Ultraplan approved - executing in Claude Code on the web. Follow along at: %s\n\nResults will land as a pull request when the remote session finishes. There is nothing to do here.", firstNonEmpty(event.SessionURL, next.SessionURL)),
		Mode:    NotificationModeTask,
	}
	appendNotification(&next, notification)
	return next
}

func transitionFailed(next TaskState, event PollEvent) TaskState {
	now := normalizedNow(event.Now)
	next.Status = TaskFailed
	next.Phase = PhaseFailed
	next.Error = firstNonEmpty(event.Error, event.Message, string(event.FailureReason), "ultraplan poll failed")
	next.RejectCount = event.RejectCount
	next.EndTime = now
	sessionID := firstNonEmpty(event.SessionID, next.SessionID)
	sessionURL := firstNonEmpty(event.SessionURL, next.SessionURL)
	appendTaskAction(&next, Action{Kind: ActionArchiveRemoteSession, TaskID: firstNonEmpty(next.TaskID, next.ID), SessionID: sessionID, SessionURL: sessionURL})
	if shouldClearActiveURL(next.ActiveURL, event.SessionURL) {
		next.ActiveURL = ""
		appendTaskAction(&next, Action{Kind: ActionClearActiveURL, TaskID: firstNonEmpty(next.TaskID, next.ID), SessionID: sessionID, SessionURL: sessionURL})
	}
	appendNotification(&next, Notification{
		Message: fmt.Sprintf("Ultraplan failed: %s\n\nSession: %s", next.Error, sessionURL),
		Mode:    NotificationModeTask,
	})
	return next
}

func transitionStopped(next TaskState, event PollEvent) TaskState {
	next.Status = TaskStopped
	next.Phase = PhaseStopped
	next.EndTime = normalizedNow(event.Now)
	if shouldClearActiveURL(next.ActiveURL, event.SessionURL) {
		next.ActiveURL = ""
		appendTaskAction(&next, Action{Kind: ActionClearActiveURL, TaskID: firstNonEmpty(next.TaskID, next.ID), SessionID: firstNonEmpty(event.SessionID, next.SessionID), SessionURL: event.SessionURL})
	}
	return next
}

func pollKind(event PollEvent) PollEventKind {
	if event.Kind != "" {
		return event.Kind
	}
	switch event.Phase {
	case PhaseNeedsInput:
		return PollEventNeedsInput
	case PhasePlanReady:
		return PollEventPlanReady
	case PhaseApproved:
		return PollEventApproved
	case PhaseFailed:
		return PollEventFailed
	case PhaseStopped:
		return PollEventStopped
	case PhaseRunning:
		return PollEventRunning
	default:
		return ""
	}
}

type StopInput struct {
	Task          TaskState
	TaskID        string
	SessionID     string
	SessionURL    string
	ActiveURL     string
	PendingChoice *PendingChoice
	Launching     bool
	Now           time.Time
}

type StopResult struct {
	State         TaskState
	ActiveURL     string
	PendingChoice *PendingChoice
	Launching     bool
	Actions       []Action
	Notifications []Notification
	Message       string
}

func StopPlan(input StopInput) StopResult {
	now := normalizedNow(input.Now)
	state := cloneTaskState(input.Task)
	taskID := firstNonEmpty(input.TaskID, state.TaskID, state.ID)
	sessionID := firstNonEmpty(input.SessionID, state.SessionID)
	sessionURL := firstNonEmpty(input.SessionURL, state.SessionURL, RemoteSessionURL(sessionID, ""))
	if state.Status == "" {
		state.Status = TaskStopped
	} else if state.Status == TaskRunning {
		state.Status = TaskStopped
	}
	state.Phase = PhaseStopped
	state.ActiveURL = ""
	state.PendingChoice = nil
	state.EndTime = now

	notifications := []Notification{
		{
			Message: fmt.Sprintf("Ultraplan stopped.\n\nSession: %s", sessionURL),
			Mode:    NotificationModeTask,
		},
		{
			Message: "The user stopped the ultraplan session above. Do not respond to the stop notification - wait for their next message.",
			Mode:    NotificationModeTask,
			IsMeta:  true,
		},
	}
	actions := []Action{
		{Kind: ActionArchiveRemoteSession, TaskID: taskID, SessionID: sessionID, SessionURL: sessionURL},
		{Kind: ActionClearActiveURL, TaskID: taskID, SessionID: sessionID, SessionURL: sessionURL},
		notificationAction(notifications[0]),
		notificationAction(notifications[1]),
	}
	state.Actions = append(state.Actions, actions...)
	state.Notifications = append(state.Notifications, notifications...)

	return StopResult{
		State:         state,
		ActiveURL:     "",
		PendingChoice: nil,
		Launching:     false,
		Actions:       actions,
		Notifications: notifications,
		Message:       notifications[0].Message,
	}
}

type ActionKind string

const (
	ActionCreateRemoteSession  ActionKind = "create_remote_session"
	ActionRegisterRemoteTask   ActionKind = "register_remote_task"
	ActionStartDetachedPoll    ActionKind = "start_detached_poll"
	ActionArchiveRemoteSession ActionKind = "archive_remote_session"
	ActionClearActiveURL       ActionKind = "clear_active_url"
	ActionEnqueueNotification  ActionKind = "enqueue_notification"
)

type Action struct {
	Kind                  ActionKind
	TaskID                string
	SessionID             string
	SessionURL            string
	Prompt                string
	Description           string
	ModelID               string
	Timeout               time.Duration
	TimeoutMS             int64
	PermissionMode        string
	RemoteTaskType        string
	UseDefaultEnvironment bool
	ExecutionTarget       ExecutionTarget
	Message               string
	Notification          Notification
}

type Notification struct {
	Message string
	Mode    string
	IsMeta  bool
}

func notificationAction(notification Notification) Action {
	return Action{
		Kind:         ActionEnqueueNotification,
		Message:      notification.Message,
		Notification: notification,
	}
}

func appendNotification(state *TaskState, notification Notification) {
	state.Notifications = append(state.Notifications, notification)
	appendTaskAction(state, notificationAction(notification))
}

func appendTaskAction(state *TaskState, action Action) {
	state.Actions = append(state.Actions, action)
}

func shouldClearActiveURL(activeURL, eventURL string) bool {
	return activeURL != "" && (eventURL == "" || activeURL == eventURL)
}

func cloneTaskState(state TaskState) TaskState {
	next := state
	if state.Actions != nil {
		next.Actions = append([]Action(nil), state.Actions...)
	}
	if state.Notifications != nil {
		next.Notifications = append([]Notification(nil), state.Notifications...)
	}
	if state.PendingChoice != nil {
		choice := *state.PendingChoice
		next.PendingChoice = &choice
	}
	return next
}

func RemoteSessionURL(sessionID, ingressURL string) string {
	if sessionID == "" {
		return ""
	}
	compatID := sessionID
	if strings.HasPrefix(compatID, "cse_") {
		compatID = "session_" + strings.TrimPrefix(compatID, "cse_")
	}
	baseURL := "https://claude.ai"
	if strings.Contains(compatID, "_local_") || strings.Contains(ingressURL, "localhost") {
		baseURL = "http://localhost:4000"
	} else if strings.Contains(compatID, "_staging_") || strings.Contains(ingressURL, "staging") {
		baseURL = "https://claude-ai.staging.ant.dev"
	}
	return baseURL + "/code/" + compatID
}

func normalizedNow(t time.Time) time.Time {
	if t.IsZero() {
		return time.Now()
	}
	return t
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
