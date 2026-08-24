package plugins

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Export copies a recorded plugin package to a previously absent authoring
// directory. It never changes installed state or the live package tree.
func (s *Store) Export(name, destination string) (*InstalledPlugin, error) {
	if s == nil {
		return nil, ErrHomeRequired
	}
	name = strings.TrimSpace(name)
	destination = filepath.Clean(strings.TrimSpace(destination))
	if name == "" || destination == "" || destination == "." || filepath.Dir(destination) == destination {
		return nil, fmt.Errorf("plugin name and export destination are required")
	}
	if !filepath.IsAbs(destination) {
		return nil, fmt.Errorf("plugin export destination must be absolute")
	}
	if stayWithin(s.pluginsRoot, destination) == nil {
		return nil, fmt.Errorf("plugin export destination must remain outside the protected plugin store")
	}
	var exported *InstalledPlugin
	err := s.withLock(func() error {
		state, err := s.readStateUnlocked()
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		rec, ok := state.Plugins[name]
		if !ok {
			return fmt.Errorf("%w: %s", ErrNotFound, name)
		}
		source := filepath.Join(s.packagesRoot, name)
		if err := s.rejectUncontrolledPath(source); err != nil {
			return err
		}
		info, err := os.Lstat(source)
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("installed plugin package is not a regular directory")
		}
		if _, err := os.Lstat(destination); err == nil {
			return fmt.Errorf("plugin export destination already exists: %s", destination)
		} else if !os.IsNotExist(err) {
			return err
		}
		parent := filepath.Dir(destination)
		if err := os.MkdirAll(parent, 0o700); err != nil {
			return err
		}
		tempDir, err := os.MkdirTemp(parent, "."+filepath.Base(destination)+".tmp-*")
		if err != nil {
			return err
		}
		tempPresent := true
		defer func() {
			if tempPresent {
				_ = os.RemoveAll(tempDir)
			}
		}()
		if err := copyTree(source, tempDir); err != nil {
			return fmt.Errorf("copy plugin authoring candidate: %w", err)
		}
		if err := os.Rename(tempDir, destination); err != nil {
			return fmt.Errorf("activate plugin authoring candidate: %w", err)
		}
		tempPresent = false
		exported = s.loadRecord(name, rec)
		return nil
	})
	return exported, err
}
