package envguard

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const DefaultCredentialMountBase = "/home/awaken"

var (
	ErrAbsolutePath          = errors.New("credential file path must be relative to home")
	ErrPathTraversal         = errors.New("credential file path must not contain traversal")
	ErrPathEscapesHome       = errors.New("credential file resolves outside home")
	ErrCredentialFileMissing = errors.New("credential file does not exist")
	ErrCredentialFileType    = errors.New("credential file must be a regular file")
)

type HomeRelativePath struct {
	relativePath string
	resolvedPath string
}

func ResolveHomeRelativePath(homeDir, relativePath string) (HomeRelativePath, error) {
	homeDir = strings.TrimSpace(homeDir)
	relativePath = strings.TrimSpace(relativePath)
	if filepath.IsAbs(relativePath) {
		return HomeRelativePath{}, ErrAbsolutePath
	}

	cleanRelative, err := cleanHomeRelativePath(relativePath)
	if err != nil {
		return HomeRelativePath{}, err
	}

	resolvedHome, err := filepath.EvalSymlinks(homeDir)
	if err != nil {
		return HomeRelativePath{}, err
	}
	resolvedHome, err = filepath.Abs(resolvedHome)
	if err != nil {
		return HomeRelativePath{}, err
	}

	candidate := filepath.Join(resolvedHome, filepath.FromSlash(cleanRelative))
	resolvedCandidate, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		if os.IsNotExist(err) {
			return HomeRelativePath{}, ErrCredentialFileMissing
		}
		return HomeRelativePath{}, err
	}
	resolvedCandidate, err = filepath.Abs(resolvedCandidate)
	if err != nil {
		return HomeRelativePath{}, err
	}
	if !isWithinDir(resolvedHome, resolvedCandidate) {
		return HomeRelativePath{}, ErrPathEscapesHome
	}

	info, err := os.Stat(resolvedCandidate)
	if err != nil {
		if os.IsNotExist(err) {
			return HomeRelativePath{}, ErrCredentialFileMissing
		}
		return HomeRelativePath{}, err
	}
	if !info.Mode().IsRegular() {
		return HomeRelativePath{}, ErrCredentialFileType
	}

	return HomeRelativePath{
		relativePath: cleanRelative,
		resolvedPath: resolvedCandidate,
	}, nil
}

func (p HomeRelativePath) RelativePath() string {
	return p.relativePath
}

func (p HomeRelativePath) ResolvedPath() string {
	return p.resolvedPath
}

type CredentialFileMount struct {
	relativePath string
	hostPath     string
	mountPath    string
}

type CredentialFileMetadata struct {
	RelativePath string `json:"relative_path"`
	MountPath    string `json:"mount_path"`
}

func (s *Session) RegisterCredentialFile(homeDir, relativePath string) (CredentialFileMount, error) {
	return s.RegisterCredentialFileWithMountBase(homeDir, relativePath, DefaultCredentialMountBase)
}

func (s *Session) RegisterCredentialFileWithMountBase(homeDir, relativePath, mountBase string) (CredentialFileMount, error) {
	homePath, err := ResolveHomeRelativePath(homeDir, relativePath)
	if err != nil {
		return CredentialFileMount{}, err
	}

	mount := CredentialFileMount{
		relativePath: homePath.RelativePath(),
		hostPath:     homePath.ResolvedPath(),
		mountPath:    joinMountPath(mountBase, homePath.RelativePath()),
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.credentialFiles[mount.relativePath] = credentialFileRegistration(mount)
	return mount, nil
}

func (s *Session) CredentialFileMounts() []CredentialFileMount {
	s.mu.RLock()
	defer s.mu.RUnlock()

	mounts := make([]CredentialFileMount, 0, len(s.credentialFiles))
	for _, mount := range s.credentialFiles {
		mounts = append(mounts, CredentialFileMount(mount))
	}
	sort.Slice(mounts, func(i, j int) bool {
		return mounts[i].relativePath < mounts[j].relativePath
	})
	return mounts
}

func (m CredentialFileMount) RelativePath() string {
	return m.relativePath
}

func (m CredentialFileMount) HostPath() string {
	return m.hostPath
}

func (m CredentialFileMount) MountPath() string {
	return m.mountPath
}

func (m CredentialFileMount) Metadata() CredentialFileMetadata {
	return CredentialFileMetadata{
		RelativePath: m.relativePath,
		MountPath:    m.mountPath,
	}
}

func (m CredentialFileMount) String() string {
	return fmt.Sprintf("credential_file{relative_path:%q mount_path:%q}", m.relativePath, m.mountPath)
}

func (m CredentialFileMount) MarshalJSON() ([]byte, error) {
	return json.Marshal(m.Metadata())
}

type credentialFileRegistration CredentialFileMount

func cleanHomeRelativePath(path string) (string, error) {
	if path == "" || path == "." {
		return "", ErrPathTraversal
	}
	for _, part := range strings.FieldsFunc(path, func(r rune) bool {
		return r == '/' || r == filepath.Separator
	}) {
		if part == ".." {
			return "", ErrPathTraversal
		}
	}
	clean := filepath.Clean(path)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", ErrPathTraversal
	}
	return filepath.ToSlash(clean), nil
}

func isWithinDir(base, target string) bool {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel))
}

func joinMountPath(base, relativePath string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" {
		base = DefaultCredentialMountBase
	}
	return base + "/" + filepath.ToSlash(relativePath)
}
