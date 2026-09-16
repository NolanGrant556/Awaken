package managedenv

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestOfficeBackgroundPolicy(t *testing.T) {
	for _, tc := range []struct {
		name, plist string
		ready       bool
	}{
		{"enabled", `<plist><dict><key>CFBundleName</key><string>LibreOffice</string><key>LSUIElement</key><true/></dict></plist>`, true},
		{"disabled", `<plist><dict><key>LSUIElement</key><false/></dict></plist>`, false},
		{"absent", `<plist><dict><key>CFBundleName</key><string>LSUIElement</string></dict></plist>`, false},
		{"malformed", `{broken`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "dependencies/native/libreoffice/Contents/Info.plist")
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(tc.plist), 0600); err != nil {
				t.Fatal(err)
			}
			if got := officeBackgroundReady(Manifest{Kind: "office", Target: "darwin-arm64", Root: root}); got != tc.ready {
				t.Fatalf("got %v", got)
			}
			if !officeBackgroundReady(Manifest{Kind: "office", Target: "windows-x64", Root: root}) {
				t.Fatal("macOS policy applied on Windows")
			}
		})
	}
}

func TestOfficeForegroundCopyRecoversUsingExistingPreparation(t *testing.T) {
	m := managerForTest(t, Options{Root: t.TempDir(), Catalog: officeCatalogForTest(t)})
	if m.target != "darwin-arm64" && m.target != "darwin-x64" {
		t.Skip("macOS policy")
	}
	old := officeStoredFixture(t, m, "foreground-old", "")
	owner := testOwner("foreground-task")
	before, err := m.bind(old, owner, true)
	if err != nil {
		t.Fatal(err)
	}
	next := officeStoredFixture(t, m, "background-new", old.InstanceID)
	if err = m.activateOfficeBundle(context.Background(), next); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(old.Root, "dependencies/native/libreoffice/Contents/Info.plist"), []byte(`<plist><dict/></plist>`), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := m.EnsureOfficeBundle(context.Background(), owner, nil)
	if err != nil || got.InstanceID != next.InstanceID || got.EnvironmentRef == before.EnvironmentRef {
		t.Fatalf("did not recover old foreground bundle: %+v %v", got, err)
	}
}
