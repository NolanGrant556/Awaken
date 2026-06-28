package tool

import "testing"

func TestPermissionPolicyNormalizeDefaultsToAsk(t *testing.T) {
	policy := PermissionPolicy{
		AllowTools: []string{" Shell ", "shell"},
		DenyTools:  []string{"APPLY_PATCH", "apply_patch"},
	}.Normalize()

	if policy.Mode != PermissionModeAsk {
		t.Fatalf("expected default mode ask, got %q", policy.Mode)
	}
	if len(policy.AllowTools) != 1 || policy.AllowTools[0] != "shell" {
		t.Fatalf("unexpected normalized allow tools: %+v", policy.AllowTools)
	}
	if len(policy.DenyTools) != 1 || policy.DenyTools[0] != "apply_patch" {
		t.Fatalf("unexpected normalized deny tools: %+v", policy.DenyTools)
	}
}

func TestApplyPermissionPolicyAllowListOverridesAsk(t *testing.T) {
	decision := ApplyPermissionPolicy(
		AskPermission("needs approval", ApprovalRequest{}),
		PermissionPolicy{Mode: PermissionModeAsk, AllowTools: []string{"shell"}},
		"shell",
		false,
	)

	if decision.Behavior != PermissionAllow {
		t.Fatalf("expected allowlisted tool to be allowed, got %+v", decision)
	}
}

func TestApplyPermissionPolicyDenyListOverridesAllow(t *testing.T) {
	decision := ApplyPermissionPolicy(
		AllowPermission(),
		PermissionPolicy{Mode: PermissionModeAllow, DenyTools: []string{"shell"}},
		"shell",
		false,
	)

	if decision.Behavior != PermissionDeny {
		t.Fatalf("expected denylisted tool to be denied, got %+v", decision)
	}
}

func TestApplyPermissionPolicyDenyModeKeepsReadOnlyDecision(t *testing.T) {
	decision := ApplyPermissionPolicy(
		AllowPermission(),
		PermissionPolicy{Mode: PermissionModeDeny},
		"read_file",
		true,
	)

	if decision.Behavior != PermissionAllow {
		t.Fatalf("expected read-only tool to remain allowed in deny mode, got %+v", decision)
	}
}
