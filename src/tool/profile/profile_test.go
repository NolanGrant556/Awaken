package profile

import (
	"strings"
	"testing"
)

// --- Built-in profile tests ---

func TestGuardianReadOnly(t *testing.T) {
	p := GuardianReadOnly()
	if p.Name != "guardian_read_only" {
		t.Errorf("Name = %q, want %q", p.Name, "guardian_read_only")
	}
	if p.PermissionProfile != PermissionReadOnly {
		t.Errorf("PermissionProfile = %q, want %q", p.PermissionProfile, PermissionReadOnly)
	}
	if p.SandboxPolicy != SandboxRequired {
		t.Errorf("SandboxPolicy = %q, want %q", p.SandboxPolicy, SandboxRequired)
	}
	if !equalStringSlices(p.SandboxPermissions, []string{"read"}) {
		t.Errorf("SandboxPermissions = %v, want [read]", p.SandboxPermissions)
	}
	if p.NetworkPolicy != NetworkRestricted {
		t.Errorf("NetworkPolicy = %q, want %q", p.NetworkPolicy, NetworkRestricted)
	}
	if p.EnvPolicy != EnvMinimal {
		t.Errorf("EnvPolicy = %q, want %q", p.EnvPolicy, EnvMinimal)
	}
	if p.VisibilityPolicy != VisibilityFull {
		t.Errorf("VisibilityPolicy = %q, want %q", p.VisibilityPolicy, VisibilityFull)
	}
}

func TestWorkspaceReadOnly(t *testing.T) {
	p := WorkspaceReadOnly()
	if p.Name != "workspace_read_only" {
		t.Errorf("Name = %q, want %q", p.Name, "workspace_read_only")
	}
	if p.PermissionProfile != PermissionReadOnly {
		t.Errorf("PermissionProfile = %q, want %q", p.PermissionProfile, PermissionReadOnly)
	}
	if p.SandboxPolicy != SandboxBestEffort {
		t.Errorf("SandboxPolicy = %q, want %q", p.SandboxPolicy, SandboxBestEffort)
	}
	if !equalStringSlices(p.SandboxPermissions, []string{"read"}) {
		t.Errorf("SandboxPermissions = %v, want [read]", p.SandboxPermissions)
	}
	if p.NetworkPolicy != NetworkNone {
		t.Errorf("NetworkPolicy = %q, want %q", p.NetworkPolicy, NetworkNone)
	}
	if p.EnvPolicy != EnvMinimal {
		t.Errorf("EnvPolicy = %q, want %q", p.EnvPolicy, EnvMinimal)
	}
	if p.VisibilityPolicy != VisibilityFull {
		t.Errorf("VisibilityPolicy = %q, want %q", p.VisibilityPolicy, VisibilityFull)
	}
}

func TestWorkspaceWrite(t *testing.T) {
	p := WorkspaceWrite()
	if p.Name != "workspace_write" {
		t.Errorf("Name = %q, want %q", p.Name, "workspace_write")
	}
	if p.PermissionProfile != PermissionWorkspaceWrite {
		t.Errorf("PermissionProfile = %q, want %q", p.PermissionProfile, PermissionWorkspaceWrite)
	}
	if p.SandboxPolicy != SandboxBestEffort {
		t.Errorf("SandboxPolicy = %q, want %q", p.SandboxPolicy, SandboxBestEffort)
	}
	if !equalStringSlices(p.SandboxPermissions, []string{"read", "write"}) {
		t.Errorf("SandboxPermissions = %v, want [read write]", p.SandboxPermissions)
	}
	if p.NetworkPolicy != NetworkNone {
		t.Errorf("NetworkPolicy = %q, want %q", p.NetworkPolicy, NetworkNone)
	}
	if p.EnvPolicy != EnvInherit {
		t.Errorf("EnvPolicy = %q, want %q", p.EnvPolicy, EnvInherit)
	}
	if p.VisibilityPolicy != VisibilityFull {
		t.Errorf("VisibilityPolicy = %q, want %q", p.VisibilityPolicy, VisibilityFull)
	}
}

func TestOpenWorldRestricted(t *testing.T) {
	p := OpenWorldRestricted()
	if p.Name != "open_world_restricted" {
		t.Errorf("Name = %q, want %q", p.Name, "open_world_restricted")
	}
	if p.PermissionProfile != PermissionOpenWorld {
		t.Errorf("PermissionProfile = %q, want %q", p.PermissionProfile, PermissionOpenWorld)
	}
	if p.SandboxPolicy != SandboxRequired {
		t.Errorf("SandboxPolicy = %q, want %q", p.SandboxPolicy, SandboxRequired)
	}
	if !equalStringSlices(p.SandboxPermissions, []string{"read", "write"}) {
		t.Errorf("SandboxPermissions = %v, want [read write]", p.SandboxPermissions)
	}
	if p.NetworkPolicy != NetworkRestricted {
		t.Errorf("NetworkPolicy = %q, want %q", p.NetworkPolicy, NetworkRestricted)
	}
	if p.EnvPolicy != EnvMinimal {
		t.Errorf("EnvPolicy = %q, want %q", p.EnvPolicy, EnvMinimal)
	}
	if p.VisibilityPolicy != VisibilityFull {
		t.Errorf("VisibilityPolicy = %q, want %q", p.VisibilityPolicy, VisibilityFull)
	}
}

