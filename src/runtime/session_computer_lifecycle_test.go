package runtime

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"awaken/src/computer"
	computerfake "awaken/src/computer/fake"
)

func computerForOwner(t *testing.T, leases *computer.LeaseManager, run, agent string) *computer.Runtime {
	t.Helper()
	policy, err := computer.NewPolicySnapshot(computer.DefaultPolicy())
	if err != nil {
		t.Fatal(err)
	}
	binding, err := computer.NewBinding(run, "thread", agent, "host://local/desktop", policy)
	if err != nil {
		t.Fatal(err)
	}
	binding.WaitForResource = true
	cr := computer.NewRuntimeWithLeaseManager(binding, computerfake.NewDriver(), leases)
	t.Cleanup(func() { _ = cr.Close(context.Background(), "test_end") })
	return cr
}

func TestRootComputerCleanupDoesNotWaitForBackgroundRetention(t *testing.T) {
	leases := computer.NewLeaseManager()
	root := computerForOwner(t, leases, "run", "root")
	child := computerForOwner(t, leases, "run", "child")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := root.OpenSession(ctx, computer.CaptureDesktop, computer.TargetRef{}); err != nil {
		t.Fatal(err)
	}
	session := NewSession("run", "thread", "", nil, nil, nil)
	session.SetComputerRuntime(root)
	var pluginReleases atomic.Int32
	session.SetPluginRuntimeLease("generation", func() { pluginReleases.Add(1) })
	releaseChild := session.RetainAsyncHook()
	defer releaseChild()
	acquired := make(chan error, 1)
	go func() {
		_, err := child.OpenSession(ctx, computer.CaptureDesktop, computer.TargetRef{})
		acquired <- err
	}()
	session.ReleasePluginRuntimeLease()
	session.CloseRootComputer()
	if err := <-acquired; err != nil {
		t.Fatalf("retained child could not acquire released desktop: %v", err)
	}
	if pluginReleases.Load() != 0 {
		t.Fatal("background Plugin resources released prematurely")
	}
	session.CloseRootComputer()
	releaseChild()
	releaseChild()
	if pluginReleases.Load() != 1 {
		t.Fatal("Plugin resources must release exactly once")
	}
	lease, held := leases.Current("host://local/desktop")
	if !held || lease.BindingID != child.Binding().BindingID {
		t.Fatal("root cleanup released child ownership")
	}
	if _, err := root.OpenSession(ctx, computer.CaptureDesktop, computer.TargetRef{}); err == nil {
		t.Fatal("finished root reopened Computer")
	}
}

func TestComputerSessionControlStaysWithOriginalRun(t *testing.T) {
	leases := computer.NewLeaseManager()
	child := computerForOwner(t, leases, "old", "child")
	session := NewSession("old", "thread", "", nil, nil, nil)
	session.childComputers = map[string]*computer.Runtime{string(child.Binding().BindingID): child}
	newer := NewSession("new", "thread", "", nil, nil, nil)
	newer.SetComputerRuntime(computerForOwner(t, leases, "new", "root"))
	service := &RunService{sessions: map[string]*Session{"old": session, "new": newer}, computerLeaseManager: leases}
	active, err := child.OpenSession(context.Background(), computer.CaptureDesktop, computer.TargetRef{})
	if err != nil {
		t.Fatal(err)
	}
	if owner, err := service.ComputerRuntimeForSession("old", string(active.SessionID)); err != nil || owner != child {
		t.Fatalf("original owner not resolved: %v", err)
	}
	if _, err := service.ComputerRuntimeForSession("new", string(active.SessionID)); err == nil {
		t.Fatal("session resolved through wrong run")
	}
	if _, err := service.ComputerRuntimeForSession("old", "expired-session"); err == nil {
		t.Fatal("stale session retargeted to current holder")
	}
	if err := child.Takeover(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := child.ReturnToSession(context.Background(), "expired-session"); err == nil {
		t.Fatal("stale handoff resumed active session")
	}
	if state, _ := child.Session(); state.State != computer.SessionTakenOver {
		t.Fatal("stale return changed current state")
	}
	if err := child.ReturnToSession(context.Background(), active.SessionID); err != nil {
		t.Fatal(err)
	}
}
