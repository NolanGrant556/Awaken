package computer

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"awaken/src/filestore"
)

// Settings holds user-configurable Computer Use preferences. These settings
// are persisted per user and apply to all future runs.
type Settings struct {
	// Enabled controls whether Computer Use is globally available. When false,
	// RunService will not create ComputerBinding and the tools are not exposed.
	Enabled bool `json:"enabled"`
	// AllowBackgroundWindowActions permits input to background windows when the
	// underlying platform supports it.
	AllowBackgroundWindowActions bool `json:"allow_background_window_actions"`
}

// SettingsPatch is a partial update for Settings.
type SettingsPatch struct {
	Enabled                      *bool `json:"enabled,omitempty"`
	AllowBackgroundWindowActions *bool `json:"allow_background_window_actions,omitempty"`
}

// SettingsDocument is a versioned Settings value.
type SettingsDocument = filestore.VersionedDocument[Settings]

// SettingsStore persists Computer Use preferences.
type SettingsStore struct {
	path string
}

var computerSettingsMu sync.Mutex

// DefaultSettings returns the factory default Computer Use settings.
func DefaultSettings() Settings {
	return Settings{
		Enabled:                      false,
		AllowBackgroundWindowActions: false,
	}
}

func normalizeSettings(settings Settings) Settings {
	return settings
}

// NewSettingsStore creates a store backed by a file in the Awaken home config
// directory.
func NewSettingsStore() (*SettingsStore, error) {
	home, err := filestore.ResolveAwakenHome("")
	if err != nil {
		return nil, err
	}
	return &SettingsStore{path: filepath.Join(home, "config", "computer.json")}, nil
}

// Get reads the persisted settings, returning defaults if the file does not
// exist.
func (s *SettingsStore) Get() (SettingsDocument, error) {
	if s == nil {
		return SettingsDocument{Value: DefaultSettings()}, nil
	}
	computerSettingsMu.Lock()
	defer computerSettingsMu.Unlock()
	var document SettingsDocument
	err := filestore.ReadJSON(s.path, &document)
	if errors.Is(err, os.ErrNotExist) {
		return SettingsDocument{Revision: 0, Value: DefaultSettings()}, nil
	}
	document.Value = normalizeSettings(document.Value)
	return document, err
}

// Update applies a patch with optimistic revision control.
func (s *SettingsStore) Update(expectedRevision uint64, patch SettingsPatch) (SettingsDocument, error) {
	if s == nil {
		return SettingsDocument{}, errors.New("computer: settings store unavailable")
	}
	computerSettingsMu.Lock()
	defer computerSettingsMu.Unlock()
	if err := filestore.EnsureDir(filepath.Dir(s.path)); err != nil {
		return SettingsDocument{}, err
	}
	if expectedRevision == 0 {
		var existing SettingsDocument
		if err := filestore.ReadJSON(s.path, &existing); err == nil {
			return SettingsDocument{}, filestore.ErrRevisionConflict
		} else if !errors.Is(err, os.ErrNotExist) {
			return SettingsDocument{}, err
		}
	}
	return filestore.ReplaceJSONWithRevision(s.path, expectedRevision, func(current Settings) (Settings, error) {
		if expectedRevision == 0 {
			current = DefaultSettings()
		}
		current = normalizeSettings(current)
		if patch.Enabled != nil {
			current.Enabled = *patch.Enabled
		}
		if patch.AllowBackgroundWindowActions != nil {
			current.AllowBackgroundWindowActions = *patch.AllowBackgroundWindowActions
		}
		return normalizeSettings(current), nil
	})
}

// DefaultMediaRetentionOptions returns the fixed Computer media lifecycle.
// Retention is intentionally not user-configurable: media is kept for at most
// 24 hours or until the private store reaches 500 MiB.
func DefaultMediaRetentionOptions() RetentionOptions {
	return RetentionOptions{
		MaxAge:          24 * time.Hour,
		MaxTotalBytes:   500 * 1024 * 1024,
		CleanupInterval: 5 * time.Minute,
	}
}
