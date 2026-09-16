package managedenv

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// CLIInstallation is a projection of the component registry, not a second
// installation receipt. Main owns activation/auth/Skill state only.
type CLIInstallation struct {
	ExecutablePath string `json:"executablePath"`
	Version        string `json:"version"`
	Healthy        bool   `json:"healthy"`
}

var cliVersionPattern = regexp.MustCompile(`[0-9]+\.[0-9]+\.[0-9]+`)

func cliCommand(id string) string {
	if id == "dingtalk-workspace" {
		return "dws"
	}
	return id
}

func officialCLIAliasName(id string) string {
	return cliCommand(id) + "-official-alias"
}

func cliCommandFileName(id, target string) string {
	name := cliCommand(id)
	if strings.HasPrefix(target, "windows-") {
		name += ".exe"
	}
	return name
}

func (m *Manager) cliRoot() string {
	return filepath.Join(filepath.Dir(filepath.Dir(m.root)), "runtime", "cli")
}

func (m *Manager) awakenHome() string {
	return filepath.Dir(filepath.Dir(m.root))
}

func (m *Manager) userHome() string {
	awakenHome := m.awakenHome()
	if filepath.Base(awakenHome) == ".awaken" {
		return filepath.Dir(awakenHome)
	}
	return awakenHome
}

func (m *Manager) officialUserBin() string {
	return filepath.Join(m.userHome(), ".local", "bin")
}

func (m *Manager) officialCLICommandPath(id string) string {
	return filepath.Join(m.officialUserBin(), cliCommandFileName(id, m.target))
}

func (m *Manager) isOfficialCLICommandPath(id, path string) bool {
	return sameHostPath(filepath.Clean(path), m.officialCLICommandPath(id))
}

func (m *Manager) officialCLICommandOwned(f Manifest, path string) bool {
	if !m.isOfficialCLICommandPath(f.ComponentID, path) {
		return false
	}
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		return false
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false
	}
	launch, ok := f.Executables[cliCommand(f.ComponentID)]
	if !ok {
		return false
	}
	target := launch.Executable
	if canonical, err := filepath.EvalSymlinks(target); err == nil {
		target = canonical
	}
	return sameHostPath(resolved, target)
}

func (m *Manager) ownedCLIFileAllowed(f Manifest, path string) bool {
	if removableComponent(f.ComponentID) && m.officialCLICommandOwned(f, path) {
		return true
	}
	return ownedPath(m.cliRoot(), path) == nil
}

func (m *Manager) resolvedCLIExecutable(launch Launch) (string, error) {
	if canonical, err := filepath.EvalSymlinks(launch.Executable); err == nil {
		return canonical, nil
	}
	return filepath.Abs(launch.Executable)
}

func (m *Manager) inspectCLIExecutablePath(f Manifest) string {
	launch, ok := f.Executables[cliCommand(f.ComponentID)]
	if !ok {
		return ""
	}
	if alias, ok := f.Executables[officialCLIAliasName(f.ComponentID)]; ok {
		if info, err := os.Stat(alias.Executable); err == nil && info.Mode().IsRegular() {
			return alias.Executable
		}
	}
	return launch.Executable
}

func (m *Manager) recordOfficialCLIAlias(f Manifest, dest, hash string) (Manifest, error) {
	alias := officialCLIAliasName(f.ComponentID)
	if f.OwnedFiles[dest] == hash {
		if launch, ok := f.Executables[alias]; ok && sameHostPath(launch.Executable, dest) {
			return f, nil
		}
	}
	f = clone(f)
	if f.OwnedFiles == nil {
		f.OwnedFiles = map[string]string{}
	}
	f.OwnedFiles[dest] = hash
	if f.Executables == nil {
		f.Executables = map[string]Launch{}
	}
	f.Executables[alias] = Launch{Executable: dest}
	if f.Fingerprints == nil {
		f.Fingerprints = map[string]string{}
	}
	if info, err := os.Stat(dest); err == nil {
		f.Fingerprints[dest] = fileIdentity(info)
	}
	if err := m.save(f); err != nil {
		return f, err
	}
	m.invalidateHealth()
	return f, nil
}

func (m *Manager) dropOfficialCLIAlias(f Manifest) (Manifest, error) {
	dest := m.officialCLICommandPath(f.ComponentID)
	alias := officialCLIAliasName(f.ComponentID)
	_, hasOwned := f.OwnedFiles[dest]
	_, hasLaunch := f.Executables[alias]
	_, hasPrint := f.Fingerprints[dest]
	if !hasOwned && !hasLaunch && !hasPrint {
		return f, nil
	}
	f = clone(f)
	if f.OwnedFiles != nil {
		delete(f.OwnedFiles, dest)
	}
	if f.Executables != nil {
		delete(f.Executables, alias)
	}
	if f.Fingerprints != nil {
		delete(f.Fingerprints, dest)
	}
	if err := m.save(f); err != nil {
		return f, err
	}
	m.invalidateHealth()
	return f, nil
}

