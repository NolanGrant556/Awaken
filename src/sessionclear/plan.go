package sessionclear

import (
	"crypto/rand"
	"fmt"
	"strings"
	"time"
)

const (
	DefaultSessionEndHookTimeoutMS = 1500
	ClearReason                    = "clear"
	CacheEvictionScope             = "conversation_clear"
)

type TaskKind string

const (
	TaskKindOther             TaskKind = "other"
	TaskKindLocalShell        TaskKind = "local_shell"
	TaskKindLocalAgent        TaskKind = "local_agent"
	TaskKindInProcessTeammate TaskKind = "in_process_teammate"
)

type TaskStatus string

const (
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusFinished  TaskStatus = "finished"
	TaskStatusCancelled TaskStatus = "cancelled"
	TaskStatusFailed    TaskStatus = "failed"
)

type Task struct {
	ID string

	Kind   TaskKind
	Status TaskStatus

	// BackgroundStateKnown mirrors Claude Code's "isBackgrounded" in-task check.
	// A task is killed only when this flag is set and IsBackgrounded is false.
	BackgroundStateKnown bool
	IsBackgrounded       bool

	AgentID         string
	IdentityAgentID string
}

type Input struct {
	CurrentSessionID string
	NewSessionID     string
	OriginalCWD      string
	LastRequestID    string

	SessionEndHookTimeoutMS int
	Tasks                   []Task
	PluginReconnectKey      string
}

type PlanResult struct {
	SessionEndHook SessionEndHookPlan

	CacheEvictionHint *CacheEvictionHint
	ClearMessages     bool
	ClearStates       []StateClear

	Session SessionTransition
	Restore RestorePlan

	PreservedTasks    []Task
	PreservedAgentIDs []string
	TaskActions       []TaskAction
	SymlinkRepoints   []SymlinkRepoint

	CacheResets []ResetAction
	MCPReset    MCPResetPlan

	ClearProactiveContextBlocked bool
}

type SessionEndHookPlan struct {
	Reason    string
	TimeoutMS int
}

type CacheEvictionHint struct {
	Scope         string
	LastRequestID string
}

type SessionTransition struct {
	PreviousID string
	NewID      string
	ParentID   string
}

type RestorePlan struct {
	CWD string
}

type StateClearTarget string

const (
	StateMessages          StateClearTarget = "messages"
	StateReadFile          StateClearTarget = "read_file_state"
	StateDiscoveredSkills  StateClearTarget = "discovered_skills"
	StateNestedMemory      StateClearTarget = "nested_memory"
	StateAttribution       StateClearTarget = "attribution"
	StateStandaloneAgent   StateClearTarget = "standalone_agent_context"
	StateFileHistory       StateClearTarget = "file_history"
	StatePlanSlugs         StateClearTarget = "plan_slugs"
	StateSessionMetadata   StateClearTarget = "session_metadata"
	StateConversationID    StateClearTarget = "conversation_id"
	StateSessionFile       StateClearTarget = "session_file_pointer"
	StateWorktreeState     StateClearTarget = "worktree_state"
	StateSessionStartHooks StateClearTarget = "session_start_hooks"
)

type StateClear struct {
	Target StateClearTarget
	Action string
}

type TaskActionKind string

const (
	TaskActionPreserve    TaskActionKind = "preserve_task"
	TaskActionKill        TaskActionKind = "kill_task"
	TaskActionCleanup     TaskActionKind = "cleanup_task"
	TaskActionEvictOutput TaskActionKind = "evict_task_output"
	TaskActionRemove      TaskActionKind = "remove_task"
)

type TaskAction struct {
	TaskID string
	Action TaskActionKind
	Reason string
}

type SymlinkRepoint struct {
	TaskID       string
	AgentID      string
	NewSessionID string
	Target       string
}

type MCPResetPlan struct {
	ClearClients   bool
	ClearTools     bool
	ClearCommands  bool
	ClearResources bool

	PreservePluginReconnectKey string
}

type ResetTarget string

