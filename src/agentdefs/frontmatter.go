package agentdefs

import (
	"strings"
	"unicode/utf8"
)

type metadata struct {
	Name            string
	Description     string
	Model           string
	Tools           []string
	DisallowedTools []string
	PermissionMode  string
}

func parseMarkdown(markdown string) (metadata, string, []string) {
	markdown = strings.ToValidUTF8(markdown, "")
	markdown = strings.TrimPrefix(markdown, "\xef\xbb\xbf")
	normalized := strings.ReplaceAll(strings.ReplaceAll(markdown, "\r\n", "\n"), "\r", "\n")
	lines := strings.Split(normalized, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return metadata{}, normalized, nil
	}

	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			rawFrontmatter := strings.Join(lines[1:i], "\n")
			body := strings.Join(lines[i+1:], "\n")
			meta, warnings := parseFrontmatter(rawFrontmatter)
			return meta, body, warnings
		}
	}

	return metadata{}, normalized, []string{"frontmatter_not_closed"}
}

func parseFrontmatter(raw string) (metadata, []string) {
	var meta metadata
	var warnings []string
	lines := strings.Split(raw, "\n")

	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, ":")
		if !ok {
			warnings = append(warnings, "frontmatter_line_ignored")
			continue
		}
		key = normalizeKey(key)
		value = strings.TrimSpace(value)

		switch key {
		case "name":
			meta.Name = parseScalar(value)
		case "description":
			meta.Description = parseScalar(value)
		case "model":
			meta.Model = parseScalar(value)
		case "permission_mode":
			meta.PermissionMode = parseScalar(value)
		case "tools":
			items, consumed, listWarnings := parseListValue(value, lines[i+1:])
			meta.Tools = items
			i += consumed
			warnings = append(warnings, listWarnings...)
		case "disallowed_tools":
			items, consumed, listWarnings := parseListValue(value, lines[i+1:])
			meta.DisallowedTools = items
			i += consumed
			warnings = append(warnings, listWarnings...)
		default:
			continue
		}
	}

	return meta, dedupeStrings(warnings)
}

func normalizeKey(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func parseScalar(value string) string {
	value = stripInlineComment(strings.TrimSpace(value))
	return unquoteScalar(value)
}

func parseListValue(value string, following []string) ([]string, int, []string) {
	value = strings.TrimSpace(value)
	if value != "" {
		return normalizeListItems(parseInlineList(value)), 0, nil
	}

	items := []string{}
	warnings := []string{}
	consumed := 0
	for _, rawLine := range following {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			consumed++
			continue
		}
		if isTopLevelKey(rawLine) {
			break
		}
		consumed++
		if !strings.HasPrefix(line, "-") {
			warnings = append(warnings, "frontmatter_list_item_ignored")
			continue
		}
		item := strings.TrimSpace(strings.TrimPrefix(line, "-"))
		if item == "" {
			continue
		}
		items = append(items, item)
	}

	return normalizeListItems(items), consumed, warnings
}

func isTopLevelKey(line string) bool {
	if strings.TrimSpace(line) == "" {
		return false
	}
	if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
		return false
	}
	key, _, ok := strings.Cut(line, ":")
	return ok && strings.TrimSpace(key) != ""
}

func parseInlineList(value string) []string {
	value = stripInlineComment(value)
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
		return splitCommaList(strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(value, "["), "]")))
	}
	if strings.Contains(value, ",") {
		return splitCommaList(value)
	}
	return []string{value}
}

func splitCommaList(value string) []string {
	if value == "" {
		return nil
	}
	items := []string{}
	var b strings.Builder
	var quote rune
	escaped := false
	for _, r := range value {
		if escaped {
			b.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' && quote == '"' {
			escaped = true
			b.WriteRune(r)
			continue
		}
		if (r == '\'' || r == '"') && quote == 0 {
			quote = r
			b.WriteRune(r)
			continue
		}
		if r == quote {
			quote = 0
			b.WriteRune(r)
			continue
		}
		if r == ',' && quote == 0 {
			items = append(items, b.String())
			b.Reset()
			continue
		}
		b.WriteRune(r)
	}
	items = append(items, b.String())
	return items
}

func normalizeListItems(items []string) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		item = unquoteScalar(stripInlineComment(strings.TrimSpace(item)))
		if item == "" {
			continue
		}
		if len(item) > 128 {
			item = item[:safeUTF8Cut(item, 128)]
		}
		result = append(result, item)
		if len(result) >= 64 {
			break
		}
	}
	return dedupeStrings(result)
}

func stripInlineComment(value string) string {
	var quote rune
	escaped := false
	for i, r := range value {
		if escaped {
			escaped = false
			continue
		}
		if r == '\\' && quote == '"' {
			escaped = true
			continue
		}
		if (r == '\'' || r == '"') && quote == 0 {
			quote = r
			continue
		}
		if r == quote {
			quote = 0
			continue
		}
		if r == '#' && quote == 0 {
			if i == 0 || value[i-1] == ' ' || value[i-1] == '\t' {
				return strings.TrimSpace(value[:i])
			}
		}
	}
	return strings.TrimSpace(value)
}

func unquoteScalar(value string) string {
	value = strings.TrimSpace(value)
	if len(value) < 2 {
		return value
	}
	if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
		return strings.TrimSpace(value[1 : len(value)-1])
	}
	return value
}

func truncateStringBytes(value string, limit int) (string, bool) {
	value = strings.ToValidUTF8(value, "")
	if limit <= 0 || len(value) <= limit {
		return value, false
	}
	return value[:safeUTF8Cut(value, limit)], true
}

func safeUTF8Cut(value string, limit int) int {
	if limit >= len(value) {
		return len(value)
	}
	for limit > 0 && !utf8.ValidString(value[:limit]) {
		limit--
	}
	return limit
}

func dedupeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	result := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		if seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}
