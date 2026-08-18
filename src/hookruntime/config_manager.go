package hookruntime

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type ConfigMutationRequest struct {
	Action      string
	Source      SourceKind
	ProjectRoot string
	Event       Event
	Matcher     Matcher
	Hook        CommandHook
	HookID      string
	Enabled     *bool
}

type ConfigMutationResult struct {
	Action        string       `json:"action"`
	Source        SourceKind   `json:"source"`
	Path          string       `json:"path"`
	Exists        bool         `json:"exists"`
	Enabled       bool         `json:"enabled"`
	HookCount     int          `json:"hookCount"`
	RequiresTrust bool         `json:"requiresTrust"`
	Hooks         []ReviewHook `json:"hooks"`
}

// MutateConfig is the only model-facing write path for persistent Hook
// configuration. It preserves a valid existing document, validates the whole
// next document before writing, and replaces it atomically. Project and local
// project files remain untrusted after a content change.
func (s *Store) MutateConfig(request ConfigMutationRequest) (ConfigMutationResult, error) {
	if s == nil {
		return ConfigMutationResult{}, errors.New("hookruntime: hook store is not configured")
	}
	path, root, err := s.configMutationTarget(request.Source, request.ProjectRoot)
	if err != nil {
		return ConfigMutationResult{}, err
	}

	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()

	config, exists, err := loadConfigForMutation(path, request.Source)
	if err != nil {
		return ConfigMutationResult{}, err
	}
	if !exists {
		config = Config{Version: ConfigVersion, Enabled: true, Hooks: map[Event][]MatcherGroup{}}
	}

	action := strings.ToLower(strings.TrimSpace(request.Action))
	switch action {
	case "list":
		return mutationResult(action, request.Source, path, exists, config), nil
	case "set_enabled":
		if request.Enabled == nil {
			return ConfigMutationResult{}, errors.New("hookruntime: enabled is required for set_enabled")
		}
		config.Enabled = *request.Enabled
	case "upsert":
		if !validEvent(request.Event) {
			return ConfigMutationResult{}, fmt.Errorf("hookruntime: unsupported event %q", request.Event)
		}
		if strings.TrimSpace(request.Hook.ID) == "" {
			return ConfigMutationResult{}, errors.New("hookruntime: hook.id is required for upsert")
		}
		removeHookByID(&config, request.Hook.ID)
		config.Hooks[request.Event] = appendHookToMatcher(config.Hooks[request.Event], request.Matcher, request.Hook)
	case "delete":
		hookID := strings.TrimSpace(request.HookID)
		if hookID == "" {
			return ConfigMutationResult{}, errors.New("hookruntime: hook_id is required for delete")
		}
		if !removeHookByID(&config, hookID) {
			return ConfigMutationResult{}, fmt.Errorf("hookruntime: hook %q was not found", hookID)
		}
	default:
		return ConfigMutationResult{}, fmt.Errorf("hookruntime: unsupported config action %q", request.Action)
	}

	normalized, err := normalizeConfigForWrite(config)
	if err != nil {
		return ConfigMutationResult{}, err
	}
	config, err = decodeConfig(normalized)
	if err != nil {
		return ConfigMutationResult{}, err
	}
	if err := prepareConfigDirectory(path, root, request.Source); err != nil {
		return ConfigMutationResult{}, err
	}
	mode := os.FileMode(0o644)
	if request.Source == SourceUser || request.Source == SourceProjectLocal {
		mode = 0o600
	}
	if err := atomicWriteConfig(path, normalized, mode); err != nil {
		return ConfigMutationResult{}, err
	}
	return mutationResult(action, request.Source, path, true, config), nil
}

func (s *Store) configMutationTarget(source SourceKind, projectRoot string) (string, string, error) {
	switch source {
	case SourceUser:
		return s.UserConfigPath(), s.root, nil
	case SourceProject, SourceProjectLocal:
		root := filepath.Clean(strings.TrimSpace(projectRoot))
		if root == "." || !filepath.IsAbs(root) {
			return "", "", errors.New("hookruntime: an absolute project root is required")
		}
		canonical, err := filepath.EvalSymlinks(root)
		if err != nil {
			return "", "", fmt.Errorf("hookruntime: resolve project root: %w", err)
		}
		canonical, err = filepath.Abs(canonical)
		if err != nil {
			return "", "", err
		}
		filename := "hooks.json"
		if source == SourceProjectLocal {
			filename = "hooks.local.json"
		}
		return filepath.Join(filepath.Clean(canonical), ".awaken", filename), filepath.Clean(canonical), nil
	default:
		return "", "", fmt.Errorf("hookruntime: source %q is not writable", source)
	}
}

