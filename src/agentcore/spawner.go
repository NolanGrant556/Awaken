package agentcore

import (
	"errors"

	"awaken/src/agentcore/guardian"
	"awaken/src/agentcore/subagent"
	"awaken/src/tool/profile"
)

// SpawnConfig configures a single CreateSubagent call. It is the minimal
// input the spawner needs besides the parent AgentConfig: who the new
// sub-agent is, who spawned it, whether it is a guardian, and (optionally)
// what profile it requests.
type SpawnConfig struct {
	// AgentID is the ID of the new sub-agent.
	AgentID string
	// ParentAgentID is the ID of the agent spawning the sub-agent.
	ParentAgentID string
	// IsGuardian marks the sub-agent as a guardian/review agent. When true,
	// the sub-agent is forced to GuardianReadOnly regardless of
	// RequestedProfile.
	IsGuardian bool
	// RequestedProfile is the profile the sub-agent requests. When nil, the
	// sub-agent inherits the parent's profile verbatim. When non-nil, it
	// must not escalate the parent's profile (see
	// subagent.ValidateNoEscalation). Ignored when IsGuardian is true.
	RequestedProfile *profile.ExecutionProfile
}

// CreateSubagent builds a child AgentConfig for a sub-agent. It is a pure
// function: it does NOT start a runner loop or register the agent with any
// runtime. The caller is responsible for the agent's lifecycle.
//
// The function:
//  1. Extracts a ParentBoundary from the parent's workspace, profile, and
//     sub-agent origin (depth).
//  2. If spawn.IsGuardian, resolves the boundary via
//     guardian.ResolveGuardianBoundary (forces GuardianReadOnly).
//     Otherwise, resolves via subagent.ResolveSubagentBoundary (inherits
//     or downgrades, validating no escalation).
//  3. Builds the child's SubagentOrigin via subagent.BuildSubagentOrigin
//     (increments depth).
//  4. Returns a child AgentConfig with SubagentBoundary, SubagentOrigin,
//     and ExecutionProfile set. For guardians, ExecutionProfile is forced
//     to GuardianReadOnly.
//
// The returned AgentConfig copies the parent's SystemPrompt, MaxThoughts,
// ModelConfig, EntryPoint, Workspace, and AutoToolPolicy so the child is
// immediately runnable. The caller may override any of these fields before
// starting the agent.
func CreateSubagent(parent AgentConfig, spawn SpawnConfig) (AgentConfig, error) {
	if spawn.AgentID == "" {
		return AgentConfig{}, errors.New("subagent spawner: spawn.AgentID is empty")
	}
	if spawn.ParentAgentID == "" {
		return AgentConfig{}, errors.New("subagent spawner: spawn.ParentAgentID is empty")
	}

	// Step 1: extract the parent boundary. The parent's depth comes from
	// its SubagentOrigin (0 for the root agent).
	parentDepth := 0
	if parent.SubagentOrigin != nil {
		parentDepth = parent.SubagentOrigin.Depth
	}
	parentBoundary := subagent.ParentBoundary{
		WorkspaceRoot: parent.Workspace.Root,
		WritableRoots: parent.Workspace.WritableRoots,
		Profile:       parent.ExecutionProfile,
		Depth:         parentDepth,
	}

	// Step 2: resolve the child boundary.
	var childBoundary subagent.ParentBoundary
	if spawn.IsGuardian {
		childBoundary = guardian.ResolveGuardianBoundary(parentBoundary)
	} else {
		var err error
		childBoundary, err = subagent.ResolveSubagentBoundary(parentBoundary, spawn.RequestedProfile)
		if err != nil {
			return AgentConfig{}, err
		}
	}

	// Step 3: build the child origin (increments depth).
	childOrigin := subagent.BuildSubagentOrigin(
		parent.SubagentOrigin,
		spawn.AgentID,
		spawn.ParentAgentID,
		spawn.IsGuardian,
	)

	// Step 4: build the child AgentConfig.
	child := AgentConfig{
		// Copy runnable fields from the parent so the child is immediately
		// usable. The caller may override these.
		SystemPrompt:                 parent.SystemPrompt,
		MaxThoughts:                  parent.MaxThoughts,
		MaxToolExecutions:            parent.MaxToolExecutions,
		ModelConfig:                  parent.ModelConfig,
		EntryPoint:                   parent.EntryPoint,
		Workspace:                    parent.Workspace,
		SkillSummary:                 parent.SkillSummary,
		AutoToolPolicy:               parent.AutoToolPolicy,
		ToolExposure:                 parent.ToolExposure,
		ToolAvailabilitySnapshot:     parent.ToolAvailabilitySnapshot,
		ExecutionEnvironmentSnapshot: parent.ExecutionEnvironmentSnapshot,
		SkillContextBlocks:           append([]ContextBlock(nil), parent.SkillContextBlocks...),
		SubagentBoundary:             &childBoundary,
		SubagentOrigin:               childOrigin,
	}
	// Step 5: set the effective profile. Guardians are forced to
	// GuardianReadOnly regardless of what the boundary resolution produced.
	if spawn.IsGuardian {
		guardianProfile := profile.GuardianReadOnly()
		child.ExecutionProfile = &guardianProfile
	} else if childBoundary.Profile != nil {
		child.ExecutionProfile = childBoundary.Profile
	}

	return child, nil
}
