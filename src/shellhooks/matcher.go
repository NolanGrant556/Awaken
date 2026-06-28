package shellhooks

import (
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

func MatchesMatcher(matcher string, query string) bool {
	matcher = strings.TrimSpace(matcher)
	if matcher == "" || matcher == "*" {
		return true
	}
	if query == "" {
		return false
	}

	if isSimpleMatcher(matcher) {
		for _, part := range strings.Split(matcher, "|") {
			if strings.TrimSpace(part) == query {
				return true
			}
		}
		return false
	}

	re, err := regexp.Compile(matcher)
	if err != nil {
		return false
	}
	return re.MatchString(query)
}

func MatchesInput(spec Spec, input Input) bool {
	return MatchesMatcher(spec.Matcher, MatchQuery(spec.Event, input))
}

func MatchQuery(event Event, input Input) string {
	if strings.TrimSpace(input.MatchQuery) != "" {
		return strings.TrimSpace(input.MatchQuery)
	}
	switch event {
	case EventPreToolCall, EventPostToolCall:
		return input.ToolName
	case EventPreLLMCall, EventPostLLMCall:
		return input.Model
	case EventNotification:
		return input.NotificationType
	case EventFileChanged:
		if input.FilePath == "" {
			return ""
		}
		return filepath.Base(input.FilePath)
	case EventCWDChanged:
		return input.NewCWD
	case EventPreCompact, EventPostCompact:
		return input.CompactTrigger
	case EventSessionStart, EventSessionEnd:
		return input.SessionReason
	default:
		return ""
	}
}

func isSimpleMatcher(matcher string) bool {
	for _, r := range matcher {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			continue
		}
		switch r {
		case '_', '-', '.', ':', '|':
			continue
		default:
			return false
		}
	}
	return true
}
