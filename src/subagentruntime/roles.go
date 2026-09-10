package subagentruntime

import (
	"strings"
	"sync"
	"time"
)

// Role is a host-owned child definition. Models cannot submit arbitrary
// system prompts, permission profiles, or extra tool names.
type Role struct {
	Type            string
	WhenToUse       string
	SystemPrompt    string
	ReadOnly        bool
	DisallowedTools []string
	AllowedTools    []string
	ModelHint       string
	MaxTurns        int
	Timeout         time.Duration
	PluginName      string
	PluginDigest    string
	LogicalID       string
	RevisionID      string
}

type RoleResolver func(agentType string) (Role, bool)

var roleResolverState struct {
	sync.RWMutex
	resolver RoleResolver
}

func SetRoleResolver(resolver RoleResolver) {
	roleResolverState.Lock()
	roleResolverState.resolver = resolver
	roleResolverState.Unlock()
}

const (
	orchestrationHidden = "subagent"
	askUserHidden       = "ask_user"
	acpHidden           = "delegate_external_agent"
	acpConfigHidden     = "configure_acp_runner"
	hookConfigHidden    = "configure_hook"
	// runPlanHidden keeps the root-only update_plan tool out of child tool
	// pools; child progress uses native Subagent facts ([工具定义](docs/02_主智能体闭环/运行计划/实现方案.md#工具定义)).
	runPlanHidden = "update_plan"
	// archivedHistoryHidden keeps cross-thread private history in the root
	// Agent. Children receive only the task context their parent delegates.
	archivedHistoryHidden = "archived_history"
)

var mutationTools = []string{
	"Write", "Edit", "trash_file", "media_generate", "dependency_install", "git_remote",
}

var orchestrationTools = []string{
	orchestrationHidden, askUserHidden, acpHidden, acpConfigHidden, hookConfigHidden, runPlanHidden, archivedHistoryHidden,
}

func RoleCatalog() map[string]Role {
	return map[string]Role{
		AgentTypeExplore: {
			Type:      AgentTypeExplore,
			WhenToUse: "快速只读搜索代码、定位入口、梳理事实；不修改文件。",
			ReadOnly:  true,
			SystemPrompt: "You are a local Awaken Subagent acting as a codebase explorer. " +
				"Work only on the assigned task. You are strictly READ-ONLY: do not edit, write, or delete files, " +
				"do not run mutating shell commands against the project tree, cannot ask the user questions, " +
				"cannot spawn another Subagent, and must return a concise factual handoff to the parent Agent. " +
				"Prefer Read, Glob, and Grep. Summarize paths, symbols, and risks with evidence.",
			DisallowedTools: append(append([]string{}, orchestrationTools...), mutationTools...),
		},
		AgentTypePlan: {
			Type:      AgentTypePlan,
			WhenToUse: "只读规划实现路径，列出必须改动的关键文件和顺序；不修改文件。",
			ReadOnly:  true,
			SystemPrompt: "You are a local Awaken Subagent acting as a planning agent. " +
				"You are strictly READ-ONLY. Explore the codebase only as needed to produce an implementation plan. " +
				"You cannot ask the user questions, cannot spawn another Subagent, and must not modify files. " +
				"Your handoff MUST include a section titled \"Critical Files for Implementation\" listing concrete paths " +
				"the parent Agent should touch, plus ordered steps, risks, and open questions for the parent to resolve.",
			DisallowedTools: append(append([]string{}, orchestrationTools...), mutationTools...),
		},
		AgentTypeReview: {
			Type:      AgentTypeReview,
			WhenToUse: "独立复核风险、测试缺口或实现是否满足任务；项目目录只读。",
			ReadOnly:  true,
			SystemPrompt: "You are a local Awaken Subagent acting as an independent reviewer. " +
				"The project workspace is READ-ONLY. Do not edit or write project files. " +
				"You cannot ask the user questions or spawn another Subagent. " +
				"Verify claims against current files. Report defects, missing tests, and residual risks. " +
				"If a temporary script is required, say so in the handoff; do not treat /tmp write access as permission to change the project.",
			DisallowedTools: append(append([]string{}, orchestrationTools...), mutationTools...),
		},
		AgentTypeGeneral: {
			Type:      AgentTypeGeneral,
			WhenToUse: "需要在隔离或共享工作区中独立实现并改文件时。",
			ReadOnly:  false,
			SystemPrompt: "You are a local Awaken Subagent acting as a general-purpose coding agent. " +
				"Complete the assigned implementation. You may use the inherited file and shell tools inside the granted workspace. " +
				"Return missing information to the parent Agent instead of asking the user directly. You may delegate authorized work. Preserve other Agents' and the user's changes. Do not widen permissions. " +
				"If you are running in a git worktree, keep all writes inside that worktree and return a concise handoff " +
				"listing changed paths. The parent Agent reviews and integrates; you do not merge.",
			DisallowedTools: []string{askUserHidden},
		},
	}
}

