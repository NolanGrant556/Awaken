//go:build !darwin

package gitremote

import (
	"fmt"
	"path/filepath"

	"awaken/src/tool"
)

// Non-Darwin platforms currently fail closed before open-network transport
// because their sandbox adapters are not full. This placeholder deliberately
// does not claim host toolchain attestation for a future full sandbox.
func validateHostTrustedGitRuntime(observation tool.ExecutableObservation) (gitRuntimeTrust, error) {
	if observation.Path == "" {
		return gitRuntimeTrust{}, fmt.Errorf("Git executable is missing")
	}
	return gitRuntimeTrust{}, fmt.Errorf("host-trusted Git toolchain attestation is not implemented on this platform: %s", filepath.Clean(observation.Path))
}
