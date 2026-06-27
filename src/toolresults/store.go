package toolresults

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const DefaultPreviewChars = 4000

var (
	ErrInvalidHandleID          = errors.New("invalid tool result handle id")
	ErrStorageInsideWorkspace   = errors.New("tool result storage directory must be outside workspace")
	ErrStorageDirectoryRequired = errors.New("tool result storage directory is required")
)

type Config struct {
	Dir           string
	WorkspaceRoot string
}

type Handle struct {
	ID            string
	Path          string
	RelativePath  string
	OriginalBytes int
	OriginalChars int
	Preview       string
	Truncated     bool
}

type Store struct {
	dir           string
	workspaceRoot string
}

func NewStore(config Config) *Store {
	return &Store{
		dir:           strings.TrimSpace(config.Dir),
		workspaceRoot: strings.TrimSpace(config.WorkspaceRoot),
	}
}

func DefaultDir() (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil || strings.TrimSpace(cacheDir) == "" {
		cacheDir = os.TempDir()
	}
	if strings.TrimSpace(cacheDir) == "" {
		if err != nil {
			return "", err
		}
		return "", ErrStorageDirectoryRequired
	}
	return filepath.Join(cacheDir, "awaken", "tool-results"), nil
}

func (s *Store) Persist(content string, previewChars int) (Handle, error) {
	baseDir, err := s.baseDir()
	if err != nil {
		return Handle{}, err
	}
	if err := ensureOutsideWorkspace(baseDir, s.workspaceRoot); err != nil {
		return Handle{}, err
	}
	if err := os.MkdirAll(baseDir, 0700); err != nil {
		return Handle{}, err
	}

	preview, truncated := Preview(content, previewChars)
	id, path, err := writeNewHandleFile(baseDir, content)
	if err != nil {
		return Handle{}, err
	}

	return Handle{
		ID:            id,
		Path:          path,
		RelativePath:  filepath.Base(path),
		OriginalBytes: len([]byte(content)),
		OriginalChars: utf8.RuneCountInString(content),
		Preview:       preview,
		Truncated:     truncated,
	}, nil
}

func (s *Store) Read(id string) (string, error) {
	if !validHandleID(id) {
		return "", ErrInvalidHandleID
	}
	baseDir, err := s.baseDir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(baseDir, id+".txt")
	if !isWithinDir(path, baseDir) {
		return "", ErrInvalidHandleID
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func Preview(content string, maxChars int) (string, bool) {
	originalChars := utf8.RuneCountInString(content)
	if maxChars <= 0 {
		return "", originalChars > 0
	}
	if originalChars <= maxChars {
		return content, false
	}
	runes := []rune(content)
	return string(runes[:maxChars]), true
}

func FormatModelContent(handle Handle) string {
	return fmt.Sprintf(
		"<persisted-tool-result>\n"+
			"This tool result exceeded the inline limit and was saved outside the workspace.\n"+
			"handle_id: %s\n"+
			"relative_path: %s\n"+
			"original_bytes: %d\n"+
			"original_chars: %d\n"+
			"truncated: %t\n\n"+
			"preview_chars: %d\n"+
			"preview:\n"+
			"%s\n"+
			"</persisted-tool-result>",
		handle.ID,
		handle.RelativePath,
		handle.OriginalBytes,
		handle.OriginalChars,
		handle.Truncated,
		utf8.RuneCountInString(handle.Preview),
		handle.Preview,
	)
}

func (s *Store) baseDir() (string, error) {
	dir := s.dir
	if dir == "" {
		defaultDir, err := DefaultDir()
		if err != nil {
			return "", err
		}
		dir = defaultDir
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

func writeNewHandleFile(baseDir string, content string) (string, string, error) {
	var lastErr error
	for range 5 {
		id, err := newHandleID()
		if err != nil {
			return "", "", err
		}
		path := filepath.Join(baseDir, id+".txt")
		if !isWithinDir(path, baseDir) {
			return "", "", ErrInvalidHandleID
		}
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			if errors.Is(err, os.ErrExist) {
				lastErr = err
				continue
			}
			return "", "", err
		}
		if _, err := file.WriteString(content); err != nil {
			_ = file.Close()
			return "", "", err
		}
		if err := file.Close(); err != nil {
			return "", "", err
		}
		return id, path, nil
	}
	return "", "", lastErr
}

func newHandleID() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	return "tr_" + hex.EncodeToString(bytes[:]), nil
}

func validHandleID(id string) bool {
	if id == "" || strings.Contains(id, "..") {
		return false
	}
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '_' || r == '-':
		default:
			return false
		}
	}
	return true
}

func ensureOutsideWorkspace(path string, workspaceRoot string) error {
	if strings.TrimSpace(workspaceRoot) == "" {
		return nil
	}
	workspaceAbs, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return err
	}
	workspaceAbs = filepath.Clean(workspaceAbs)
	if isWithinDir(path, workspaceAbs) {
		return ErrStorageInsideWorkspace
	}
	return nil
}

func isWithinDir(path string, dir string) bool {
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	dirAbs, err := filepath.Abs(dir)
	if err != nil {
		return false
	}
	pathAbs = filepath.Clean(pathAbs)
	dirAbs = filepath.Clean(dirAbs)
	rel, err := filepath.Rel(dirAbs, pathAbs)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)))
}
