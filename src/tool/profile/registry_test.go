package profile

import (
	"testing"
)

// --- ClientAllowedProfiles ---

func TestClientAllowedProfilesReturnsClientSet(t *testing.T) {
	got := ClientAllowedProfiles()
	if len(got) != 3 {
		t.Fatalf("expected 3 client-allowed profiles, got %d", len(got))
	}

	wantNames := map[string]bool{
		WorkspaceReadOnly().Name:   false,
		WorkspaceWrite().Name:      false,
		OpenWorldRestricted().Name: false,
	}
	for _, p := range got {
		if _, ok := wantNames[p.Name]; !ok {
			t.Fatalf("unexpected client-allowed profile name %q", p.Name)
		}
		wantNames[p.Name] = true
	}
	for name, seen := range wantNames {
		if !seen {
			t.Fatalf("client-allowed set missing %q", name)
		}
	}

	// GuardianReadOnly and DangerFullAccess must NOT appear in the set.
	for _, p := range got {
		if p.Name == GuardianReadOnly().Name {
			t.Fatalf("guardian_read_only must not be in ClientAllowedProfiles")
		}
		if p.Name == DangerFullAccess().Name {
			t.Fatalf("danger_full_access must not be in ClientAllowedProfiles")
		}
	}
}

// --- IsClientAllowed ---

