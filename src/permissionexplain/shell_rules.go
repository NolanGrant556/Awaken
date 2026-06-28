package permissionexplain

import (
	"regexp"
	"strings"
)

const (
	escapedStarPlaceholder      = "\x00ESCAPED_STAR\x00"
	escapedBackslashPlaceholder = "\x00ESCAPED_BACKSLASH\x00"
)

func ParseShellRule(permissionRule string) ParsedShellRule {
	rule := strings.TrimSpace(permissionRule)
	if prefix, ok := shellRulePrefix(rule); ok {
		return ParsedShellRule{Type: RuleTypePrefix, Pattern: prefix}
	}
	if HasShellWildcards(rule) {
		return ParsedShellRule{Type: RuleTypeWildcard, Pattern: rule}
	}
	return ParsedShellRule{Type: RuleTypeExact, Pattern: rule}
}

func SuggestExactShellRule(command string) ShellRule {
	return ShellRule{Pattern: strings.TrimSpace(command), RiskLevel: RiskLevelMedium}
}

func SuggestPrefixShellRule(prefix string) ShellRule {
	return ShellRule{Pattern: strings.TrimSpace(prefix) + ":*", RiskLevel: RiskLevelMedium}
}

func HasShellWildcards(pattern string) bool {
	if strings.HasSuffix(pattern, ":*") {
		return false
	}
	for i := 0; i < len(pattern); i++ {
		if pattern[i] != '*' {
			continue
		}
		backslashes := 0
		for j := i - 1; j >= 0 && pattern[j] == '\\'; j-- {
			backslashes++
		}
		if backslashes%2 == 0 {
			return true
		}
	}
	return false
}

func MatchWildcardShellPattern(pattern, command string, caseInsensitive bool) bool {
	pattern = strings.TrimSpace(pattern)
	processed := strings.Builder{}
	for i := 0; i < len(pattern); {
		if pattern[i] == '\\' && i+1 < len(pattern) {
			switch pattern[i+1] {
			case '*':
				processed.WriteString(escapedStarPlaceholder)
				i += 2
				continue
			case '\\':
				processed.WriteString(escapedBackslashPlaceholder)
				i += 2
				continue
			}
		}
		processed.WriteByte(pattern[i])
		i++
	}

	body := regexp.QuoteMeta(processed.String())
	body = strings.ReplaceAll(body, "\\*", ".*")
	body = strings.ReplaceAll(body, regexp.QuoteMeta(escapedStarPlaceholder), "\\*")
	body = strings.ReplaceAll(body, regexp.QuoteMeta(escapedBackslashPlaceholder), "\\\\")

	if strings.HasSuffix(body, " .*") && countUnescapedStars(processed.String()) == 1 {
		body = strings.TrimSuffix(body, " .*") + "(?: .*)?"
	}

	flags := "(?s)"
	if caseInsensitive {
		flags = "(?is)"
	}
	re := regexp.MustCompile(flags + "^" + body + "$")
	return re.MatchString(command)
}

func matchParsedShellRule(rule ParsedShellRule, command string, caseInsensitive bool) bool {
	pattern := strings.TrimSpace(rule.Pattern)
	if caseInsensitive {
		pattern = strings.ToLower(pattern)
		command = strings.ToLower(command)
	}
	switch rule.Type {
	case RuleTypeExact:
		return command == pattern
	case RuleTypePrefix:
		return command == pattern || strings.HasPrefix(command, pattern+" ")
	case RuleTypeWildcard:
		return MatchWildcardShellPattern(pattern, command, caseInsensitive)
	default:
		return false
	}
}

func shellRulePrefix(rule string) (string, bool) {
	if strings.HasSuffix(rule, ":*") && len(rule) > 2 {
		return strings.TrimSpace(strings.TrimSuffix(rule, ":*")), true
	}
	return "", false
}

func countUnescapedStars(pattern string) int {
	count := 0
	for i := 0; i < len(pattern); i++ {
		if pattern[i] != '*' {
			continue
		}
		backslashes := 0
		for j := i - 1; j >= 0 && pattern[j] == '\\'; j-- {
			backslashes++
		}
		if backslashes%2 == 0 {
			count++
		}
	}
	return count
}
