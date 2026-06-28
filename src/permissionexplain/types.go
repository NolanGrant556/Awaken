package permissionexplain

import "strings"

type RiskLevel string

const (
	RiskLevelLow    RiskLevel = "LOW"
	RiskLevelMedium RiskLevel = "MEDIUM"
	RiskLevelHigh   RiskLevel = "HIGH"
)

type Request struct {
	ToolName    string
	Input       string
	Description string
	Context     string
	ShellRules  []ShellRule
}

type Result struct {
	Explanation string    `json:"explanation"`
	Reasoning   string    `json:"reasoning"`
	Risk        string    `json:"risk"`
	RiskLevel   RiskLevel `json:"riskLevel"`
}

type RuleType string

const (
	RuleTypeExact    RuleType = "exact"
	RuleTypePrefix   RuleType = "prefix"
	RuleTypeWildcard RuleType = "wildcard"
)

type ShellRule struct {
	Pattern         string
	RiskLevel       RiskLevel
	Explanation     string
	Risk            string
	CaseInsensitive bool
}

type ParsedShellRule struct {
	Type    RuleType
	Pattern string
}

type ShellRuleMatch struct {
	Rule       ShellRule
	ParsedRule ParsedShellRule
}

type Matcher struct {
	shellRules []ShellRule
}

func NewMatcher(shellRules []ShellRule) Matcher {
	rules := make([]ShellRule, 0, len(shellRules))
	for _, rule := range shellRules {
		rule.Pattern = strings.TrimSpace(rule.Pattern)
		if rule.Pattern == "" {
			continue
		}
		if !isRiskLevel(rule.RiskLevel) {
			rule.RiskLevel = RiskLevelMedium
		}
		rules = append(rules, rule)
	}
	return Matcher{shellRules: rules}
}

func (m Matcher) MatchShell(command string) (ShellRuleMatch, bool) {
	command = strings.TrimSpace(command)
	var best ShellRuleMatch
	bestRank := 0
	matched := false
	for _, rule := range m.shellRules {
		parsed := ParseShellRule(rule.Pattern)
		if matchParsedShellRule(parsed, command, rule.CaseInsensitive) {
			rank := riskRank(rule.RiskLevel)
			if !matched || rank > bestRank {
				best = ShellRuleMatch{Rule: rule, ParsedRule: parsed}
				bestRank = rank
				matched = true
			}
		}
	}
	if matched {
		return best, true
	}
	return ShellRuleMatch{}, false
}

func isRiskLevel(level RiskLevel) bool {
	return level == RiskLevelLow || level == RiskLevelMedium || level == RiskLevelHigh
}

func riskRank(level RiskLevel) int {
	switch level {
	case RiskLevelLow:
		return 1
	case RiskLevelHigh:
		return 3
	case RiskLevelMedium:
		return 2
	default:
		return 2
	}
}
