package ultraplan

import (
	"strings"
	"testing"
	"time"
)

func TestBuildPromptWithSeedPlanAndBlurb(t *testing.T) {
	prompt := BuildPrompt(PromptInput{
		Blurb:          "Please plan the billing migration.",
		SeedPlan:       "1. Read the schema\n2. Add tests",
		SystemReminder: "Hidden remote planner scaffolding.",
	})

	if !strings.Contains(prompt, "Here is a draft plan to refine:\n\n1. Read the schema\n2. Add tests") {
		t.Fatalf("expected seed plan preface, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "<system-reminder>\nHidden remote planner scaffolding.\n</system-reminder>") {
		t.Fatalf("expected hidden system reminder, got:\n%s", prompt)
	}
	if !strings.HasSuffix(prompt, "Please plan the billing migration.") {
		t.Fatalf("expected blurb at the end, got:\n%s", prompt)
	}
	if strings.Index(prompt, "Here is a draft plan to refine:") > strings.Index(prompt, "<system-reminder>") {
		t.Fatalf("expected seed plan to appear before hidden instructions, got:\n%s", prompt)
	}
}

func TestPlanLaunchBlockedByPreconditions(t *testing.T) {
	result := PlanLaunch(LaunchInput{
		Blurb: "Plan repo setup.",
		PreconditionErrors: []PreconditionError{
			{Type: PreconditionNotLoggedIn},
			{Type: PreconditionNoGitRemote, Message: "origin remote missing"},
		},
	})

	if result.Status != LaunchBlocked {
		t.Fatalf("expected blocked launch, got %q", result.Status)
	}
	if !strings.Contains(result.Message, "cannot launch remote session") || !strings.Contains(result.Message, "origin remote missing") {
		t.Fatalf("unexpected blocked message: %q", result.Message)
	}
	if len(result.Notifications) != 1 || result.Notifications[0].Mode != NotificationModeTask {
		t.Fatalf("expected one task notification, got %+v", result.Notifications)
	}
	if len(result.Actions) != 1 || result.Actions[0].Kind != ActionEnqueueNotification {
		t.Fatalf("expected enqueue notification action, got %+v", result.Actions)
	}
}

func TestPlanLaunchAlreadyActiveAndAlreadyLaunchingMessages(t *testing.T) {
	active := PlanLaunch(LaunchInput{
		Blurb:     "Plan something",
		ActiveURL: "https://claude.ai/code/session_active",
	})
	if active.Status != LaunchAlreadyActive {
		t.Fatalf("expected already active, got %q", active.Status)
	}
	if !strings.Contains(active.Message, "already polling") || !strings.Contains(active.Message, active.ActiveURL) {
		t.Fatalf("unexpected active message: %q", active.Message)
	}

	launching := PlanLaunch(LaunchInput{
		Blurb:     "Plan something",
		Launching: true,
	})
	if launching.Status != LaunchAlreadyLaunching {
		t.Fatalf("expected already launching, got %q", launching.Status)
	}
	if !strings.Contains(launching.Message, "already launching") {
		t.Fatalf("unexpected launching message: %q", launching.Message)
	}
}

func TestPlanLaunchCreatesRemoteTaskWithTimeoutAndCanonicalModel(t *testing.T) {
	now := time.Date(2026, 6, 28, 9, 0, 0, 0, time.UTC)
	result := PlanLaunch(LaunchInput{
		Blurb:              "Plan the approval workflow.",
		ProviderLocalModel: "us.anthropic.claude-opus-4-6-v1",
		ModelID:            "us.anthropic.claude-opus-4-6-v1",
		TaskID:             "task-1",
		Session:            RemoteSession{ID: "cse_local_abc"},
		Now:                now,
	})

	if result.Status != LaunchStarted {
		t.Fatalf("expected launch started, got %q", result.Status)
	}
	if result.ModelID != CanonicalRemoteModelID || result.RemoteTask.ModelID != CanonicalRemoteModelID {
		t.Fatalf("expected canonical remote model, got result=%q task=%q", result.ModelID, result.RemoteTask.ModelID)
	}
	if strings.Contains(result.ModelID, "us.anthropic") {
		t.Fatalf("provider-local model leaked into result: %q", result.ModelID)
	}
	if result.Timeout != DefaultTimeout || result.TimeoutMS != int64(30*time.Minute/time.Millisecond) {
		t.Fatalf("unexpected timeout: %s %d", result.Timeout, result.TimeoutMS)
	}
	if result.RemoteTask.PermissionMode != PermissionModePlan || !result.RemoteTask.UseDefaultEnvironment {
		t.Fatalf("unexpected remote task options: %+v", result.RemoteTask)
	}
	if result.Task.Status != TaskRunning || result.Task.Phase != PhaseRunning {
		t.Fatalf("unexpected task state: %+v", result.Task)
	}
	if result.ActiveURL != "http://localhost:4000/code/session_local_abc" {
		t.Fatalf("expected compat local session URL, got %q", result.ActiveURL)
	}
	if !hasAction(result.Actions, ActionCreateRemoteSession) || !hasAction(result.Actions, ActionRegisterRemoteTask) || !hasAction(result.Actions, ActionStartDetachedPoll) {
		t.Fatalf("expected create/register/poll actions, got %+v", result.Actions)
	}
}

func TestPollApprovedRemoteCompletesAndClearsURL(t *testing.T) {
	state := runningTaskState()

	next := PollTransition(state, PollEvent{
		Kind:            PollEventApproved,
		ExecutionTarget: ExecutionTargetRemote,
		Plan:            "Ship remotely.",
		SessionID:       "session_123",
		SessionURL:      "https://claude.ai/code/session_123",
	})

	if next.Status != TaskCompleted || next.Phase != PhaseApproved {
		t.Fatalf("expected completed approved state, got %+v", next)
	}
	if next.ActiveURL != "" {
		t.Fatalf("expected active URL cleared, got %q", next.ActiveURL)
	}
	if next.PendingChoice != nil {
		t.Fatalf("remote approval should not create pending local choice: %+v", next.PendingChoice)
	}
	if !hasAction(next.Actions, ActionClearActiveURL) || !hasNotificationContaining(next.Notifications, "executing in Claude Code on the web") {
		t.Fatalf("expected clear action and remote execution notification, actions=%+v notifications=%+v", next.Actions, next.Notifications)
	}
}

func TestPollApprovedLocalCreatesPendingChoice(t *testing.T) {
	state := runningTaskState()

	next := PollTransition(state, PollEvent{
		Kind:            PollEventApproved,
		ExecutionTarget: ExecutionTargetLocal,
		Plan:            "Teleport this plan back.",
		SessionID:       "session_123",
		SessionURL:      "https://claude.ai/code/session_123",
	})

	if next.Status != TaskRunning || next.Phase != PhaseApproved {
		t.Fatalf("expected running approved state for local choice, got %+v", next)
	}
	if next.ActiveURL != state.ActiveURL {
		t.Fatalf("local teleport should leave URL for dialog owner, got %q", next.ActiveURL)
	}
	if next.PendingChoice == nil || next.PendingChoice.Plan != "Teleport this plan back." || next.PendingChoice.TaskID != state.TaskID {
		t.Fatalf("expected pending local choice, got %+v", next.PendingChoice)
	}
}

func TestPollFailureMarksFailedAndArchives(t *testing.T) {
	state := runningTaskState()

	next := PollTransition(state, PollEvent{
		Kind:          PollEventFailed,
		Error:         "network_or_unknown",
		SessionID:     "session_123",
		SessionURL:    "https://claude.ai/code/session_123",
		RejectCount:   2,
		FailureReason: PollFailureNetworkOrUnknown,
	})

	if next.Status != TaskFailed || next.Phase != PhaseFailed || next.RejectCount != 2 {
		t.Fatalf("expected failed state, got %+v", next)
	}
	if next.ActiveURL != "" {
		t.Fatalf("expected active URL cleared, got %q", next.ActiveURL)
	}
	if !hasAction(next.Actions, ActionArchiveRemoteSession) || !hasAction(next.Actions, ActionClearActiveURL) {
		t.Fatalf("expected archive and clear actions, got %+v", next.Actions)
	}
	if !hasNotificationContaining(next.Notifications, "Ultraplan failed") {
		t.Fatalf("expected failure notification, got %+v", next.Notifications)
	}
}

func TestStopPlanClearsURLAndQueuesTwoNotifications(t *testing.T) {
	state := runningTaskState()
	result := StopPlan(StopInput{
		Task:      state,
		ActiveURL: state.ActiveURL,
	})

	if result.State.Status != TaskStopped || result.State.Phase != PhaseStopped {
		t.Fatalf("expected stopped state, got %+v", result.State)
	}
	if result.ActiveURL != "" || result.PendingChoice != nil || result.Launching {
		t.Fatalf("expected cleared launch state, got active=%q pending=%+v launching=%v", result.ActiveURL, result.PendingChoice, result.Launching)
	}
	if len(result.Notifications) != 2 || !result.Notifications[1].IsMeta {
		t.Fatalf("expected user and meta notifications, got %+v", result.Notifications)
	}
	if !hasAction(result.Actions, ActionArchiveRemoteSession) || !hasAction(result.Actions, ActionClearActiveURL) {
		t.Fatalf("expected archive and clear actions, got %+v", result.Actions)
	}
}

func TestPollFailureKeepsNewerActiveURLForStalePoll(t *testing.T) {
	state := runningTaskState()
	state.ActiveURL = "https://claude.ai/code/session_new"

	next := PollTransition(state, PollEvent{
		Kind:       PollEventFailed,
		Error:      "old poll failed",
		SessionID:  "session_old",
		SessionURL: "https://claude.ai/code/session_old",
	})

	if next.Status != TaskFailed {
		t.Fatalf("expected stale poll to mark its task failed, got %+v", next)
	}
	if next.ActiveURL != "https://claude.ai/code/session_new" {
		t.Fatalf("stale poll cleared a newer active URL: %q", next.ActiveURL)
	}
	if !hasAction(next.Actions, ActionArchiveRemoteSession) {
		t.Fatalf("expected stale remote session archive action, got %+v", next.Actions)
	}
	if hasAction(next.Actions, ActionClearActiveURL) {
		t.Fatalf("stale poll should not clear current active URL, got %+v", next.Actions)
	}
}

func runningTaskState() TaskState {
	return TaskState{
		ID:             "task-123",
		TaskID:         "task-123",
		SessionID:      "session_123",
		SessionURL:     "https://claude.ai/code/session_123",
		ActiveURL:      "https://claude.ai/code/session_123",
		Status:         TaskRunning,
		Phase:          PhaseRunning,
		RemoteTaskType: RemoteTaskTypeUltraplan,
	}
}

func hasAction(actions []Action, kind ActionKind) bool {
	for _, action := range actions {
		if action.Kind == kind {
			return true
		}
	}
	return false
}

func hasNotificationContaining(notifications []Notification, needle string) bool {
	for _, notification := range notifications {
		if strings.Contains(notification.Message, needle) {
			return true
		}
	}
	return false
}