func TestIsClientAllowedForBuiltInProfiles(t *testing.T) {
	cases := []struct {
		name string
		p    ExecutionProfile
		want bool
	}{
		{"workspace_write", WorkspaceWrite(), true},
		{"workspace_read_only", WorkspaceReadOnly(), true},
		{"open_world_restricted", OpenWorldRestricted(), true},
		{"guardian_read_only", GuardianReadOnly(), false},
		{"danger_full_access", DangerFullAccess(), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsClientAllowed(tc.p); got != tc.want {
				t.Fatalf("IsClientAllowed(%s) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}

func TestIsClientAllowedRejectsTamperedFields(t *testing.T) {
	// A WorkspaceWrite with SandboxNone instead of SandboxBestEffort must
	// no longer match the built-in WorkspaceWrite.
	tampered := WorkspaceWrite()
	tampered.SandboxPolicy = SandboxNone
	if IsClientAllowed(tampered) {
		t.Fatalf("IsClientAllowed accepted a tampered WorkspaceWrite (SandboxPolicy changed)")
	}

	// A WorkspaceWrite with an escalated PermissionProfile must also be rejected.
	tamperedPerm := WorkspaceWrite()
	tamperedPerm.PermissionProfile = PermissionDangerFullAccess
	if IsClientAllowed(tamperedPerm) {
		t.Fatalf("IsClientAllowed accepted a tampered WorkspaceWrite (PermissionProfile changed)")
	}

	// A WorkspaceWrite with NetworkOpen must also be rejected.
	tamperedNet := WorkspaceWrite()
	tamperedNet.NetworkPolicy = NetworkOpen
	if IsClientAllowed(tamperedNet) {
		t.Fatalf("IsClientAllowed accepted a tampered WorkspaceWrite (NetworkPolicy changed)")
	}
}

// --- NormalizeClientProfile: nil and built-in cases ---

func TestNormalizeClientProfileNilReturnsDefault(t *testing.T) {
	got := NormalizeClientProfile(nil)
	expected := DefaultProfile()
	if !profilesEqual(got, expected) {
		t.Fatalf("NormalizeClientProfile(nil) = %+v, want %+v", got, expected)
	}
}

func TestNormalizeClientProfileBuiltInReturnsExact(t *testing.T) {
	cases := []ExecutionProfile{
		WorkspaceWrite(),
		WorkspaceReadOnly(),
		OpenWorldRestricted(),
	}
	for _, requested := range cases {
		t.Run(requested.Name, func(t *testing.T) {
			input := requested // copy so we can take the address without aliasing the loop var
			got := NormalizeClientProfile(&input)
			if !profilesEqual(got, requested) {
				t.Fatalf("NormalizeClientProfile(%s) = %+v, want %+v (built-in)", requested.Name, got, requested)
			}
			// The returned profile must be the built-in value, not the
			// caller's struct: mutating the caller's struct afterwards
			// must not affect the returned profile.
			input.SandboxPolicy = SandboxNone
			if !profilesEqual(got, requested) {
				t.Fatalf("returned profile aliases caller struct: mutating input changed the result to %+v", got)
			}
		})
	}
}

// --- NormalizeClientProfile: tampered fields fall back to built-in ---

func TestNormalizeClientProfileTamperedReturnsBuiltIn(t *testing.T) {
	// Tamper SandboxPolicy on a WorkspaceWrite request: the name is
	// allowed but the fields do not match the built-in. The result
	// must be the built-in WorkspaceWrite (tampered field overwritten).
	tampered := WorkspaceWrite()
	tampered.SandboxPolicy = SandboxNone
	got := NormalizeClientProfile(&tampered)
	expected := WorkspaceWrite()
	if !profilesEqual(got, expected) {
		t.Fatalf("NormalizeClientProfile(tampered workspace_write) = %+v, want %+v", got, expected)
	}
	if got.SandboxPolicy != SandboxBestEffort {
		t.Fatalf("expected SandboxPolicy overwritten to %q, got %q", SandboxBestEffort, got.SandboxPolicy)
	}

	// Tamper SandboxPermissions: the 6 key fields still match, so the
	// request is considered "allowed", but the returned profile must
	// still be the built-in (SandboxPermissions reset).
	tamperedPerms := WorkspaceWrite()
	tamperedPerms.SandboxPermissions = []string{"read", "write", "execute", "dangerous"}
	got = NormalizeClientProfile(&tamperedPerms)
	if !profilesEqual(got, WorkspaceWrite()) {
		t.Fatalf("NormalizeClientProfile(tampered SandboxPermissions) = %+v, want %+v", got, WorkspaceWrite())
	}
	if !equalStringSlices(got.SandboxPermissions, []string{"read", "write"}) {
		t.Fatalf("expected SandboxPermissions reset to [read write], got %v", got.SandboxPermissions)
	}
}

// --- NormalizeClientProfile: server-only and unknown names ---

func TestNormalizeClientProfileGuardianReadOnlyFallsBackToDefault(t *testing.T) {
	requested := GuardianReadOnly()
	got := NormalizeClientProfile(&requested)
	expected := DefaultProfile()
	if !profilesEqual(got, expected) {
		t.Fatalf("NormalizeClientProfile(guardian_read_only) = %+v, want %+v", got, expected)
	}
}

func TestNormalizeClientProfileUnknownNameFallsBackToDefault(t *testing.T) {
	requested := ExecutionProfile{Name: "totally_made_up"}
	got := NormalizeClientProfile(&requested)
	expected := DefaultProfile()
	if !profilesEqual(got, expected) {
		t.Fatalf("NormalizeClientProfile(unknown) = %+v, want %+v", got, expected)
	}
}

// --- NormalizeClientProfile: danger_full_access ---

func TestNormalizeClientProfileDangerFullAccessFallsBackWhenDisabled(t *testing.T) {
	// AWAKEN_ALLOW_DANGER_PROFILE is not set in the test environment by
	// default; explicitly clear it so this test does not depend on the
	// ambient shell.
	t.Setenv(dangerEnvVar, "")
	requested := DangerFullAccess()
	got := NormalizeClientProfile(&requested)
	expected := DefaultProfile()
	if !profilesEqual(got, expected) {
		t.Fatalf("NormalizeClientProfile(danger_full_access, disabled) = %+v, want %+v", got, expected)
	}
}

func TestNormalizeClientProfileDangerFullAccessReturnsBuiltInWhenEnabled(t *testing.T) {
	t.Setenv(dangerEnvVar, "true")
	requested := DangerFullAccess()
	got := NormalizeClientProfile(&requested)
	expected := DangerFullAccess()
	if !profilesEqual(got, expected) {
		t.Fatalf("NormalizeClientProfile(danger_full_access, enabled) = %+v, want %+v", got, expected)
	}

	// Even when danger is enabled, a tampered danger_full_access request
	// must still be normalized back to the built-in fields.
	tampered := DangerFullAccess()
	tampered.SandboxPolicy = SandboxRequired // would normally be SandboxNone
	got = NormalizeClientProfile(&tampered)
	if !profilesEqual(got, DangerFullAccess()) {
		t.Fatalf("NormalizeClientProfile(tampered danger_full_access) = %+v, want %+v", got, DangerFullAccess())
	}
	if got.SandboxPolicy != SandboxNone {
		t.Fatalf("expected SandboxPolicy reset to %q, got %q", SandboxNone, got.SandboxPolicy)
	}
}

// --- IsDangerFullAccessEnabled ---

func TestIsDangerFullAccessEnabled(t *testing.T) {
	t.Run("unset", func(t *testing.T) {
		t.Setenv(dangerEnvVar, "")
		if IsDangerFullAccessEnabled() {
			t.Fatalf("IsDangerFullAccessEnabled() = true, want false (env unset)")
		}
	})
	t.Run("true", func(t *testing.T) {
		t.Setenv(dangerEnvVar, "true")
		if !IsDangerFullAccessEnabled() {
			t.Fatalf("IsDangerFullAccessEnabled() = false, want true (env=true)")
		}
	})
	t.Run("false", func(t *testing.T) {
		t.Setenv(dangerEnvVar, "false")
		if IsDangerFullAccessEnabled() {
			t.Fatalf("IsDangerFullAccessEnabled() = true, want false (env=false)")
		}
	})
	t.Run("TRUE_uppercase", func(t *testing.T) {
		t.Setenv(dangerEnvVar, "TRUE")
		if IsDangerFullAccessEnabled() {
			t.Fatalf("IsDangerFullAccessEnabled() = true, want false (env=TRUE should be case-sensitive)")
		}
	})
	t.Run("garbage", func(t *testing.T) {
		t.Setenv(dangerEnvVar, "yes")
		if IsDangerFullAccessEnabled() {
			t.Fatalf("IsDangerFullAccessEnabled() = true, want false (env=yes)")
		}
	})
}
