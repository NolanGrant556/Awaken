package plugins

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"awaken/src/desktopdata"
	"awaken/src/filestore"
	"awaken/src/mcpclient"
	"awaken/src/tool/trash"
)

const stateLockTTL = 30 * time.Second

// Store is the user-scope plugin package manager.
type Store struct {
	home                 string
	pluginsRoot          string
	packagesRoot         string
	dataRoot             string
	stagingRoot          string
	statePath            string
	lockPath             string
	trash                trash.TrashAdapter
	mu                   sync.Mutex
	changeMu             sync.Mutex
	corruptErr           error
	onChange             func()
	changeListeners      map[uint64]func()
	nextChangeListenerID uint64
}

// NewStore opens the production store under AWAKEN_HOME. Missing home fails
// closed so the process cannot write the real user directory.
func NewStore() (*Store, error) {
	home := desktopdata.AwakenHomeRoot()
	if home == "" {
		return nil, ErrHomeRequired
	}
	return NewStoreAt(home), nil
}

// NewStoreAt opens a store rooted at an already-resolved Awaken home.
func NewStoreAt(home string) *Store {
	return NewStoreAtWithTrash(home, trash.DefaultAdapter())
}

// NewStoreAtWithTrash is the testable constructor.
func NewStoreAtWithTrash(home string, adapter trash.TrashAdapter) *Store {
	home = filepath.Clean(home)
	root := desktopdata.PluginsRoot(home)
	if adapter == nil {
		adapter = trash.DefaultAdapter()
	}
	store := &Store{
		home:            home,
		pluginsRoot:     root,
		packagesRoot:    filepath.Join(root, "packages"),
		dataRoot:        filepath.Join(root, "data"),
		stagingRoot:     filepath.Join(root, ".staging"),
		statePath:       desktopdata.PluginsConfigPath(home),
		lockPath:        desktopdata.PluginsConfigPath(home) + ".lock",
		trash:           adapter,
		changeListeners: make(map[uint64]func()),
	}
	if err := store.ensureDirs(); err != nil {
		store.corruptErr = err
		return store
	}
	if _, err := store.readStateUnlocked(); err != nil && !errors.Is(err, os.ErrNotExist) {
		store.corruptErr = err
	}
	store.cleanupOrphanStaging()
	return store
}

// SetOnChange registers a callback after a successful mutation.
func (s *Store) SetOnChange(fn func()) {
	if s == nil {
		return
	}
	s.changeMu.Lock()
	s.onChange = fn
	s.changeMu.Unlock()
}

// SubscribeChanges adds a lifecycle listener without replacing the Core-owned
// runtime refresh callback installed through SetOnChange.
func (s *Store) SubscribeChanges(fn func()) func() {
	if s == nil || fn == nil {
		return func() {}
	}
	s.changeMu.Lock()
	s.nextChangeListenerID++
	id := s.nextChangeListenerID
	s.changeListeners[id] = fn
	s.changeMu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			s.changeMu.Lock()
			delete(s.changeListeners, id)
			s.changeMu.Unlock()
		})
	}
}

// Available reports whether the store can serve plugin operations.
func (s *Store) Available() error {
	if s == nil {
		return ErrHomeRequired
	}
	if s.corruptErr != nil {
		if errors.Is(s.corruptErr, ErrStateCorrupt) {
			return s.corruptErr
		}
		return fmt.Errorf("%w: %v", ErrStateCorrupt, s.corruptErr)
	}
	return nil
}