const (
	ResetUserContext                ResetTarget = "user_context"
	ResetSystemContext              ResetTarget = "system_context"
	ResetGitStatus                  ResetTarget = "git_status"
	ResetSessionStartDate           ResetTarget = "session_start_date"
	ResetFileSuggestions            ResetTarget = "file_suggestions"
	ResetCommands                   ResetTarget = "commands"
	ResetPromptCacheBreakDetection  ResetTarget = "prompt_cache_break_detection"
	ResetSystemPromptInjection      ResetTarget = "system_prompt_injection"
	ResetLastEmittedDate            ResetTarget = "last_emitted_date"
	ResetPostCompactCleanup         ResetTarget = "post_compact_cleanup"
	ResetSentSkillNames             ResetTarget = "sent_skill_names"
	ResetMemoryFiles                ResetTarget = "memory_files"
	ResetStoredImagePaths           ResetTarget = "stored_image_paths"
	ResetSessionIngress             ResetTarget = "session_ingress"
	ResetPendingPermissionCallbacks ResetTarget = "pending_permission_callbacks"
	ResetAttributionCaches          ResetTarget = "attribution_caches"
	ResetRepositoryCaches           ResetTarget = "repository_caches"
	ResetCommandPrefixCaches        ResetTarget = "command_prefix_caches"
	ResetDumpPromptState            ResetTarget = "dump_prompt_state"
	ResetInvokedSkills              ResetTarget = "invoked_skills"
	ResetGitDirResolution           ResetTarget = "git_dir_resolution"
	ResetDynamicSkills              ResetTarget = "dynamic_skills"
	ResetLSPDiagnostics             ResetTarget = "lsp_diagnostics"
	ResetMagicDocs                  ResetTarget = "magic_docs"
	ResetSessionEnv                 ResetTarget = "session_env"
	ResetWebFetch                   ResetTarget = "web_fetch"
	ResetToolSearchDescriptions     ResetTarget = "tool_search_descriptions"
	ResetAgentDefinitions           ResetTarget = "agent_definitions"
	ResetSkillPrompt                ResetTarget = "skill_prompt"
)

type ResetOperation string

const (
	ResetOperationClear                ResetOperation = "clear"
	ResetOperationReset                ResetOperation = "reset"
	ResetOperationRun                  ResetOperation = "run"
	ResetOperationClearExceptPreserved ResetOperation = "clear_except_preserved_agents"
	ResetOperationPreserve             ResetOperation = "preserve"
)

type ResetAction struct {
	Target            ResetTarget
	Operation         ResetOperation
	PreservedAgentIDs []string
	Reason            string
}

func PlanClear(input Input) PlanResult {
	timeoutMS := input.SessionEndHookTimeoutMS
	if timeoutMS <= 0 {
		timeoutMS = DefaultSessionEndHookTimeoutMS
	}

	newSessionID := strings.TrimSpace(input.NewSessionID)
	if newSessionID == "" {
		newSessionID = generateSessionID()
	}

	preservedTasks := PreservedTasks(input.Tasks)
	preservedAgentIDs := preservedAgentIDs(preservedTasks)

	result := PlanResult{
		SessionEndHook: SessionEndHookPlan{
			Reason:    ClearReason,
			TimeoutMS: timeoutMS,
		},
		ClearMessages: true,
		ClearStates: []StateClear{
			{Target: StateMessages, Action: "clear"},
			{Target: StateReadFile, Action: "clear"},
			{Target: StateDiscoveredSkills, Action: "clear"},
			{Target: StateNestedMemory, Action: "clear"},
			{Target: StateAttribution, Action: "reset"},
			{Target: StateStandaloneAgent, Action: "clear"},
			{Target: StateFileHistory, Action: "reset"},
			{Target: StatePlanSlugs, Action: "clear"},
			{Target: StateSessionMetadata, Action: "clear"},
			{Target: StateConversationID, Action: "regenerate"},
			{Target: StateSessionFile, Action: "reset_pointer"},
			{Target: StateWorktreeState, Action: "save_after_clear"},
			{Target: StateSessionStartHooks, Action: "run_after_clear"},
		},
		Session: SessionTransition{
			PreviousID: input.CurrentSessionID,
			NewID:      newSessionID,
			ParentID:   input.CurrentSessionID,
		},
		Restore: RestorePlan{
			CWD: input.OriginalCWD,
		},
		PreservedTasks:    preservedTasks,
		PreservedAgentIDs: preservedAgentIDs,
		TaskActions:       taskActions(input.Tasks),
		SymlinkRepoints:   symlinkRepoints(preservedTasks, newSessionID),
		CacheResets:       CacheResetPlan(preservedAgentIDs),
		MCPReset: MCPResetPlan{
			ClearClients:               true,
			ClearTools:                 true,
			ClearCommands:              true,
			ClearResources:             true,
			PreservePluginReconnectKey: input.PluginReconnectKey,
		},
		ClearProactiveContextBlocked: true,
	}

	if strings.TrimSpace(input.LastRequestID) != "" {
		result.CacheEvictionHint = &CacheEvictionHint{
			Scope:         CacheEvictionScope,
			LastRequestID: input.LastRequestID,
		}
	}

	return result
}

func PreservedTasks(tasks []Task) []Task {
	preserved := make([]Task, 0, len(tasks))
	for _, task := range tasks {
		if shouldKillTask(task) {
			continue
		}
		preserved = append(preserved, task)
	}
	return preserved
}