func (m *Manager) publishOfficialCLICommand(f Manifest) (Manifest, error) {
	if !removableComponent(f.ComponentID) || f.Source != "awaken-managed" {
		return f, nil
	}
	launch, ok := f.Executables[cliCommand(f.ComponentID)]
	if !ok {
		return f, nil
	}
	target, err := m.resolvedCLIExecutable(launch)
	if err != nil {
		return f, nil
	}
	hash, err := fileSHA256(target)
	if err != nil {
		return f, nil
	}
	dest := m.officialCLICommandPath(f.ComponentID)
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return m.dropOfficialCLIAlias(f)
	}
	info, err := os.Lstat(dest)
	if os.IsNotExist(err) {
		if err := os.Symlink(target, dest); err != nil {
			info, err = os.Lstat(dest)
			if err != nil {
				return m.dropOfficialCLIAlias(f)
			}
		} else {
			return m.recordOfficialCLIAlias(f, dest, hash)
		}
	} else if err != nil {
		return m.dropOfficialCLIAlias(f)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		resolved, err := filepath.EvalSymlinks(dest)
		if err == nil && sameHostPath(resolved, target) {
			return m.recordOfficialCLIAlias(f, dest, hash)
		}
	}
	return m.dropOfficialCLIAlias(f)
}

