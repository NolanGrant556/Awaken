package teammemorysync

import (
	"regexp"
	"strings"
)

type SecretMatch struct {
	RuleID string
	Label  string
}

type secretRule struct {
	id    string
	label string
	re    *regexp.Regexp
}

var secretRules = []secretRule{
	{
		id:    "aws-access-token",
		label: "AWS Access Token",
		re:    regexp.MustCompile(`\b((?:A3T[A-Z0-9]|AKIA|ASIA|ABIA|ACCA)[A-Z2-7]{16})\b`),
	},
	{
		id:    "anthropic-api-key",
		label: "Anthropic API Key",
		re:    regexp.MustCompile(`\b(sk-ant-api03-[A-Za-z0-9_-]{20,}AA)(?:[` + "`" + `'"\\\s;]|\\[nr]|$)`),
	},
	{
		id:    "openai-api-key",
		label: "OpenAI API Key",
		re:    regexp.MustCompile(`\b(sk-(?:proj|svcacct|admin)-[A-Za-z0-9_-]{20,}|sk-[A-Za-z0-9]{20}T3BlbkFJ[A-Za-z0-9]{20})(?:[` + "`" + `'"\\\s;]|\\[nr]|$)`),
	},
	{
		id:    "github-pat",
		label: "GitHub PAT",
		re:    regexp.MustCompile(`\b(ghp_[0-9A-Za-z]{36})\b`),
	},
	{
		id:    "github-fine-grained-pat",
		label: "GitHub Fine-Grained PAT",
		re:    regexp.MustCompile(`\b(github_pat_\w{82})\b`),
	},
	{
		id:    "github-app-token",
		label: "GitHub App Token",
		re:    regexp.MustCompile(`\b((?:ghu|ghs)_[0-9A-Za-z]{36})\b`),
	},
	{
		id:    "github-oauth-token",
		label: "GitHub OAuth Token",
		re:    regexp.MustCompile(`\b(gho_[0-9A-Za-z]{36})\b`),
	},
	{
		id:    "gitlab-pat",
		label: "GitLab PAT",
		re:    regexp.MustCompile(`\b(glpat-[\w-]{20})\b`),
	},
	{
		id:    "slack-bot-token",
		label: "Slack Bot Token",
		re:    regexp.MustCompile(`\b(xoxb-[0-9]{10,13}-[0-9]{10,13}[A-Za-z0-9-]*)\b`),
	},
	{
		id:    "slack-user-token",
		label: "Slack User Token",
		re:    regexp.MustCompile(`\b(xox[pe](?:-[0-9]{10,13}){3}-[A-Za-z0-9-]{28,34})\b`),
	},
	{
		id:    "stripe-access-token",
		label: "Stripe Access Token",
		re:    regexp.MustCompile(`\b((?:sk|rk)_(?:test|live|prod)_[A-Za-z0-9]{10,99})(?:[` + "`" + `'"\\\s;]|\\[nr]|$)`),
	},
	{
		id:    "private-key",
		label: "Private Key",
		re:    regexp.MustCompile(`(?is)-----BEGIN[ A-Z0-9_-]{0,100}PRIVATE KEY(?: BLOCK)?-----.*?-----END[ A-Z0-9_-]{0,100}PRIVATE KEY(?: BLOCK)?-----`),
	},
}

// ScanSecrets returns one match per high-confidence rule that fires.
// Matched secret values are intentionally not returned.
func ScanSecrets(content string) []SecretMatch {
	matches := make([]SecretMatch, 0)
	seen := make(map[string]bool)
	for _, rule := range secretRules {
		if seen[rule.id] || !rule.re.MatchString(content) {
			continue
		}
		seen[rule.id] = true
		matches = append(matches, SecretMatch{
			RuleID: rule.id,
			Label:  rule.label,
		})
	}
	return matches
}

// SecretLabel returns the stable label for a rule ID.
func SecretLabel(ruleID string) string {
	for _, rule := range secretRules {
		if rule.id == ruleID {
			return rule.label
		}
	}
	parts := strings.Split(ruleID, "-")
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, " ")
}
