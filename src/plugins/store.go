package plugins

import (
	"crypto/sha256"
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
	"awaken/src/plugins/dialect"
	"awaken/src/plugins/ir"
	pluginvariables "awaken/src/plugins/variables"
	"awaken/src/skills"
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
	secrets              pluginvariables.SecretStore
	setupReadiness       SetupReadinessProvider
	mu                   sync.Mutex
	changeMu             sync.Mutex
	corruptErr           error
	onChange             func()
	changeListeners      map[uint64]func()
	nextChangeListenerID uint64
	revision             uint64
	leaseMu              sync.Mutex
	activeLeases         map[string]RuntimeLease
	nextLeaseID          uint64
}

// SetSetupReadinessProvider installs the host-owned readiness projection used
// for OAuth, CLI and external setup requirements. The provider exposes state
// only; credentials remain in their owning runtime.
func (s *Store) SetSetupReadinessProvider(provider SetupReadinessProvider) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.setupReadiness = provider
	s.mu.Unlock()
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
	return NewStoreAtWithTrashAndSecrets(home, adapter, pluginvariables.NewPersistentSecretStore())
}

// NewStoreAtWithTrashAndSecrets is the fully injectable constructor used by
// variable lifecycle tests so test secrets never reach the OS keychain.
func NewStoreAtWithTrashAndSecrets(home string, adapter trash.TrashAdapter, secrets pluginvariables.SecretStore) *Store {
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
		secrets:         secrets,
		changeListeners: make(map[uint64]func()),
		activeLeases:    make(map[string]RuntimeLease),
	}
	if err := store.ensureDirs(); err != nil {
		store.corruptErr = err
		return store
	}
	if _, err := store.readStateUnlocked(); err != nil && !errors.Is(err, os.ErrNotExist) {
		store.corruptErr = err
	}
	// Recovery failures preserve their backups and are surfaced again on the
	// next explicit mutation/recovery attempt. Catalog reads remain available.
	_ = store.RecoverPendingOperations()
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
		s.pluginsRoot,
		s.packagesRoot,
		s.dataRoot,
		s.stagingRoot,
	} {
		if err := filestore.EnsureDir(dir); err != nil {
			return err
		}
		info, err := os.Lstat(dir)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("plugin store directory must be a regular directory: %s", dir)
		}
		if err := resolvedStayWithin(s.home, dir); err != nil {
			return fmt.Errorf("plugin store directory escapes AWAKEN_HOME: %s: %w", dir, err)
		}
	}
	return nil
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

func (s *Store) withMutationLock(fn func() error) error {
	return s.withLock(func() error {
		s.leaseMu.Lock()
		active := len(s.activeLeases)
		s.leaseMu.Unlock()
		if active > 0 {
			return fmt.Errorf("%w: %d active run lease(s); cancel or finish them before changing plugins", ErrRuntimeLeaseActive, active)
		}
		if err := s.recoverPendingOperationsLocked(); err != nil {
			return err
		}
		return fn()
	})
}

func (s *Store) AcquireRuntimeLease() (RuntimeLease, error) {
	if s == nil {
		return RuntimeLease{}, ErrHomeRequired
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.readStateUnlocked()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return RuntimeLease{}, err
	}
	encoded, err := json.Marshal(state)
	if err != nil {
		return RuntimeLease{}, err
	}
	generation := sha256.Sum256(encoded)
	s.leaseMu.Lock()
	s.nextLeaseID++
	lease := RuntimeLease{ID: fmt.Sprintf("pluginlease_%d", s.nextLeaseID), Generation: fmt.Sprintf("sha256:%x", generation[:])}
	s.activeLeases[lease.ID] = lease
	s.leaseMu.Unlock()
	return lease, nil
}

func (s *Store) ReleaseRuntimeLease(id string) {
	if s == nil || strings.TrimSpace(id) == "" {
		return
	}
	s.leaseMu.Lock()
	delete(s.activeLeases, id)
	s.leaseMu.Unlock()
}

func (s *Store) ActiveRuntimeLeases() int {
	if s == nil {
		return 0
	}
	s.leaseMu.Lock()
	defer s.leaseMu.Unlock()
	return len(s.activeLeases)
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
	if state.Version == 1 {
		state = migrateStateV1(state)
	} else if state.Version != StateVersion {
		return StateFile{}, fmt.Errorf("%w: unsupported version %d", ErrStateCorrupt, state.Version)
	}
	if state.Plugins == nil {
		state.Plugins = map[string]Record{}
	}
	return state, nil
}

