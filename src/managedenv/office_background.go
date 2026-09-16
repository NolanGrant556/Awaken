package managedenv

import (
	"bytes"
	"context"
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
)

// Headless LibreOffice still registers as a foreground macOS application.
// Prepare the private copy before verification/fingerprinting, so every caller
// (including nested conversions) gets the same background application policy.
func (m *Manager) prepareOfficeBackground(ctx context.Context, root string) error {
	if !strings.HasPrefix(m.target, "darwin-") {
		return nil
	}
	base := filepath.Join(root, "dependencies", "native", "libreoffice", "Contents")
	// Native archives carry Awaken metadata beside Contents. macOS refuses to
	// re-sign that bundle root; keep this metadata inside its Resources instead.
	for _, name := range []string{"awaken-licenses", "awaken-provenance.json"} {
		source := filepath.Join(filepath.Dir(base), name)
		if _, err := os.Lstat(source); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return err
		}
		destination := filepath.Join(base, "Resources", "Awaken", name)
		if _, err := os.Lstat(destination); !os.IsNotExist(err) {
			return failure("environment_validation_failed", "duplicate LibreOffice bundle metadata")
		}
		if err := os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
			return err
		}
		if err := os.Rename(source, destination); err != nil {
			return err
		}
	}
	for _, command := range []struct {
		executable string
		args       []string
	}{
		{"/usr/bin/plutil", []string{"-replace", "LSUIElement", "-bool", "YES", filepath.Join(base, "Info.plist")}},
		{"/usr/bin/codesign", []string{"--force", "--sign", "-", filepath.Join(base, "MacOS", "soffice")}},
	} {
		if _, err := m.run(ctx, root, Launch{Executable: command.executable}, command.args...); err != nil {
			return err
		}
	}
	return nil
}

func officeBackgroundReady(f Manifest) bool {
	if f.Kind != "office" || !strings.HasPrefix(f.Target, "darwin-") {
		return true
	}
	data, err := os.ReadFile(filepath.Join(f.Root, "dependencies", "native", "libreoffice", "Contents", "Info.plist"))
	if err != nil {
		return false
	}
	d := xml.NewDecoder(bytes.NewReader(data))
	key := ""
	for {
		token, err := d.Token()
		if err != nil {
			return false
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		if start.Name.Local == "key" {
			if d.DecodeElement(&key, &start) != nil {
				return false
			}
		} else if key == "LSUIElement" {
			return start.Name.Local == "true"
		}
	}
}
