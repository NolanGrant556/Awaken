package guardian

import (
	"errors"
	"strings"
	"testing"

	"awaken/src/agentcore/subagent"
	"awaken/src/tool/profile"
)

// ptrProfile returns a pointer to the given ExecutionProfile. Used in tests
// to construct subagent.ParentBoundary values (whose Profile field is a
// pointer).
func ptrProfile(p profile.ExecutionProfile) *profile.ExecutionProfile {
	return &p
}

// TestEnforceGuardianConstraints_ReadOnlyToolAllowed verifies that a
// read-only tool (e.g. file_read) is allowed under the guardian profile.
func TestEnforceGuardianConstraints_ReadOnlyToolAllowed(t *testing.T) {
	intent := profile.ExecutionIntent{
		ToolName:        "file_read",
		ToolKind:        profile.ToolKindFileChain,
		RequestedAction: profile.ActionRead,
		NetworkScope:    profile.NetworkScopeNone,
	}
	err := EnforceGuardianConstraints(intent, profile.GuardianReadOnly())
	if err != nil {
		t.Fatalf("expected nil for read-only tool under guardian profile, got %v", err)
	}
}

// TestEnforceGuardianConstraints_InspectActionAllowed verifies that an
// inspect action (e.g. list_files, grep_search) is allowed.
func TestEnforceGuardianConstraints_InspectActionAllowed(t *testing.T) {
	intent := profile.ExecutionIntent{
		ToolName:        "list_files",
		ToolKind:        profile.ToolKindFileChain,
		RequestedAction: profile.ActionInspect,
		NetworkScope:    profile.NetworkScopeNone,
	}
	err := EnforceGuardianConstraints(intent, profile.GuardianReadOnly())
	if err != nil {
		t.Fatalf("expected nil for inspect tool under guardian profile, got %v", err)
	}
}

// TestEnforceGuardianConstraints_WriteToolRejected verifies that a write
// tool (e.g. apply_patch with ActionWrite) is rejected.
func TestEnforceGuardianConstraints_WriteToolRejected(t *testing.T) {
	intent := profile.ExecutionIntent{
		ToolName:        "apply_patch",
		ToolKind:        profile.ToolKindFileChain,
		RequestedAction: profile.ActionWrite,
		NetworkScope:    profile.NetworkScopeNone,
	}
	err := EnforceGuardianConstraints(intent, profile.GuardianReadOnly())
	if err == nil {
		t.Fatal("expected error for write tool under guardian profile, got nil")
	}
	var violation *GuardianViolationError
	if !errors.As(err, &violation) {
		t.Fatalf("expected *GuardianViolationError, got %T: %v", err, err)
	}
	if violation.ToolName != "apply_patch" {
		t.Errorf("expected ToolName %q, got %q", "apply_patch", violation.ToolName)
	}
	if !strings.Contains(strings.ToLower(violation.Reason), "read-only") && !strings.Contains(strings.ToLower(violation.Reason), "forbids") {
		t.Errorf("expected reason to mention read-only or forbids, got %q", violation.Reason)
	}
}

// TestEnforceGuardianConstraints_DestructiveToolRejected verifies that a
// destructive tool (delete action) is rejected.
func TestEnforceGuardianConstraints_DestructiveToolRejected(t *testing.T) {
	intent := profile.ExecutionIntent{
		ToolName:        "apply_patch",
		ToolKind:        profile.ToolKindFileChain,
		RequestedAction: profile.ActionDelete,
		NetworkScope:    profile.NetworkScopeNone,
	}
	err := EnforceGuardianConstraints(intent, profile.GuardianReadOnly())
	if err == nil {
		t.Fatal("expected error for destructive (delete) tool under guardian profile, got nil")
	}
	var violation *GuardianViolationError
	if !errors.As(err, &violation) {
		t.Fatalf("expected *GuardianViolationError, got %T: %v", err, err)
	}
	if violation.ToolName != "apply_patch" {
		t.Errorf("expected ToolName %q, got %q", "apply_patch", violation.ToolName)
	}
}

