package sessionlite

import (
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf16"
)

const (
	LiteReadBufSize    = 65536
	MaxSanitizedLength = 200
)

type HeadTail struct {
	Head string
	Tail string
	Size int
}

type Candidate struct {
	ProjectPath string
	ProjectDir  string
	FilePath    string
}

func ReadHeadTail(content []byte, limit int) HeadTail {
	if limit <= 0 {
		limit = LiteReadBufSize
	}

	size := len(content)
	if size <= limit {
		text := string(content)
		return HeadTail{Head: text, Tail: text, Size: size}
	}

	return HeadTail{
		Head: string(content[:limit]),
		Tail: string(content[size-limit:]),
		Size: size,
	}
}

func ExtractJSONStringField(text, key string) (string, bool) {
	return extractJSONStringField(text, key, false)
}

func ExtractLastJSONStringField(text, key string) (string, bool) {
	return extractJSONStringField(text, key, true)
}

func extractJSONStringField(text, key string, last bool) (string, bool) {
	quotedKey := strconv.Quote(key)
	searchFrom := 0
	var value string
	found := false

	for searchFrom < len(text) {
		idx := strings.Index(text[searchFrom:], quotedKey)
		if idx < 0 {
			break
		}
		idx += searchFrom

		candidate, end, ok := readJSONStringValueAfterKey(text, idx+len(quotedKey))
		if ok {
			if !last {
				return candidate, true
			}
			value = candidate
			found = true
			searchFrom = end
			continue
		}
		searchFrom = idx + len(quotedKey)
	}

	return value, found
}

func readJSONStringValueAfterKey(text string, pos int) (string, int, bool) {
	i := skipJSONWhitespace(text, pos)
	if i >= len(text) || text[i] != ':' {
		return "", i, false
	}

	i = skipJSONWhitespace(text, i+1)
	if i >= len(text) || text[i] != '"' {
		return "", i, false
	}

	valueStart := i + 1
	for i = valueStart; i < len(text); i++ {
		switch text[i] {
		case '\\':
			i++
		case '"':
			raw := text[valueStart:i]
			value, err := strconv.Unquote(`"` + raw + `"`)
			if err != nil {
				return raw, i + 1, true
			}
			return value, i + 1, true
		}
	}

	return "", len(text), false
}

func skipJSONWhitespace(text string, pos int) int {
	for pos < len(text) {
		switch text[pos] {
		case ' ', '\n', '\r', '\t':
			pos++
		default:
			return pos
		}
	}
	return pos
}

func SanitizePath(name string) string {
	var b strings.Builder
	b.Grow(len(name))
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('-')
	}

	sanitized := b.String()
	if len(sanitized) <= MaxSanitizedLength {
		return sanitized
	}

	return sanitized[:MaxSanitizedLength] + "-" + stableBase36Hash(name)
}

func stableBase36Hash(text string) string {
	var hash int32
	for _, unit := range utf16.Encode([]rune(text)) {
		hash = hash*31 + int32(unit)
	}

	value := int64(hash)
	if value < 0 {
		value = -value
	}
	return strconv.FormatInt(value, 36)
}

func ProjectDir(projectsRoot, projectPath string) string {
	return filepath.Join(projectsRoot, SanitizePath(cleanSlashPath(projectPath)))
}

func ResolveSessionFileCandidates(sessionID, projectPath string, worktreePaths []string, projectsRoot string) []Candidate {
	fileName := sessionID + ".jsonl"
	paths := make([]string, 0, len(worktreePaths)+1)
	if projectPath != "" {
		paths = append(paths, projectPath)
	}
	paths = append(paths, worktreePaths...)

	candidates := make([]Candidate, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, p := range paths {
		cleaned := cleanSlashPath(p)
		projectDir := ProjectDir(projectsRoot, cleaned)
		filePath := filepath.Join(projectDir, fileName)
		if _, ok := seen[filePath]; ok {
			continue
		}
		seen[filePath] = struct{}{}
		candidates = append(candidates, Candidate{
			ProjectPath: cleaned,
			ProjectDir:  projectDir,
			FilePath:    filePath,
		})
	}

	return candidates
}

func cleanSlashPath(p string) string {
	if p == "" {
		return ""
	}

	cleaned := path.Clean(strings.ReplaceAll(p, "\\", "/"))
	if cleaned == "." {
		return ""
	}
	return cleaned
}
