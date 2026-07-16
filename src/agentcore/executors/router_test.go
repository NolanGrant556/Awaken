package executors

import (
	"testing"

	"awaken/src/tool/profile"
)

// TestRouterRoutesFileOperationToArtifactExecutor verifies the artifact lane.
func TestRouterRoutesFileOperationToArtifactExecutor(t *testing.T) {
	router := NewRouter()
	intent := profile.ExecutionIntent{
		ToolName: "Edit",
		ToolKind: profile.ToolKindFileOperation,
	}
	executor, err := router.Route(intent, profile.WorkspaceWrite())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := executor.(*ArtifactExecutor); !ok {
		t.Fatalf("expected *ArtifactExecutor, got %T", executor)
	}
}

// TestRouterRoutesBashCommandToSystemExecutor verifies the system lane.
func TestRouterRoutesBashCommandToSystemExecutor(t *testing.T) {
	router := NewRouter()
	intent := profile.ExecutionIntent{
		ToolName: "Bash",
		ToolKind: profile.ToolKindShellCommand,
	}
	executor, err := router.Route(intent, profile.WorkspaceWrite())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := executor.(*SystemExecutor); !ok {
		t.Fatalf("expected *SystemExecutor, got %T", executor)
	}
}

// TestRouterRoutesMcpAppToMcpExecutor verifies that a legacy mcp_app intent
// routes into the ConnectorExecutor lane.
func TestRouterRoutesMcpAppToMcpExecutor(t *testing.T) {
	router := NewRouter()
	intent := profile.ExecutionIntent{
		ToolName: "mcp__notion__search",
		ToolKind: profile.ToolKindMcpApp,
	}
	executor, err := router.Route(intent, profile.WorkspaceWrite())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := executor.(*ConnectorExecutor); !ok {
		t.Fatalf("expected *ConnectorExecutor, got %T", executor)
	}
}

// TestRouterRoutesReviewAgentToGuardianExecutor verifies that a
// review_agent intent routes to the GuardianExecutor.
func TestRouterRoutesReviewAgentToGuardianExecutor(t *testing.T) {
	router := NewRouter()
	intent := profile.ExecutionIntent{
		ToolName: "review_code",
		ToolKind: profile.ToolKindReviewAgent,
	}
	executor, err := router.Route(intent, profile.WorkspaceWrite())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := executor.(*GuardianExecutor); !ok {
		t.Fatalf("expected *GuardianExecutor, got %T", executor)
	}
}

// TestRouterGuardianReadOnlyRoutesAllToGuardianExecutor verifies that when
// the profile is GuardianReadOnly, every tool kind routes to the
// GuardianExecutor regardless of the declared ToolKind.
func TestRouterGuardianReadOnlyRoutesAllToGuardianExecutor(t *testing.T) {
	router := NewRouter()
	guardianProfile := profile.GuardianReadOnly()
	kinds := []profile.ToolKind{
		profile.ToolKindFileOperation,
		profile.ToolKindShellCommand,
		profile.ToolKindMcpApp,
		profile.ToolKindReviewAgent,
	}
	for _, kind := range kinds {
		intent := profile.ExecutionIntent{
			ToolName: "test_tool",
			ToolKind: kind,
		}
		executor, err := router.Route(intent, guardianProfile)
		if err != nil {
			t.Fatalf("unexpected error for kind %s: %v", kind, err)
		}
		if _, ok := executor.(*GuardianExecutor); !ok {
			t.Fatalf("expected *GuardianExecutor for kind %s under GuardianReadOnly, got %T", kind, executor)
		}
	}
}

// TestRouterUnknownToolKindReturnsError verifies that an unknown tool kind
// produces an error (fail-closed).
func TestRouterUnknownToolKindReturnsError(t *testing.T) {
	router := NewRouter()
	intent := profile.ExecutionIntent{
		ToolName: "mystery_tool",
		ToolKind: profile.ToolKind("unknown_kind"),
	}
	_, err := router.Route(intent, profile.WorkspaceWrite())
	if err == nil {
		t.Fatalf("expected error for unknown tool kind, got nil")
	}
}