// TestEnforceGuardianConstraints_MoveActionRejected verifies that a move
// action is rejected as destructive.
func TestEnforceGuardianConstraints_MoveActionRejected(t *testing.T) {
	intent := profile.ExecutionIntent{
		ToolName:        "apply_patch",
		ToolKind:        profile.ToolKindFileChain,
		RequestedAction: profile.ActionMove,
		NetworkScope:    profile.NetworkScopeNone,
	}
	err := EnforceGuardianConstraints(intent, profile.GuardianReadOnly())
	if err == nil {
		t.Fatal("expected error for move action under guardian profile, got nil")
	}
	var violation *GuardianViolationError
	if !errors.As(err, &violation) {
		t.Fatalf("expected *GuardianViolationError, got %T: %v", err, err)
	}
}

// TestEnforceGuardianConstraints_ExecuteActionRejected verifies that an
// execute action (e.g. shell command) is rejected.
func TestEnforceGuardianConstraints_ExecuteActionRejected(t *testing.T) {
	intent := profile.ExecutionIntent{
		ToolName:        "shell",
		ToolKind:        profile.ToolKindShellCommand,
		RequestedAction: profile.ActionExecute,
		NetworkScope:    profile.NetworkScopeNone,
	}
	err := EnforceGuardianConstraints(intent, profile.GuardianReadOnly())
	if err == nil {
		t.Fatal("expected error for execute action under guardian profile, got nil")
	}
	var violation *GuardianViolationError
	if !errors.As(err, &violation) {
		t.Fatalf("expected *GuardianViolationError, got %T: %v", err, err)
	}
}

// TestEnforceGuardianConstraints_OpenWorldToolRejected verifies that an
// open-world tool (NetworkScope == open) is rejected.
func TestEnforceGuardianConstraints_OpenWorldToolRejected(t *testing.T) {
	intent := profile.ExecutionIntent{
		ToolName:        "web_search",
		ToolKind:        profile.ToolKindShellCommand,
		RequestedAction: profile.ActionRead,
		NetworkScope:    profile.NetworkScopeOpen,
	}
	err := EnforceGuardianConstraints(intent, profile.GuardianReadOnly())
	if err == nil {
		t.Fatal("expected error for open-world tool under guardian profile, got nil")
	}
	var violation *GuardianViolationError
	if !errors.As(err, &violation) {
		t.Fatalf("expected *GuardianViolationError, got %T: %v", err, err)
	}
	if !strings.Contains(strings.ToLower(violation.Reason), "open") {
		t.Errorf("expected reason to mention open, got %q", violation.Reason)
	}
}

// TestEnforceGuardianConstraints_OpenNetworkScopeRejected verifies that an
// open network scope is rejected even for a read action.
func TestEnforceGuardianConstraints_OpenNetworkScopeRejected(t *testing.T) {
	intent := profile.ExecutionIntent{
		ToolName:        "mcp__fetch",
		ToolKind:        profile.ToolKindMcpApp,
		RequestedAction: profile.ActionRead,
		NetworkScope:    profile.NetworkScopeOpen,
	}
	err := EnforceGuardianConstraints(intent, profile.GuardianReadOnly())
	if err == nil {
		t.Fatal("expected error for open network scope under guardian profile, got nil")
	}
}

// TestEnforceGuardianConstraints_RestrictedNetworkAllowed verifies that
// restricted network scope (e.g. a named MCP server) is allowed for a
// read-only guardian tool.
func TestEnforceGuardianConstraints_RestrictedNetworkAllowed(t *testing.T) {
	intent := profile.ExecutionIntent{
		ToolName:        "mcp__review_server__inspect",
		ToolKind:        profile.ToolKindMcpApp,
		RequestedAction: profile.ActionRead,
		NetworkScope:    profile.NetworkScopeRestricted,
	}
	err := EnforceGuardianConstraints(intent, profile.GuardianReadOnly())
	if err != nil {
		t.Fatalf("expected nil for restricted-network read tool under guardian profile, got %v", err)
	}
}

