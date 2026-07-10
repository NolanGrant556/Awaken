package applypatch

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

type replacement struct {
	start    int
	oldLen   int
	newLines []string
}

func deriveUpdatedContent(path string, chunks []updateChunk) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("failed to read file to update %s: file does not exist", path)
		}
		return "", fmt.Errorf("failed to read file to update %s: %w", path, err)
	}

	originalLines := strings.Split(string(data), "\n")
	if len(originalLines) > 0 && originalLines[len(originalLines)-1] == "" {
		originalLines = originalLines[:len(originalLines)-1]
	}

	replacements, err := computeReplacements(originalLines, path, chunks)
	if err != nil {
		return "", err
	}
	newLines := applyReplacements(originalLines, replacements)
	if len(newLines) == 0 || newLines[len(newLines)-1] != "" {
		newLines = append(newLines, "")
	}
	return strings.Join(newLines, "\n"), nil
}

func computeReplacements(originalLines []string, path string, chunks []updateChunk) ([]replacement, error) {
	var replacements []replacement
	lineIndex := 0

	for _, chunk := range chunks {
		if chunk.changeContext != nil {
			index := seekSequence(originalLines, []string{*chunk.changeContext}, lineIndex, false)
			if index < 0 {
				return nil, patchErrorf(ErrorCodePatchContextNotFound, "failed to find context %q in %s", *chunk.changeContext, path)
			}
			lineIndex = index + 1
		}

		if len(chunk.oldLines) == 0 {
			replacements = append(replacements, replacement{
				start:    len(originalLines),
				oldLen:   0,
				newLines: append([]string(nil), chunk.newLines...),
			})
			continue
		}

		pattern := chunk.oldLines
		newLines := chunk.newLines
		found := seekSequence(originalLines, pattern, lineIndex, chunk.endOfFile)
		if found < 0 && len(pattern) > 0 && pattern[len(pattern)-1] == "" {
			pattern = pattern[:len(pattern)-1]
			if len(newLines) > 0 && newLines[len(newLines)-1] == "" {
				newLines = newLines[:len(newLines)-1]
			}
			found = seekSequence(originalLines, pattern, lineIndex, chunk.endOfFile)
		}

		if found < 0 {
			return nil, patchErrorf(ErrorCodePatchContextNotFound, "failed to find expected lines in %s:\n%s", path, strings.Join(chunk.oldLines, "\n"))
		}
		replacements = append(replacements, replacement{
			start:    found,
			oldLen:   len(pattern),
			newLines: append([]string(nil), newLines...),
		})
		lineIndex = found + len(pattern)
	}

	sort.SliceStable(replacements, func(i, j int) bool {
		return replacements[i].start < replacements[j].start
	})
	return replacements, nil
}

func applyReplacements(lines []string, replacements []replacement) []string {
	result := append([]string(nil), lines...)
	for i := len(replacements) - 1; i >= 0; i-- {
		rep := replacements[i]
		if rep.start > len(result) {
			rep.start = len(result)
		}
		end := rep.start + rep.oldLen
		if end > len(result) {
			end = len(result)
		}
		next := make([]string, 0, len(result)-rep.oldLen+len(rep.newLines))
		next = append(next, result[:rep.start]...)
		next = append(next, rep.newLines...)
		next = append(next, result[end:]...)
		result = next
	}
	return result
}

func seekSequence(lines, pattern []string, start int, eof bool) int {
	if len(pattern) == 0 {
		return start
	}
	if len(pattern) > len(lines) {
		return -1
	}
	if start < 0 {
		start = 0
	}
	searchStart := start
	if eof && len(lines) >= len(pattern) {
		searchStart = len(lines) - len(pattern)
	}
	if searchStart > len(lines)-len(pattern) {
		return -1
	}

	matchers := []func(string, string) bool{
		func(a, b string) bool { return a == b },
		func(a, b string) bool { return strings.TrimRight(a, " \t") == strings.TrimRight(b, " \t") },
		func(a, b string) bool { return strings.TrimSpace(a) == strings.TrimSpace(b) },
		func(a, b string) bool { return normalizeLine(a) == normalizeLine(b) },
	}

	for _, matches := range matchers {
		for i := searchStart; i <= len(lines)-len(pattern); i++ {
			if sequenceMatches(lines, pattern, i, matches) {
				return i
			}
		}
	}
	return -1
}

func sequenceMatches(lines, pattern []string, start int, matches func(string, string) bool) bool {
	for i := range pattern {
		if !matches(lines[start+i], pattern[i]) {
			return false
		}
	}
	return true
}

func normalizeLine(value string) string {
	var builder strings.Builder
	for _, char := range strings.TrimSpace(value) {
		switch char {
		case '\u2010', '\u2011', '\u2012', '\u2013', '\u2014', '\u2015', '\u2212':
			builder.WriteRune('-')
		case '\u2018', '\u2019', '\u201A', '\u201B':
			builder.WriteRune('\'')
		case '\u201C', '\u201D', '\u201E', '\u201F':
			builder.WriteRune('"')
		case '\u00A0', '\u2002', '\u2003', '\u2004', '\u2005', '\u2006', '\u2007', '\u2008', '\u2009', '\u200A', '\u202F', '\u205F', '\u3000':
			builder.WriteRune(' ')
		default:
			builder.WriteRune(char)
		}
	}
	return builder.String()
}
