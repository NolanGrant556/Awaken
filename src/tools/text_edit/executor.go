package textedit

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// atomicWriteFile writes content to path via a same-directory temp file,
// preserves existing permissions when present, fsyncs, then renames atomically.
// On any failure before rename completes, the original file is left untouched.
var atomicWriteFile = atomicWriteFileImpl

func countOccurrences(content, old string) int {
	if old == "" {
		return 0
	}
	return strings.Count(content, old)
}

func applyReplacements(content, old, new string, replaceAll bool) (string, int, error) {
	if old == new {
		return "", 0, fmt.Errorf("old_string and new_string must differ")
	}
	count := countOccurrences(content, old)
	switch {
	case count == 0:
		return "", 0, errTextNotFound
	case count > 1 && !replaceAll:
		return "", count, errTextNotUnique
	case replaceAll:
		return strings.ReplaceAll(content, old, new), count, nil
	default:
		return strings.Replace(content, old, new, 1), 1, nil
	}
}

func fileDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func readFileForEdit(path string) (content string, digest string, mode os.FileMode, err error) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", "", 0, fmt.Errorf("file does not exist: %s", path)
		}
		return "", "", 0, err
	}
	if info.IsDir() {
		return "", "", 0, fmt.Errorf("path is a directory: %s", path)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", "", 0, fmt.Errorf("refusing to edit symlink: %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", 0, err
	}
	return string(data), fileDigest(data), info.Mode().Perm(), nil
}

func atomicWriteFileImpl(path string, content []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if mode == 0 {
		mode = 0o644
	}

	tmp, err := os.CreateTemp(dir, ".text_edit-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()

	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("fsync temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("atomic rename: %w", err)
	}
	cleanup = false

	// Best-effort fsync of the parent directory for durability on POSIX.
	if dirFile, err := os.Open(dir); err == nil {
		_ = dirFile.Sync()
		_ = dirFile.Close()
	}
	return nil
}

func buildUnifiedDiff(path, before, after string) string {
	beforeLines := splitLines(before)
	afterLines := splitLines(after)
	var b strings.Builder
	fmt.Fprintf(&b, "--- a/%s\n", path)
	fmt.Fprintf(&b, "+++ b/%s\n", path)

	// Simple whole-file hunk when content differs. Adequate for tool output;
	// models primarily need before/after visibility, not a perfect Myers diff.
	if before == after {
		return b.String()
	}
	fmt.Fprintf(&b, "@@ -1,%d +1,%d @@\n", len(beforeLines), len(afterLines))
	for _, line := range beforeLines {
		fmt.Fprintf(&b, "-%s\n", line)
	}
	for _, line := range afterLines {
		fmt.Fprintf(&b, "+%s\n", line)
	}
	return strings.TrimSuffix(b.String(), "\n")
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	// Preserve a final empty line representation consistently with Split.
	parts := strings.Split(s, "\n")
	if len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	return parts
}