// TestEnforceGuardianConstraints_NonGuardianProfileNotEnforced verifies
// that a non-guardian profile (e.g. WorkspaceWrite) does not trigger
// guardian enforcement — even for a write action.
func TestEnforceGuardianConstraints_NonGuardianProfileNotEnforced(t *testing.T) {
	intent := profile.ExecutionIntent{
		ToolName:        "apply_patch",
		ToolKind:        profile.ToolKindFileChain,
		RequestedAction: profile.ActionWrite,
		NetworkScope:    profile.NetworkScopeNone,
	}
	err := EnforceGuardianConstraints(intent, profile.WorkspaceWrite())
	if err != nil {
		t.Fatalf("expected nil for write tool under non-guardian profile, got %v", err)
	}
}

// TestEnforceGuardianConstraints_NonGuardianReadOnlyProfileNotEnforced
// verifies that WorkspaceReadOnly (which shares PermissionReadOnly but is
// not a guardian profile) does not trigger guardian enforcement.
func TestEnforceGuardianConstraints_NonGuardianReadOnlyProfileNotEnforced(t *testing.T) {
	intent := profile.ExecutionIntent{
		ToolName:        "apply_patch",
		ToolKind:        profile.ToolKindFileChain,
		RequestedAction: profile.ActionWrite,
		NetworkScope:    profile.NetworkScopeNone,
	}
	err := EnforceGuardianConstraints(intent, profile.WorkspaceReadOnly())
	if err != nil {
		t.Fatalf("expected nil for write tool under workspace_read_only (non-guardian) profile, got %v", err)
	}
}

// TestEnforceGuardianConstraints_DangerFullAccessNotEnforced verifies that
// the danger profile does not trigger guardian enforcement.
func TestEnforceGuardianConstraints_DangerFullAccessNotEnforced(t *testing.T) {
	intent := profile.ExecutionIntent{
		ToolName:        "shell",
		ToolKind:        profile.ToolKindShellCommand,
		RequestedAction: profile.ActionExecute,
		NetworkScope:    profile.NetworkScopeOpen,
	}
	err := EnforceGuardianConstraints(intent, profile.DangerFullAccess())
	if err != nil {
		t.Fatalf("expected nil for execute tool under danger profile, got %v", err)
	}
}

// TestEnforceGuardianConstraints_EmptyToolName verifies that the error
// handles an empty tool name gracefully.
func TestEnforceGuardianConstraints_EmptyToolName(t *testing.T) {
	intent := profile.ExecutionIntent{
		ToolName:        "",
		ToolKind:        profile.ToolKindShellCommand,
		RequestedAction: profile.ActionExecute,
		NetworkScope:    profile.NetworkScopeNone,
	}
	err := EnforceGuardianConstraints(intent, profile.GuardianReadOnly())
	if err == nil {
		t.Fatal("expected error for execute action with empty tool name, got nil")
	}
}

// TestGuardianViolationError_Error verifies the Error() method produces a
// readable message.
func TestGuardianViolationError_Error(t *testing.T) {
	e := &GuardianViolationError{
		ToolName: "apply_patch",
		Reason:   "write action forbidden",
	}
	msg := e.Error()
	if !strings.Contains(msg, "apply_patch") {
		t.Errorf("expected error message to contain tool name, got %q", msg)
	}
	if !strings.Contains(msg, "write action forbidden") {
		t.Errorf("expected error message to contain reason, got %q", msg)
	}
}

// TestGuardianViolationError_Error_EmptyFields verifies the Error() method
// handles empty fields gracefully.
func TestGuardianViolationError_Error_EmptyFields(t *testing.T) {
	e := &GuardianViolationError{}
	msg := e.Error()
	if !strings.Contains(msg, "<unknown>") {
		t.Errorf("expected error message to contain <unknown> for empty tool name, got %q", msg)
	}
	if !strings.Contains(msg, "guardian read-only constraint violated") {
		t.Errorf("expected default reason in error message, got %q", msg)
	}
}

