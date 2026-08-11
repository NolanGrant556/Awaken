package computer

import "context"

// HostStatus summarizes the global availability of the Computer Host sidecar.
// It is intentionally independent of any run or session so that settings and
// readiness checks can report status before a run starts.
type HostStatus struct {
	// Available is true when the Host sidecar is installed and healthy.
	Available bool `json:"available"`
	// Healthy is true when the most recent health probe succeeded.
	Healthy bool `json:"healthy"`
	// Version is the Host/Driver version when available.
	Version string `json:"version,omitempty"`
	// ProtocolVersion is the wire protocol version when available.
	ProtocolVersion int `json:"protocol_version,omitempty"`
	// OS is the reported operating system.
	OS string `json:"os,omitempty"`
	// TCCScreenRecording reports whether the Host has Screen Recording consent.
	TCCScreenRecording bool `json:"tcc_screen_recording"`
	// TCCAccessibility reports whether the Host has Accessibility consent.
	TCCAccessibility bool `json:"tcc_accessibility"`
	// NodePath is the Node runtime binary used by the Host, when known.
	NodePath string `json:"node_path,omitempty"`
	// HostScriptPath is the Host entry script used, when known.
	HostScriptPath string `json:"host_script_path,omitempty"`
	// Message carries a human-readable status or error summary.
	Message string `json:"message,omitempty"`
}

// Ready reports whether the global Host can back a Computer-enabled run. This
// is the run-independent equivalent of Runtime.Ready and is used by projection
// paths that must not create a run-bound Computer session.
func (s HostStatus) Ready() bool {
	return s.Available && s.Healthy && s.TCCScreenRecording && s.TCCAccessibility
}

// HostProbe probes the global Computer Host sidecar without binding it to a
// run. Implementations must be safe to call concurrently and must return
// quickly (e.g. by reusing a shared Host client).
type HostProbe interface {
	Probe(ctx context.Context) (HostStatus, error)
}

// HostProbeFunc adapts a function to the HostProbe interface.
type HostProbeFunc func(ctx context.Context) (HostStatus, error)

// Probe implements HostProbe.
func (f HostProbeFunc) Probe(ctx context.Context) (HostStatus, error) {
	return f(ctx)
}
