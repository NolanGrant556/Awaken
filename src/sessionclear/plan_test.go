package sessionclear

import (
	"reflect"
	"testing"
)

func TestPlanClearTasksAndPreservedAgents(t *testing.T) {
	tasks := []Task{
		{
			ID:                   "fg-shell",
			Kind:                 TaskKindLocalShell,
			Status:               TaskStatusRunning,
			BackgroundStateKnown: true,
			IsBackgrounded:       false,
		},
		{
			ID:                   "bg-agent",
			Kind:                 TaskKindLocalAgent,
			Status:               TaskStatusRunning,
			BackgroundStateKnown: true,
			IsBackgrounded:       true,
			AgentID:              "agent-bg",
		},
		{
			ID:                   "fg-agent",
			Kind:                 TaskKindLocalAgent,
			Status:               TaskStatusRunning,
			BackgroundStateKnown: true,
			IsBackgrounded:       false,
			AgentID:              "agent-fg",
		},
		{
			ID:              "teammate",
			Kind:            TaskKindInProcessTeammate,
			Status:          TaskStatusRunning,
			IdentityAgentID: "agent-team",
		},
	}

	plan := PlanClear(Input{
		CurrentSessionID: "session-old",
		NewSessionID:     "session-new",
		Tasks:            tasks,
	})

	if got, want := taskIDs(plan.PreservedTasks), []string{"bg-agent", "teammate"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("preserved tasks = %v, want %v", got, want)
	}
	if got, want := plan.PreservedAgentIDs, []string{"agent-bg", "agent-team"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("preserved agent IDs = %v, want %v", got, want)
	}

	for _, action := range []TaskActionKind{TaskActionKill, TaskActionCleanup, TaskActionEvictOutput, TaskActionRemove} {
		if !hasTaskAction(plan.TaskActions, "fg-shell", action) {
			t.Fatalf("expected foreground shell action %s, got %+v", action, plan.TaskActions)
		}
		if !hasTaskAction(plan.TaskActions, "fg-agent", action) {
			t.Fatalf("expected foreground agent action %s, got %+v", action, plan.TaskActions)
		}
	}
	if hasTaskAction(plan.TaskActions, "bg-agent", TaskActionKill) {
		t.Fatalf("background agent should not be killed: %+v", plan.TaskActions)
	}
	if hasTaskAction(plan.TaskActions, "teammate", TaskActionKill) {
		t.Fatalf("in-process teammate should not be killed: %+v", plan.TaskActions)
	}
}

func TestCacheResetPlanPreservesRequestKeyedStateForPreservedAgents(t *testing.T) {
	actions := resetActionsByTarget(CacheResetPlan([]string{"agent-bg", "agent-bg", ""}))

	for _, target := range []ResetTarget{
		ResetPromptCacheBreakDetection,
		ResetPendingPermissionCallbacks,
		ResetDumpPromptState,
	} {
		if got := actions[target].Operation; got != ResetOperationPreserve {
			t.Fatalf("%s operation = %s, want %s", target, got, ResetOperationPreserve)
		}
	}

	invokedSkills := actions[ResetInvokedSkills]
	if invokedSkills.Operation != ResetOperationClearExceptPreserved {
		t.Fatalf("invoked skills operation = %s, want %s", invokedSkills.Operation, ResetOperationClearExceptPreserved)
	}
	if got, want := invokedSkills.PreservedAgentIDs, []string{"agent-bg"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("invoked skills preserved IDs = %v, want %v", got, want)
	}
}

func TestCacheResetPlanClearsRequestKeyedStateWithoutPreservedAgents(t *testing.T) {
	actions := resetActionsByTarget(CacheResetPlan(nil))

	for _, target := range []ResetTarget{
		ResetPromptCacheBreakDetection,
		ResetPendingPermissionCallbacks,
		ResetDumpPromptState,
	} {
		if got := actions[target].Operation; got != ResetOperationClear {
			t.Fatalf("%s operation = %s, want %s", target, got, ResetOperationClear)
		}
	}
	if got := actions[ResetInvokedSkills].Operation; got != ResetOperationClear {
		t.Fatalf("invoked skills operation = %s, want %s", got, ResetOperationClear)
	}
}