// TestIsGuardianProfile_GuardianReadOnly verifies IsGuardianProfile returns
// true for the GuardianReadOnly profile.
func TestIsGuardianProfile_GuardianReadOnly(t *testing.T) {
	if !IsGuardianProfile(profile.GuardianReadOnly()) {
		t.Error("expected IsGuardianProfile to return true for GuardianReadOnly()")
	}
}

// TestIsGuardianProfile_WorkspaceWrite verifies IsGuardianProfile returns
// false for WorkspaceWrite.
func TestIsGuardianProfile_WorkspaceWrite(t *testing.T) {
	if IsGuardianProfile(profile.WorkspaceWrite()) {
		t.Error("expected IsGuardianProfile to return false for WorkspaceWrite()")
	}
}

// TestIsGuardianProfile_WorkspaceReadOnly verifies that WorkspaceReadOnly
// (which shares PermissionReadOnly but has NetworkNone and no restricted
// requirement) is NOT identified as a guardian profile.
func TestIsGuardianProfile_WorkspaceReadOnly(t *testing.T) {
	if IsGuardianProfile(profile.WorkspaceReadOnly()) {
		t.Error("expected IsGuardianProfile to return false for WorkspaceReadOnly (network=none, not restricted)")
	}
}

// TestIsGuardianProfile_OpenWorldRestricted verifies that
// OpenWorldRestricted is NOT a guardian profile.
func TestIsGuardianProfile_OpenWorldRestricted(t *testing.T) {
	if IsGuardianProfile(profile.OpenWorldRestricted()) {
		t.Error("expected IsGuardianProfile to return false for OpenWorldRestricted")
	}
}

// TestIsGuardianProfile_DangerFullAccess verifies that DangerFullAccess is
// NOT a guardian profile.
func TestIsGuardianProfile_DangerFullAccess(t *testing.T) {
	if IsGuardianProfile(profile.DangerFullAccess()) {
		t.Error("expected IsGuardianProfile to return false for DangerFullAccess")
	}
}

// TestIsGuardianProfile_ProfileForGuardian verifies that ProfileForGuardian
// returns a profile identified as guardian.
func TestIsGuardianProfile_ProfileForGuardian(t *testing.T) {
	if !IsGuardianProfile(profile.ProfileForGuardian()) {
		t.Error("expected IsGuardianProfile to return true for ProfileForGuardian()")
	}
}

// TestResolveGuardianBoundary_ForcesGuardianProfile verifies that a parent
// with a higher-privilege profile (workspace_write) produces a child
// boundary with the guardian profile.
func TestResolveGuardianBoundary_ForcesGuardianProfile(t *testing.T) {
	parent := subagent.ParentBoundary{
		Profile:       ptrProfile(profile.WorkspaceWrite()),
		WorkspaceRoot: "/workspace",
		WritableRoots: []string{"/workspace/sub"},
	}
	child := ResolveGuardianBoundary(parent)
	if child.Profile == nil {
		t.Fatal("expected child profile to be non-nil")
	}
	if !IsGuardianProfile(*child.Profile) {
		t.Errorf("expected child profile to be guardian, got %s", *child.Profile)
	}
	if child.Profile.Name != "guardian_read_only" {
		t.Errorf("expected child profile name %q, got %q", "guardian_read_only", child.Profile.Name)
	}
}

