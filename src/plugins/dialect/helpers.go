package dialect

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"awaken/src/plugins/ir"
	"gopkg.in/yaml.v3"
)

func readManifest(root, relative string) ([]byte, map[string]any, error) {
	raw, err := safeRead(root, relative)
	if err != nil {
		return nil, nil, err
	}
	var object map[string]any
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, nil, fmt.Errorf("%s is not valid JSON: %w", relative, err)
	}
	if object == nil {
		return nil, nil, fmt.Errorf("%s must contain a JSON object", relative)
	}
	return raw, object, nil
}

func safeRead(root, relative string) ([]byte, error) {
	clean, err := safeRelative(relative)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(root, filepath.FromSlash(clean))
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s must be a regular file", relative)
	}
	if hasMultipleHardlinks(info) {
		return nil, fmt.Errorf("%s must not be a hardlink", relative)
	}
	if err := resolvedWithin(root, path); err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

func safeRelative(value string) (string, error) {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	value = strings.TrimPrefix(value, "./")
	if value == "" || value == "." {
		return "", nil
	}
	if strings.HasPrefix(value, "/") || filepath.IsAbs(value) || strings.Contains(value, ":/") {
		return "", fmt.Errorf("absolute plugin path is not allowed: %q", value)
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(value)))
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("plugin path escapes package root: %q", value)
	}
	return clean, nil
}

func declaredRelative(value string) (string, error) {
	trimmed := strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if trimmed != "." && trimmed != "./" && !strings.HasPrefix(trimmed, "./") {
		return "", fmt.Errorf("declared plugin path must start with ./: %q", value)
	}
	return safeRelative(trimmed)
}

func resolvedWithin(root, path string) error {
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(resolvedRoot, resolvedPath)
	if err != nil {
		return err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("plugin path escapes package root: %s", path)
	}
	return nil
}

func pathsField(object map[string]any, key string) ([]string, bool, error) {
	value, ok := object[key]
	if !ok {
		return nil, false, nil
	}
	switch typed := value.(type) {
	case string:
		return []string{typed}, true, nil
	case []any:
		out := make([]string, 0, len(typed))
		for i, item := range typed {
			text, ok := item.(string)
			if !ok {
				return nil, true, fmt.Errorf("%s[%d] must be a string", key, i)
			}
			out = append(out, text)
		}
		return out, true, nil
	default:
		return nil, true, fmt.Errorf("%s must be a path or array of paths", key)
	}
}

func stringField(object map[string]any, key string) string {
	value, _ := object[key].(string)
	return value
}

func stringList(value any) []string {
	switch typed := value.(type) {
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil
		}
		parts := strings.Split(typed, ",")
		out := make([]string, 0, len(parts))
		for _, part := range parts {
			if part = strings.TrimSpace(part); part != "" {
				out = append(out, part)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
				out = append(out, strings.TrimSpace(text))
			}
		}
		return out
	case []string:
		return append([]string(nil), typed...)
	default:
		return nil
	}
}

func identityFromManifest(object map[string]any) ir.Identity {
	identity := ir.Identity{
		Name:        stringField(object, "name"),
		DisplayName: stringField(object, "displayName"),
		Version:     stringField(object, "version"),
		Description: stringField(object, "description"),
		Homepage:    stringField(object, "homepage"),
		Repository:  stringField(object, "repository"),
		License:     stringField(object, "license"),
		Category:    stringField(object, "category"),
		Logo:        stringField(object, "logo"),
		Keywords:    stringList(object["keywords"]),
		Tags:        stringList(object["tags"]),
	}
	if rawAuthor, ok := object["author"].(map[string]any); ok {
		identity.Author = &ir.Author{
			Name: stringField(rawAuthor, "name"), Email: stringField(rawAuthor, "email"), URL: stringField(rawAuthor, "url"),
		}
	} else if authorName, ok := object["author"].(string); ok {
		identity.Author = &ir.Author{Name: authorName}
	}
	if rawInterface, ok := object["interface"]; ok {
		identity.InterfaceRaw, _ = json.Marshal(rawInterface)
		if values, ok := rawInterface.(map[string]any); ok && identity.DisplayName == "" {
			identity.DisplayName = stringField(values, "displayName")
		}
	}
	if identity.DisplayName == "" {
		identity.DisplayName = identity.Name
	}
	return identity
}

