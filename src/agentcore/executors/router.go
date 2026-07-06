package executors

import (
	"fmt"

	"awaken/src/tool/profile"
)

// Router maps an ExecutionIntent + ExecutionProfile to the appropriate
// ToolExecutor. It is the single dispatch point the ToolRuntime consults
// after the auto tool policy produces an "allow" (or transform) decision.
//
// Routing rules:
//   - GuardianReadOnly always routes to GuardianExecutor.
//   - artifact_lane      → ArtifactExecutor
//   - connector_lane     → ConnectorExecutor
//   - local_compute_lane → LocalComputeExecutor
//   - system_lane        → SystemExecutor
//   - legacy review_agent → GuardianExecutor
//   - unknown lane/kind  → error (fail-closed)
type Router struct {
	artifact     ToolExecutor
	connector    ToolExecutor
	localCompute ToolExecutor
	system       ToolExecutor
	guardian     ToolExecutor
}

// NewRouter returns a Router wired with the current lane executors plus the
// guardian override executor.
func NewRouter() *Router {
	return &Router{
		artifact:     NewArtifactExecutor(),
		connector:    NewConnectorExecutor(),
		localCompute: NewLocalComputeExecutor(),
		system:       NewSystemExecutor(),
		guardian:     NewGuardianExecutor(),
	}
}

// Route returns the ToolExecutor that should handle the given intent under
// the given profile. Unknown tool kinds produce an error so the runtime
// fails closed instead of silently dispatching to a wrong executor.
func (r *Router) Route(intent profile.ExecutionIntent, execProfile profile.ExecutionProfile) (ToolExecutor, error) {
	// GuardianReadOnly forces every intent through the GuardianExecutor,
	// regardless of the declared tool kind. This is the executor-layer
	// enforcement of the "guardian 不继承父代理的高权限 profile" rule.
	if execProfile.Name == profile.GuardianReadOnly().Name {
		return r.guardian, nil
	}
	if intent.ToolKind == profile.ToolKindReviewAgent {
		return r.guardian, nil
	}
	lane := intent.Lane
	if lane == "" {
		lane = legacyLaneForKind(intent.ToolKind)
	}
	switch lane {
	case profile.LaneArtifact:
		return r.artifact, nil
	case profile.LaneConnector:
		return r.connector, nil
	case profile.LaneLocalCompute:
		return r.localCompute, nil
	case profile.LaneSystem:
		return r.system, nil
	default:
		return nil, fmt.Errorf("unknown execution lane %q for tool %q; cannot route to an executor", lane, intent.ToolName)
	}
}

func legacyLaneForKind(kind profile.ToolKind) profile.ExecutionLane {
	switch kind {
	case profile.ToolKindFileChain, profile.ToolKindReviewAgent:
		return profile.LaneArtifact
	case profile.ToolKindMcpApp:
		return profile.LaneConnector
	case profile.ToolKindShellCommand:
		return profile.LaneSystem
	default:
		return ""
	}
}
