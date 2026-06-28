package permissionrules

import "strings"

// Behavior describes the action a permission rule requests.
type Behavior string

const (
	BehaviorAllow Behavior = "allow"
	BehaviorDeny  Behavior = "deny"
	BehaviorAsk   Behavior = "ask"
)

// Source describes where a permission rule came from.
type Source string

const (
	SourceUserSettings    Source = "userSettings"
	SourceProjectSettings Source = "projectSettings"
	SourceLocalSettings   Source = "localSettings"
	SourceFlagSettings    Source = "flagSettings"
	SourcePolicySettings  Source = "policySettings"
	SourceCLIArg          Source = "cliArg"
	SourceCommand         Source = "command"
	SourceSession         Source = "session"
)

const bashToolName = "Bash"

// RuleValue identifies a tool-wide rule or a specific rule for a tool.
type RuleValue struct {
	ToolName    string
	RuleContent *string
}

// String serializes a rule value as ToolName or ToolName(content).
func (v RuleValue) String() string {
	if v.RuleContent == nil || *v.RuleContent == "" || *v.RuleContent == "*" {
		return v.ToolName
	}
	return v.ToolName + "(" + EscapeRuleContent(*v.RuleContent) + ")"
}

// Rule is a permission rule with its origin and behavior.
type Rule struct {
	Source    Source
	Behavior  Behavior
	RuleValue RuleValue
}

// Context contains the permission rules to inspect.
type Context struct {
	Rules []Rule
}

// DetectOptions controls unreachable rule detection.
type DetectOptions struct {
	SandboxAutoAllowEnabled bool
}

// ShadowType describes the kind of rule that made another rule unreachable.
type ShadowType string

const (
	ShadowTypeDeny ShadowType = "deny"
	ShadowTypeAsk  ShadowType = "ask"
)

// UnreachableRule describes a rule that cannot be reached during evaluation.
type UnreachableRule struct {
	Rule       Rule
	Reason     string
	ShadowedBy Rule
	ShadowType ShadowType
	Fix        string
}

// ParseRuleValue parses ToolName or ToolName(content). Malformed values are
// treated as tool-wide rule names so existing settings can round-trip safely.
func ParseRuleValue(rule string) RuleValue {
	open := findFirstUnescapedChar(rule, '(')
	if open == -1 {
		return RuleValue{ToolName: rule}
	}

	close := findLastUnescapedChar(rule, ')')
	if close == -1 || close <= open || close != len(rule)-1 {
		return RuleValue{ToolName: rule}
	}

	toolName := rule[:open]
	if toolName == "" {
		return RuleValue{ToolName: rule}
	}

	rawContent := rule[open+1 : close]
	if rawContent == "" || rawContent == "*" {
		return RuleValue{ToolName: toolName}
	}

	content := UnescapeRuleContent(rawContent)
	return RuleValue{ToolName: toolName, RuleContent: &content}
}

// EscapeRuleContent escapes rule content for Tool(content) storage.
func EscapeRuleContent(content string) string {
	escaped := make([]rune, 0, len(content))
	for _, r := range content {
		switch r {
		case '\\', '(', ')':
			escaped = append(escaped, '\\')
		}
		escaped = append(escaped, r)
	}
	return string(escaped)
}

// UnescapeRuleContent reverses EscapeRuleContent for parentheses and backslash.
func UnescapeRuleContent(content string) string {
	var unescaped strings.Builder
	unescaped.Grow(len(content))
	for i := 0; i < len(content); i++ {
		if content[i] == '\\' && i+1 < len(content) {
			next := content[i+1]
			if next == '(' || next == ')' || next == '\\' {
				unescaped.WriteByte(next)
				i++
				continue
			}
		}
		unescaped.WriteByte(content[i])
	}
	return unescaped.String()
}

// IsSharedSource reports whether a rule source is shared with other users.
func IsSharedSource(source Source) bool {
	return source == SourceProjectSettings ||
		source == SourcePolicySettings ||
		source == SourceCommand
}

// DetectUnreachableRules finds specific allow rules blocked by tool-wide deny
// or ask rules for the same tool.
func DetectUnreachableRules(ctx Context, options DetectOptions) []UnreachableRule {
	allowRules := rulesWithBehavior(ctx.Rules, BehaviorAllow)
	denyRules := rulesWithBehavior(ctx.Rules, BehaviorDeny)
	askRules := rulesWithBehavior(ctx.Rules, BehaviorAsk)

	unreachable := make([]UnreachableRule, 0)
	for _, allowRule := range allowRules {
		if allowRule.RuleValue.RuleContent == nil {
			continue
		}

		if denyRule, ok := firstToolWideRuleForTool(denyRules, allowRule.RuleValue.ToolName); ok {
			unreachable = append(unreachable, UnreachableRule{
				Rule:       allowRule,
				Reason:     "Blocked by tool-wide deny rule from " + string(denyRule.Source) + ".",
				ShadowedBy: denyRule,
				ShadowType: ShadowTypeDeny,
				Fix:        "Remove the tool-wide deny rule or the specific allow rule.",
			})
			continue
		}

		askRule, ok := firstToolWideRuleForTool(askRules, allowRule.RuleValue.ToolName)
		if !ok {
			continue
		}
		if options.SandboxAutoAllowEnabled &&
			allowRule.RuleValue.ToolName == bashToolName &&
			!IsSharedSource(askRule.Source) {
			continue
		}

		unreachable = append(unreachable, UnreachableRule{
			Rule:       allowRule,
			Reason:     "Shadowed by tool-wide ask rule from " + string(askRule.Source) + ".",
			ShadowedBy: askRule,
			ShadowType: ShadowTypeAsk,
			Fix:        "Remove the tool-wide ask rule or the specific allow rule.",
		})
	}

	return unreachable
}

func rulesWithBehavior(rules []Rule, behavior Behavior) []Rule {
	filtered := make([]Rule, 0, len(rules))
	for _, rule := range rules {
		if rule.Behavior == behavior {
			filtered = append(filtered, rule)
		}
	}
	return filtered
}

func firstToolWideRuleForTool(rules []Rule, toolName string) (Rule, bool) {
	for _, rule := range rules {
		if rule.RuleValue.ToolName == toolName && rule.RuleValue.RuleContent == nil {
			return rule, true
		}
	}
	return Rule{}, false
}

func findFirstUnescapedChar(s string, target byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == target && !isEscaped(s, i) {
			return i
		}
	}
	return -1
}

func findLastUnescapedChar(s string, target byte) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == target && !isEscaped(s, i) {
			return i
		}
	}
	return -1
}

func isEscaped(s string, index int) bool {
	backslashes := 0
	for i := index - 1; i >= 0 && s[i] == '\\'; i-- {
		backslashes++
	}
	return backslashes%2 == 1
}