func componentDiagnostic(code, message string, severity string, component ir.Component) ir.Diagnostic {
	return ir.Diagnostic{
		Code: code, Message: message, Severity: severity, Phase: "parse",
		ComponentKind: component.Kind, ComponentID: component.LogicalID,
		SourcePath: component.SourcePath, SourceDialect: component.SourceDialect,
	}
}

func walkFiles(root, relative string, extensions map[string]bool) ([]string, error) {
	clean, err := safeRelative(relative)
	if err != nil {
		return nil, err
	}
	base := filepath.Join(root, filepath.FromSlash(clean))
	info, err := os.Lstat(base)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("component path must not be a symlink: %s", relative)
	}
	if !info.IsDir() {
		if info.Mode().IsRegular() && (len(extensions) == 0 || extensions[strings.ToLower(filepath.Ext(base))]) {
			return []string{clean}, nil
		}
		return nil, fmt.Errorf("component path is not a supported file or directory: %s", relative)
	}
	var out []string
	err = filepath.WalkDir(base, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("component path contains a symlink: %s", path)
		}
		if len(extensions) > 0 && !extensions[strings.ToLower(filepath.Ext(path))] {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	sort.Strings(out)
	return out, err
}

func directSkillFiles(root, relative string) ([]string, error) {
	clean, err := safeRelative(relative)
	if err != nil {
		return nil, err
	}
	base := filepath.Join(root, filepath.FromSlash(clean))
	info, err := os.Lstat(base)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, fmt.Errorf("skill path must be a regular directory: %s", relative)
	}
	if direct := filepath.Join(base, "SKILL.md"); fileRegular(direct) {
		rel, _ := filepath.Rel(root, direct)
		return []string{filepath.ToSlash(rel)}, nil
	}
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, entry := range entries {
		if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		path := filepath.Join(base, entry.Name(), "SKILL.md")
		if fileRegular(path) {
			rel, _ := filepath.Rel(root, path)
			out = append(out, filepath.ToSlash(rel))
		}
	}
	sort.Strings(out)
	return out, nil
}

func fileRegular(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0
}

func parseMarkdown(root, relative string) (map[string]any, string, []byte, error) {
	raw, err := safeRead(root, relative)
	if err != nil {
		return nil, "", nil, err
	}
	text := string(raw)
	frontmatter := map[string]any{}
	body := text
	if strings.HasPrefix(text, "---\n") || strings.HasPrefix(text, "---\r\n") {
		normalized := strings.ReplaceAll(text, "\r\n", "\n")
		end := strings.Index(normalized[4:], "\n---")
		if end < 0 {
			return nil, "", raw, fmt.Errorf("frontmatter is missing closing delimiter")
		}
		end += 4
		if err := yaml.Unmarshal([]byte(normalized[4:end]), &frontmatter); err != nil {
			return nil, "", raw, fmt.Errorf("parse frontmatter: %w", err)
		}
		bodyStart := end + len("\n---")
		if len(normalized) > bodyStart && normalized[bodyStart] == '\n' {
			bodyStart++
		}
		body = normalized[bodyStart:]
	}
	return frontmatter, body, raw, nil
}

func digestBytes(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func canonicalDigest(value any) string {
	raw, _ := json.Marshal(value)
	return digestBytes(raw)
}

func sourceName(relative string) string {
	base := filepath.Base(relative)
	if strings.EqualFold(base, "SKILL.md") {
		return filepath.Base(filepath.Dir(relative))
	}
	return strings.TrimSuffix(base, filepath.Ext(base))
}

func boolPointer(value any) *bool {
	if boolean, ok := value.(bool); ok {
		return &boolean
	}
	return nil
}
