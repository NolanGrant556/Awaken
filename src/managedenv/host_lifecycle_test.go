package managedenv

import (
	"context"
	"os"
	"testing"
)

func TestHostUsageSurvivesCoreRestartAndSerializesRemoval(t *testing.T) {
	o, downloads := fixture(t)
	component := o.Catalog.Components["fixture"]
	o.Catalog.Components["lark-cli"] = component
	o.Catalog.Capabilities["lark-cli"] = Spec{Components: []string{"lark-cli"}}
	m := managerForTest(t, o)
	ctx := context.Background()
	owner := testOwner("host-fixture")
	b, err := m.Ensure(ctx, Request{Capability: "lark-cli"}, owner, nil)
	if err != nil {
		t.Fatal(err)
	}
	m.ReleaseRun(owner.RunID)
	releasePreparation, err := m.holdComponentPreparation("lark-cli")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = m.ReserveRemoval(ctx, "lark-cli", os.Getpid()); err == nil {
		t.Fatal("uninstall passed an active component preparation")
	}
	releasePreparation()
	ref, err := m.HoldHostExecutable(b.Executables["fixture"].Executable, os.Getpid(), "")
	if err != nil || ref == "" {
		t.Fatalf("host usage not acquired: %s %v", ref, err)
	}
	if err = m.Close(); err != nil {
		t.Fatal(err)
	}
	m = managerForTest(t, o)
	if _, err = m.ReserveRemoval(ctx, "lark-cli", os.Getpid()); err == nil {
		t.Fatal("Core restart lost a live Main usage")
	}
	if err = m.ReleaseHostExecutable(ref); err != nil {
		t.Fatal(err)
	}
	token, err := m.ReserveRemoval(ctx, "lark-cli", os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = m.ReserveRemoval(ctx, "lark-cli", os.Getpid()); err == nil {
		t.Fatal("second uninstall owner accepted")
	}
	if _, err = m.Binding(ctx, b.EnvironmentRef, owner); err == nil {
		t.Fatal("task acquired a component during uninstall")
	}
	if _, err = m.HoldHostExecutable(b.Executables["fixture"].Executable, os.Getpid(), ""); err == nil {
		t.Fatal("foreign host acquired component during uninstall")
	}
	ref, err = m.HoldHostExecutable(b.Executables["fixture"].Executable, os.Getpid(), token)
	if err != nil {
		t.Fatal("uninstall owner cannot run logout", err)
	}
	if err = m.RemoveComponent(ctx, "lark-cli", token); err == nil {
		t.Fatal("removed a running logout component")
	}
	if err = m.ReleaseHostExecutable(ref); err != nil {
		t.Fatal(err)
	}
	if err = m.RemoveComponent(ctx, "lark-cli"); err == nil {
		t.Fatal("removed without reservation identity")
	}
	if err = m.RemoveComponent(ctx, "lark-cli", token); err != nil {
		t.Fatal(err)
	}
	if err = m.ReleaseRemoval("lark-cli", token); err != nil {
		t.Fatal(err)
	}
	if downloads.Load() != 1 {
		t.Fatal("lifecycle checks reinstalled component")
	}
}
