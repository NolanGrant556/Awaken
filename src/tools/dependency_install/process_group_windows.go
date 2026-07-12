//go:build windows

package dependencyinstall

import "os/exec"

// Windows cannot start dependency installation until the platform adapter has
// a real AppContainer/Job Object boundary, so these remain unreachable guards.
func configureProcessTree(*exec.Cmd) {}
func terminateProcessTree(*exec.Cmd) {}
