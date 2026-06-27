package memoryextract

import (
	"regexp"
)

type secretRule struct {
	id     string
	label  string
	regexp *regexp.Regexp
}

type RegexSecretScanner struct {
	rules []secretRule
}

func NewDefaultSecretScanner() *RegexSecretScanner {
	return &RegexSecretScanner{rules: []secretRule{
		{id: "openai-api-key", label: "OpenAI API Key", regexp: regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{10,}\b`)},
		{id: "anthropic-api-key", label: "Anthropic API Key", regexp: regexp.MustCompile(`\bsk-ant-[A-Za-z0-9_-]{10,}\b`)},
		{id: "aws-access-token", label: "AWS Access Token", regexp: regexp.MustCompile(`\b(?:AKIA|ASIA)[A-Z0-9]{16}\b`)},
		{id: "github-pat", label: "GitHub PAT", regexp: regexp.MustCompile(`\bghp_[0-9A-Za-z]{36}\b`)},
		{id: "github-fine-grained-pat", label: "GitHub Fine-Grained PAT", regexp: regexp.MustCompile(`\bgithub_pat_[0-9A-Za-z_]{82}\b`)},
		{id: "gitlab-pat", label: "GitLab PAT", regexp: regexp.MustCompile(`\bglpat-[A-Za-z0-9_-]{20}\b`)},
		{id: "slack-token", label: "Slack Token", regexp: regexp.MustCompile(`\bxox[baprs]-[A-Za-z0-9-]{10,}\b`)},
		{id: "stripe-token", label: "Stripe Token", regexp: regexp.MustCompile(`\b(?:sk|rk)_(?:test|live|prod)_[A-Za-z0-9]{10,99}\b`)},
		{id: "private-key", label: "Private Key", regexp: regexp.MustCompile(`(?is)-----BEGIN[ A-Z0-9_-]{0,100}PRIVATE KEY(?: BLOCK)?-----.*-----END[ A-Z0-9_-]{0,100}PRIVATE KEY(?: BLOCK)?-----`)},
	}}
}

func (s *RegexSecretScanner) Scan(content string) []SecretMatch {
	if s == nil {
		return nil
	}

	matches := []SecretMatch{}
	seen := map[string]struct{}{}
	for _, rule := range s.rules {
		if _, ok := seen[rule.id]; ok {
			continue
		}
		if rule.regexp.MatchString(content) {
			seen[rule.id] = struct{}{}
			matches = append(matches, SecretMatch{
				RuleID: rule.id,
				Label:  rule.label,
			})
		}
	}
	return matches
}
