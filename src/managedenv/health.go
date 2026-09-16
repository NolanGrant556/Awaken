package managedenv

import (
	"os"
	"path/filepath"
	"time"
)

// Manifests are immutable after publication. Their file identity invalidates
// parsed data across repair, deletion and another Core's atomic replacement.
type cachedManifest struct {
	stamp string
	value Manifest
}
type cachedHealth struct {
	stamp     string
	checkedAt time.Time
}

const healthReuseWindow = 30 * time.Second

func (m *Manager) readManifest(root string) (Manifest, error) {
	path := filepath.Join(root, "manifest.json")
	if err := ownedPath(m.root, path); err != nil {
		return Manifest{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return Manifest{}, err
	}
	stamp := fileIdentity(info)
	m.cacheMu.Lock()
	cached, ok := m.manifests[root]
	m.cacheMu.Unlock()
	if ok && cached.stamp == stamp {
		return cached.value, nil
	}
	var f Manifest
	if err := readJSON(path, &f); err != nil {
		return Manifest{}, err
	}
	if f.Root != root || f.SchemaVersion != SchemaVersion || f.State != "ready" || !safeID.MatchString(f.InstanceID) {
		return Manifest{}, failure("environment_validation_failed", "invalid instance manifest")
	}
	m.cacheMu.Lock()
	if m.manifests == nil || len(m.manifests) >= 64 {
		m.manifests = map[string]cachedManifest{}
	}
	m.manifests[root] = cachedManifest{stamp, f}
	m.cacheMu.Unlock()
	return f, nil
}

// Launch files are checked on every use. The expensive walk of library files
// is shared by nearby lookups; explicit preparation invalidates this cache.
func (m *Manager) reusable(f Manifest) bool {
	for path := range f.OwnedFiles {
		if ownedPath(m.cliRoot(), path) != nil {
			return false
		}
	}
	info, err := os.Stat(filepath.Join(f.Root, "manifest.json"))
	if err != nil {
		return false
	}
	for _, launch := range f.Executables {
		for _, path := range append([]string{launch.Executable}, launch.Args...) {
			if !filepath.IsAbs(path) {
				continue
			}
			want, tracked := f.Fingerprints[path]
			if !tracked && path != launch.Executable {
				continue
			}
			stat, err := os.Stat(path)
			if err != nil || !stat.Mode().IsRegular() || (tracked && fileIdentity(stat) != want) {
				return false
			}
		}
	}
	key := f.InstanceID + ":" + digest(f.Executables)
	stamp := fileIdentity(info)
	now := m.now()
	m.cacheMu.Lock()
	cached, ok := m.health[key]
	generation := m.healthGeneration
	m.cacheMu.Unlock()
	if ok && cached.stamp == stamp && !now.Before(cached.checkedAt) && now.Sub(cached.checkedAt) < healthReuseWindow {
		return true
	}
	if !m.healthy(f) {
		return false
	}
	m.cacheMu.Lock()
	// Do not republish a check that started before explicit invalidation.
	if generation != m.healthGeneration {
		m.cacheMu.Unlock()
		return true
	}
	if m.health == nil || len(m.health) >= 256 {
		m.health = map[string]cachedHealth{}
	}
	m.health[key] = cachedHealth{stamp, now}
	m.cacheMu.Unlock()
	return true
}
func (m *Manager) invalidateHealth() {
	m.cacheMu.Lock()
	m.health = nil
	m.healthGeneration++
	m.cacheMu.Unlock()
}
