// This file deliberately has NO build tag. It contains the pure,
// side-effect-free string-generation logic for the Windows sandbox
// adapter so that it can be unit-tested on any platform (macOS, Linux,
// Windows). The Windows-only SandboxAdapterWindows type in
// sandbox_windows.go calls into GeneratePowerShellWrapper from
// ApplySandbox; keeping the logic here lets `go test ./...` on macOS
// exercise the wrapper generation without cross-compiling.

package sandbox

import (
	"errors"
	"fmt"
	"strings"
)

// ErrPowerShellWrapperUnsupported is returned by GeneratePowerShellWrapper
// when the requested tier requires SandboxFull support, which the Windows
// degraded adapter does not provide. open_world_system commands must be
// refused until a real AppContainer/Job Object implementation lands.
var ErrPowerShellWrapperUnsupported = errors.New(
	"powershell wrapper for this tier requires SandboxFull support, " +
		"which is not available on the degraded Windows adapter",
)

// GeneratePowerShellWrapper produces a PowerShell command string that would
// run the given shell command with tier-appropriate restrictions inside
// PowerShell. This is a PURE string-generation function — it does not execute
// anything — and is testable on any platform.
//
// The wrapper is the documented escalation path for the Windows adapter: a
// future AppContainer/Job Object implementation would invoke this wrapper (or
// a richer variant) inside a restricted process context. Until that lands,
// the adapter reports SandboxDegraded and the ShellExecutor fail-closes for
// open_world_system commands.
//
// tier is one of the Tier* constants in this package:
//
//   - TierInspectReadOnly: the command runs inside PowerShell with
//     $ErrorActionPreference set to 'Stop' and the working location pinned
//     to workspaceRoot. Write-oriented cmdlets that respect -WhatIf are NOT
//     automatically intercepted here; the file-chain boundary and the
//     sensitive-path checks in the permission layer provide the real
//     protection for this tier. This wrapper documents the intent and gives
//     the future sandbox a single place to inject -WhatIf / ConstrainedLanguage
//     mode restrictions.
//   - TierWorkspaceMutation: the command runs inside PowerShell with the
//     working location pinned to workspaceRoot. File writes are confined to
//     the workspace by the permission layer's writable-roots checks.
//   - TierOpenWorldSystem: returns ErrPowerShellWrapperUnsupported. A real
//     AppContainer/Job Object profile is required for this tier, which is
//     out of scope for the degraded adapter.
//
// command and workspaceRoot are trimmed; empty values are rejected.
// Single quotes inside workspaceRoot are escaped by doubling (' -> '') so
// the path can be safely embedded in a PowerShell single-quoted literal.
func GeneratePowerShellWrapper(command, tier, workspaceRoot string) (string, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return "", fmt.Errorf("command is empty")
	}
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	if workspaceRoot == "" {
		return "", fmt.Errorf("workspaceRoot is empty")
	}

	escapedRoot := powerShellSingleQuote(workspaceRoot)

	switch tier {
	case TierInspectReadOnly:
		// Pin location and fail fast on errors. A real sandbox would also
		// intercept Set-Content/Remove-Item/etc. via -WhatIf or a
		// ConstrainedLanguage mode; the permission layer's path checks
		// remain the primary boundary for this tier.
		return fmt.Sprintf(
			"$ErrorActionPreference = 'Stop'; Set-Location -LiteralPath '%s'; %s",
			escapedRoot,
			command,
		), nil

	case TierWorkspaceMutation:
		// Pin location to the workspace. Writes are still gated by the
		// permission layer's writable-roots checks.
		return fmt.Sprintf(
			"Set-Location -LiteralPath '%s'; %s",
			escapedRoot,
			command,
		), nil

	case TierOpenWorldSystem:
		return "", ErrPowerShellWrapperUnsupported

	default:
		return "", fmt.Errorf("unknown sandbox tier %q", tier)
	}
}

// powerShellSingleQuote escapes s for safe embedding inside a PowerShell
// single-quoted string literal. Single quotes are doubled (' -> '').
func powerShellSingleQuote(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}
