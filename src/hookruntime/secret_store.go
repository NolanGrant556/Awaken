package hookruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const maxHookSecretsBytes = 256 * 1024

// FileSecretStore is a Core-only reference resolver. The document is never
// exposed through Hook status APIs and must be owner-readable only.
type FileSecretStore struct{ path string }

func NewFileSecretStore(awakenRoot string) *FileSecretStore {
	return &FileSecretStore{path: filepath.Join(filepath.Clean(awakenRoot), "config", "hook-secrets.json")}
}

func (s *FileSecretStore) ResolveHookSecret(_ context.Context, ref string) (string, error) {
	if s == nil || !hookIDPattern.MatchString(strings.TrimSpace(ref)) {
		return "", errors.New("invalid Hook secret reference")
	}
	info, err := os.Lstat(s.path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 || info.Size() > maxHookSecretsBytes {
		return "", errors.New("Hook secret store must be a regular owner-only file")
	}
	file, err := os.Open(s.path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxHookSecretsBytes+1))
	if err != nil || len(data) > maxHookSecretsBytes || !utf8.Valid(data) {
		return "", errors.New("invalid Hook secret store")
	}
	var document struct {
		Version int               `json:"version"`
		Secrets map[string]string `json:"secrets"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil || document.Version != 1 {
		return "", errors.New("invalid Hook secret store")
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return "", errors.New("invalid Hook secret store")
	}
	value, ok := document.Secrets[ref]
	if !ok || value == "" || len(value) > 16*1024 || strings.ContainsAny(value, "\r\n\x00") {
		return "", fmt.Errorf("Hook secret %q unavailable", ref)
	}
	return value, nil
}