func migrateStateV1(state StateFile) StateFile {
	state.Version = StateVersion
	for name, record := range state.Plugins {
		if record.Dialect == "" {
			record.Dialect = "agent_plugins"
		}
		if record.ManifestPath == "" {
			record.ManifestPath = "plugin.json"
		}
		if record.InstallationID == "" {
			sum := sha256.Sum256([]byte(name + "\n" + record.Digest))
			record.InstallationID = fmt.Sprintf("migrated-%x", sum[:8])
		}
		if record.DisplayName == "" {
			record.DisplayName = name
		}
		state.Plugins[name] = record
	}
	return state
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
	s.revision++
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

func (s *Store) Revision() uint64 {
	if s == nil {
		return 0
	}
	s.changeMu.Lock()
	defer s.changeMu.Unlock()
	return s.revision
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
			InstalledAt: rec.InstalledAt, Builtin: rec.Builtin,
			BrandIcon: rec.BrandIcon, BuiltinOrder: rec.BuiltinOrder,
			DisplayName: rec.DisplayName,
			Diagnostics: []Diagnostic{errorDiag("plugin_digest_changed", message)},
		}
	}
	var plugin *InstalledPlugin
	if rec.Dialect == "" || rec.Dialect == ir.DialectAgentPlugins {
		plugin = LoadPackage(root, dataDir, rec)
		plugin.Dialect = ir.DialectAgentPlugins
		plugin.ManifestPath = "plugin.json"
	} else {
		candidate := ir.ManifestCandidate{Dialect: rec.Dialect, ManifestPath: rec.ManifestPath, Manifestless: rec.ManifestPath == "", IdentityHint: name, Valid: true}
		normalized, parseErr := dialect.NewDefaultRegistry().Parse(root, candidate, dialect.ParseOptions{PluginDigest: actualDigest})
		if parseErr != nil {
			return &InstalledPlugin{
				Name: name, Root: root, DataDir: dataDir, Enabled: rec.Enabled, Source: rec.Source,
				Digest: rec.Digest, ActualDigest: actualDigest, InstalledAt: rec.InstalledAt,
				Dialect: rec.Dialect, ManifestPath: rec.ManifestPath, InstallationID: rec.InstallationID,
				DisplayName: rec.DisplayName, Builtin: rec.Builtin,
				BrandIcon: rec.BrandIcon, BuiltinOrder: rec.BuiltinOrder,
				Diagnostics: []Diagnostic{errorDiag("plugin_manifest_invalid", parseErr.Error())},
			}
		}
		manifest := Manifest{
			Name: name, Version: normalized.Identity.Version, Description: normalized.Identity.Description,
			Homepage: normalized.Identity.Homepage, Repository: normalized.Identity.Repository, License: normalized.Identity.License,
			Keywords: append([]string(nil), normalized.Identity.Keywords...),
		}
		if normalized.Identity.Author != nil {
			manifest.Author = &Author{Name: normalized.Identity.Author.Name, Email: normalized.Identity.Author.Email, URL: normalized.Identity.Author.URL}
		}
		plugin = &InstalledPlugin{
			Name: name, Root: root, DataDir: dataDir, Enabled: rec.Enabled, Source: rec.Source,
			Digest: rec.Digest, InstalledAt: rec.InstalledAt, Manifest: manifest,
			Dialect: rec.Dialect, ManifestPath: rec.ManifestPath, InstallationID: rec.InstallationID,
			DisplayName: normalized.Identity.DisplayName, Components: normalized.Components, Setup: normalized.Setup, Diagnostics: normalized.Diagnostics,
		}
	}
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
	plugin.Dialect = rec.Dialect
	if plugin.Dialect == "" {
		plugin.Dialect = ir.DialectAgentPlugins
	}
	plugin.ManifestPath = rec.ManifestPath
	if plugin.ManifestPath == "" && plugin.Dialect == ir.DialectAgentPlugins {
		plugin.ManifestPath = "plugin.json"
	}
	plugin.InstallationID = rec.InstallationID
	plugin.DisplayName = firstNonEmptyPlugin(rec.DisplayName, plugin.DisplayName, plugin.Manifest.Name)
	plugin.Builtin = rec.Builtin
	plugin.BrandIcon = rec.BrandIcon
	plugin.BuiltinOrder = rec.BuiltinOrder
	if len(plugin.Components) == 0 {
		candidate := ir.ManifestCandidate{Dialect: plugin.Dialect, ManifestPath: plugin.ManifestPath, Manifestless: plugin.ManifestPath == "", IdentityHint: name, Valid: true}
		if normalized, parseErr := dialect.NewDefaultRegistry().Parse(root, candidate, dialect.ParseOptions{PluginDigest: actualDigest}); parseErr == nil {
			plugin.Components = normalized.Components
			plugin.Setup = normalized.Setup
		}
	}
	if len(plugin.Setup) == 0 {
		candidate := ir.ManifestCandidate{Dialect: plugin.Dialect, ManifestPath: plugin.ManifestPath, Manifestless: plugin.ManifestPath == "", IdentityHint: name, Valid: true}
		if normalized, parseErr := dialect.NewDefaultRegistry().Parse(root, candidate, dialect.ParseOptions{PluginDigest: actualDigest}); parseErr == nil {
			plugin.Setup = normalized.Setup
		}
	}
	if plugin.Dialect != ir.DialectAgentPlugins {
		servers, diagnostics := s.loadVendorMCPServers(plugin, rec)
		plugin.MCPServers = servers
		plugin.Diagnostics = append(plugin.Diagnostics, diagnostics...)
	} else {
		for index := range plugin.MCPServers {
			server := &plugin.MCPServers[index]
			for _, component := range plugin.Components {
				if component.Kind != ir.KindMCPServer || component.MCP == nil || component.MCP.ShortName != server.ShortName {
					continue
				}
				if override, ok := rec.Components[component.LogicalID]; ok {
					if override.Desired != "" {
						server.Enabled = override.Desired == "enabled"
						server.Config.Enabled = boolPointer(server.Enabled)
					}
					for toolName, toolEnabled := range override.ToolOverrides {
						if !toolEnabled {
							server.Config.DisabledTools = append(server.Config.DisabledTools, toolName)
						}
					}
					sort.Strings(server.Config.DisabledTools)
				}
				break
			}
		}
	}
	if !rec.Enabled {
		for index := range plugin.MCPServers {
			plugin.MCPServers[index].Enabled = false
			plugin.MCPServers[index].Config.Enabled = boolPointer(false)
		}
	}
	s.attachMCPSetupBindings(name, plugin, rec)
	plugin.ComponentState, plugin.ConfigurationState, plugin.AuthenticationState, plugin.CompatibilityState = s.componentStates(name, plugin.Components, plugin.Setup, rec)
	for index := range plugin.MCPServers {
		server := &plugin.MCPServers[index]
		for _, state := range plugin.ComponentState {
			if state.Component.MCP == nil || state.Component.MCP.ShortName != server.ShortName {
				continue
			}
			if state.Configuration != "ready" || state.Authentication != "ready" && state.Authentication != "not_required" {
				server.Enabled = false
				server.Config.Enabled = boolPointer(false)
			}
			break
		}
	}
	if rec.Version != "" && plugin.Manifest.Version == "" {
		plugin.Manifest.Version = rec.Version
	}
	return plugin
}

