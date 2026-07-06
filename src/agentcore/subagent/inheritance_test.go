package subagent

import (
	"errors"
	"strings"
	"testing"

	"awaken/src/tool/profile"
)

// helper: build a profile with a given permission level, copying the rest
// from WorkspaceWrite so the profile is otherwise well-formed.
func profileWith(permission profile.PermissionProfile) *profile.ExecutionProfile {
	p := profile.WorkspaceWrite()
	p.PermissionProfile = permission
	return &p
}

func TestInheritBoundary_CopiesAllFields(t *testing.T) {
	parentProfile := profile.WorkspaceWrite()
	parent := ParentBoundary{
		WorkspaceRoot: "/workspace/parent",
		WritableRoots: []string{"/workspace/parent/sub", "/tmp/parent"},
		Profile:       &parentProfile,
	}

	child := InheritBoundary(parent)

	if child.WorkspaceRoot != parent.WorkspaceRoot {
		t.Errorf("WorkspaceRoot: got %q want %q", child.WorkspaceRoot, parent.WorkspaceRoot)
	}
	if len(child.WritableRoots) != len(parent.WritableRoots) {
		t.Fatalf("WritableRoots len: got %d want %d", len(child.WritableRoots), len(parent.WritableRoots))
	}
	for i, want := range parent.WritableRoots {
		if child.WritableRoots[i] != want {
			t.Errorf("WritableRoots[%d]: got %q want %q", i, child.WritableRoots[i], want)
		}
	}
	if child.Profile == nil {
		t.Fatal("Profile: nil after inheritance")
	}
	if child.Profile == parent.Profile {
		t.Error("Profile: child aliases parent pointer (should be a deep copy)")
	}
	if child.Profile.PermissionProfile != parent.Profile.PermissionProfile {
		t.Errorf("Profile.PermissionProfile: got %q want %q",
			child.Profile.PermissionProfile, parent.Profile.PermissionProfile)
	}
}

func TestInheritBoundary_IsDeepCopy(t *testing.T) {
	parentProfile := profile.WorkspaceWrite()
	parentProfile.SandboxPermissions = []string{"read", "write"}
	parent := ParentBoundary{
		WorkspaceRoot: "/workspace/parent",
		WritableRoots: []string{"/workspace/parent/sub"},
		Profile:       &parentProfile,
	}

	child := InheritBoundary(parent)

	// Mutate the child and verify the parent is untouched.
	child.WritableRoots[0] = "/workspace/child/elsewhere"
	child.WritableRoots = append(child.WritableRoots, "/workspace/child/extra")
	child.Profile.SandboxPermissions[0] = "execute"

	if parent.WritableRoots[0] != "/workspace/parent/sub" {
		t.Errorf("parent WritableRoots mutated: got %q want %q",
			parent.WritableRoots[0], "/workspace/parent/sub")
	}
	if len(parent.WritableRoots) != 1 {
		t.Errorf("parent WritableRoots length mutated: got %d want 1", len(parent.WritableRoots))
	}
	if parent.Profile.SandboxPermissions[0] != "read" {
		t.Errorf("parent SandboxPermissions mutated: got %q want %q",
			parent.Profile.SandboxPermissions[0], "read")
	}
}

func TestInheritBoundary_NilProfile(t *testing.T) {
	parent := ParentBoundary{
		WorkspaceRoot: "/workspace/parent",
		WritableRoots: []string{"/workspace/parent/sub"},
		Profile:       nil,
	}
	child := InheritBoundary(parent)
	if child.Profile != nil {
		t.Errorf("Profile: expected nil, got non-nil")
	}
	if child.WorkspaceRoot != parent.WorkspaceRoot {
		t.Errorf("WorkspaceRoot: got %q want %q", child.WorkspaceRoot, parent.WorkspaceRoot)
	}
}

