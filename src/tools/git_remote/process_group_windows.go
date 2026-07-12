//go:build windows

package gitremote

import "os/exec"

// Windows production execution fails closed before process start until a real
// AppContainer/Job Object sandbox exists. These are compile-safe no-ops.
func configureProcessTree(*exec.Cmd) {}
func terminateProcessTree(*exec.Cmd) {}
