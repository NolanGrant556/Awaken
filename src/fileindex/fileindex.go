package fileindex

import (
	"path"
	"sort"
	"strings"
)

const (
	scoreMatch       = 16
	bonusBoundary    = 8
	bonusCamel       = 6
	bonusConsecutive = 4
	bonusFirstChar   = 8
	penaltyGapStart  = 3
	testPathPenalty  = 6
)

// Result is one file-index search result.
//
// Score is a normalized rank where lower is better. The best returned match is
// 0.0, and later results move toward 1.0 based on their position after fuzzy
// ranking and test-path penalties.
type Result struct {
	Path  string
	Score float64
}

// Index stores slash-normalized paths and provides fuzzy file search.
//
// It is independent of the filesystem: Load only consumes the given path list.
type Index struct {
	paths      []string
	lowerPaths []string
}

// Load replaces the index contents with the given paths.
//
// Empty strings are ignored, paths are slash-normalized, and duplicates after
// normalization are removed while preserving first-seen order.
func (idx *Index) Load(paths []string) {
	seen := make(map[string]struct{}, len(paths))
	idx.paths = idx.paths[:0]
	idx.lowerPaths = idx.lowerPaths[:0]

	for _, p := range paths {
		normalized := normalizePath(p)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		idx.paths = append(idx.paths, normalized)
		idx.lowerPaths = append(idx.lowerPaths, strings.ToLower(normalized))
	}
}

// Search returns the best fuzzy matches for query, limited to limit results.
//
// Empty queries return unique top-level entries sorted by length, then
// alphabetically. Non-empty queries use smart case: all-lowercase queries are
// case-insensitive, while queries containing uppercase are case-sensitive.
func (idx *Index) Search(query string, limit int) []Result {
	if limit <= 0 {
		return nil
	}
	if query == "" {
		return idx.topLevelEntries(limit)
	}

	query = strings.ReplaceAll(query, "\\", "/")
	caseSensitive := query != strings.ToLower(query)
	needle := query
	if !caseSensitive {
		needle = strings.ToLower(query)
	}

	matches := make([]match, 0, min(limit, len(idx.paths)))
	for i, p := range idx.paths {
		haystack := p
		if !caseSensitive {
			haystack = idx.lowerPaths[i]
		}

		score, ok := fuzzyScore(p, haystack, needle)
		if !ok {
			continue
		}
		if strings.Contains(strings.ToLower(p), "test") {
			score -= testPathPenalty
		}
		matches = append(matches, match{path: p, score: score})
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score > matches[j].score
		}
		if len(matches[i].path) != len(matches[j].path) {
			return len(matches[i].path) < len(matches[j].path)
		}
		return matches[i].path < matches[j].path
	})

	if len(matches) > limit {
		matches = matches[:limit]
	}

	results := make([]Result, len(matches))
	denom := max(len(matches), 1)
	for i, m := range matches {
		results[i] = Result{
			Path:  m.path,
			Score: float64(i) / float64(denom),
		}
	}
	return results
}

type match struct {
	path  string
	score int
}

func normalizePath(value string) string {
	value = strings.ReplaceAll(value, "\\", "/")
	cleaned := path.Clean(value)
	if cleaned == "." {
		return ""
	}
	return strings.TrimPrefix(cleaned, "./")
}

func (idx *Index) topLevelEntries(limit int) []Result {
	seen := make(map[string]struct{})
	entries := make([]string, 0, min(limit, len(idx.paths)))

	for _, p := range idx.paths {
		segment := topLevelSegment(p)
		if segment == "" {
			continue
		}
		if _, ok := seen[segment]; ok {
			continue
		}
		seen[segment] = struct{}{}
		entries = append(entries, segment)
	}

	sort.Slice(entries, func(i, j int) bool {
		if len(entries[i]) != len(entries[j]) {
			return len(entries[i]) < len(entries[j])
		}
		return entries[i] < entries[j]
	})

	if len(entries) > limit {
		entries = entries[:limit]
	}

	results := make([]Result, len(entries))
	for i, entry := range entries {
		results[i] = Result{Path: entry, Score: 0}
	}
	return results
}

func topLevelSegment(p string) string {
	if i := strings.IndexByte(p, '/'); i >= 0 {
		return p[:i]
	}
	return p
}

func fuzzyScore(originalPath, haystack, needle string) (int, bool) {
	if needle == "" {
		return 0, true
	}

	positions := make([]int, 0, len(needle))
	prev := -1
	score := len(needle) * scoreMatch

	for i := 0; i < len(needle); i++ {
		pos := strings.IndexByte(haystack[prev+1:], needle[i])
		if pos < 0 {
			return 0, false
		}
		pos += prev + 1
		positions = append(positions, pos)

		if prev >= 0 {
			gap := pos - prev - 1
			if gap == 0 {
				score += bonusConsecutive
			} else {
				score -= penaltyGapStart + gap
			}
		}
		prev = pos
	}

	score += scoreBonusAt(originalPath, positions[0], true)
	for _, pos := range positions[1:] {
		score += scoreBonusAt(originalPath, pos, false)
	}
	score += max(0, 32-(len(originalPath)/4))

	return score, true
}

func scoreBonusAt(p string, pos int, first bool) int {
	if pos == 0 {
		if first {
			return bonusFirstChar
		}
		return 0
	}
	prev := p[pos-1]
	if isBoundary(prev) {
		return bonusBoundary
	}
	if isLower(prev) && isUpper(p[pos]) {
		return bonusCamel
	}
	return 0
}

func isBoundary(ch byte) bool {
	return ch == '/' || ch == '-' || ch == '_' || ch == '.' || ch == ' '
}

func isLower(ch byte) bool {
	return ch >= 'a' && ch <= 'z'
}

func isUpper(ch byte) bool {
	return ch >= 'A' && ch <= 'Z'
}
