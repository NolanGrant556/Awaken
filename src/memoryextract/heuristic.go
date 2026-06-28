package memoryextract

import (
	"context"
	"regexp"
	"strings"
)

type HeuristicExtractor struct{}

type textSegment struct {
	Text  string
	Start int
	End   int
}

var (
	backtickPattern = regexp.MustCompile("`([^`]+)`")
	commandPattern  = regexp.MustCompile(`\b((?:go\s+(?:test|run|build|fmt|vet|mod)\b|npm\s+run\b|pnpm\s+\S+|yarn\s+\S+|make\b|git\s+(?:status|show|diff|log|branch|switch|commit|push|pull)\b|python3?\s+-m\b)[^.;\n]*)`)
	absolutePath    = regexp.MustCompile(`/(?:[A-Za-z0-9._@+-]+/?)+`)
	relativePath    = regexp.MustCompile(`(?:^|[\s` + "`" + `])((?:\.{1,2}/)?[A-Za-z0-9._@+-]+(?:/[A-Za-z0-9._@+-]+)+)`)
)

func (HeuristicExtractor) Extract(_ context.Context, input ExtractionInput) ([]CandidateMemory, error) {
	candidates := []CandidateMemory{}
	for _, message := range input.Messages {
		if !message.ModelVisible() {
			continue
		}
		for _, segment := range splitSegments(message.Content) {
			if isEphemeral(segment.Text) {
				continue
			}
			candidates = append(candidates, commandCandidates(message, segment)...)
			candidates = append(candidates, pathCandidates(message, segment)...)
			if isPreference(segment.Text) {
				candidates = append(candidates, CandidateMemory{
					Text:       cleanSentence(segment.Text),
					Category:   CategoryPreference,
					Confidence: 0.86,
					SourceRefs: []SourceRef{sourceRef(message, segment.Start, segment.End)},
				})
			}
			if isProjectFact(segment.Text) {
				candidates = append(candidates, CandidateMemory{
					Text:       cleanSentence(segment.Text),
					Category:   CategoryProject,
					Confidence: 0.78,
					SourceRefs: []SourceRef{sourceRef(message, segment.Start, segment.End)},
				})
			}
		}
	}
	return candidates, nil
}

func commandCandidates(message TranscriptMessage, segment textSegment) []CandidateMemory {
	candidates := []CandidateMemory{}
	seen := map[string]struct{}{}

	for _, match := range backtickPattern.FindAllStringSubmatchIndex(segment.Text, -1) {
		command := strings.TrimSpace(segment.Text[match[2]:match[3]])
		if !looksLikeCommand(command) {
			continue
		}
		if _, ok := seen[command]; ok {
			continue
		}
		seen[command] = struct{}{}
		candidates = append(candidates, CandidateMemory{
			Text:       "Command: `" + command + "`.",
			Category:   CategoryCommand,
			Confidence: 0.9,
			SourceRefs: []SourceRef{sourceRef(message, segment.Start+match[2], segment.Start+match[3])},
		})
	}

	for _, match := range commandPattern.FindAllStringSubmatchIndex(segment.Text, -1) {
		command := strings.TrimSpace(segment.Text[match[2]:match[3]])
		if beforeBacktick, _, ok := strings.Cut(command, "`"); ok {
			command = strings.TrimSpace(beforeBacktick)
		}
		command = strings.TrimRight(command, " .,;:")
		if !looksLikeCommand(command) {
			continue
		}
		if _, ok := seen[command]; ok {
			continue
		}
		seen[command] = struct{}{}
		candidates = append(candidates, CandidateMemory{
			Text:       "Command: `" + command + "`.",
			Category:   CategoryCommand,
			Confidence: 0.84,
			SourceRefs: []SourceRef{sourceRef(message, segment.Start+match[2], segment.Start+match[3])},
		})
	}

	return candidates
}