func LookupRole(agentType string) (Role, bool) {
	normalized := NormalizeAgentType(agentType)
	if role, ok := RoleCatalog()[normalized]; ok {
		return role, true
	}
	roleResolverState.RLock()
	resolver := roleResolverState.resolver
	roleResolverState.RUnlock()
	if resolver == nil {
		return Role{}, false
	}
	role, ok := resolver(normalized)
	resolvedType := NormalizeAgentType(role.Type)
	if !ok || !strings.HasPrefix(resolvedType, "plugin__") {
		return Role{}, false
	}
	// Canonical Plugin Agent identities must match exactly. A resolver may also
	// accept a vendor manifest name and return its canonical namespaced role;
	// the resolver owns uniqueness and collision rejection for that alias.
	if strings.HasPrefix(normalized, "plugin__") && resolvedType != normalized {
		return Role{}, false
	}
	if role.ReadOnly {
		role.DisallowedTools = appendUniqueRoleStrings(role.DisallowedTools, orchestrationTools...)
	}
	return role, true
}

func NormalizeAgentType(agentType string) string {
	agentType = strings.ToLower(strings.TrimSpace(agentType))
	if agentType == "" {
		return AgentTypeGeneral
	}
	return agentType
}

func NormalizeWorkspaceMode(agentType, workspaceMode string) string {
	workspaceMode = strings.ToLower(strings.TrimSpace(workspaceMode))
	if workspaceMode != "" {
		return workspaceMode
	}
	if role, ok := LookupRole(agentType); ok && !role.ReadOnly {
		return WorkspaceShared
	}
	return WorkspaceSharedReadOnly
}

func ValidateSpawnCombo(agentType, workspaceMode string) error {
	agentType = NormalizeAgentType(agentType)
	workspaceMode = NormalizeWorkspaceMode(agentType, workspaceMode)
	role, ok := LookupRole(agentType)
	if !ok {
		return runtimeError(ErrorInvalidInput, "unsupported agent_type")
	}
	if role.ReadOnly {
		if workspaceMode != WorkspaceSharedReadOnly {
			return runtimeError(ErrorWorkspaceMode, "read-only agent types require shared_readonly")
		}
		return nil
	}
	if workspaceMode != WorkspaceShared && workspaceMode != WorkspaceGitWorktree && workspaceMode != WorkspaceSharedReadOnly {
		return runtimeError(ErrorWorkspaceMode, "write-capable agent types require shared or git_worktree")
	}
	return nil
}

func appendUniqueRoleStrings(values []string, extras ...string) []string {
	seen := map[string]bool{}
	for _, value := range values {
		seen[value] = true
	}
	for _, value := range extras {
		if !seen[value] {
			values = append(values, value)
			seen[value] = true
		}
	}
	return values
}

func IsReadOnlyType(agentType string) bool {
	role, ok := LookupRole(agentType)
	return ok && role.ReadOnly
}

func IsWriteCapableMode(workspaceMode string) bool {
	switch strings.ToLower(strings.TrimSpace(workspaceMode)) {
	case WorkspaceShared, WorkspaceGitWorktree:
		return true
	default:
		return false
	}
}
