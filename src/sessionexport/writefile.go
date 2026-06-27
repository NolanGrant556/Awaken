package sessionexport

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Format string

const (
	FormatPlainText Format = "plain_text"
	FormatJSONL     Format = "jsonl"
)

const ExportFileMode os.FileMode = 0o600

func WriteFile(targetPath string, content string, format Format) (string, error) {
	if strings.TrimSpace(targetPath) == "" {
		return "", errors.New("export path is required")
	}
	if isDirectoryTarget(targetPath) {
		return "", fmt.Errorf("export path must be a file, got directory: %s", targetPath)
	}

	finalPath, err := EnsureExtension(targetPath, format)
	if err != nil {
		return "", err
	}
	if isDirectoryTarget(finalPath) {
		return "", fmt.Errorf("export path must be a file, got directory: %s", finalPath)
	}

	if err := os.WriteFile(finalPath, []byte(content), ExportFileMode); err != nil {
		return "", err
	}
	return finalPath, nil
}

func EnsureExtension(targetPath string, format Format) (string, error) {
	extension, err := format.Extension()
	if err != nil {
		return "", err
	}

	dir, base := filepath.Split(targetPath)
	current := filepath.Ext(base)
	if current == "" || current == base {
		return targetPath + extension, nil
	}

	stem := strings.TrimSuffix(base, current)
	if stem == "" {
		return targetPath + extension, nil
	}
	return filepath.Join(dir, stem+extension), nil
}

func (format Format) Extension() (string, error) {
	switch format {
	case FormatPlainText:
		return ".txt", nil
	case FormatJSONL:
		return ".jsonl", nil
	default:
		return "", fmt.Errorf("unsupported export format: %s", format)
	}
}

func isDirectoryTarget(targetPath string) bool {
	if strings.HasSuffix(targetPath, string(os.PathSeparator)) {
		return true
	}
	info, err := os.Stat(targetPath)
	return err == nil && info.IsDir()
}
