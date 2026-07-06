package agentcore

import (
	"strings"
	"testing"

	"awaken/src/tool/profile"
)

// rootAgentConfig builds a minimal root AgentConfig for spawner tests.
// The root agent has no SubagentBoundary and no SubagentOrigin.
func rootAgentConfig(execProfile profile.ExecutionProfile) AgentConfig {
	return AgentConfig{
		SystemPrompt: "you are a test agent",
		MaxThoughts:  10,
		Workspace: WorkspaceConfig{
			Root:          "/workspace/root",
			WritableRoots: []string{"/workspace/root/sub"},
		},
		ExecutionProfile: &execProfile,
	}
}

func TestCreateSubagent_NilRequestedProfile_InheritsParent(t *testing.T) {
	parentProfile := profile.WorkspaceWrite()
	parent := rootAgentConfig(parentProfile)

	child, err := CreateSubagent(parent, SpawnConfig{
		AgentID:       "child-1",
		ParentAgentID: "root",
		IsGuardian:    false,
		// RequestedProfile nil → inherit parent
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if child.ExecutionProfile == nil {
		t.Fatal("ExecutionProfile: nil")
	}
	if child.ExecutionProfile.PermissionProfile != profile.PermissionWorkspaceWrite {
		t.Errorf("PermissionProfile: got %q want %q",
			child.ExecutionProfile.PermissionProfile, profile.PermissionWorkspaceWrite)
	}
	if child.ExecutionProfile.Name != "workspace_write" {
		t.Errorf("Profile.Name: got %q want %q",
			child.ExecutionProfile.Name, "workspace_write")
	}
}

func TestCreateSubagent_Guardian_ForcesGuardianReadOnly(t *testing.T) {
	parentProfile := profile.DangerFullAccess()
	parent := rootAgentConfig(parentProfile)

	child, err := CreateSubagent(parent, SpawnConfig{
		AgentID:       "guardian-1",
		ParentAgentID: "root",
		IsGuardian:    true,
		// RequestedProfile nil, but guardian forces GuardianReadOnly
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if child.ExecutionProfile == nil {
		t.Fatal("ExecutionProfile: nil")
	}
	if child.ExecutionProfile.Name != "guardian_read_only" {
		t.Errorf("Profile.Name: got %q want %q",
			child.ExecutionProfile.Name, "guardian_read_only")
	}
	if child.ExecutionProfile.PermissionProfile != profile.PermissionReadOnly {
		t.Errorf("PermissionProfile: got %q want %q",
			child.ExecutionProfile.PermissionProfile, profile.PermissionReadOnly)
	}
	if child.ExecutionProfile.SandboxPolicy != profile.SandboxRequired {
		t.Errorf("SandboxPolicy: got %q want %q",
			child.ExecutionProfile.SandboxPolicy, profile.SandboxRequired)
	}
	if child.ExecutionProfile.NetworkPolicy != profile.NetworkRestricted {
		t.Errorf("NetworkPolicy: got %q want %q",
			child.ExecutionProfile.NetworkPolicy, profile.NetworkRestricted)
	}
}

func TestCreateSubagent_Guardian_IgnoresRequestedProfile(t *testing.T) {
	parentProfile := profile.DangerFullAccess()
	parent := rootAgentConfig(parentProfile)

	// Even with an escalating requested profile, guardian forces
	// GuardianReadOnly.
	requested := profile.DangerFullAccess()
	child, err := CreateSubagent(parent, SpawnConfig{
		AgentID:          "guardian-1",
		ParentAgentID:    "root",
		IsGuardian:       true,
		RequestedProfile: &requested,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if child.ExecutionProfile.Name != "guardian_read_only" {
		t.Errorf("Profile.Name: got %q want %q",
			child.ExecutionProfile.Name, "guardian_read_only")
	}
}

func TestCreateSubagent_EscalatingRequestedProfile_ReturnsError(t *testing.T) {
	parentProfile := profile.WorkspaceReadOnly() // read_only parent
	parent := rootAgentConfig(parentProfile)

	requested := profile.WorkspaceWrite() // escalation: read_only → workspace_write
	_, err := CreateSubagent(parent, SpawnConfig{
		AgentID:          "child-1",
		ParentAgentID:    "root",
		IsGuardian:       false,
		RequestedProfile: &requested,
	})
	if err == nil {
		t.Fatal("expected escalation error, got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "escalation") {
		t.Errorf("error should mention escalation, got %v", err)
	}
}

func TestCreateSubagent_NonEscalatingRequestedProfile_GetsRequested(t *testing.T) {
	parentProfile := profile.DangerFullAccess() // most permissive parent
	parent := rootAgentConfig(parentProfile)

	requested := profile.WorkspaceReadOnly() // downgrade
	child, err := CreateSubagent(parent, SpawnConfig{
		AgentID:          "child-1",
		ParentAgentID:    "root",
		IsGuardian:       false,
		RequestedProfile: &requested,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if child.ExecutionProfile == nil {
		t.Fatal("ExecutionProfile: nil")
	}
	if child.ExecutionProfile.Name != "workspace_read_only" {
		t.Errorf("Profile.Name: got %q want %q",
			child.ExecutionProfile.Name, "workspace_read_only")
	}
	if child.ExecutionProfile.PermissionProfile != profile.PermissionReadOnly {
		t.Errorf("PermissionProfile: got %q want %q",
			child.ExecutionProfile.PermissionProfile, profile.PermissionReadOnly)
	}
}

func TestCreateSubagent_SetsSubagentBoundaryAndOrigin(t *testing.T) {
	parentProfile := profile.WorkspaceWrite()
	parent := rootAgentConfig(parentProfile)

	child, err := CreateSubagent(parent, SpawnConfig{
		AgentID:       "child-1",
		ParentAgentID: "root",
		IsGuardian:    false,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// SubagentBoundary must be set and non-nil.
	if child.SubagentBoundary == nil {
		t.Fatal("SubagentBoundary: nil")
	}
	if child.SubagentBoundary.WorkspaceRoot != "/workspace/root" {
		t.Errorf("SubagentBoundary.WorkspaceRoot: got %q want %q",
			child.SubagentBoundary.WorkspaceRoot, "/workspace/root")
	}
	if len(child.SubagentBoundary.WritableRoots) != 1 {
		t.Fatalf("SubagentBoundary.WritableRoots len: got %d want 1",
			len(child.SubagentBoundary.WritableRoots))
	}
	if child.SubagentBoundary.WritableRoots[0] != "/workspace/root/sub" {
		t.Errorf("SubagentBoundary.WritableRoots[0]: got %q want %q",
			child.SubagentBoundary.WritableRoots[0], "/workspace/root/sub")
	}
	if child.SubagentBoundary.Profile == nil {
		t.Error("SubagentBoundary.Profile: nil")
	}

	// SubagentOrigin must be set with the spawn IDs.
	if child.SubagentOrigin == nil {
		t.Fatal("SubagentOrigin: nil")
	}
	if child.SubagentOrigin.AgentID != "child-1" {
		t.Errorf("SubagentOrigin.AgentID: got %q want %q",
			child.SubagentOrigin.AgentID, "child-1")
	}
	if child.SubagentOrigin.ParentAgentID != "root" {
		t.Errorf("SubagentOrigin.ParentAgentID: got %q want %q",
			child.SubagentOrigin.ParentAgentID, "root")
	}
	if child.SubagentOrigin.IsGuardian {
		t.Error("SubagentOrigin.IsGuardian: got true want false")
	}
}

func TestCreateSubagent_DepthIncrements_RootToDepth1(t *testing.T) {
	parentProfile := profile.WorkspaceWrite()
	parent := rootAgentConfig(parentProfile)
	// Root agent: no SubagentOrigin → child depth should be 1.

	child, err := CreateSubagent(parent, SpawnConfig{
		AgentID:       "child-1",
		ParentAgentID: "root",
		IsGuardian:    false,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if child.SubagentOrigin == nil {
		t.Fatal("SubagentOrigin: nil")
	}
	if child.SubagentOrigin.Depth != 1 {
		t.Errorf("Depth: got %d want 1 (root parent → depth 1 child)",
			child.SubagentOrigin.Depth)
	}
}

func TestCreateSubagent_DepthIncrements_Depth1ToDepth2(t *testing.T) {
	parentProfile := profile.WorkspaceWrite()
	parent := rootAgentConfig(parentProfile)
	// Make the parent a depth-1 sub-agent.
	parent.SubagentOrigin = &profile.SubagentOrigin{
		AgentID:       "child-1",
		ParentAgentID: "root",
		Depth:         1,
	}

	child, err := CreateSubagent(parent, SpawnConfig{
		AgentID:       "child-2",
		ParentAgentID: "child-1",
		IsGuardian:    false,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if child.SubagentOrigin == nil {
		t.Fatal("SubagentOrigin: nil")
	}
	if child.SubagentOrigin.Depth != 2 {
		t.Errorf("Depth: got %d want 2 (depth-1 parent → depth-2 child)",
			child.SubagentOrigin.Depth)
	}
}

func TestCreateSubagent_DepthIncrements_Depth2ToDepth3(t *testing.T) {
	parentProfile := profile.WorkspaceWrite()
	parent := rootAgentConfig(parentProfile)
	parent.SubagentOrigin = &profile.SubagentOrigin{
		AgentID:       "child-2",
		ParentAgentID: "child-1",
		Depth:         2,
	}

	child, err := CreateSubagent(parent, SpawnConfig{
		AgentID:       "child-3",
		ParentAgentID: "child-2",
		IsGuardian:    false,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if child.SubagentOrigin.Depth != 3 {
		t.Errorf("Depth: got %d want 3", child.SubagentOrigin.Depth)
	}
}

func TestCreateSubagent_GuardianOrigin_FlagSet(t *testing.T) {
	parentProfile := profile.WorkspaceWrite()
	parent := rootAgentConfig(parentProfile)

	child, err := CreateSubagent(parent, SpawnConfig{
		AgentID:       "guardian-1",
		ParentAgentID: "root",
		IsGuardian:    true,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if child.SubagentOrigin == nil {
		t.Fatal("SubagentOrigin: nil")
	}
	if !child.SubagentOrigin.IsGuardian {
		t.Error("IsGuardian: got false want true")
	}
}

func TestCreateSubagent_EmptyAgentID_ReturnsError(t *testing.T) {
	parent := rootAgentConfig(profile.WorkspaceWrite())
	_, err := CreateSubagent(parent, SpawnConfig{
		AgentID:       "",
		ParentAgentID: "root",
	})
	if err == nil {
		t.Fatal("expected error for empty AgentID, got nil")
	}
}

func TestCreateSubagent_EmptyParentAgentID_ReturnsError(t *testing.T) {
	parent := rootAgentConfig(profile.WorkspaceWrite())
	_, err := CreateSubagent(parent, SpawnConfig{
		AgentID:       "child-1",
		ParentAgentID: "",
	})
	if err == nil {
		t.Fatal("expected error for empty ParentAgentID, got nil")
	}
}

func TestCreateSubagent_CopiesParentRunnableFields(t *testing.T) {
	parentProfile := profile.WorkspaceWrite()
	parent := rootAgentConfig(parentProfile)
	parent.SystemPrompt = "custom system prompt"
	parent.MaxThoughts = 42

	child, err := CreateSubagent(parent, SpawnConfig{
		AgentID:       "child-1",
		ParentAgentID: "root",
		IsGuardian:    false,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if child.SystemPrompt != "custom system prompt" {
		t.Errorf("SystemPrompt: got %q want %q",
			child.SystemPrompt, "custom system prompt")
	}
	if child.MaxThoughts != 42 {
		t.Errorf("MaxThoughts: got %d want 42", child.MaxThoughts)
	}
	if child.Workspace.Root != parent.Workspace.Root {
		t.Errorf("Workspace.Root: got %q want %q",
			child.Workspace.Root, parent.Workspace.Root)
	}
}