func TestPlanClearSessionStateAndCacheCoverage(t *testing.T) {
	plan := PlanClear(Input{
		CurrentSessionID:        "session-old",
		NewSessionID:            "session-new",
		OriginalCWD:             "/repo",
		LastRequestID:           "req-123",
		SessionEndHookTimeoutMS: 2500,
		PluginReconnectKey:      "plugin-key",
	})

	if plan.SessionEndHook.Reason != ClearReason || plan.SessionEndHook.TimeoutMS != 2500 {
		t.Fatalf("session end hook = %+v, want reason %q timeout 2500", plan.SessionEndHook, ClearReason)
	}
	if plan.CacheEvictionHint == nil {
		t.Fatalf("expected cache eviction hint")
	}
	if plan.CacheEvictionHint.Scope != CacheEvictionScope || plan.CacheEvictionHint.LastRequestID != "req-123" {
		t.Fatalf("cache eviction hint = %+v", plan.CacheEvictionHint)
	}
	if plan.Session.PreviousID != "session-old" || plan.Session.NewID != "session-new" || plan.Session.ParentID != "session-old" {
		t.Fatalf("session transition = %+v", plan.Session)
	}
	if plan.Restore.CWD != "/repo" {
		t.Fatalf("restore cwd = %q, want /repo", plan.Restore.CWD)
	}
	if !plan.ClearMessages {
		t.Fatalf("expected messages to be cleared")
	}
	if !plan.ClearProactiveContextBlocked {
		t.Fatalf("expected proactive context blocked flag to be cleared")
	}
	if !plan.MCPReset.ClearClients || !plan.MCPReset.ClearTools || !plan.MCPReset.ClearCommands || !plan.MCPReset.ClearResources {
		t.Fatalf("MCP reset should clear clients/tools/commands/resources: %+v", plan.MCPReset)
	}
	if got := plan.MCPReset.PreservePluginReconnectKey; got != "plugin-key" {
		t.Fatalf("plugin reconnect key = %q, want plugin-key", got)
	}

	clearStates := stateClearsByTarget(plan.ClearStates)
	for _, target := range []StateClearTarget{
		StateMessages,
		StateReadFile,
		StateDiscoveredSkills,
		StateNestedMemory,
		StateAttribution,
		StateFileHistory,
		StatePlanSlugs,
		StateSessionMetadata,
	} {
		if _, ok := clearStates[target]; !ok {
			t.Fatalf("missing state clear target %s in %+v", target, plan.ClearStates)
		}
	}

	resetActions := resetActionsByTarget(plan.CacheResets)
	for _, target := range []ResetTarget{
		ResetLSPDiagnostics,
		ResetMagicDocs,
		ResetSessionEnv,
		ResetWebFetch,
		ResetToolSearchDescriptions,
		ResetAgentDefinitions,
		ResetSkillPrompt,
	} {
		if _, ok := resetActions[target]; !ok {
			t.Fatalf("missing reset target %s in %+v", target, plan.CacheResets)
		}
	}
}

func TestPlanClearDefaultsTimeoutAndSkipsCacheEvictionWithoutRequestID(t *testing.T) {
	plan := PlanClear(Input{
		CurrentSessionID: "session-old",
		NewSessionID:     "session-new",
	})

	if plan.SessionEndHook.TimeoutMS != DefaultSessionEndHookTimeoutMS {
		t.Fatalf("default timeout = %d, want %d", plan.SessionEndHook.TimeoutMS, DefaultSessionEndHookTimeoutMS)
	}
	if plan.CacheEvictionHint != nil {
		t.Fatalf("cache eviction hint = %+v, want nil", plan.CacheEvictionHint)
	}
}

func TestPlanClearSymlinkRepointsOnlyRunningPreservedLocalAgents(t *testing.T) {
	plan := PlanClear(Input{
		CurrentSessionID: "session-old",
		NewSessionID:     "session-new",
		Tasks: []Task{
			{
				ID:                   "running-agent",
				Kind:                 TaskKindLocalAgent,
				Status:               TaskStatusRunning,
				BackgroundStateKnown: true,
				IsBackgrounded:       true,
				AgentID:              "agent-running",
			},
			{
				ID:                   "finished-agent",
				Kind:                 TaskKindLocalAgent,
				Status:               TaskStatusFinished,
				BackgroundStateKnown: true,
				IsBackgrounded:       true,
				AgentID:              "agent-finished",
			},
			{
				ID:              "teammate",
				Kind:            TaskKindInProcessTeammate,
				Status:          TaskStatusRunning,
				IdentityAgentID: "agent-team",
			},
			{
				ID:                   "foreground-agent",
				Kind:                 TaskKindLocalAgent,
				Status:               TaskStatusRunning,
				BackgroundStateKnown: true,
				IsBackgrounded:       false,
				AgentID:              "agent-foreground",
			},
		},
	})

	if got, want := len(plan.SymlinkRepoints), 1; got != want {
		t.Fatalf("symlink repoints = %+v, want %d entry", plan.SymlinkRepoints, want)
	}
	repoint := plan.SymlinkRepoints[0]
	if repoint.TaskID != "running-agent" || repoint.AgentID != "agent-running" || repoint.NewSessionID != "session-new" {
		t.Fatalf("symlink repoint = %+v", repoint)
	}
}

func taskIDs(tasks []Task) []string {
	ids := make([]string, 0, len(tasks))
	for _, task := range tasks {
		ids = append(ids, task.ID)
	}
	return ids
}

func hasTaskAction(actions []TaskAction, taskID string, kind TaskActionKind) bool {
	for _, action := range actions {
		if action.TaskID == taskID && action.Action == kind {
			return true
		}
	}
	return false
}

func resetActionsByTarget(actions []ResetAction) map[ResetTarget]ResetAction {
	byTarget := make(map[ResetTarget]ResetAction, len(actions))
	for _, action := range actions {
		byTarget[action.Target] = action
	}
	return byTarget
}

func stateClearsByTarget(clears []StateClear) map[StateClearTarget]StateClear {
	byTarget := make(map[StateClearTarget]StateClear, len(clears))
	for _, clear := range clears {
		byTarget[clear.Target] = clear
	}
	return byTarget
}