func (s *Store) ensureDirs() error {
	for _, dir := range []string{
		filepath.Dir(s.statePath),
		s.packagesRoot,
		s.dataRoot,
		s.stagingRoot,
	} {
		if err := filestore.EnsureDir(dir); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) cleanupOrphanStaging() {
	entries, err := os.ReadDir(s.stagingRoot)
	if err != nil {
		return
	}
	for _, entry := range entries {
		_ = os.RemoveAll(filepath.Join(s.stagingRoot, entry.Name()))
	}
}

func (s *Store) withLock(fn func() error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.Available(); err != nil {
		return err
	}
	lock, err := filestore.AcquireLock(s.lockPath, "awaken-plugins", stateLockTTL)
	if err != nil {
		return fmt.Errorf("acquire plugins.json lock: %w", err)
	}
	defer func() { _ = lock.Release() }()
	return fn()
}

func (s *Store) readStateUnlocked() (StateFile, error) {
	data, err := os.ReadFile(s.statePath)
	if err != nil {
		if os.IsNotExist(err) {
			return StateFile{Version: StateVersion, Plugins: map[string]Record{}}, os.ErrNotExist
		}
		return StateFile{}, err
	}
	var state StateFile
	if err := json.Unmarshal(data, &state); err != nil {
		return StateFile{}, fmt.Errorf("%w: %v", ErrStateCorrupt, err)
	}
	if state.Version != StateVersion {
		return StateFile{}, fmt.Errorf("%w: unsupported version %d", ErrStateCorrupt, state.Version)
	}
	if state.Plugins == nil {
		state.Plugins = map[string]Record{}
	}
	return state, nil
}

func (s *Store) writeStateUnlocked(state StateFile) error {
	state.Version = StateVersion
	if state.Plugins == nil {
		state.Plugins = map[string]Record{}
	}
	encoded, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return filestore.WriteFileAtomic(s.statePath, append(encoded, '\n'))
}

func (s *Store) notify() {
	s.changeMu.Lock()
	primary := s.onChange
	listeners := make([]func(), 0, len(s.changeListeners))
	for _, listener := range s.changeListeners {
		listeners = append(listeners, listener)
	}
	s.changeMu.Unlock()
	if primary != nil {
		primary()
	}
	for _, listener := range listeners {
		listener()
	}
}

// List returns every recorded plugin, including disabled and invalid packages.
func (s *Store) List() ([]*InstalledPlugin, error) {
	var out []*InstalledPlugin
	err := s.withLock(func() error {
		state, err := s.readStateUnlocked()
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		names := make([]string, 0, len(state.Plugins))
		for name := range state.Plugins {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			out = append(out, s.loadRecord(name, state.Plugins[name]))
		}
		return nil
	})
	return out, err
}

// Get returns one installed plugin.
func (s *Store) Get(name string) (*InstalledPlugin, error) {
	name = strings.TrimSpace(name)
	var plugin *InstalledPlugin
	err := s.withLock(func() error {
		state, err := s.readStateUnlocked()
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		rec, ok := state.Plugins[name]
		if !ok {
			return fmt.Errorf("%w: %s", ErrNotFound, name)
		}
		plugin = s.loadRecord(name, rec)
		return nil
	})
	return plugin, err
}

func (s *Store) loadRecord(name string, rec Record) *InstalledPlugin {
	root := filepath.Join(s.packagesRoot, name)
	dataDir := filepath.Join(s.dataRoot, name)
	actualDigest, matches, digestErr := verifyTreeDigest(root, rec.Digest)
	if digestErr != nil || !matches {
		message := fmt.Sprintf("installed plugin package digest changed: recorded %s, actual %s", rec.Digest, actualDigest)
		if digestErr != nil {
			message = "verify installed plugin package digest: " + digestErr.Error()
		}
		return &InstalledPlugin{
			Name: name, Root: root, DataDir: dataDir, Enabled: rec.Enabled,
			Source: rec.Source, Digest: rec.Digest, ActualDigest: actualDigest,
			InstalledAt: rec.InstalledAt,
			Diagnostics: []Diagnostic{errorDiag("plugin_digest_changed", message)},
		}
	}
	plugin := LoadPackage(root, dataDir, rec)
	if dataErr := validateManagedPluginDataDir(s.dataRoot, dataDir); dataErr != nil {
		plugin.MCPServers = nil
		plugin.Diagnostics = append(plugin.Diagnostics, errorDiag("plugin_data_invalid", dataErr.Error()))
	}
	if plugin.Name == "" {
		plugin.Name = name
	}
	plugin.Enabled = rec.Enabled
	plugin.Source = rec.Source
	plugin.Digest = rec.Digest
	plugin.ActualDigest = actualDigest
	plugin.IntegrityValid = true
	if rec.Version != "" && plugin.Manifest.Version == "" {
		plugin.Manifest.Version = rec.Version
	}
	return plugin
}

func validateManagedPluginDataDir(root, dataDir string) error {
	info, err := os.Lstat(dataDir)
	if err != nil {
		return fmt.Errorf("plugin data directory is unavailable: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("plugin data path must be a regular directory")
	}
	if err := resolvedStayWithin(root, dataDir); err != nil {
		return fmt.Errorf("plugin data path escapes the managed data root: %w", err)
	}
	return nil
}

// EnabledSkillRoots returns skills/ directories for enabled plugins.
func (s *Store) EnabledSkillRoots() []string {
	plugins, err := s.List()
	if err != nil {
		return nil
	}
	var roots []string
	for _, plugin := range plugins {
		if plugin == nil || !plugin.Enabled || plugin.SkillRoot == "" {
			continue
		}
		roots = append(roots, plugin.SkillRoot)
	}
	return roots
}

// PluginMCPServers returns plugin-scope MCP configs for enabled plugins.
// Per-server disabled entries are retained with enabled=false.
func (s *Store) PluginMCPServers() []mcpclient.ScopedMcpServerConfig {
	plugins, err := s.List()
	if err != nil {
		return nil
	}
	var out []mcpclient.ScopedMcpServerConfig
	for _, plugin := range plugins {
		if plugin == nil || !plugin.Enabled {
			continue
		}
		for _, server := range plugin.MCPServers {
			if server.Invalid != nil {
				continue
			}
			out = append(out, mcpclient.ScopedMcpServerConfig{
				Name:   server.Identity,
				Scope:  mcpclient.ScopePlugin,
				Config: server.Config,
			})
		}
	}
	return out
}

// SetEnabled toggles the whole plugin.
func (s *Store) SetEnabled(name string, enabled bool) (*InstalledPlugin, error) {
	var plugin *InstalledPlugin
	err := s.withLock(func() error {
		state, err := s.readStateUnlocked()
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		rec, ok := state.Plugins[name]
		if !ok {
			return fmt.Errorf("%w: %s", ErrNotFound, name)
		}
		if enabled {
			loaded := s.loadRecord(name, rec)
			if !loaded.IntegrityValid {
				return fmt.Errorf("%w: %s", ErrDigestChanged, name)
			}
		}
		rec.Enabled = enabled
		state.Plugins[name] = rec
		if err := s.writeStateUnlocked(state); err != nil {
			return err
		}
		plugin = s.loadRecord(name, rec)
		return nil
	})
	if err == nil {
		s.notify()
	}
	return plugin, err
}

// SetServerEnabled toggles one MCP server inside an installed plugin.
func (s *Store) SetServerEnabled(name, shortName string, enabled bool) (*InstalledPlugin, error) {
	var plugin *InstalledPlugin
	err := s.withLock(func() error {
		state, err := s.readStateUnlocked()
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		rec, ok := state.Plugins[name]
		if !ok {
			return fmt.Errorf("%w: %s", ErrNotFound, name)
		}
		loaded := s.loadRecord(name, rec)
		found := false
		for _, server := range loaded.MCPServers {
			if server.ShortName == shortName {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("%w: server %s", ErrNotFound, shortName)
		}
		if rec.MCPServers == nil {
			rec.MCPServers = map[string]MCPServerToggle{}
		}
		rec.MCPServers[shortName] = MCPServerToggle{Enabled: enabled}
		state.Plugins[name] = rec
		if err := s.writeStateUnlocked(state); err != nil {
			return err
		}
		plugin = s.loadRecord(name, rec)
		return nil
	})
	if err == nil {
		s.notify()
	}
	return plugin, err
}
