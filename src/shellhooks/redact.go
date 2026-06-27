package shellhooks

import (
	"regexp"
	"strings"
)

const redactedMarker = "[REDACTED]"

type Redactor struct {
	secrets     []string
	patterns    []*regexp.Regexp
	replacement string
}

func DefaultRedactor() *Redactor {
	return &Redactor{
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`sk-[A-Za-z0-9_-]{8,}`),
			regexp.MustCompile(`ghp_[A-Za-z0-9_]{8,}`),
			regexp.MustCompile(`github_pat_[A-Za-z0-9_]{8,}`),
			regexp.MustCompile(`(?i)(api[_-]?key|token|secret|password|authorization)(["']?\s*[:=]\s*["']?)([^"'\s,}]+)`),
		},
		replacement: redactedMarker,
	}
}

func NewRedactor(secrets []string) *Redactor {
	r := DefaultRedactor()
	for _, secret := range secrets {
		if secret != "" {
			r.secrets = append(r.secrets, secret)
		}
	}
	return r
}

func (r *Redactor) Redact(s string) string {
	if r == nil || s == "" {
		return s
	}
	out := s
	for _, secret := range r.secrets {
		out = strings.ReplaceAll(out, secret, r.marker())
	}
	for _, pattern := range r.patterns {
		if pattern.NumSubexp() >= 3 {
			out = pattern.ReplaceAllString(out, `${1}${2}`+r.marker())
			continue
		}
		out = pattern.ReplaceAllString(out, r.marker())
	}
	return out
}

func (r *Redactor) RedactValue(v any) any {
	switch x := v.(type) {
	case string:
		return r.Redact(x)
	case []any:
		out := make([]any, len(x))
		for i, item := range x {
			out[i] = r.RedactValue(item)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(x))
		for key, value := range x {
			out[key] = r.RedactValue(value)
		}
		return out
	default:
		return v
	}
}

func (r *Redactor) marker() string {
	if r != nil && r.replacement != "" {
		return r.replacement
	}
	return redactedMarker
}