func (s *Store) attachMCPSetupBindings(pluginName string, plugin *InstalledPlugin, record Record) {
	if plugin == nil {
		return
	}
	provider, _ := s.setupReadiness.(SetupCredentialReferenceProvider)
	componentByID := make(map[string]ir.Component, len(plugin.Components))
	for _, component := range plugin.Components {
		componentByID[component.LogicalID] = component
	}
	serverByShortName := make(map[string]*LoadedServer, len(plugin.MCPServers))
	for index := range plugin.MCPServers {
		serverByShortName[plugin.MCPServers[index].ShortName] = &plugin.MCPServers[index]
	}
	for _, requirement := range plugin.Setup {
		for _, binding := range requirement.Bindings {
			component := componentByID[binding.ComponentID]
			if component.MCP == nil {
				continue
			}
			// Generic OAuth mcp_transport is attached as an OAuthHandler by the
			// broker. Header/env bindings and Secret transport bindings use the
			// runtime secret resolver below.
			if binding.Target == ir.SetupBindingMCPTransport && requirement.Kind != ir.SetupSecret {
				continue
			}
			server := serverByShortName[component.MCP.ShortName]
			if server == nil || server.Invalid != nil {
				continue
			}
			if requirement.Kind == ir.SetupConfiguration {
				value, ok := setupConfigurationValue(record, requirement.Field)
				if !ok {
					continue
				}
				switch binding.Target {
				case ir.SetupBindingProcessEnv:
					if server.Config.Stdio == nil {
						continue
					}
					if server.Config.Stdio.Env == nil {
						server.Config.Stdio.Env = map[string]string{}
					}
					server.Config.Stdio.Env[binding.Name] = value
				case ir.SetupBindingHTTPHeader:
					target := server.Config.HTTP
					if target == nil {
						target = server.Config.SSE
					}
					if target == nil {
						continue
					}
					if target.Headers == nil {
						target.Headers = map[string]string{}
					}
					target.Headers[binding.Name] = value
				}
				continue
			}
			if provider == nil {
				continue
			}
			ref, err := provider.SetupCredentialReference(pluginName, requirement, binding)
			if err != nil {
				continue
			}
			server.Config.SecretBindings = append(server.Config.SecretBindings, mcpclient.RuntimeSecretBinding{Target: string(binding.Target), Name: binding.Name, Ref: ref})
			switch binding.Target {
			case ir.SetupBindingProcessEnv:
				if server.Config.Stdio != nil {
					delete(server.Config.Stdio.Env, binding.Name)
				}
			case ir.SetupBindingHTTPHeader:
				if server.Config.HTTP != nil {
					delete(server.Config.HTTP.Headers, binding.Name)
				}
				if server.Config.SSE != nil {
					delete(server.Config.SSE.Headers, binding.Name)
				}
			}
		}
	}
}

