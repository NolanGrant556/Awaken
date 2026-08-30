package runtime

import (
	"sync/atomic"
	"testing"
)

func TestSessionDefersPluginLeaseReleaseUntilAsyncHooksFinish(t *testing.T) {
	session := NewSession("run", "thread", "input", nil, nil, nil)
	var releases atomic.Int32
	session.SetPluginRuntimeLease("generation", func() { releases.Add(1) })
	releaseFirst := session.RetainAsyncHook()
	releaseSecond := session.RetainAsyncHook()
	session.ReleasePluginRuntimeLease()
	if releases.Load() != 0 {
		t.Fatalf("lease released while async Hooks were active: %d", releases.Load())
	}
	releaseFirst()
	releaseFirst()
	if releases.Load() != 0 {
		t.Fatalf("lease released before all async Hooks finished: %d", releases.Load())
	}
	releaseSecond()
	if releases.Load() != 1 {
		t.Fatalf("lease releases=%d want=1", releases.Load())
	}
	session.ReleasePluginRuntimeLease()
	if releases.Load() != 1 {
		t.Fatalf("lease released more than once: %d", releases.Load())
	}
}
