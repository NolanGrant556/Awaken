package acp

import (
	"errors"
	"testing"

	"awaken/src/managedenv"
)

func TestManagedRunnerDoesNotRequireSystemPackageManager(t *testing.T) {
	catalog, err := managedenv.BuiltinCatalog()
	if err != nil {
		t.Fatal(err)
	}
	m, err := managedenv.New(managedenv.Options{Root: t.TempDir(), Catalog: catalog})
	if err != nil {
		t.Fatal(err)
	}
	previous := managedenv.Default()
	managedenv.SetDefault(m)
	defer func() { managedenv.SetDefault(previous); _ = m.Close() }()
	lookup := execLookPath
	execLookPath = func(string) (string, error) { return "", errors.New("no external package manager") }
	defer func() { execLookPath = lookup }()
	r := &Registry{runners: map[string]RunnerConfig{
		"managed": {ID: "managed", Command: "npx", Args: []string{"-y", "@example/agent@1.0.0"}},
		"custom":  {ID: "custom", Command: "custom-agent"},
	}}
	if err = r.Probe("managed"); err != nil {
		t.Fatal("managed route was blocked before preparation", err)
	}
	if err = r.Probe("custom"); err == nil {
		t.Fatal("unknown external command was treated as managed")
	}
}