// TestResolveGuardianBoundary_InheritsWorkspaceRoots verifies that the
// child boundary inherits the parent's workspace root and writable roots.
func TestResolveGuardianBoundary_InheritsWorkspaceRoots(t *testing.T) {
	parent := subagent.ParentBoundary{
		Profile:       ptrProfile(profile.DangerFullAccess()),
		WorkspaceRoot: "/projects/awaken",
		WritableRoots: []string{"/projects/awaken/src", "/tmp/build"},
	}
	child := ResolveGuardianBoundary(parent)
	if child.WorkspaceRoot != "/projects/awaken" {
		t.Errorf("expected workspace root %q, got %q", "/projects/awaken", child.WorkspaceRoot)
	}
	if len(child.WritableRoots) != 2 {
		t.Fatalf("expected 2 writable roots, got %d", len(child.WritableRoots))
	}
	if child.WritableRoots[0] != "/projects/awaken/src" {
		t.Errorf("expected writable root[0] %q, got %q", "/projects/awaken/src", child.WritableRoots[0])
	}
	if child.WritableRoots[1] != "/tmp/build" {
		t.Errorf("expected writable root[1] %q, got %q", "/tmp/build", child.WritableRoots[1])
	}
}

// TestResolveGuardianBoundary_DoesNotMutateParent verifies that the
// returned boundary is a copy: mutating it does not affect the parent.
func TestResolveGuardianBoundary_DoesNotMutateParent(t *testing.T) {
	parentProfile := profile.WorkspaceWrite()
	parent := subagent.ParentBoundary{
		Profile:       &parentProfile,
		WorkspaceRoot: "/workspace",
		WritableRoots: []string{"/workspace/sub"},
	}
	child := ResolveGuardianBoundary(parent)

	// Mutate the child
	child.WritableRoots[0] = "/mutated"
	child.WorkspaceRoot = "/mutated"
	if child.Profile != nil {
		child.Profile.Name = "mutated"
	}

	// Parent should be unaffected
	if parent.WorkspaceRoot != "/workspace" {
		t.Errorf("parent workspace root was mutated: %q", parent.WorkspaceRoot)
	}
	if parent.WritableRoots[0] != "/workspace/sub" {
		t.Errorf("parent writable root was mutated: %q", parent.WritableRoots[0])
	}
	if parent.Profile.Name != "workspace_write" {
		t.Errorf("parent profile was mutated: %s", parent.Profile.Name)
	}
}

// TestResolveGuardianBoundary_AlreadyGuardian verifies that resolving a
// boundary that is already guardian still produces a guardian boundary
// (idempotent).
func TestResolveGuardianBoundary_AlreadyGuardian(t *testing.T) {
	parent := subagent.ParentBoundary{
		Profile:       ptrProfile(profile.GuardianReadOnly()),
		WorkspaceRoot: "/workspace",
	}
	child := ResolveGuardianBoundary(parent)
	if child.Profile == nil {
		t.Fatal("expected child profile to be non-nil")
	}
	if !IsGuardianProfile(*child.Profile) {
		t.Error("expected child to remain guardian when parent is already guardian")
	}
}

// TestResolveGuardianBoundary_EmptyWritableRoots verifies that a parent
// with no writable roots produces a child with no writable roots.
func TestResolveGuardianBoundary_EmptyWritableRoots(t *testing.T) {
	parent := subagent.ParentBoundary{
		Profile:       ptrProfile(profile.WorkspaceWrite()),
		WorkspaceRoot: "/workspace",
		WritableRoots: nil,
	}
	child := ResolveGuardianBoundary(parent)
	if len(child.WritableRoots) != 0 {
		t.Errorf("expected 0 writable roots, got %d", len(child.WritableRoots))
	}
}

// TestResolveGuardianBoundary_NilParentProfile verifies that a parent with
// no pinned profile still produces a child with the guardian profile.
func TestResolveGuardianBoundary_NilParentProfile(t *testing.T) {
	parent := subagent.ParentBoundary{
		Profile:       nil,
		WorkspaceRoot: "/workspace",
		WritableRoots: []string{"/workspace/sub"},
	}
	child := ResolveGuardianBoundary(parent)
	if child.Profile == nil {
		t.Fatal("expected child profile to be non-nil even when parent profile is nil")
	}
	if !IsGuardianProfile(*child.Profile) {
		t.Errorf("expected child profile to be guardian, got %s", *child.Profile)
	}
}
