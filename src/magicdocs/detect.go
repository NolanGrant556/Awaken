package magicdocs

import (
	"regexp"
	"strings"
)

var magicDocHeaderPattern = regexp.MustCompile(`(?i)^[ \t]*#\s*MAGIC\s+DOC\s*:\s*(.+?)\s*$`)

// Detection describes a Magic Doc header and its optional document-specific
// instruction line as found in the current document contents.
type Detection struct {
	Title           string
	Instructions    string
	HasInstructions bool
	HeaderLine      string
	ItalicLine      string
}

// DetectMagicDocHeader finds a "# MAGIC DOC: title" header in markdown
// content. Header matching is case-insensitive and tolerant of extra spaces.
// If the line after the header, or the line after one blank line, is italic,
// it is returned as document-specific update instructions.
func DetectMagicDocHeader(content string) (Detection, bool) {
	lines := markdownLines(content)
	for i, line := range lines {
		match := magicDocHeaderPattern.FindStringSubmatch(line)
		if len(match) != 2 {
			continue
		}

		title := strings.TrimSpace(match[1])
		if title == "" {
			continue
		}

		detection := Detection{
			Title:      title,
			HeaderLine: line,
		}

		instructionIndex := i + 1
		if instructionIndex < len(lines) && strings.TrimSpace(lines[instructionIndex]) == "" {
			instructionIndex++
		}
		if instructionIndex < len(lines) {
			if instructions, ok := parseItalicInstruction(lines[instructionIndex]); ok {
				detection.Instructions = instructions
				detection.HasInstructions = true
				detection.ItalicLine = lines[instructionIndex]
			}
		}

		return detection, true
	}
	return Detection{}, false
}

func markdownLines(content string) []string {
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	return strings.Split(normalized, "\n")
}

func parseItalicInstruction(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if len(trimmed) < 3 {
		return "", false
	}

	marker := trimmed[0]
	if marker != '*' && marker != '_' {
		return "", false
	}
	if trimmed[len(trimmed)-1] != marker {
		return "", false
	}

	inner := trimmed[1 : len(trimmed)-1]
	if strings.HasPrefix(inner, string(marker)) || strings.HasSuffix(inner, string(marker)) {
		return "", false
	}

	instructions := strings.TrimSpace(inner)
	if instructions == "" {
		return "", false
	}
	return instructions, true
}