func loadConfigForMutation(path string, source SourceKind) (Config, bool, error) {
	data, canonical, err := readConfigFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Config{}, false, nil
	}
	if err != nil {
		return Config{}, false, err
	}
	expectedDir, resolveErr := filepath.EvalSymlinks(filepath.Dir(path))
	if resolveErr != nil {
		return Config{}, false, resolveErr
	}
	expected := filepath.Join(filepath.Clean(expectedDir), filepath.Base(path))
	if filepath.Clean(canonical) != expected {
		return Config{}, false, errors.New("hookruntime: config resolves to an unexpected path")
	}
	if source == SourceUser {
		info, statErr := os.Stat(canonical)
		if statErr != nil {
			return Config{}, false, statErr
		}
		if info.Mode().Perm()&0o077 != 0 {
			return Config{}, false, errors.New("hookruntime: user hooks.json permissions must not grant group or other access")
		}
	}
	config, err := decodeConfig(data)
	return config, true, err
}

func normalizeConfigForWrite(config Config) ([]byte, error) {
	data, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}
	normalized, err := decodeConfig(data)
	if err != nil {
		return nil, err
	}
	data, err = json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func prepareConfigDirectory(path, root string, source SourceKind) error {
	dir := filepath.Dir(path)
	if source.RequiresProjectTrust() {
		info, err := os.Lstat(dir)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
				return errors.New("hookruntime: .awaken must be a real directory")
			}
			resolved, resolveErr := filepath.EvalSymlinks(dir)
			if resolveErr != nil || !pathWithin(root, resolved) {
				return errors.New("hookruntime: .awaken resolves outside project root")
			}
			return nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("hookruntime: create config directory: %w", err)
	}
	return nil
}

func atomicWriteConfig(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("hookruntime: create temporary config: %w", err)
	}
	tempPath := temp.Name()
	cleanup := func() { _ = os.Remove(tempPath) }
	if err := temp.Chmod(mode); err != nil {
		_ = temp.Close()
		cleanup()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		cleanup()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		cleanup()
		return err
	}
	if err := temp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := replaceConfigFile(tempPath, path); err != nil {
		cleanup()
		return fmt.Errorf("hookruntime: replace config: %w", err)
	}
	if err := syncConfigDirectory(dir); err != nil {
		return fmt.Errorf("hookruntime: sync config directory: %w", err)
	}
	return nil
}

func removeHookByID(config *Config, hookID string) bool {
	removed := false
	for event, groups := range config.Hooks {
		nextGroups := groups[:0]
		for _, group := range groups {
			nextHooks := group.Hooks[:0]
			for _, hook := range group.Hooks {
				if hook.ID == hookID {
					removed = true
					continue
				}
				nextHooks = append(nextHooks, hook)
			}
			if len(nextHooks) > 0 {
				group.Hooks = nextHooks
				nextGroups = append(nextGroups, group)
			}
		}
		if len(nextGroups) == 0 {
			delete(config.Hooks, event)
		} else {
			config.Hooks[event] = nextGroups
		}
	}
	return removed
}

func appendHookToMatcher(groups []MatcherGroup, matcher Matcher, hook CommandHook) []MatcherGroup {
	for index := range groups {
		if sameToolNames(groups[index].Matcher.ToolNames, matcher.ToolNames) {
			groups[index].Hooks = append(groups[index].Hooks, hook)
			return groups
		}
	}
	return append(groups, MatcherGroup{Matcher: matcher, Hooks: []CommandHook{hook}})
}

func sameToolNames(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func mutationResult(action string, source SourceKind, path string, exists bool, config Config) ConfigMutationResult {
	return ConfigMutationResult{
		Action: action, Source: source, Path: filepath.Clean(path), Exists: exists,
		Enabled: config.Enabled, HookCount: countHooks(config), RequiresTrust: source.RequiresProjectTrust(),
		Hooks: reviewHooks(config),
	}
}
