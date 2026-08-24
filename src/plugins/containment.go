package plugins

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf8"
)

func isValidPluginName(name string) bool {
	if name == "" || len(name) > 64 {
		return false
	}
	if strings.Contains(name, "--") || strings.Contains(name, "..") {
		return false
	}
	first, _ := utf8.DecodeRuneInString(name)
	last, _ := utf8.DecodeLastRuneInString(name)
	if !isASCIIAlphanumeric(first) || !isASCIIAlphanumeric(last) {
		return false
	}
	for i := 0; i < len(name); i++ {
		b := name[i]
		if (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9') || b == '.' || b == '-' {
			continue
		}
		return false
	}
	return true
}

func isASCIIAlphanumeric(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
}

func schemaVersion(schema string) string {
	trimmed := strings.TrimPrefix(schema, SchemaPrefix)
	parts := strings.Split(trimmed, "/")
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}

// resolvePluginRelativePath requires a ./ relative path that stays inside root
// after cleaning. It does not follow the target if it does not exist.
func resolvePluginRelativePath(root, raw, field string) (string, error) {
	rel, err := pluginRelative(raw, field)
	if err != nil {
		return "", err
	}
	resolved := filepath.Join(root, filepath.FromSlash(rel))
	if err := stayWithin(root, resolved); err != nil {
		return "", fmt.Errorf("%s: %w", field, err)
	}
	return resolved, nil
}

func pluginRelative(raw, field string) (string, error) {
	path := strings.TrimSpace(raw)
	if path == "" {
		return "", fmt.Errorf("%s is empty", field)
	}
	if !strings.HasPrefix(path, "./") {
		return "", fmt.Errorf("%s must start with `./`", field)
	}
	rel := strings.TrimPrefix(path, "./")
	if rel == "" {
		return "", fmt.Errorf("%s must not be `./`", field)
	}
	if hasParentComponent(rel) {
		return "", fmt.Errorf("%s must not contain '..'", field)
	}
	if isAbsoluteLike(rel) {
		return "", fmt.Errorf("%s must stay within the plugin root", field)
	}
	return rel, nil
}

func hasParentComponent(rel string) bool {
	sep := []byte{'/'}
	if runtime.GOOS == "windows" {
		for _, part := range strings.FieldsFunc(rel, func(r rune) bool { return r == '/' || r == '\\' }) {
			if part == ".." {
				return true
			}
		}
		return false
	}
	_ = sep
	for _, part := range strings.Split(rel, "/") {
		if part == ".." {
			return true
		}
	}
	return false
}

func isAbsoluteLike(rel string) bool {
	if strings.HasPrefix(rel, "/") || strings.HasPrefix(rel, "\\") {
		return true
	}
	if runtime.GOOS == "windows" && len(rel) >= 2 && rel[1] == ':' &&
		((rel[0] >= 'A' && rel[0] <= 'Z') || (rel[0] >= 'a' && rel[0] <= 'z')) {
		return true
	}
	return false
}

func stayWithin(root, target string) error {
	cleanRoot := filepath.Clean(root)
	cleanTarget := filepath.Clean(target)
	rel, err := filepath.Rel(cleanRoot, cleanTarget)
	if err != nil {
		return ErrPathEscapes
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return ErrPathEscapes
	}
	return nil
}

func resolvedStayWithin(root, target string) error {
	if err := stayWithin(root, target); err != nil {
		return err
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		if os.IsNotExist(err) {
			return stayWithin(root, target)
		}
		return fmt.Errorf("resolve plugin root: %w", err)
	}
	probe := filepath.Clean(target)
	for {
		if resolved, evalErr := filepath.EvalSymlinks(probe); evalErr == nil {
			return stayWithin(resolvedRoot, resolved)
		} else if !os.IsNotExist(evalErr) {
			return evalErr
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			return stayWithin(resolvedRoot, filepath.Clean(target))
		}
		probe = parent
	}
}

func rejectSymlink(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("symlink is not allowed: %s", path)
	}
	if !info.Mode().IsRegular() && !info.IsDir() {
		return fmt.Errorf("special file is not allowed: %s", path)
	}
	return nil
}

func isRegularFile(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular()
}

func isDirectory(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.IsDir()
}