func TestValidateNoEscalation(t *testing.T) {
	cases := []struct {
		name        string
		child       profile.PermissionProfile
		parent      profile.PermissionProfile
		wantErr     bool
		errContains string
	}{
		{name: "read_only -> read_only: OK", child: profile.PermissionReadOnly, parent: profile.PermissionReadOnly},
		{name: "read_only -> workspace_write: ERROR", child: profile.PermissionWorkspaceWrite, parent: profile.PermissionReadOnly, wantErr: true, errContains: "escalation"},
		{name: "read_only -> open_world: ERROR", child: profile.PermissionOpenWorld, parent: profile.PermissionReadOnly, wantErr: true, errContains: "escalation"},
		{name: "read_only -> danger_full_access: ERROR", child: profile.PermissionDangerFullAccess, parent: profile.PermissionReadOnly, wantErr: true, errContains: "escalation"},

		{name: "workspace_write -> read_only: OK", child: profile.PermissionReadOnly, parent: profile.PermissionWorkspaceWrite},
		{name: "workspace_write -> workspace_write: OK", child: profile.PermissionWorkspaceWrite, parent: profile.PermissionWorkspaceWrite},
		{name: "workspace_write -> open_world: ERROR", child: profile.PermissionOpenWorld, parent: profile.PermissionWorkspaceWrite, wantErr: true, errContains: "escalation"},
		{name: "workspace_write -> danger_full_access: ERROR", child: profile.PermissionDangerFullAccess, parent: profile.PermissionWorkspaceWrite, wantErr: true, errContains: "escalation"},

		{name: "open_world -> read_only: OK", child: profile.PermissionReadOnly, parent: profile.PermissionOpenWorld},
		{name: "open_world -> workspace_write: OK", child: profile.PermissionWorkspaceWrite, parent: profile.PermissionOpenWorld},
		{name: "open_world -> open_world: OK", child: profile.PermissionOpenWorld, parent: profile.PermissionOpenWorld},
		{name: "open_world -> danger_full_access: ERROR", child: profile.PermissionDangerFullAccess, parent: profile.PermissionOpenWorld, wantErr: true, errContains: "escalation"},

		{name: "danger_full_access -> read_only: OK", child: profile.PermissionReadOnly, parent: profile.PermissionDangerFullAccess},
		{name: "danger_full_access -> workspace_write: OK", child: profile.PermissionWorkspaceWrite, parent: profile.PermissionDangerFullAccess},
		{name: "danger_full_access -> open_world: OK", child: profile.PermissionOpenWorld, parent: profile.PermissionDangerFullAccess},
		{name: "danger_full_access -> danger_full_access: OK", child: profile.PermissionDangerFullAccess, parent: profile.PermissionDangerFullAccess},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			child := profileWith(tc.child)
			parent := profileWith(tc.parent)
			err := ValidateNoEscalation(child, parent)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tc.errContains)) {
					t.Errorf("error does not contain %q: got %v", tc.errContains, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})
	}
}

func TestValidateNoEscalation_NilInputs(t *testing.T) {
	if err := ValidateNoEscalation(nil, profileWith(profile.PermissionReadOnly)); err == nil {
		t.Error("nil child: expected error, got nil")
	}
	if err := ValidateNoEscalation(profileWith(profile.PermissionReadOnly), nil); err == nil {
		t.Error("nil parent: expected error, got nil")
	}
}

func TestValidateNoEscalation_UnknownProfile(t *testing.T) {
	unknown := profileWith(profile.PermissionProfile("super_user"))
	parent := profileWith(profile.PermissionReadOnly)
	if err := ValidateNoEscalation(unknown, parent); err == nil {
		t.Error("unknown child profile: expected error, got nil")
	}
	if err := ValidateNoEscalation(parent, unknown); err == nil {
		t.Error("unknown parent profile: expected error, got nil")
	}
}

