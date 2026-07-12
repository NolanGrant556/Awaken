//go:build windows

package shelltool

import "os/exec"

// Windows does not yet expose a Job Object boundary in this package. The
// production workspace broker therefore remains unavailable there and raw
// direct Shell execution currently has no Windows Job Object tree containment;
// these no-ops preserve compilation without claiming descendant supervision.
func configureShellProcessTree(command *exec.Cmd) {}
func terminateShellProcessTree(command *exec.Cmd) {}
