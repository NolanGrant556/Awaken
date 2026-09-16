package managedenv

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Optional release-candidate verification exercises the real installer and
// real bundled programs. It is not a model or Desktop acceptance substitute.
func TestOfficeBundleCandidate(t *testing.T) {
	catalogPath := os.Getenv("AWAKEN_OFFICE_CANDIDATE_CATALOG")
	root := os.Getenv("AWAKEN_OFFICE_VERIFY_ROOT")
	if catalogPath == "" || root == "" {
		t.Skip("set an explicit candidate catalog and validation root")
	}
	c, err := LoadCatalog(catalogPath)
	if err != nil {
		t.Fatal(err)
	}
	m := managerForTest(t, Options{Root: root, Catalog: c, AllowFileAssets: true, Timeout: 20 * time.Minute})
	spec, err := m.officeSpec()
	if err != nil {
		t.Fatal(err)
	}
	candidateID := digest(spec.OfficeBundle)
	last := ""
	b, err := m.EnsureOfficeBundle(context.Background(), testOwner("candidate-A-"+candidateID), func(op Operation) {
		if op.State != last {
			t.Log(op.State, op.Message)
			last = op.State
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if b.Reused {
		f, err := m.InspectOfficeBundle(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if err = m.verifyOfficeBundle(context.Background(), f); err != nil {
			t.Fatal(err)
		}
	}
	again, err := m.EnsureOfficeBundle(context.Background(), testOwner("candidate-B-"+candidateID), nil)
	if err != nil || !again.Reused || again.InstanceID != b.InstanceID {
		t.Fatal("candidate reuse failed", err)
	}
	t.Log("actual bundle verified and reused:", b.BundleVersion, b.InstanceID)
}

func officeCatalogForTest(t *testing.T) Catalog {
	t.Helper()
	c, err := BuiltinCatalog()
	if err != nil {
		t.Fatal(err)
	}
	a := Asset{URL: "https://example.invalid/office.tar.gz", SHA256: strings.Repeat("a", 64), Format: "tar.gz", MaxBytes: 1024, MaxExpandedBytes: 4096, Executables: map[string]string{}, Resources: map[string]string{}}
	for _, n := range officeEntries {
		a.Executables[n] = "dependencies/bin/" + n
	}
	for _, n := range officeResources {
		a.Resources[n] = "dependencies/" + n
	}
	c.OfficeBundle.Assets[Target()] = a
	return c
}
func assertEnvironmentCode(t *testing.T, err error, code string) {
	t.Helper()
	var typed *Error
	if !errors.As(err, &typed) || typed.Code != code {
		t.Fatalf("want %s, got %v", code, err)
	}
}

func TestOfficeBundleDoesNotTreatLegacyEnvironmentAsReady(t *testing.T) {
	o, _ := fixture(t)
	m := managerForTest(t, o)
	old, err := m.Ensure(context.Background(), Request{Capability: "fixture"}, testOwner("existing"), nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = m.EnsureOfficeBundle(context.Background(), testOwner("existing"), nil)
	assertEnvironmentCode(t, err, "environment_source_unavailable")
	if _, err = m.Binding(context.Background(), old.EnvironmentRef, testOwner("existing")); err != nil {
		t.Fatal("old binding damaged:", err)
	}
}

func TestOfficeSystemRequirementFailsBeforeDownload(t *testing.T) {
	c := officeCatalogForTest(t)
	c.OfficeBundle.MinimumOS = map[string]string{strings.Split(Target(), "-")[0]: "999.0"}
	m := managerForTest(t, Options{Root: t.TempDir(), Catalog: c})
	_, err := m.EnsureOfficeBundle(context.Background(), testOwner("old-system"), nil)
	assertEnvironmentCode(t, err, "environment_platform_unsupported")
	if len(m.Operations()) != 0 {
		t.Fatal("unsupported system started preparation")
	}
}
func TestOfficeBundleRequiresAllRoutesAndEntries(t *testing.T) {
	base := officeCatalogForTest(t)
	for _, tc := range []struct {
		name   string
		change func(*Catalog)
	}{
		{"no asset", func(c *Catalog) { delete(c.OfficeBundle.Assets, Target()) }},
		{"invalid hash", func(c *Catalog) {
			a := c.OfficeBundle.Assets[Target()]
			a.SHA256 = strings.Repeat("z", 64)
			c.OfficeBundle.Assets[Target()] = a
		}},
		{"no Node package", func(c *Catalog) { delete(c.OfficeBundle.Spec.Node, "docx") }},
		{"no Python package", func(c *Catalog) { delete(c.OfficeBundle.Spec.Python, "openpyxl") }},
		{"no renderer", func(c *Catalog) { a := c.OfficeBundle.Assets[Target()]; delete(a.Executables, "soffice") }},
		{"no fonts", func(c *Catalog) { a := c.OfficeBundle.Assets[Target()]; delete(a.Resources, "fonts") }},
		{"no size budget", func(c *Catalog) {
			a := c.OfficeBundle.Assets[Target()]
			a.MaxExpandedBytes = 0
			c.OfficeBundle.Assets[Target()] = a
		}},
		{"unlocked interpreter", func(c *Catalog) { c.OfficeBundle.Spec.ComponentVersions["python"] = "0.1.0" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := clone(base)
			tc.change(&c)
			m := managerForTest(t, Options{Root: t.TempDir(), Catalog: c})
			_, err := m.officeSpec()
			assertEnvironmentCode(t, err, "environment_source_unavailable")
		})
	}
	m := managerForTest(t, Options{Root: t.TempDir(), Catalog: base})
	spec, err := m.officeSpec()
	if err != nil {
		t.Fatal(err)
	}
	old := clone(spec)
	old.OfficeBundle = nil
	if specContains(old, spec) {
		t.Fatal("old package collection was treated as verified bundle")
	}
	changed := clone(spec)
	changed.OfficeBundle.SHA256 = strings.Repeat("b", 64)
	if specContains(spec, changed) {
		t.Fatal("changed archive hash reused")
	}
}
func TestBundlePathsRejectEscapingAndWrongTypes(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "file")
	if err := os.WriteFile(p, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"../elsewhere", "/etc/passwd", "C:/runtime/python.exe", "..\\outside", "missing"} {
		if _, err := bundlePath(root, s, false); err == nil {
			t.Fatalf("accepted %s", s)
		}
	}
	if _, err := bundlePath(root, "file", true); err == nil {
		t.Fatal("file accepted as directory")
	}
	if got, err := bundlePath(root, "file", false); err != nil || got != p {
		t.Fatal(got, err)
	}
}
func TestOfficeArchiveMissingMetadataCannotPublish(t *testing.T) {
	o, _ := fixture(t)
	c := officeCatalogForTest(t)
	a := c.OfficeBundle.Assets[Target()]
	// The fixture returns plain bytes, not a valid complete office archive.
	a.URL = o.Catalog.Components["fixture"].Assets[Target()].URL
	a.SHA256 = o.Catalog.Components["fixture"].Assets[Target()].SHA256
	c.OfficeBundle.Assets[Target()] = a
	o.Catalog = c
	m := managerForTest(t, o)
	if _, err := m.EnsureOfficeBundle(context.Background(), testOwner("incomplete"), nil); err == nil {
		t.Fatal("incomplete bundle published")
	}
	if _, err := os.Stat(filepath.Join(m.root, "office", "current.json")); !os.IsNotExist(err) {
		t.Fatal("failed bundle selected")
	}
	all, err := m.instances()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 0 {
		t.Fatal("failed bundle registered")
	}
}
func TestOfficeSelectionFailurePreservesPreviousAndRegistryGate(t *testing.T) {
	c := officeCatalogForTest(t)
	m := managerForTest(t, Options{Root: t.TempDir(), Catalog: c})
	// A stored verified record is used solely to exercise selection/storage, not
	// as product evidence of running Office programs.
	spec, err := m.officeSpec()
	if err != nil {
		t.Fatal(err)
	}
	makeRecord := func(id string) Manifest {
		root := filepath.Join(m.root, "office", "instances", Target(), c.OfficeBundle.Version, id)
		if err := os.MkdirAll(root, 0700); err != nil {
			t.Fatal(err)
		}
		p := filepath.Join(root, "entry")
		if err := os.WriteFile(p, []byte("verified fixture"), 0600); err != nil {
			t.Fatal(err)
		}
		if strings.HasPrefix(m.target, "darwin-") {
			plist := filepath.Join(root, "dependencies/native/libreoffice/Contents/Info.plist")
			if err := os.MkdirAll(filepath.Dir(plist), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(plist, []byte(`<plist><dict><key>LSUIElement</key><true/></dict></plist>`), 0600); err != nil {
				t.Fatal(err)
			}
		}
		return Manifest{SchemaVersion: 1, Kind: "office", Key: id, InstanceID: id, Target: Target(), Source: "awaken-managed", Root: root, State: "ready", Spec: spec, NodeLauncherVersion: 3, Executables: map[string]Launch{"python": {Executable: p}}, Fingerprints: fingerprints(map[string]Launch{"python": {Executable: p}}), CreatedAt: m.now()}
	}
	old := makeRecord("old")
	if err = m.save(old); err != nil {
		t.Fatal(err)
	}
	if err = m.activateOfficeBundle(context.Background(), old); err != nil {
		t.Fatal(err)
	}
	latest := makeRecord("new")
	if err = m.save(latest); err != nil {
		t.Fatal(err)
	}
	m.persistRecord = func(p string, v any) error {
		if strings.HasSuffix(p, "current.json") {
			return errors.New("injected selection failure")
		}
		return writeJSON(p, v)
	}
	if err = m.activateOfficeBundle(context.Background(), latest); err == nil {
		t.Fatal("injection not exercised")
	}
	if !m.officeSelected(old) || m.officeSelected(latest) {
		t.Fatal("failed selection changed current")
	}
	m.persistRecord = nil
	raw, err := os.ReadFile(filepath.Join(m.root, "registry.json"))
	if err != nil {
		t.Fatal(err)
	}
	var r registry
	if json.Unmarshal(raw, &r) != nil || r.SchemaVersion != registrySchemaVersion {
		t.Fatal("old Core is not fenced")
	}
	if err = os.Remove(filepath.Join(m.root, "registry.json")); err != nil {
		t.Fatal(err)
	}
	all, err := m.instances()
	if err != nil || len(all) != 2 {
		t.Fatal("index reconstruction lost office manifests", err, len(all))
	}
	b, err := m.EnsureOfficeBundle(context.Background(), testOwner("restart"), nil)
	if err != nil || b.InstanceID != old.InstanceID {
		t.Fatal("did not reuse selected healthy instance", err)
	}
	if b.BundleVersion != c.OfficeBundle.Version || m.Bindings(testOwner("restart"))[0].BundleVersion != b.BundleVersion {
		t.Fatal("bundle identity was lost in binding projection")
	}
	other, err := m.EnsureOfficeBundle(context.Background(), testOwner("another-project"), nil)
	if err != nil || other.InstanceID != b.InstanceID || other.EnvironmentRef == b.EnvironmentRef {
		t.Fatal("different owners must share an instance with separate references", err)
	}
	// An application catalog update must not replace a live task's chosen version.
	m.catalog.OfficeBundle.Version = "next-version"
	m.catalog.OfficeBundle.Assets = nil
	retained, err := m.EnsureOfficeBundle(context.Background(), testOwner("restart"), nil)
	if err != nil || retained.InstanceID != b.InstanceID {
		t.Fatal("catalog update redirected the existing task", err)
	}
	_, err = m.EnsureOfficeBundle(context.Background(), testOwner("new-task"), nil)
	assertEnvironmentCode(t, err, "environment_source_unavailable")
	if err = writeJSON(filepath.Join(m.root, "office", "current.json"), nil); err != nil {
		t.Fatal(err)
	}
	if err := m.activateOfficeBundle(context.Background(), old); err != nil {
		t.Fatal("corrupt selection did not recover", err)
	}
	if _, err := m.Collect(context.Background()); err != nil {
		t.Fatal(err)
	}
	preserved, _ := filepath.Glob(filepath.Join(m.root, "office", "current.json.invalid-*"))
	if len(preserved) != 1 {
		t.Fatal("corrupt selection was not preserved")
	}
}