func TestResolveSubagentBoundary_NilRequestedInheritsParent(t *testing.T) {
	parentProfile := profile.WorkspaceWrite()
	parent := ParentBoundary{
		WorkspaceRoot: "/workspace/parent",
		WritableRoots: []string{"/workspace/parent/sub"},
		Profile:       &parentProfile,
	}

	child, err := ResolveSubagentBoundary(parent, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if child.Profile == nil {
		t.Fatal("Profile: nil after resolution")
	}
	if child.Profile == parent.Profile {
		t.Error("Profile: child aliases parent pointer (should be a deep copy)")
	}
	if child.Profile.PermissionProfile != parent.Profile.PermissionProfile {
		t.Errorf("PermissionProfile: got %q want %q",
			child.Profile.PermissionProfile, parent.Profile.PermissionProfile)
	}
	if child.WorkspaceRoot != parent.WorkspaceRoot {
		t.Errorf("WorkspaceRoot: got %q want %q", child.WorkspaceRoot, parent.WorkspaceRoot)
	}
}

func TestResolveSubagentBoundary_NonEscalatingRequestedIsUsed(t *testing.T) {
	// Use DangerFullAccess as the parent so every dimension is at max
	// rank; WorkspaceReadOnly is a strict downgrade across ALL fields
	// (permission, sandbox, network, env, approval, perms).
	parentProfile := profile.DangerFullAccess()
	parent := ParentBoundary{
		WorkspaceRoot: "/workspace/parent",
		WritableRoots: []string{"/workspace/parent/sub"},
		Profile:       &parentProfile,
	}

	requested := profile.WorkspaceReadOnly() // downgrade from danger_full_access → read_only
	child, err := ResolveSubagentBoundary(parent, &requested)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if child.Profile == nil {
		t.Fatal("Profile: nil after resolution")
	}
	if child.Profile.PermissionProfile != profile.PermissionReadOnly {
		t.Errorf("PermissionProfile: got %q want %q",
			child.Profile.PermissionProfile, profile.PermissionReadOnly)
	}
	if child.Profile.Name != "workspace_read_only" {
		t.Errorf("Profile.Name: got %q want %q", child.Profile.Name, "workspace_read_only")
	}
}

func TestResolveSubagentBoundary_EscalatingRequestedReturnsError(t *testing.T) {
	parentProfile := profile.WorkspaceReadOnly() // read_only parent
	parent := ParentBoundary{
		WorkspaceRoot: "/workspace/parent",
		WritableRoots: []string{"/workspace/parent/sub"},
		Profile:       &parentProfile,
	}

	requested := profile.WorkspaceWrite() // escalation: read_only → workspace_write
	child, err := ResolveSubagentBoundary(parent, &requested)
	if err == nil {
		t.Fatalf("expected escalation error, got nil (child=%+v)", child)
	}
	if !strings.Contains(strings.ToLower(err.Error()), "escalation") {
		t.Errorf("error does not mention escalation: got %v", err)
	}
}

func TestResolveSubagentBoundary_NilParentProfileWithRequestReturnsError(t *testing.T) {
	parent := ParentBoundary{
		WorkspaceRoot: "/workspace/parent",
		WritableRoots: []string{"/workspace/parent/sub"},
		Profile:       nil,
	}
	requested := profile.WorkspaceWrite()
	if _, err := ResolveSubagentBoundary(parent, &requested); err == nil {
		t.Error("expected error when parent profile is nil and request is non-nil, got nil")
	}
}

func TestResolveSubagentBoundary_ReturnedBoundaryIsDeepCopy(t *testing.T) {
	parentProfile := profile.WorkspaceWrite()
	parentProfile.SandboxPermissions = []string{"read", "write"}
	parent := ParentBoundary{
		WorkspaceRoot: "/workspace/parent",
		WritableRoots: []string{"/workspace/parent/sub"},
		Profile:       &parentProfile,
	}

	requested := profile.WorkspaceReadOnly()
	requestedPerms := append([]string(nil), requested.SandboxPermissions...)
	child, err := ResolveSubagentBoundary(parent, &requested)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Mutate the child and verify the requested profile is untouched.
	child.Profile.SandboxPermissions = append(child.Profile.SandboxPermissions, "execute")
	if len(requested.SandboxPermissions) != len(requestedPerms) {
		t.Errorf("requested.SandboxPermissions length mutated: got %v want %v",
			requested.SandboxPermissions, requestedPerms)
	}
	for i, want := range requestedPerms {
		if i >= len(requested.SandboxPermissions) || requested.SandboxPermissions[i] != want {
			t.Errorf("requested.SandboxPermissions[%d] mutated: got %v want %q",
				i, requested.SandboxPermissions, want)
			break
		}
	}
}

func TestBuildSubagentOrigin_RootAgentDepth1(t *testing.T) {
	origin := BuildSubagentOrigin(nil, "agent-1", "root", false)
	if origin == nil {
		t.Fatal("origin: nil")
	}
	if origin.AgentID != "agent-1" {
		t.Errorf("AgentID: got %q want %q", origin.AgentID, "agent-1")
	}
	if origin.ParentAgentID != "root" {
		t.Errorf("ParentAgentID: got %q want %q", origin.ParentAgentID, "root")
	}
	if origin.IsGuardian {
		t.Errorf("IsGuardian: got true want false")
	}
	if origin.Depth != 1 {
		t.Errorf("Depth: got %d want 1", origin.Depth)
	}
}

func TestBuildSubagentOrigin_Depth1ParentDepth2(t *testing.T) {
	parentOrigin := &profile.SubagentOrigin{
		AgentID:       "agent-1",
		ParentAgentID: "root",
		IsGuardian:    false,
		Depth:         1,
	}
	origin := BuildSubagentOrigin(parentOrigin, "agent-2", "agent-1", false)
	if origin == nil {
		t.Fatal("origin: nil")
	}
	if origin.AgentID != "agent-2" {
		t.Errorf("AgentID: got %q want %q", origin.AgentID, "agent-2")
	}
	if origin.ParentAgentID != "agent-1" {
		t.Errorf("ParentAgentID: got %q want %q", origin.ParentAgentID, "agent-1")
	}
	if origin.Depth != 2 {
		t.Errorf("Depth: got %d want 2", origin.Depth)
	}
}

func TestBuildSubagentOrigin_Depth2ParentDepth3(t *testing.T) {
	parentOrigin := &profile.SubagentOrigin{
		AgentID:       "agent-2",
		ParentAgentID: "agent-1",
		IsGuardian:    false,
		Depth:         2,
	}
	origin := BuildSubagentOrigin(parentOrigin, "agent-3", "agent-2", false)
	if origin.Depth != 3 {
		t.Errorf("Depth: got %d want 3", origin.Depth)
	}
}

func TestBuildSubagentOrigin_GuardianFlagSet(t *testing.T) {
	parentOrigin := &profile.SubagentOrigin{
		AgentID:       "agent-1",
		ParentAgentID: "root",
		IsGuardian:    false,
		Depth:         1,
	}
	origin := BuildSubagentOrigin(parentOrigin, "guardian-1", "agent-1", true)
	if !origin.IsGuardian {
		t.Errorf("IsGuardian: got false want true")
	}
	if origin.AgentID != "guardian-1" {
		t.Errorf("AgentID: got %q want %q", origin.AgentID, "guardian-1")
	}
}

func TestBuildSubagentOrigin_GuardianFlagUnsetWhenFalse(t *testing.T) {
	parentOrigin := &profile.SubagentOrigin{
		AgentID:       "agent-1",
		ParentAgentID: "root",
		IsGuardian:    true, // parent is a guardian; child is NOT
		Depth:         1,
	}
	origin := BuildSubagentOrigin(parentOrigin, "agent-2", "agent-1", false)
	if origin.IsGuardian {
		t.Errorf("IsGuardian: got true want false (child should not inherit guardian flag)")
	}
}

func TestBuildSubagentOrigin_DoesNotMutateParent(t *testing.T) {
	parentOrigin := &profile.SubagentOrigin{
		AgentID:       "agent-1",
		ParentAgentID: "root",
		IsGuardian:    false,
		Depth:         1,
	}
	_ = BuildSubagentOrigin(parentOrigin, "agent-2", "agent-1", false)
	if parentOrigin.Depth != 1 {
		t.Errorf("parentOrigin.Depth mutated: got %d want 1", parentOrigin.Depth)
	}
	if parentOrigin.AgentID != "agent-1" {
		t.Errorf("parentOrigin.AgentID mutated: got %q want %q", parentOrigin.AgentID, "agent-1")
	}
}

// TestResolveSubagentBoundary_FullScenario exercises the end-to-end flow a
// sub-agent spawner would follow: inherit from a workspace_write parent,
// downgrade to read_only, and verify the resulting boundary.
func TestResolveSubagentBoundary_FullScenario_Downgrade(t *testing.T) {
	parentProfile := profile.WorkspaceWrite()
	parent := ParentBoundary{
		WorkspaceRoot: "/workspace/parent",
		WritableRoots: []string{"/workspace/parent/sub", "/tmp/parent"},
		Profile:       &parentProfile,
	}

	requested := profile.WorkspaceReadOnly()
	child, err := ResolveSubagentBoundary(parent, &requested)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if err := ValidateNoEscalation(child.Profile, parent.Profile); err != nil {
		t.Errorf("post-resolution validation failed: %v", err)
	}
	if child.WorkspaceRoot != parent.WorkspaceRoot {
		t.Errorf("WorkspaceRoot not inherited: got %q want %q",
			child.WorkspaceRoot, parent.WorkspaceRoot)
	}
	if len(child.WritableRoots) != len(parent.WritableRoots) {
		t.Errorf("WritableRoots not inherited: got %d want %d",
			len(child.WritableRoots), len(parent.WritableRoots))
	}
}

// TestResolveSubagentBoundary_FullScenario_Inherit exercises the case where
// the sub-agent inherits the parent's profile verbatim (no request).
func TestResolveSubagentBoundary_FullScenario_Inherit(t *testing.T) {
	parentProfile := profile.OpenWorldRestricted()
	parent := ParentBoundary{
		WorkspaceRoot: "/workspace/parent",
		WritableRoots: []string{"/workspace/parent/sub"},
		Profile:       &parentProfile,
	}

	child, err := ResolveSubagentBoundary(parent, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if child.Profile == nil {
		t.Fatal("Profile: nil")
	}
	if child.Profile.PermissionProfile != profile.PermissionOpenWorld {
		t.Errorf("PermissionProfile: got %q want %q",
			child.Profile.PermissionProfile, profile.PermissionOpenWorld)
	}
}

// Ensure the error returned by ValidateNoEscalation is non-wrapping so
// callers can use errors.Is/As if needed. This is a regression guard.
func TestValidateNoEscalation_ErrorIsNotWrapped(t *testing.T) {
	child := profileWith(profile.PermissionWorkspaceWrite)
	parent := profileWith(profile.PermissionReadOnly)
	err := ValidateNoEscalation(child, parent)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errors.Unwrap(err) != nil {
		t.Errorf("error should not be wrapped, got unwrap != nil: %v", err)
	}
}

// copyProfile returns a pointer to a deep copy of the given profile so
// tests can mutate one field without affecting the base.
func copyProfile(base profile.ExecutionProfile) *profile.ExecutionProfile {
	p := base
	if len(base.SandboxPermissions) > 0 {
		p.SandboxPermissions = append([]string(nil), base.SandboxPermissions...)
	}
	return &p
}

func TestValidateNoEscalation_SandboxPolicy(t *testing.T) {
	// required → none: escalation error
	parent := copyProfile(profile.GuardianReadOnly()) // SandboxRequired
	child := copyProfile(profile.GuardianReadOnly())
	child.SandboxPolicy = profile.SandboxNone
	if err := ValidateNoEscalation(child, parent); err == nil {
		t.Error("required → none: expected escalation error, got nil")
	} else if !strings.Contains(strings.ToLower(err.Error()), "sandbox") {
		t.Errorf("required → none: error should mention sandbox, got %v", err)
	}

	// none → required: OK (downgrade)
	parent = copyProfile(profile.DangerFullAccess()) // SandboxNone
	child = copyProfile(profile.DangerFullAccess())
	child.SandboxPolicy = profile.SandboxRequired
	if err := ValidateNoEscalation(child, parent); err != nil {
		t.Errorf("none → required: expected no error, got %v", err)
	}

	// required → best_effort: escalation error
	parent = copyProfile(profile.GuardianReadOnly()) // SandboxRequired
	child = copyProfile(profile.GuardianReadOnly())
	child.SandboxPolicy = profile.SandboxBestEffort
	if err := ValidateNoEscalation(child, parent); err == nil {
		t.Error("required → best_effort: expected escalation error, got nil")
	}

	// best_effort → required: OK (downgrade)
	parent = copyProfile(profile.WorkspaceWrite()) // SandboxBestEffort
	child = copyProfile(profile.WorkspaceWrite())
	child.SandboxPolicy = profile.SandboxRequired
	if err := ValidateNoEscalation(child, parent); err != nil {
		t.Errorf("best_effort → required: expected no error, got %v", err)
	}
}

func TestValidateNoEscalation_NetworkPolicy(t *testing.T) {
	// none → open: escalation error
	parent := copyProfile(profile.WorkspaceWrite()) // NetworkNone
	child := copyProfile(profile.WorkspaceWrite())
	child.NetworkPolicy = profile.NetworkOpen
	if err := ValidateNoEscalation(child, parent); err == nil {
		t.Error("none → open: expected escalation error, got nil")
	} else if !strings.Contains(strings.ToLower(err.Error()), "network") {
		t.Errorf("none → open: error should mention network, got %v", err)
	}

	// open → none: OK (downgrade)
	parent = copyProfile(profile.DangerFullAccess()) // NetworkOpen
	child = copyProfile(profile.DangerFullAccess())
	child.NetworkPolicy = profile.NetworkNone
	if err := ValidateNoEscalation(child, parent); err != nil {
		t.Errorf("open → none: expected no error, got %v", err)
	}

	// none → restricted: escalation error
	parent = copyProfile(profile.WorkspaceWrite()) // NetworkNone
	child = copyProfile(profile.WorkspaceWrite())
	child.NetworkPolicy = profile.NetworkRestricted
	if err := ValidateNoEscalation(child, parent); err == nil {
		t.Error("none → restricted: expected escalation error, got nil")
	}

	// restricted → none: OK (downgrade)
	parent = copyProfile(profile.OpenWorldRestricted()) // NetworkRestricted
	child = copyProfile(profile.OpenWorldRestricted())
	child.NetworkPolicy = profile.NetworkNone
	if err := ValidateNoEscalation(child, parent); err != nil {
		t.Errorf("restricted → none: expected no error, got %v", err)
	}
}

func TestValidateNoEscalation_EnvPolicy(t *testing.T) {
	// minimal → inherit: escalation error
	parent := copyProfile(profile.GuardianReadOnly()) // EnvMinimal
	child := copyProfile(profile.GuardianReadOnly())
	child.EnvPolicy = profile.EnvInherit
	if err := ValidateNoEscalation(child, parent); err == nil {
		t.Error("minimal → inherit: expected escalation error, got nil")
	} else if !strings.Contains(strings.ToLower(err.Error()), "env") {
		t.Errorf("minimal → inherit: error should mention env, got %v", err)
	}

	// inherit → minimal: OK (downgrade)
	parent = copyProfile(profile.DangerFullAccess()) // EnvInherit
	child = copyProfile(profile.DangerFullAccess())
	child.EnvPolicy = profile.EnvMinimal
	if err := ValidateNoEscalation(child, parent); err != nil {
		t.Errorf("inherit → minimal: expected no error, got %v", err)
	}
}

func TestValidateNoEscalation_ApprovalPolicy(t *testing.T) {
	// never → on_deny: escalation error
	parent := copyProfile(profile.WorkspaceWrite()) // ApprovalNever
	child := copyProfile(profile.WorkspaceWrite())
	child.ApprovalPolicy = profile.ApprovalOnDeny
	if err := ValidateNoEscalation(child, parent); err == nil {
		t.Error("never → on_deny: expected escalation error, got nil")
	} else if !strings.Contains(strings.ToLower(err.Error()), "approval") {
		t.Errorf("never → on_deny: error should mention approval, got %v", err)
	}

	// on_deny → never: OK (downgrade)
	parent = copyProfile(profile.DangerFullAccess()) // ApprovalOnDeny
	child = copyProfile(profile.DangerFullAccess())
	child.ApprovalPolicy = profile.ApprovalNever
	if err := ValidateNoEscalation(child, parent); err != nil {
		t.Errorf("on_deny → never: expected no error, got %v", err)
	}

	// never → always: escalation error
	parent = copyProfile(profile.WorkspaceWrite()) // ApprovalNever
	child = copyProfile(profile.WorkspaceWrite())
	child.ApprovalPolicy = profile.ApprovalAlways
	if err := ValidateNoEscalation(child, parent); err == nil {
		t.Error("never → always: expected escalation error, got nil")
	}
}

func TestValidateNoEscalation_SandboxPermissions(t *testing.T) {
	// parent [read,write], child [read,write,execute]: escalation error
	parent := copyProfile(profile.WorkspaceWrite()) // perms=["read","write"]
	child := copyProfile(profile.WorkspaceWrite())
	child.SandboxPermissions = []string{"read", "write", "execute"}
	if err := ValidateNoEscalation(child, parent); err == nil {
		t.Error("[read,write] → [read,write,execute]: expected escalation error, got nil")
	} else if !strings.Contains(strings.ToLower(err.Error()), "permission") {
		t.Errorf("[read,write] → [read,write,execute]: error should mention permission, got %v", err)
	}

	// parent [read,write,execute], child [read]: OK (subset)
	parent = copyProfile(profile.DangerFullAccess()) // perms=["read","write","execute"]
	child = copyProfile(profile.DangerFullAccess())
	child.SandboxPermissions = []string{"read"}
	if err := ValidateNoEscalation(child, parent); err != nil {
		t.Errorf("[read,write,execute] → [read]: expected no error, got %v", err)
	}

	// parent [read,write], child [read]: OK (subset)
	parent = copyProfile(profile.WorkspaceWrite()) // perms=["read","write"]
	child = copyProfile(profile.WorkspaceWrite())
	child.SandboxPermissions = []string{"read"}
	if err := ValidateNoEscalation(child, parent); err != nil {
		t.Errorf("[read,write] → [read]: expected no error, got %v", err)
	}

	// parent [read,write], child [read,execute]: escalation error
	parent = copyProfile(profile.WorkspaceWrite())
	child = copyProfile(profile.WorkspaceWrite())
	child.SandboxPermissions = []string{"read", "execute"}
	if err := ValidateNoEscalation(child, parent); err == nil {
		t.Error("[read,write] → [read,execute]: expected escalation error, got nil")
	}

	// parent [read,write], child []: OK (empty subset)
	parent = copyProfile(profile.WorkspaceWrite())
	child = copyProfile(profile.WorkspaceWrite())
	child.SandboxPermissions = nil
	if err := ValidateNoEscalation(child, parent); err != nil {
		t.Errorf("[read,write] → []: expected no error, got %v", err)
	}
}