func TestDangerFullAccess(t *testing.T) {
	p := DangerFullAccess()
	if p.Name != "danger_full_access" {
		t.Errorf("Name = %q, want %q", p.Name, "danger_full_access")
	}
	if p.PermissionProfile != PermissionDangerFullAccess {
		t.Errorf("PermissionProfile = %q, want %q", p.PermissionProfile, PermissionDangerFullAccess)
	}
	if p.SandboxPolicy != SandboxNone {
		t.Errorf("SandboxPolicy = %q, want %q", p.SandboxPolicy, SandboxNone)
	}
	if !equalStringSlices(p.SandboxPermissions, []string{"read", "write", "execute"}) {
		t.Errorf("SandboxPermissions = %v, want [read write execute]", p.SandboxPermissions)
	}
	if p.NetworkPolicy != NetworkOpen {
		t.Errorf("NetworkPolicy = %q, want %q", p.NetworkPolicy, NetworkOpen)
	}
	if p.EnvPolicy != EnvInherit {
		t.Errorf("EnvPolicy = %q, want %q", p.EnvPolicy, EnvInherit)
	}
	if p.VisibilityPolicy != VisibilityFull {
		t.Errorf("VisibilityPolicy = %q, want %q", p.VisibilityPolicy, VisibilityFull)
	}
}

// --- Default / guardian accessor tests ---

func TestDefaultProfile(t *testing.T) {
	p := DefaultProfile()
	if p.Name != "workspace_write" {
		t.Errorf("DefaultProfile().Name = %q, want %q", p.Name, "workspace_write")
	}
	// Verify it matches WorkspaceWrite() exactly.
	expected := WorkspaceWrite()
	if !profilesEqual(p, expected) {
		t.Errorf("DefaultProfile() = %+v, want %+v", p, expected)
	}
}

func TestProfileForGuardian(t *testing.T) {
	p := ProfileForGuardian()
	if p.Name != "guardian_read_only" {
		t.Errorf("ProfileForGuardian().Name = %q, want %q", p.Name, "guardian_read_only")
	}
	// Verify it matches GuardianReadOnly() exactly.
	expected := GuardianReadOnly()
	if !profilesEqual(p, expected) {
		t.Errorf("ProfileForGuardian() = %+v, want %+v", p, expected)
	}
}

// --- Guardian-specific invariants from the spec ---

func TestGuardianProfileInvariants(t *testing.T) {
	p := ProfileForGuardian()
	if p.PermissionProfile != PermissionReadOnly {
		t.Errorf("guardian PermissionProfile = %q, want %q (read_only)", p.PermissionProfile, PermissionReadOnly)
	}
	if p.NetworkPolicy != NetworkRestricted {
		t.Errorf("guardian NetworkPolicy = %q, want %q (restricted)", p.NetworkPolicy, NetworkRestricted)
	}
}

// --- String() test ---

func TestExecutionProfileString(t *testing.T) {
	p := WorkspaceWrite()
	s := p.String()
	if !strings.Contains(s, "name=workspace_write") {
		t.Errorf("String() missing name: %q", s)
	}
	if !strings.Contains(s, "permission=workspace_write") {
		t.Errorf("String() missing permission: %q", s)
	}
}

// --- Profile isolation test ---

// TestProfileConstructorReturnsFreshCopy verifies that each call to a
// profile constructor returns an independent copy. Callers may swap fields
// on a copy without affecting other callers.
func TestProfileConstructorReturnsFreshCopy(t *testing.T) {
	p1 := WorkspaceWrite()
	p1.SandboxPermissions = append(p1.SandboxPermissions, "execute")
	p2 := WorkspaceWrite()
	if equalStringSlices(p2.SandboxPermissions, p1.SandboxPermissions) {
		t.Errorf("modifying p1.SandboxPermissions affected p2: both = %v", p2.SandboxPermissions)
	}
}

// --- helpers ---

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i, v := range a {
		if v != b[i] {
			return false
		}
	}
	return true
}

func profilesEqual(a, b ExecutionProfile) bool {
	if a.Name != b.Name {
		return false
	}
	if a.PermissionProfile != b.PermissionProfile {
		return false
	}
	if a.SandboxPolicy != b.SandboxPolicy {
		return false
	}
	if !equalStringSlices(a.SandboxPermissions, b.SandboxPermissions) {
		return false
	}
	if a.NetworkPolicy != b.NetworkPolicy {
		return false
	}
	if a.EnvPolicy != b.EnvPolicy {
		return false
	}
	if a.VisibilityPolicy != b.VisibilityPolicy {
		return false
	}
	return true
}
