package hookruntime

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"awaken/src/filestore"
)

type TrustEntry struct {
	SHA256    string    `json:"sha256"`
	TrustedAt time.Time `json:"trustedAt"`
}
type ProjectTrust struct {
	Sources map[string]TrustEntry `json:"sources"`
}
type TrustDocument struct {
	Version  int                     `json:"version"`
	Projects map[string]ProjectTrust `json:"projects"`
}
type TrustStore struct {
	path string
	mu   sync.Mutex
}

func NewTrustStore(path string) *TrustStore { return &TrustStore{path: path} }

func (s *TrustStore) load() (TrustDocument, error) {
	var document TrustDocument
	err := filestore.ReadJSON(s.path, &document)
	if errors.Is(err, os.ErrNotExist) {
		return TrustDocument{Version: 1, Projects: map[string]ProjectTrust{}}, nil
	}
	if err != nil {
		return TrustDocument{}, err
	}
	if document.Version != 1 {
		return TrustDocument{}, errors.New("hookruntime: unsupported trust document version")
	}
	if document.Projects == nil {
		document.Projects = map[string]ProjectTrust{}
	}
	return document, nil
}

func (s *TrustStore) IsTrusted(projectID, sourcePath, digest string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	document, err := s.load()
	if err != nil {
		return false, err
	}
	project, ok := document.Projects[strings.TrimSpace(projectID)]
	if !ok {
		return false, nil
	}
	entry, ok := project.Sources[filepath.Clean(sourcePath)]
	return ok && entry.SHA256 == digest, nil
}

func (s *TrustStore) Trust(projectID, sourcePath, digest string) error {
	projectID, digest = strings.TrimSpace(projectID), strings.TrimSpace(digest)
	if projectID == "" || digest == "" || !filepath.IsAbs(sourcePath) {
		return errors.New("hookruntime: projectId, absolute sourcePath and digest are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	document, err := s.load()
	if err != nil {
		return err
	}
	project := document.Projects[projectID]
	if project.Sources == nil {
		project.Sources = map[string]TrustEntry{}
	}
	project.Sources[filepath.Clean(sourcePath)] = TrustEntry{SHA256: digest, TrustedAt: time.Now().UTC()}
	document.Projects[projectID] = project
	if err := filestore.EnsureDir(filepath.Dir(s.path)); err != nil {
		return err
	}
	return writeTrustDocument(s.path, document)
}

func (s *TrustStore) Revoke(projectID, sourcePath string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	document, err := s.load()
	if err != nil {
		return err
	}
	project, ok := document.Projects[strings.TrimSpace(projectID)]
	if !ok {
		return nil
	}
	delete(project.Sources, filepath.Clean(sourcePath))
	if len(project.Sources) == 0 {
		delete(document.Projects, projectID)
	} else {
		document.Projects[projectID] = project
	}
	if err := filestore.EnsureDir(filepath.Dir(s.path)); err != nil {
		return err
	}
	return writeTrustDocument(s.path, document)
}

func writeTrustDocument(path string, document TrustDocument) error {
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return err
	}
	return filestore.WriteFileAtomic(path, append(data, '\n'))
}