func pathCandidates(message TranscriptMessage, segment textSegment) []CandidateMemory {
	if !hasPathContext(segment.Text) {
		return nil
	}

	candidates := []CandidateMemory{}
	seen := map[string]struct{}{}
	addPath := func(path string, start int, end int) {
		path = strings.Trim(path, "`'\".,;:)")
		if path == "" {
			return
		}
		if _, ok := seen[path]; ok {
			return
		}
		seen[path] = struct{}{}
		candidates = append(candidates, CandidateMemory{
			Text:       "Path: `" + path + "`.",
			Category:   CategoryPath,
			Confidence: 0.76,
			SourceRefs: []SourceRef{sourceRef(message, segment.Start+start, segment.Start+end)},
		})
	}

	for _, match := range absolutePath.FindAllStringIndex(segment.Text, -1) {
		addPath(segment.Text[match[0]:match[1]], match[0], match[1])
	}
	for _, match := range relativePath.FindAllStringSubmatchIndex(segment.Text, -1) {
		addPath(segment.Text[match[2]:match[3]], match[2], match[3])
	}

	return candidates
}

func splitSegments(content string) []textSegment {
	segments := []textSegment{}
	start := 0
	for i := 0; i < len(content); i++ {
		if !isSegmentBreak(content, i) {
			continue
		}
		segments = appendTrimmedSegment(segments, content, start, i+1)
		start = i + 1
	}
	segments = appendTrimmedSegment(segments, content, start, len(content))
	return segments
}

func isSegmentBreak(content string, i int) bool {
	switch content[i] {
	case '\n', ';':
		return true
	case '.', '!', '?':
		return i == len(content)-1 || content[i+1] == ' ' || content[i+1] == '\n' || content[i+1] == '\t'
	default:
		return false
	}
}

func appendTrimmedSegment(segments []textSegment, content string, start int, end int) []textSegment {
	for start < end && isSpace(content[start]) {
		start++
	}
	for end > start && isSpace(content[end-1]) {
		end--
	}
	if start == end {
		return segments
	}
	return append(segments, textSegment{Text: content[start:end], Start: start, End: end})
}

func isSpace(b byte) bool {
	return b == ' ' || b == '\n' || b == '\r' || b == '\t'
}

func sourceRef(message TranscriptMessage, start int, end int) SourceRef {
	return SourceRef{
		MessageID: message.ID,
		Role:      message.Role,
		Start:     start,
		End:       end,
	}
}

func looksLikeCommand(text string) bool {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return false
	}
	switch fields[0] {
	case "go", "npm", "pnpm", "yarn", "make", "git", "cargo", "python", "python3", "node", "bun", "deno":
		return true
	}
	return strings.HasPrefix(fields[0], "./")
}

func isPreference(text string) bool {
	lower := strings.ToLower(text)
	markers := []string{
		"i prefer", "we prefer", "prefer to", "always ", "never ", "do not ", "don't ",
		"avoid ", "please use", "use chinese", "use english", "默认", "不要", "优先", "总是",
	}
	return containsAny(lower, markers)
}

func isProjectFact(text string) bool {
	lower := strings.ToLower(text)
	markers := []string{
		"project ", "repo ", "repository ", "workspace ", "worktree ", "branch ",
		"package ", "module ", "awaken ", " uses ", "source of truth",
		"entrypoint", "stored in", "located at", "lives at", "lives in",
	}
	return containsAny(lower, markers)
}

func hasPathContext(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(text, "/") && containsAny(lower, []string{
		"path", "file", "folder", "directory", "repo", "repository", "workspace",
		"worktree", "under", "stored", "located", "lives", "use ", "run ",
	})
}

func containsAny(text string, markers []string) bool {
	for _, marker := range markers {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func isEphemeral(text string) bool {
	lower := strings.ToLower(strings.Trim(strings.TrimSpace(text), " .!,;:"))
	switch lower {
	case "", "thanks", "thank you", "ok", "okay", "cool", "great", "sounds good", "hello", "hi", "yes", "no", "sure", "好的", "谢谢", "嗯":
		return true
	}
	if len([]rune(lower)) <= 3 {
		return true
	}
	if strings.HasPrefix(lower, "can you ") && !isPreference(lower) && !isProjectFact(lower) && !hasPathContext(lower) {
		return true
	}
	return false
}

func cleanSentence(text string) string {
	return strings.TrimSpace(text)
}
