package pluginauth

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"awaken/src/desktopdata"
	"awaken/src/filestore"
)

type accountIndexEntry struct {
	PluginName  string `json:"pluginName"`
	SetupID     string `json:"setupId"`
	Fingerprint string `json:"fingerprint"`
	Identity    string `json:"credentialRef"`
	Revision    int64  `json:"revision"`
	UpdatedAt   string `json:"updatedAt"`
	Tombstone   bool   `json:"pendingCleanup,omitempty"`
}

type accountIndexFile struct {
	Version  int                          `json:"version"`
	Accounts map[string]accountIndexEntry `json:"accounts"`
}

type accountIndexStore struct {
	path string
	mu   sync.Mutex
}

func newAccountIndexStore() *accountIndexStore {
	root := desktopdata.AwakenHomeRoot()
	if root == "" {
		return nil
	}
	return &accountIndexStore{path: filepath.Join(root, "auth", "plugin-accounts.json")}
}

func accountIndexKey(pluginName, setupID string) string { return pluginName + "\x00" + setupID }

func (s *accountIndexStore) readLocked() (accountIndexFile, error) {
	file := accountIndexFile{Version: 1, Accounts: map[string]accountIndexEntry{}}
	if s == nil {
		return file, nil
	}
	err := filestore.ReadJSON(s.path, &file)
	if errors.Is(err, os.ErrNotExist) {
		return file, nil
	}
	if err != nil {
		return accountIndexFile{}, err
	}
	if file.Version != 1 {
		return accountIndexFile{}, errors.New("unsupported plugin account index version")
	}
	if file.Accounts == nil {
		file.Accounts = map[string]accountIndexEntry{}
	}
	return file, nil
}

func (s *accountIndexStore) writeLocked(file accountIndexFile) error {
	if s == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	file.Version = 1
	encoded, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	return filestore.WriteFileAtomic(s.path, append(encoded, '\n'))
}

func (s *accountIndexStore) upsert(pluginName, setupID, fingerprint, identity string) (accountIndexEntry, bool, error) {
	if s == nil {
		return accountIndexEntry{}, false, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	file, err := s.readLocked()
	if err != nil {
		return accountIndexEntry{}, false, err
	}
	var previous accountIndexEntry
	hadPrevious := false
	for _, candidate := range file.Accounts {
		if candidate.PluginName == pluginName && candidate.SetupID == setupID && (!hadPrevious || candidate.Revision > previous.Revision) {
			previous, hadPrevious = candidate, true
		}
	}
	revision := previous.Revision + 1
	file.Accounts[identity] = accountIndexEntry{PluginName: pluginName, SetupID: setupID, Fingerprint: fingerprint, Identity: identity, Revision: revision, UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	if err := s.writeLocked(file); err != nil {
		return accountIndexEntry{}, false, err
	}
	return previous, hadPrevious, nil
}

func (s *accountIndexStore) entries() ([]accountIndexEntry, error) {
	if s == nil {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	file, err := s.readLocked()
	if err != nil {
		return nil, err
	}
	result := make([]accountIndexEntry, 0, len(file.Accounts))
	for _, entry := range file.Accounts {
		result = append(result, entry)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].PluginName != result[j].PluginName {
			return result[i].PluginName < result[j].PluginName
		}
		return result[i].SetupID < result[j].SetupID
	})
	return result, nil
}

func (s *accountIndexStore) removeOrTombstone(entry accountIndexEntry, tombstone bool) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	file, err := s.readLocked()
	if err != nil {
		return err
	}
	current, ok := file.Accounts[entry.Identity]
	if !ok || current.Identity != entry.Identity {
		return nil
	}
	if tombstone {
		current.Tombstone = true
		current.Revision++
		current.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		file.Accounts[entry.Identity] = current
	} else {
		delete(file.Accounts, entry.Identity)
	}
	return s.writeLocked(file)
}
