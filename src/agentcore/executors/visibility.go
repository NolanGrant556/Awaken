package executors

import (
	"awaken/src/tool"
	"awaken/src/tool/profile"
)

// VisibilityFilterObserver wraps an observer according to a VisibilityPolicy.
// It is the single point where the executor layer enforces visibility so
// routeToExecutor does not have to branch on policy at every observer
// construction site.
//
//   - VisibilityHidden: returns a wrapper whose OnCommandOutput is a no-op,
//     so no stdout/stderr deltas reach the event stream. The tool still
//     executes; only its streamed I/O is suppressed.
//   - VisibilitySummary: returns a wrapper whose OnCommandOutput is a no-op
//     (real-time deltas are suppressed), but the wrapper records whether any
//     output was observed. routeToExecutor reads that flag (via
//     SummaryHadOutput) after the tool completes and emits a single
//     truncated summary event in its place.
//   - VisibilityFull: returns the original observer unchanged so the
//     existing streaming behavior is preserved.
//
// A nil observer is returned unchanged so the executor layer can pass it
// through to tools that do not implement ObservableTool.
func VisibilityFilterObserver(observer tool.ToolObserver, policy profile.VisibilityPolicy) tool.ToolObserver {
	if observer == nil {
		return nil
	}
	switch policy {
	case profile.VisibilityHidden:
		return &hiddenObserver{inner: observer}
	case profile.VisibilitySummary:
		return &summarySuppressingObserver{inner: observer}
	default:
		return observer
	}
}

// hiddenObserver drops all OnCommandOutput calls. It is used by
// VisibilityHidden so the event stream never sees stdout/stderr deltas for
// the tool call. The inner observer is retained for future extensions
// (e.g. a final "tool hidden" marker) but is not invoked today.
type hiddenObserver struct {
	inner tool.ToolObserver
}

func (h *hiddenObserver) OnCommandOutput(output tool.CommandOutput) {
	// no-op: hidden policy drops all deltas.
}

// summarySuppressingObserver drops real-time OnCommandOutput calls (so the
// event stream does not show streaming deltas) but records whether any
// output was observed. routeToExecutor reads HadOutput after the tool
// completes and, when true, emits a single truncated summary event.
type summarySuppressingObserver struct {
	inner     tool.ToolObserver
	hadOutput bool
}

func (s *summarySuppressingObserver) OnCommandOutput(output tool.CommandOutput) {
	if output.Delta != "" {
		s.hadOutput = true
	}
	// Real-time deltas are suppressed; routeToExecutor emits a single
	// truncated summary after the tool completes.
}

// HadOutput reports whether the tool produced any observable output
// (non-empty delta) that was suppressed by this wrapper.
func (s *summarySuppressingObserver) HadOutput() bool {
	return s.hadOutput
}

// SummaryHadOutput reports whether the given observer is a summary-
// suppressing observer that recorded any non-empty delta. It returns false
// for any other observer type (including nil and non-summary wrappers).
// routeToExecutor uses this to decide whether to emit a truncated summary
// event after the tool completes.
func SummaryHadOutput(observer tool.ToolObserver) bool {
	s, ok := observer.(*summarySuppressingObserver)
	return ok && s.HadOutput()
}
