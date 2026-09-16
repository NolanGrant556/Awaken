package managedenv

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDurableBindingRecoveryDoesNotInstall(t *testing.T) {
	options, count := fixture(t)
	m := managerForTest(t, options)
	ctx := context.Background()
	owner := testOwner("A")
	b, err := m.Ensure(ctx, Request{Capability: "fixture"}, owner, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.RemoveAll(filepath.Join(m.root, "bindings", owner.key())); err != nil {
		t.Fatal(err)
	}
	recovered, err := m.RestoreBinding(ctx, b.EnvironmentRef, b.InstanceID, owner)
	if err != nil || recovered.InstanceID != b.InstanceID || !recovered.Reused || count.Load() != 1 {
		t.Fatalf("failed recovery: %+v %v", recovered, err)
	}
	if _, err = m.RestoreBinding(ctx, b.EnvironmentRef, b.InstanceID, testOwner("foreign")); err == nil {
		t.Fatal("foreign durable binding accepted")
	}
	if err = os.Remove(b.Executables["fixture"].Executable); err != nil {
		t.Fatal(err)
	}
	if _, err = m.RestoreBinding(ctx, b.EnvironmentRef, b.InstanceID, owner); err == nil {
		t.Fatal("damaged environment restored")
	}
	if count.Load() != 1 {
		t.Fatal("recovery installed automatically")
	}
}

func TestChildReleaseDoesNotDropParentLease(t *testing.T) {
	options, _ := fixture(t)
	m := managerForTest(t, options)
	ctx := context.Background()
	parent := testOwner("A")
	b, err := m.Ensure(ctx, Request{Capability: "fixture"}, parent, nil)
	if err != nil {
		t.Fatal(err)
	}
	child := parent
	child.AgentID = "child"
	if err = m.Inherit(parent, child); err != nil {
		t.Fatal(err)
	}
	m.ReleaseOwner(child)
	if !m.hasLeases(b.InstanceID) {
		t.Fatal("child released parent's shared run lease")
	}
	m.ReleaseOwner(parent)
	if m.hasLeases(b.InstanceID) {
		t.Fatal("completed owners leaked their leases")
	}
}

func TestForgetThreadRemovesOnlyItsBindingsAndPreservesProcessLease(t *testing.T) {
	options, _ := fixture(t)
	m := managerForTest(t, options)
	owner := testOwner("deleted")
	binding, err := m.Ensure(context.Background(), Request{Capability: "fixture"}, owner, nil)
	if err != nil {
		t.Fatal(err)
	}
	child := owner
	child.AgentID = "child"
	if err := m.Inherit(owner, child); err != nil {
		t.Fatal(err)
	}
	other := testOwner("survivor")
	if _, err := m.Ensure(context.Background(), Request{Capability: "fixture"}, other, nil); err != nil {
		t.Fatal(err)
	}
	release, err := m.HoldProcess(binding, os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	m.ReleaseOwner(owner)
	m.ReleaseOwner(child)
	m.ReleaseOwner(other)
	for i := 0; i < 2; i++ {
		if err := m.ForgetThread(owner.ThreadID); err != nil {
			t.Fatal(err)
		}
	}
	for _, o := range []Owner{owner, child} {
		if _, err := os.Stat(filepath.Join(m.root, "bindings", o.key())); !os.IsNotExist(err) {
			t.Fatalf("deleted owner remains: %v", err)
		}
	}
	if _, err := os.Stat(filepath.Join(m.root, "bindings", other.key())); err != nil {
		t.Fatal(err)
	}
	if !m.hasLeases(binding.InstanceID) {
		t.Fatal("live process lost protection")
	}
	if _, err := os.Stat(binding.Executables["fixture"].Executable); err != nil {
		t.Fatal(err)
	}
}
