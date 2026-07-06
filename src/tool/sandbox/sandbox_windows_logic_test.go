// This file has NO build tag so the GeneratePowerShellWrapper logic can be
// tested on any platform (macOS, Linux, Windows). The wrapper generation is
// pure string construction — it never touches the Windows API.

package sandbox

import (
	"errors"
	"strings"
	"testing"
)

func TestGeneratePowerShellWrapper_InspectReadOnly(t *testing.T) {
	wrapper, err := GeneratePowerShellWrapper("Get-ChildItem", TierInspectReadOnly, `C:\workspace`)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.Contains(wrapper, "Set-Location -LiteralPath") {
		t.Errorf("expected wrapper to pin location, got %q", wrapper)
	}
	if !strings.Contains(wrapper, `C:\workspace`) {
		t.Errorf("expected wrapper to contain workspace root, got %q", wrapper)
	}
	if !strings.Contains(wrapper, "Get-ChildItem") {
		t.Errorf("expected wrapper to contain command, got %q", wrapper)
	}
	if !strings.Contains(wrapper, "$ErrorActionPreference = 'Stop'") {
		t.Errorf("expected inspect wrapper to set ErrorActionPreference=Stop, got %q", wrapper)
	}
}

func TestGeneratePowerShellWrapper_WorkspaceMutation(t *testing.T) {
	wrapper, err := GeneratePowerShellWrapper("echo hello", TierWorkspaceMutation, `C:\workspace`)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.Contains(wrapper, "Set-Location -LiteralPath") {
		t.Errorf("expected wrapper to pin location, got %q", wrapper)
	}
	if !strings.Contains(wrapper, "echo hello") {
		t.Errorf("expected wrapper to contain command, got %q", wrapper)
	}
	// workspace_mutation must NOT force $ErrorActionPreference = 'Stop':
	// legitimate write commands (e.g. mkdir, Set-Content) should not be
	// converted into hard errors by the wrapper.
	if strings.Contains(wrapper, "$ErrorActionPreference = 'Stop'") {
		t.Errorf("workspace_mutation wrapper should not force Stop, got %q", wrapper)
	}
}

func TestGeneratePowerShellWrapper_OpenWorldSystemReturnsError(t *testing.T) {
	_, err := GeneratePowerShellWrapper("curl https://example.com", TierOpenWorldSystem, `C:\workspace`)
	if err == nil {
		t.Fatal("expected error for open_world_system tier, got nil")
	}
	if !errors.Is(err, ErrPowerShellWrapperUnsupported) {
		t.Fatalf("expected ErrPowerShellWrapperUnsupported, got %v", err)
	}
}

func TestGeneratePowerShellWrapper_EmptyCommandReturnsError(t *testing.T) {
	if _, err := GeneratePowerShellWrapper("", TierInspectReadOnly, `C:\workspace`); err == nil {
		t.Fatal("expected error for empty command, got nil")
	}
	// Whitespace-only command is also rejected after trimming.
	if _, err := GeneratePowerShellWrapper("   ", TierInspectReadOnly, `C:\workspace`); err == nil {
		t.Fatal("expected error for whitespace-only command, got nil")
	}
}

func TestGeneratePowerShellWrapper_EmptyWorkspaceRootReturnsError(t *testing.T) {
	if _, err := GeneratePowerShellWrapper("ls", TierInspectReadOnly, ""); err == nil {
		t.Fatal("expected error for empty workspace root, got nil")
	}
	if _, err := GeneratePowerShellWrapper("ls", TierInspectReadOnly, "   "); err == nil {
		t.Fatal("expected error for whitespace-only workspace root, got nil")
	}
}

func TestGeneratePowerShellWrapper_UnknownTierReturnsError(t *testing.T) {
	_, err := GeneratePowerShellWrapper("ls", "bogus_tier", `C:\workspace`)
	if err == nil {
		t.Fatal("expected error for unknown tier, got nil")
	}
	if errors.Is(err, ErrPowerShellWrapperUnsupported) {
		t.Fatalf("unknown tier should return a generic error, not ErrPowerShellWrapperUnsupported: %v", err)
	}
}

func TestGeneratePowerShellWrapper_TrimsInputs(t *testing.T) {
	wrapper, err := GeneratePowerShellWrapper("  ls  ", TierWorkspaceMutation, `  C:\ws  `)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	// The trimmed path should appear; surrounding spaces should not leak
	// into the Set-Location argument.
	if !strings.Contains(wrapper, `Set-Location -LiteralPath 'C:\ws'`) {
		t.Errorf("expected trimmed workspace root in wrapper, got %q", wrapper)
	}
}

func TestGeneratePowerShellWrapper_EscapesSingleQuotesInPath(t *testing.T) {
	// A workspace root containing a single quote must be escaped by doubling
	// (' -> '') so the path is safe inside a PowerShell single-quoted literal.
	wrapper, err := GeneratePowerShellWrapper("ls", TierInspectReadOnly, `C:\o'brien`)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.Contains(wrapper, `C:\o''brien`) {
		t.Errorf("expected single quote to be doubled, got %q", wrapper)
	}
	// The raw unescaped path must NOT appear, otherwise the wrapper is
	// injectable via the path argument.
	if strings.Contains(wrapper, `Set-Location -LiteralPath 'C:\o'brien'`) {
		t.Errorf("unescaped single quote leaked into wrapper: %q", wrapper)
	}
}

func TestGeneratePowerShellWrapper_InspectReadOnlyFormatStable(t *testing.T) {
	// Pin the exact format so a future refactor that breaks the contract
	// (e.g. drops the ErrorActionPreference or the Set-Location) is caught.
	wrapper, err := GeneratePowerShellWrapper("ls", TierInspectReadOnly, `C:\ws`)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	expected := "$ErrorActionPreference = 'Stop'; Set-Location -LiteralPath 'C:\\ws'; ls"
	if wrapper != expected {
		t.Fatalf("inspect wrapper format mismatch:\nwant: %q\ngot:  %q", expected, wrapper)
	}
}

func TestGeneratePowerShellWrapper_WorkspaceMutationFormatStable(t *testing.T) {
	wrapper, err := GeneratePowerShellWrapper("touch f.txt", TierWorkspaceMutation, `C:\ws`)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	expected := "Set-Location -LiteralPath 'C:\\ws'; touch f.txt"
	if wrapper != expected {
		t.Fatalf("workspace_mutation wrapper format mismatch:\nwant: %q\ngot:  %q", expected, wrapper)
	}
}