func CacheResetPlan(preservedAgentIDs []string) []ResetAction {
	ids := uniqueNonEmpty(preservedAgentIDs)
	hasPreservedAgents := len(ids) > 0

	requestKeyedOperation := ResetOperationClear
	requestKeyedReason := ""
	if hasPreservedAgents {
		requestKeyedOperation = ResetOperationPreserve
		requestKeyedReason = "request-keyed state cannot be safely scoped while preserved agents continue running"
	}

	invokedSkillsOperation := ResetOperationClear
	if hasPreservedAgents {
		invokedSkillsOperation = ResetOperationClearExceptPreserved
	}

	return []ResetAction{
		{Target: ResetUserContext, Operation: ResetOperationClear},
		{Target: ResetSystemContext, Operation: ResetOperationClear},
		{Target: ResetGitStatus, Operation: ResetOperationClear},
		{Target: ResetSessionStartDate, Operation: ResetOperationClear},
		{Target: ResetFileSuggestions, Operation: ResetOperationClear},
		{Target: ResetCommands, Operation: ResetOperationClear},
		{Target: ResetPromptCacheBreakDetection, Operation: requestKeyedOperation, Reason: requestKeyedReason},
		{Target: ResetSystemPromptInjection, Operation: ResetOperationClear},
		{Target: ResetLastEmittedDate, Operation: ResetOperationClear},
		{Target: ResetPostCompactCleanup, Operation: ResetOperationRun},
		{Target: ResetSentSkillNames, Operation: ResetOperationReset},
		{Target: ResetMemoryFiles, Operation: ResetOperationReset, Reason: "session_start"},
		{Target: ResetStoredImagePaths, Operation: ResetOperationClear},
		{Target: ResetSessionIngress, Operation: ResetOperationClear},
		{Target: ResetPendingPermissionCallbacks, Operation: requestKeyedOperation, Reason: requestKeyedReason},
		{Target: ResetAttributionCaches, Operation: ResetOperationClear},
		{Target: ResetRepositoryCaches, Operation: ResetOperationClear},
		{Target: ResetCommandPrefixCaches, Operation: ResetOperationClear},
		{Target: ResetDumpPromptState, Operation: requestKeyedOperation, Reason: requestKeyedReason},
		{Target: ResetInvokedSkills, Operation: invokedSkillsOperation, PreservedAgentIDs: ids},
		{Target: ResetGitDirResolution, Operation: ResetOperationClear},
		{Target: ResetDynamicSkills, Operation: ResetOperationClear},
		{Target: ResetLSPDiagnostics, Operation: ResetOperationReset},
		{Target: ResetMagicDocs, Operation: ResetOperationClear},
		{Target: ResetSessionEnv, Operation: ResetOperationClear},
		{Target: ResetWebFetch, Operation: ResetOperationClear},
		{Target: ResetToolSearchDescriptions, Operation: ResetOperationClear},
		{Target: ResetAgentDefinitions, Operation: ResetOperationClear},
		{Target: ResetSkillPrompt, Operation: ResetOperationClear},
	}
}

func shouldKillTask(task Task) bool {
	return task.BackgroundStateKnown && !task.IsBackgrounded
}

func preservedAgentIDs(tasks []Task) []string {
	ids := make([]string, 0, len(tasks))
	seen := map[string]bool{}
	for _, task := range tasks {
		id := taskAgentID(task)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids
}

func taskAgentID(task Task) string {
	switch task.Kind {
	case TaskKindLocalAgent:
		return strings.TrimSpace(task.AgentID)
	case TaskKindInProcessTeammate:
		return strings.TrimSpace(task.IdentityAgentID)
	default:
		return ""
	}
}

func taskActions(tasks []Task) []TaskAction {
	actions := make([]TaskAction, 0, len(tasks)*4)
	for _, task := range tasks {
		if !shouldKillTask(task) {
			actions = append(actions, TaskAction{
				TaskID: task.ID,
				Action: TaskActionPreserve,
				Reason: "task is backgrounded or has no foreground marker",
			})
			continue
		}

		if task.Status == TaskStatusRunning {
			actions = append(actions,
				TaskAction{TaskID: task.ID, Action: TaskActionKill, Reason: "foreground running task"},
				TaskAction{TaskID: task.ID, Action: TaskActionCleanup, Reason: "foreground running task"},
			)
		}
		actions = append(actions,
			TaskAction{TaskID: task.ID, Action: TaskActionEvictOutput, Reason: "foreground task output must not survive clear"},
			TaskAction{TaskID: task.ID, Action: TaskActionRemove, Reason: "foreground task is removed from post-clear state"},
		)
	}
	return actions
}

func symlinkRepoints(tasks []Task, newSessionID string) []SymlinkRepoint {
	repoints := []SymlinkRepoint{}
	for _, task := range tasks {
		if task.Kind != TaskKindLocalAgent || task.Status != TaskStatusRunning {
			continue
		}
		agentID := strings.TrimSpace(task.AgentID)
		if agentID == "" {
			continue
		}
		repoints = append(repoints, SymlinkRepoint{
			TaskID:       task.ID,
			AgentID:      agentID,
			NewSessionID: newSessionID,
			Target:       "agent_transcript",
		})
	}
	return repoints
}

func uniqueNonEmpty(values []string) []string {
	seen := map[string]bool{}
	unique := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		unique = append(unique, value)
	}
	return unique
}

func generateSessionID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err == nil {
		return fmt.Sprintf("session_%x", b)
	}
	return fmt.Sprintf("session_%d", time.Now().UnixNano())
}
