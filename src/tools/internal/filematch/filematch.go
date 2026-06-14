package filematch

import (
	"path"
	"regexp"
	"strings"
)

// Match reports whether relPath matches a small, portable glob subset.
// It supports *, ?, and ** over slash-normalized relative paths.
func Match(pattern, relPath string) bool {
	pattern = normalizePattern(pattern)
	relPath = normalizePath(relPath)
	if pattern == "" || pattern == "." || pattern == "**/*" || pattern == "**" {
		return relPath != ""
	}
	if strings.HasPrefix(pattern, "**/") && Match(strings.TrimPrefix(pattern, "**/"), relPath) {
		return true
	}

	if !strings.Contains(pattern, "/") && !strings.Contains(pattern, "**") {
		ok, err := path.Match(pattern, path.Base(relPath))
		return err == nil && ok
	}

	ok, err := regexp.MatchString("^"+globToRegex(pattern)+"$", relPath)
	return err == nil && ok
}

func normalizePattern(pattern string) string {
	pattern = strings.TrimSpace(pattern)
	pattern = strings.ReplaceAll(pattern, "\\", "/")
	pattern = strings.TrimPrefix(path.Clean(pattern), "./")
	if pattern == "." {
		return ""
	}
	return pattern
}

func normalizePath(value string) string {
	value = strings.ReplaceAll(value, "\\", "/")
	return strings.TrimPrefix(path.Clean(value), "./")
}

func globToRegex(pattern string) string {
	var out strings.Builder
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				if i+2 < len(pattern) && pattern[i+2] == '/' {
					out.WriteString("(?:.*/)?")
					i += 2
				} else {
					out.WriteString(".*")
					i++
				}
			} else {
				out.WriteString("[^/]*")
			}
		case '?':
			out.WriteString("[^/]")
		default:
			out.WriteString(regexp.QuoteMeta(string(pattern[i])))
		}
	}
	return out.String()
}
