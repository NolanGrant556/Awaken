package managedenv

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

// Legacy discovery has host-owned roots only. Project virtual environments
// and arbitrary model-supplied paths never enter this registry.
func (m *Manager) legacyExecutables(id string, strict ...bool) map[string]Launch {
	if m.target != Target() {
		return nil
	}
	home := filepath.Dir(filepath.Dir(m.root))
	if id == "typst" {
		p := filepath.Join(home, "toolchains", "typst", "bin", "typst")
		if strings.HasPrefix(m.target, "windows-") {
			p += ".exe"
		}
		if f, e := os.Stat(p); e == nil && f.Mode().IsRegular() {
			return map[string]Launch{"typst": {Executable: p}}
		}
	}
	return nil
}

// RemoveComponent is an explicit host uninstall, separate from cache collection.
// It cannot remove external software or an instance held by another task.
func (m *Manager) RemoveComponent(ctx context.Context, id string, removalToken ...string) error {
	if id != "dingtalk-workspace" && id != "lark-cli" {
		return failure("environment_request_invalid", "component uninstall is unsupported")
	}
	if err := m.migrateCLIOwnership(ctx); err != nil {
		return err
	}
	return m.registry(func(r *registry) error {
		token := ""
		if len(removalToken) > 0 {
			token = removalToken[0]
		}
		if err := m.checkRemoval(Manifest{ComponentID: id}, r.Instances, token); err != nil {
			return err
		}
		if err := m.checkComponentIdle(r, id); err != nil {
			return err
		}
		selected := map[string]bool{}
		for key, f := range r.Instances {
			if f.ComponentID == id && f.Source == "awaken-managed" {
				selected[key] = true
				for path, hash := range f.OwnedFiles {
					if err := m.validateOwnedCLIFile(id, path, hash); err != nil {
						return err
					}
				}
			}
		}
		for key := range selected {
			if err := ctx.Err(); err != nil {
				return err
			}
			f := r.Instances[key]
			for path := range f.OwnedFiles {
				if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
					return err
				}
			}
			if err := ownedPath(m.root, f.Root); err != nil {
				return err
			}
			if err := os.RemoveAll(f.Root); err != nil {
				return err
			}
			delete(r.Instances, key)
		}
		return nil
	})
}
