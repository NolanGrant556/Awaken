package outputstyle

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func LoadStyleFile(path string) (StyleConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return StyleConfig{}, err
	}
	fallbackName := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	return parseStyleFile(string(data), fallbackName)
}

func parseStyleFile(content string, fallbackName string) (StyleConfig, error) {
	content = strings.TrimPrefix(strings.ReplaceAll(content, "\r\n", "\n"), "\ufeff")
	if strings.TrimSpace(content) == "" {
		return StyleConfig{}, fmt.Errorf("%w: file is empty", ErrInvalidStyle)
	}

	style := StyleConfig{
		Name:   strings.TrimSpace(fallbackName),
		Source: SourceUser,
	}

	body := content
	lines := strings.Split(content, "\n")
	if strings.TrimSpace(lines[0]) == "---" {
		end := -1
		for i := 1; i < len(lines); i++ {
			if strings.TrimSpace(lines[i]) == "---" {
				end = i
				break
			}
		}
		if end == -1 {
			return StyleConfig{}, fmt.Errorf("%w: frontmatter is not closed", ErrInvalidStyle)
		}
		if err := applyFrontmatter(&style, lines[1:end]); err != nil {
			return StyleConfig{}, err
		}
		body = strings.Join(lines[end+1:], "\n")
	}

	style.Prompt = strings.TrimSpace(body)
	normalized, err := normalizeStyle(style)
	if err != nil {
		return StyleConfig{}, err
	}
	return normalized, nil
}

func applyFrontmatter(style *StyleConfig, lines []string) error {
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		key, rawValue, ok := strings.Cut(trimmed, ":")
		if !ok {
			return fmt.Errorf("%w: frontmatter line %d must be key: value", ErrInvalidStyle, i+2)
		}
		key = normalizeFrontmatterKey(key)
		value, err := parseFrontmatterScalar(rawValue)
		if err != nil {
			return fmt.Errorf("%w: frontmatter line %d has invalid quoted value", ErrInvalidStyle, i+2)
		}

		switch key {
		case "name":
			style.Name = value
		case "description":
			style.Description = value
		case "keep_coding_instructions":
			parsed, err := parseFrontmatterBool(value, key, i+2)
			if err != nil {
				return err
			}
			style.KeepCodingInstructions = parsed
		case "force_for_plugin":
			parsed, err := parseFrontmatterBool(value, key, i+2)
			if err != nil {
				return err
			}
			style.ForceForPlugin = parsed
		case "source":
			source := Source(value)
			if !isValidSource(source) {
				return fmt.Errorf("%w: unsupported source %q on frontmatter line %d", ErrInvalidStyle, value, i+2)
			}
			style.Source = source
		default:
			return fmt.Errorf("%w: unsupported frontmatter key %q on line %d", ErrInvalidStyle, key, i+2)
		}
	}
	return nil
}

func normalizeFrontmatterKey(key string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(key)), "-", "_")
}

func parseFrontmatterScalar(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", nil
	}
	if strings.HasPrefix(value, `"`) || strings.HasPrefix(value, `'`) {
		return parseQuotedScalar(value)
	}
	return value, nil
}

func parseQuotedScalar(value string) (string, error) {
	if strings.HasPrefix(value, `"`) {
		return strconv.Unquote(value)
	}
	if !strings.HasSuffix(value, `'`) || len(value) < 2 {
		return "", strconv.ErrSyntax
	}
	return strings.ReplaceAll(value[1:len(value)-1], "''", "'"), nil
}

func parseFrontmatterBool(value, key string, line int) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, fmt.Errorf("%w: %s on frontmatter line %d must be true or false", ErrInvalidStyle, key, line)
	}
}
