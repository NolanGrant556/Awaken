package agentcore

import "testing"

func TestRunProfileNormalizeTreatsDefaultPermissionModeAsUnset(t *testing.T) {
	profile := RunProfile{
		PermissionMode: "default",
	}.Normalize()

	if profile.PermissionMode != PermissionModeDefault {
		t.Fatalf("expected default permission mode to normalize to unset, got %q", profile.PermissionMode)
	}
}

func TestRunProfileValidateAcceptsDefaultPermissionMode(t *testing.T) {
	if err := (RunProfile{PermissionMode: "default"}).Validate(); err != nil {
		t.Fatalf("expected default permission mode to validate, got %v", err)
	}
}
