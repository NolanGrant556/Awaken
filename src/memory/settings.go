package memory

import (
	"errors"
	"os"
	"path/filepath"
	"sync"

	"awaken/src/filestore"
)

type Settings struct {
	Enabled         bool `json:"enabled"`
	AutomaticRecall bool `json:"automatic_recall"`
	ProactiveMemory bool `json:"proactive_memory"`
	AutoDream       bool `json:"auto_dream"`
}

type SettingsPatch struct {
	Enabled         *bool `json:"enabled,omitempty"`
	AutomaticRecall *bool `json:"automatic_recall,omitempty"`
	ProactiveMemory *bool `json:"proactive_memory,omitempty"`
	AutoDream       *bool `json:"auto_dream,omitempty"`
}

type SettingsDocument = filestore.VersionedDocument[Settings]

type SettingsStore struct {
	path string
}

var settingsMu sync.Mutex

func DefaultSettings() Settings {
	return Settings{Enabled: true, AutomaticRecall: true, ProactiveMemory: true, AutoDream: true}
}

func normalizeSettings(settings Settings) Settings {
	if settings.Enabled {
		return settings
	}
	settings.AutomaticRecall = false
	settings.ProactiveMemory = false
	settings.AutoDream = false
	return settings
}

func NewSettingsStore() (*SettingsStore, error) {
	home, err := filestore.ResolveAwakenHome("")
	if err != nil {
		return nil, err
	}
	return &SettingsStore{path: filepath.Join(home, "config", "memory.json")}, nil
}

func (s *SettingsStore) Get() (SettingsDocument, error) {
	if s == nil {
		return SettingsDocument{Value: DefaultSettings()}, nil
	}
	settingsMu.Lock()
	defer settingsMu.Unlock()
	var document SettingsDocument
	err := filestore.ReadJSON(s.path, &document)
	if errors.Is(err, os.ErrNotExist) {
		return SettingsDocument{Revision: 0, Value: DefaultSettings()}, nil
	}
	document.Value = normalizeSettings(document.Value)
	return document, err
}

func (s *SettingsStore) Update(expectedRevision uint64, patch SettingsPatch) (SettingsDocument, error) {
	if s == nil {
		return SettingsDocument{}, errors.New("memory: settings store unavailable")
	}
	settingsMu.Lock()
	defer settingsMu.Unlock()
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
		wasEnabled := current.Enabled
		if patch.Enabled != nil {
			current.Enabled = *patch.Enabled
		}
		if patch.AutomaticRecall != nil {
			current.AutomaticRecall = *patch.AutomaticRecall
		}
		if patch.ProactiveMemory != nil {
			current.ProactiveMemory = *patch.ProactiveMemory
		}
		if patch.AutoDream != nil {
			current.AutoDream = *patch.AutoDream
		}
		if patch.Enabled != nil && *patch.Enabled && !wasEnabled {
			current.AutomaticRecall = true
			current.ProactiveMemory = true
			current.AutoDream = true
		}
		return normalizeSettings(current), nil
	})
}
