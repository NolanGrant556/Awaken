package sandbox

import (
	"os/exec"
	"testing"
)

func TestDefaultAdapterReturnsNonNil(t *testing.T) {
	if DefaultAdapter() == nil {
		t.Fatal("DefaultAdapter returned nil")
	}
}

func TestDefaultAdapterReportsConcreteSupportLevel(t *testing.T) {
	// The platform default must report a concrete support level, not the
	// empty string. This is a sanity check that the build-tag-selected
	// file actually returns a value.
	support := DefaultAdapter().SupportLevel()
	switch support {
	case SandboxFull, SandboxDegraded, SandboxNone:
		// ok
	default:
		t.Fatalf("unexpected support level %q", support)
	}
}

func TestSetAdapterOverride(t *testing.T) {
	t.Cleanup(func() { SetAdapter(nil) })

	mock := &recordingAdapter{support: SandboxFull}
	SetAdapter(mock)

	if GetAdapter() != mock {
		t.Fatal("GetAdapter did not return the override")
	}

	if support := DefaultAdapter().SupportLevel(); support != SandboxFull {
		t.Fatalf("expected mock support level %q, got %q", SandboxFull, support)
	}
}

func TestSetAdapterNilRestoresDefault(t *testing.T) {
	t.Cleanup(func() { SetAdapter(nil) })

	mock := &recordingAdapter{support: SandboxFull}
	SetAdapter(mock)
	SetAdapter(nil)

	if GetAdapter() == mock {
		t.Fatal("expected override to be cleared after SetAdapter(nil)")
	}
}

// recordingAdapter is a test double that lets tests inject a support
// level and observe ApplySandbox calls.
type recordingAdapter struct {
	support SandboxSupport
	applied int
}

func (r *recordingAdapter) SupportLevel() SandboxSupport {
	return r.support
}

func (r *recordingAdapter) ApplySandbox(cmd *exec.Cmd, tier string, workspaceRoot string) (SandboxResult, error) {
	r.applied++
	return SandboxResult{Support: r.support, Method: "mock", Applied: r.support == SandboxFull}, nil
}
