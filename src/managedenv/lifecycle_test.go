package managedenv

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestManagerProcessHelper(t *testing.T) {
	config := os.Getenv("AWAKEN_ENVIRONMENT_FIXTURE_CONFIG")
	if config == "" {
		t.Skip("subprocess helper")
	}
	var options Options
	if err := readJSON(config, &options); err != nil {
		t.Fatal(err)
	}
	m := managerForTest(t, options)
	if _, err := m.Ensure(context.Background(), Request{Capability: "fixture"}, testOwner("process"), nil); err != nil {
		t.Fatal(err)
	}
}

func TestServiceProcessLeaseSurvivesCoreRestart(t *testing.T) {
	options, _ := fixture(t)
	m := managerForTest(t, options)
	b, err := m.Ensure(context.Background(), Request{Capability: "fixture"}, testOwner("service"), nil)
	if err != nil {
		t.Fatal(err)
	}
	release, err := m.HoldProcess(b, os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	m.ReleaseRun(b.Owner.RunID)
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	next := managerForTest(t, options)
	if !next.hasLeases(b.InstanceID) {
		t.Fatal("live service lost protection when its Core closed")
	}
	release()
	if next.hasLeases(b.InstanceID) {
		t.Fatal("closed service retained a lease")
	}
}

func TestMaintainPrunesConfirmedDeadLeasesAndPreservesUnknownOwners(t *testing.T) {
	options, _ := fixture(t)
	m := managerForTest(t, options)
	b, err := m.Ensure(context.Background(), Request{Capability: "fixture"}, testOwner("old-core"), nil)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(m.root, "leases", b.InstanceID)
	files, err := os.ReadDir(dir)
	if err != nil || len(files) != 1 {
		t.Fatalf("expected the run lease: %v %v", files, err)
	}
	old := filepath.Join(dir, files[0].Name())
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	next := managerForTest(t, options)
	unknown := filepath.Join(dir, "unknown.json")
	if err := os.WriteFile(unknown, []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := next.Maintain(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatal("dead Core lease retained")
	}
	if _, err := os.Stat(unknown); err != nil {
		t.Fatal("unknown record removed")
	}
	if !next.hasLeases(b.InstanceID) {
		t.Fatal("unknown lease must conservatively protect the instance")
	}
}

func TestSeparateCoreProcessesSharePreparation(t *testing.T) {
	options, count := fixture(t)
	options.Client = nil
	data, err := json.Marshal(options)
	if err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(t.TempDir(), "fixture.json")
	if err = os.WriteFile(config, data, 0600); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	commands := []*exec.Cmd{}
	for range 2 {
		cmd := exec.Command(executable, "-test.run=^TestManagerProcessHelper$")
		cmd.Env = append(os.Environ(), "AWAKEN_ENVIRONMENT_FIXTURE_CONFIG="+config)
		commands = append(commands, cmd)
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
	}
	for _, cmd := range commands {
		if err := cmd.Wait(); err != nil {
			t.Fatal(err)
		}
	}
	if count.Load() != 1 {
		t.Fatalf("independent Core processes downloaded %d copies", count.Load())
	}
}

func TestDamagedComponentRepairUsesCacheAndLeavesOldInstance(t *testing.T) {
	options, count := fixture(t)
	m := managerForTest(t, options)
	a, err := m.Ensure(context.Background(), Request{Capability: "fixture"}, testOwner("A"), nil)
	if err != nil {
		t.Fatal(err)
	}
	oldPath := a.Executables["fixture"].Executable
	if err = os.WriteFile(oldPath, []byte("damaged"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err = m.Binding(context.Background(), a.EnvironmentRef, testOwner("A")); err == nil {
		t.Fatal("damaged binding accepted")
	}
	b, err := m.Ensure(context.Background(), Request{Capability: "fixture"}, testOwner("B"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if b.InstanceID == a.InstanceID || b.Executables["fixture"].Executable == oldPath || count.Load() != 1 {
		t.Fatal("repair mutated old instance or downloaded healthy cache again")
	}
	if raw, err := os.ReadFile(oldPath); err != nil || string(raw) != "damaged" {
		t.Fatal("repair overwrote old instance")
	}
}

func TestCollectionRetainsLeasedInstanceAndRollback(t *testing.T) {
	options, _ := fixture(t)
	now := time.Now()
	options.Now = func() time.Time { return now }
	m := managerForTest(t, options)
	b, err := m.Ensure(context.Background(), Request{Capability: "fixture"}, testOwner("A"), nil)
	if err != nil {
		t.Fatal(err)
	}
	release, err := m.HoldProcess(b, 0)
	if err != nil {
		t.Fatal(err)
	}
	m.ReleaseRun(testOwner("A").RunID)
	for range 2 {
		now = now.Add(time.Hour)
		if err = m.locked(context.Background(), "fixture-copy", func() error {
			all, e := m.instances()
			if e != nil {
				return e
			}
			for _, f := range all {
				if f.Kind == "environment" {
					copy := f
					copy.InstanceID = newID()
					copy.Root = filepath.Join(filepath.Dir(f.Root), copy.InstanceID)
					copy.CreatedAt = now
					return m.save(copy)
				}
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = m.Collect(context.Background()); err != nil {
		t.Fatal(err)
	}
	now = now.Add(8 * 24 * time.Hour)
	removed, err := m.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if contains(removed, b.InstanceID) {
		t.Fatal("active process environment collected")
	}
	release()
	if _, err = m.Collect(context.Background()); err != nil {
		t.Fatal(err)
	}
	now = now.Add(8 * 24 * time.Hour)
	removed, err = m.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !contains(removed, b.InstanceID) {
		t.Fatal("retired unreferenced old instance was never collected")
	}
}

func TestPackageCommandGrammar(t *testing.T) {
	for _, cmd := range []struct {
		command string
		args    []string
		name    string
	}{{"npx", []string{"-y", "@example/server@1.2.3", "--stdio"}, "@example/server"}, {"cmd", []string{"/c", "npx", "package@latest"}, "package"}, {"uvx", []string{"server==1.2.3"}, "server"}} {
		p, ok := RecognizePackageCommand(cmd.command, cmd.args)
		if !ok || p.Package != cmd.name {
			t.Fatalf("did not recognize %+v", cmd)
		}
	}
	for _, command := range []string{"/custom/npx", "sh", "node", "npm"} {
		if _, ok := RecognizePackageCommand(command, []string{"-c", "echo unsafe"}); ok {
			t.Fatal("rewrote custom command")
		}
	}
	if _, ok := RecognizePackageCommand("npx", []string{"--registry=https://other.invalid", "pkg"}); ok {
		t.Fatal("rewrote custom registry")
	}
}

func TestConflictingRequirementsRejected(t *testing.T) {
	c, _ := BuiltinCatalog()
	_, err := c.Resolve(Request{Requirements: []Requirement{{Ecosystem: "python", Name: "python_docx", Version: "1.0"}, {Ecosystem: "python", Name: "python-docx", Version: "2.0"}}})
	if err == nil || !strings.Contains(err.Error(), "conflicting") {
		t.Fatal("duplicate version requirements silently overwritten")
	}
	if satisfies("0.0.4", "^0.0.3") || satisfies("1.3.0", "~1.2") {
		t.Fatal("incompatible semver accepted")
	}
}

func TestCoreExitHandsPreparationToOtherWaiterAndFencesOldAttempt(t *testing.T) {
	options, count := fixture(t)
	firstOptions := options
	entered := make(chan struct{})
	firstOptions.Client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		close(entered)
		<-r.Context().Done()
		return nil, r.Context().Err()
	})}
	first := managerForTest(t, firstOptions)
	second := managerForTest(t, options)
	a := make(chan error, 1)
	b := make(chan error, 1)
	go func() {
		_, err := first.Ensure(context.Background(), Request{Capability: "fixture"}, testOwner("A"), nil)
		a <- err
	}()
	<-entered
	operations := first.Operations()
	if len(operations) != 1 {
		t.Fatal("preparation not recorded")
	}
	old := operations[0]
	old.started = true
	go func() {
		_, err := second.Ensure(context.Background(), Request{Capability: "fixture"}, testOwner("B"), nil)
		b <- err
	}()
	deadline := time.Now().Add(3 * time.Second)
	for {
		paths, _ := filepath.Glob(filepath.Join(first.root, "waiters", "*", "*.json"))
		if len(paths) == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("second Core never joined")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-a; err == nil {
		t.Fatal("exiting Core completed its cancelled task")
	}
	select {
	case err := <-b:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("surviving Core failed to take over")
	}
	if count.Load() != 1 {
		t.Fatalf("takeover downloaded %d copies", count.Load())
	}
	old.State = "ready"
	if err := first.record(&old); err == nil {
		t.Fatal("old execution generation committed after takeover")
	}
	current, err := second.Operation(old.ID)
	if err != nil || current.Attempt == old.Attempt || current.State != "ready" {
		t.Fatalf("late operation damaged result: %+v %v", current, err)
	}
}