// Rebase only aliases above the historical CLI root (for example macOS /var).
// Symlinks inside that root remain visible to ownedPath and are rejected.
func (m *Manager) canonicalCLIPath(path string) string {
	for parent := filepath.Dir(path); filepath.Dir(parent) != parent; parent = filepath.Dir(parent) {
		resolved, err := filepath.EvalSymlinks(parent)
		if err == nil && sameHostPath(resolved, m.cliRoot()) {
			relative, err := filepath.Rel(parent, path)
			if err == nil {
				return filepath.Join(m.cliRoot(), relative)
			}
		}
	}
	return path
}
func sameHostPath(a, b string) bool {
	if os.PathSeparator == '\\' {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(f, (128<<20)+1))
	if err != nil {
		return "", err
	}
	if n > 128<<20 {
		return "", failure("environment_validation_failed", "legacy CLI exceeds size limit")
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// This is the only importer of the retired Main binary-ownership record. It
// leaves files in place, preserves existing instance IDs/leases, and writes the
// regular component manifest before publishing the compact registry entry.
func (m *Manager) migrateCLIOwnership(ctx context.Context) error {
	if m.target != Target() {
		return nil
	}
	return m.registry(func(r *registry) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		type oldRecord struct {
			ExecutablePath string `json:"executablePath"`
			Version        string `json:"version"`
			Ownership      string `json:"ownership"`
		}
		var old struct {
			Version int                  `json:"version"`
			Records map[string]oldRecord `json:"records"`
		}
		path := filepath.Join(m.cliRoot(), "installations.json")
		if err := ownedPath(m.cliRoot(), path); err != nil {
			return err
		}
		if err := readJSON(path, &old); err != nil && !os.IsNotExist(err) {
			return err
		}
		for _, id := range []string{"dingtalk-workspace", "lark-cli"} {
			candidates := []Manifest{}
			registered := map[string]bool{}
			for _, f := range r.Instances {
				if f.ComponentID != id {
					continue
				}
				for _, launch := range f.Executables {
					registered[m.canonicalCLIPath(launch.Executable)] = true
				}
				if f.Source == "awaken-legacy" {
					candidates = append(candidates, f)
				}
			}
			record := old.Records[id]
			executable := m.canonicalCLIPath(filepath.Clean(record.ExecutablePath))
			if record.Ownership == "awaken-managed" && !registered[executable] && inside(filepath.Join(m.cliRoot(), id), executable) {
				// Main records pointing at shared components do not prove ownership of a
				// missing registry entry; recovery must use that component's manifest.
				instance := digest([]string{"cli-import", id, executable})[:32]
				key := digest([]string{"cli-import", id})
				candidates = append(candidates, Manifest{SchemaVersion: SchemaVersion, Kind: "component", ComponentID: id, Key: key, InstanceID: instance, Root: filepath.Join(m.root, "components", key, instance), Target: m.target, Source: "awaken-legacy", State: "ready", Version: record.Version, Executables: map[string]Launch{cliCommand(id): {Executable: executable}}, CreatedAt: m.now(), LastUsedAt: m.now()})
			}
			for _, candidate := range candidates {
				f := clone(candidate)
				launch, ok := f.Executables[cliCommand(id)]
				if !ok {
					continue
				}
				launch.Executable = m.canonicalCLIPath(launch.Executable)
				f.Executables[cliCommand(id)] = launch
				root := filepath.Join(m.cliRoot(), id)
				if !inside(root, launch.Executable) || ownedPath(root, launch.Executable) != nil {
					continue
				}
				owned := map[string]string{}
				hash, err := fileSHA256(launch.Executable)
				if err != nil && !os.IsNotExist(err) {
					return err
				}
				owned[launch.Executable] = hash
				name := cliCommand(id)
				if strings.HasPrefix(m.target, "windows-") {
					name += ".exe"
				}
				alias := filepath.Join(m.cliRoot(), "bin", name)
				if hash != "" && ownedPath(m.cliRoot(), alias) == nil {
					if aliasHash, err := fileSHA256(alias); err == nil && aliasHash == hash {
						owned[alias] = hash
						f.Executables[cliCommand(id)+"-legacy-alias"] = Launch{Executable: alias}
					}
				}
				f.Source, f.OwnedFiles = "awaken-managed", owned
				f.Version = cliVersionPattern.FindString(f.Version)
				f.Fingerprints = fingerprints(f.Executables)
				if err := m.persist(filepath.Join(f.Root, "manifest.json"), f); err != nil {
					return err
				}
				r.Instances[f.InstanceID] = f
			}
		}
		return nil
	})
}

func (m *Manager) InspectCLIInstallation(ctx context.Context, id string) (*CLIInstallation, error) {
	if !removableComponent(id) {
		return nil, failure("environment_request_invalid", "unsupported CLI component")
	}
	if err := m.migrateCLIOwnership(ctx); err != nil {
		return nil, err
	}
	all, err := m.instances()
	if err != nil {
		return nil, err
	}
	var result *CLIInstallation
	for _, f := range all {
		if f.Kind != "component" || f.ComponentID != id || f.Source != "awaken-managed" || f.Target != m.target {
			continue
		}
		if _, ok := f.Executables[cliCommand(id)]; !ok {
			continue
		}
		published, err := m.publishOfficialCLICommand(f)
		if err != nil {
			return nil, err
		}
		f = published
		path := m.inspectCLIExecutablePath(f)
		if path == "" {
			continue
		}
		value := &CLIInstallation{ExecutablePath: path, Version: f.Version, Healthy: m.reusable(f)}
		if result == nil || (!result.Healthy && value.Healthy) {
			result = value
		}
		if result.Healthy {
			break
		}
	}
	return result, nil
}

// Validate all owned legacy files before deleting any. Changed files and
// redirected parents are never treated as the previously imported executable.
func (m *Manager) validateOwnedCLIFile(id, path, hash string) error {
	if m.isOfficialCLICommandPath(id, path) {
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink == 0 {
			return failure("environment_validation_failed", "legacy CLI was changed; refusing removal")
		}
		actual, err := fileSHA256(path)
		if err != nil {
			return err
		}
		if hash == "" || actual != hash {
			return failure("environment_validation_failed", "legacy CLI was changed; refusing removal")
		}
		return nil
	}
	root := m.cliRoot()
	name := cliCommandFileName(id, m.target)
	if !inside(filepath.Join(root, id), path) && path != filepath.Join(root, "bin", name) {
		return failure("environment_path_invalid", "invalid legacy CLI path")
	}
	if err := ownedPath(root, path); err != nil {
		return err
	}
	actual, err := fileSHA256(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if hash == "" || actual != hash {
		return failure("environment_validation_failed", "legacy CLI was changed; refusing removal")
	}
	return nil
}

func componentUsers(r *registry, id string) map[string]bool {
	selected := map[string]bool{}
	for key, f := range r.Instances {
		if f.ComponentID == id {
			selected[key] = true
		}
	}
	for key, f := range r.Instances {
		for _, child := range f.Components {
			if selected[child] {
				selected[key] = true
			}
		}
	}
	return selected
}
func (m *Manager) checkComponentIdle(r *registry, id string) error {
	if m.componentPreparing(id) {
		return failure("environment_in_use", "运行组件正在准备，请稍后卸载")
	}
	users := componentUsers(r, id)
	for key := range users {
		if m.hasLeases(key) {
			return failure("environment_in_use", "运行组件仍被任务或服务使用")
		}
	}
	return nil
}
