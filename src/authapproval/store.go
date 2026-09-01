// Package authapproval persists the user's observation that an OAuth flow is
// waiting for an organization administrator. It stores no credential or
// browser content; the fingerprint only prevents stale approval state from
// surviving an authorization contract change.
package authapproval

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"awaken/src/desktopdata"
	"awaken/src/filestore"
)

type Entry struct {
	Fingerprint string `json:"fingerprint"`
	ObservedAt  string `json:"observedAt"`
}

type stateFile struct {
	Version int              `json:"version"`
	Items   map[string]Entry `json:"items"`
}

type Store struct {
	path string
	mu   sync.Mutex
}

// RequiredByProvider recognizes only explicit administrator-approval signals.
// Generic consent_required/interaction_required values are intentionally not
// classified because they can also mean ordinary user consent or MFA.
func RequiredByProvider(code, description string) bool {
	code = strings.ToLower(strings.TrimSpace(code))
	switch code {
	case "admin_consent_required", "administrator_approval_required", "approval_required":
		return true
	}
	description = strings.ToLower(description)
	for _, marker := range []string{
		"admin consent required",
		"administrator approval required",
		"requires approval by an administrator",
		"aadsts90094",
		"aadsts900941",
	} {
		if strings.Contains(description, marker) {
			return true
		}
	}
	return false
}

func NewStore(filename string) *Store {
	root := desktopdata.AwakenHomeRoot()
	filename = filepath.Base(strings.TrimSpace(filename))
	if root == "" || filename == "." || filename == "" {
		return nil
	}
	return &Store{path: filepath.Join(root, "auth", filename)}
}

func (s *Store) Mark(key, fingerprint string) error {
	if s == nil {
		return nil
	}
	key, fingerprint = strings.TrimSpace(key), strings.TrimSpace(fingerprint)
	if key == "" || fingerprint == "" {
		return errors.New("approval state key and fingerprint are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	file, err := s.readLocked()
	if err != nil {
		return err
	}
	file.Items[key] = Entry{Fingerprint: fingerprint, ObservedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	return s.writeLocked(file)
}

func (s *Store) Matches(key, fingerprint string) (bool, error) {
	if s == nil {
		return false, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	file, err := s.readLocked()
	if err != nil {
		return false, err
	}
	entry, ok := file.Items[strings.TrimSpace(key)]
	return ok && entry.Fingerprint == strings.TrimSpace(fingerprint), nil
}

func (s *Store) Clear(key string) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	file, err := s.readLocked()
	if err != nil {
		return err
	}
	key = strings.TrimSpace(key)
	if _, ok := file.Items[key]; !ok {
		return nil
	}
	delete(file.Items, key)
	return s.writeLocked(file)
}

func (s *Store) ClearPrefix(prefix string) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	file, err := s.readLocked()
	if err != nil {
		return err
	}
	changed := false
	for key := range file.Items {
		if strings.HasPrefix(key, prefix) {
			delete(file.Items, key)
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return s.writeLocked(file)
}

func (s *Store) readLocked() (stateFile, error) {
	file := stateFile{Version: 1, Items: map[string]Entry{}}
	err := filestore.ReadJSON(s.path, &file)
	if errors.Is(err, os.ErrNotExist) {
		return file, nil
	}
	if err != nil {
		return stateFile{}, err
	}
	if file.Version != 1 {
		return stateFile{}, errors.New("unsupported OAuth approval state version")
	}
	if file.Items == nil {
		file.Items = map[string]Entry{}
	}
	return file, nil
}

func (s *Store) writeLocked(file stateFile) error {
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