func setupConfigurationValue(record Record, field *ir.VariableSpec) (string, bool) {
	if field == nil {
		return "", false
	}
	if stored, ok := record.Variables[field.Key]; ok && stored.Value != nil {
		return variableString(stored.Value), true
	}
	if field.Default != nil {
		return variableString(field.Default), true
	}
	return "", false
}

// RefreshSetupReadiness republishes runtime projections after a host-owned
// authorization state changes. It does not mutate the installed plugin record.
func (s *Store) RefreshSetupReadiness() {
	if s != nil {
		s.notify()
	}
}

func firstNonEmptyPlugin(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
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

// EnabledSkillSelections returns exact Skill package roots after applying the
// plugin and component desired-state layers. This is the production source
// for vendor layouts; Skill discovery remains the parser/runtime owner.
func (s *Store) EnabledSkillSelections() []skills.PluginSkillSelection {
	plugins, err := s.List()
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	var selections []skills.PluginSkillSelection
	for _, plugin := range plugins {
		for _, selection := range enabledSkillSelectionsForPlugin(plugin) {
			root := selection.PackageRoot
			key := plugin.Name + "\x00" + filepath.Clean(root)
			if seen[key] {
				continue
			}
			seen[key] = true
			selections = append(selections, selection)
		}
	}
	sort.Slice(selections, func(i, j int) bool {
		if selections[i].PluginName != selections[j].PluginName {
			return selections[i].PluginName < selections[j].PluginName
		}
		return selections[i].PackageRoot < selections[j].PackageRoot
	})
	return selections
}

func enabledSkillSelectionsForPlugin(plugin *InstalledPlugin) []skills.PluginSkillSelection {
	if plugin == nil || !plugin.Enabled || !plugin.IntegrityValid {
		return nil
	}
	var selections []skills.PluginSkillSelection
	for _, state := range plugin.ComponentState {
		component := state.Component
		if component.Kind != ir.KindSkill || component.Skill == nil || state.DesiredState != "enabled" || !state.Switchable || state.Configuration != "ready" {
			continue
		}
		root := filepath.Join(plugin.Root, filepath.FromSlash(component.Skill.Path))
		if err := resolvedStayWithin(plugin.Root, root); err != nil || !isDirectory(root) {
			continue
		}
		selections = append(selections, skills.PluginSkillSelection{PluginName: plugin.Name, PackageRoot: root})
	}
	return selections
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
				Name:           server.Identity,
				Scope:          mcpclient.ScopePlugin,
				SourceRevision: plugin.Digest,
				Config:         server.Config,
			})
		}
	}
	return out
}

// SetEnabled toggles the whole plugin.
func (s *Store) SetEnabled(name string, enabled bool) (*InstalledPlugin, error) {
	var plugin *InstalledPlugin
	err := s.withMutationLock(func() error {
		state, err := s.readStateUnlocked()
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		rec, ok := state.Plugins[name]
		if !ok {
			return fmt.Errorf("%w: %s", ErrNotFound, name)
		}
		rec.Enabled = enabled
		loaded := s.loadRecord(name, rec)
		if enabled && !loaded.IntegrityValid {
			return fmt.Errorf("%w: %s", ErrDigestChanged, name)
		}
		state.Plugins[name] = rec
		if err := s.writeStateUnlocked(state); err != nil {
			return err
		}
		plugin = loaded
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
	err := s.withMutationLock(func() error {
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
