package researchruntime

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

var explicitHTTPSURLPattern = regexp.MustCompile(`https://[^\s<>"'，。；！？、（）【】]+`)

func ExplicitHTTPSURLs(query string) []string {
	matches := explicitHTTPSURLPattern.FindAllString(query, -1)
	urls := make([]string, 0, len(matches))
	for _, match := range matches {
		match = strings.TrimRight(match, "，。；！？、,;!?)）】}")
		if match != "" {
			urls = append(urls, match)
		}
	}
	return uniqueStrings(urls)
}

const (
	WorkflowID           = "deep-research"
	DefaultBreadth       = 4
	MinBreadth           = 2
	MaxBreadth           = 4
	MaxQueryRunes        = 4000
	MaxClaimsPerQuestion = 6
	MaxClaimsTotal       = 24
	MaxEvidenceTotal     = 48
	ChildMaxThoughts     = 12
	ChildMaxToolCalls    = 16
)

type SourceHint string

const (
	SourceBoth      SourceHint = "both"
	SourceWeb       SourceHint = "web"
	SourceWorkspace SourceHint = "workspace"
)

type Intent struct {
	Query      string
	Breadth    int
	SourceHint SourceHint
}

type ComposerToken struct {
	Type    string
	Payload WorkflowPayload
}

type WorkflowPayload struct {
	WorkflowID string
}

func ParseIntent(tokens []ComposerToken, query string) (*Intent, error) {
	if !HasWorkflowToken(tokens) {
		return nil, nil
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("research query is required")
	}
	if utf8.RuneCountInString(query) > MaxQueryRunes {
		return nil, fmt.Errorf("research query exceeds %d characters", MaxQueryRunes)
	}
	return &Intent{Query: query, Breadth: DefaultBreadth, SourceHint: SourceBoth}, nil
}

func HasWorkflowToken(tokens []ComposerToken) bool {
	for _, token := range tokens {
		if strings.EqualFold(strings.TrimSpace(token.Type), "workflow") &&
			strings.EqualFold(strings.TrimSpace(token.Payload.WorkflowID), WorkflowID) {
			return true
		}
	}
	return false
}

func NormalizeBreadth(value int) int {
	if value < MinBreadth {
		return DefaultBreadth
	}
	if value > MaxBreadth {
		return MaxBreadth
	}
	return value
}

func NormalizeSourceHint(value string) SourceHint {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(SourceWeb):
		return SourceWeb
	case string(SourceWorkspace):
		return SourceWorkspace
	default:
		return SourceBoth
	}
}
