package sessionshare

import (
	"path/filepath"
	"regexp"
	"strings"
)

const (
	redactedSecret = "[REDACTED_SECRET]"
	redactedPath   = "[REDACTED_PATH]"
)

var (
	sensitiveFieldKeyPattern = regexp.MustCompile(`(?i)(api[_-]?key|access[_-]?token|refresh[_-]?token|auth(?:orization)?|secret|password|passwd|private[_-]?key)`)
	defaultPathPattern       = regexp.MustCompile(`/(?:Users|home)(?:/[^\s"'<>),;]+)+`)
)

type replacementPattern struct {
	re          *regexp.Regexp
	replacement string
}

var secretPatterns = []replacementPattern{
	{regexp.MustCompile(`(?i)\b(api[_-]?key|access[_-]?token|refresh[_-]?token|auth(?:orization)?|secret|password|passwd)(\s*[:=]\s*["']?)([^\s"',;}]+)`), `${1}${2}` + redactedSecret},
	{regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._~+/=-]{8,}`), `Bearer ` + redactedSecret},
	{regexp.MustCompile(`\bsk-(?:proj-|ant-)?[A-Za-z0-9_-]{8,}\b`), redactedSecret},
	{regexp.MustCompile(`\bgh[ps]_[A-Za-z0-9_]{8,}\b`), redactedSecret},
	{regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`), redactedSecret},
}

type redactor struct {
	opts         Options
	pathPatterns []*regexp.Regexp
	findings     []PrivacyFinding
}

func newRedactor(opts Options) *redactor {
	return &redactor{
		opts:         opts,
		pathPatterns: buildPathPatterns(opts.PathRoots),
	}
}

func buildPathPatterns(roots []string) []*regexp.Regexp {
	patterns := make([]*regexp.Regexp, 0, len(roots)+1)
	for _, root := range roots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		cleaned := filepath.Clean(root)
		if cleaned == "." || cleaned == string(filepath.Separator) {
			continue
		}
		patterns = append(patterns, regexp.MustCompile(regexp.QuoteMeta(cleaned)+`(?:/[^\s"'<>),;]+)*`))
	}
	patterns = append(patterns, defaultPathPattern)
	return patterns
}

func (r *redactor) processStringMap(values map[string]string, scope string, messageIndex int) map[string]string {
	if len(values) == 0 {
		return nil
	}

	result := make(map[string]string, len(values))
	for key, value := range values {
		fieldScope := scope + "." + key
		if sensitiveFieldKeyPattern.MatchString(key) {
			r.addFinding("secret", messageIndex, fieldScope, r.opts.RedactSecrets, "high")
			if r.opts.RedactSecrets {
				result[key] = redactedSecret
				continue
			}
		}
		result[key] = r.processText(value, fieldScope, messageIndex)
	}
	return result
}

func (r *redactor) processText(text string, scope string, messageIndex int) string {
	if text == "" {
		return text
	}
	result := r.processSecrets(text, scope, messageIndex)
	result = r.processPaths(result, scope, messageIndex)
	return result
}

func (r *redactor) processSecrets(text string, scope string, messageIndex int) string {
	result := text
	for _, pattern := range secretPatterns {
		matches := pattern.re.FindAllStringIndex(result, -1)
		for range matches {
			r.addFinding("secret", messageIndex, scope, r.opts.RedactSecrets, "high")
		}
		if r.opts.RedactSecrets && len(matches) > 0 {
			result = pattern.re.ReplaceAllString(result, pattern.replacement)
		}
	}
	return result
}

func (r *redactor) processPaths(text string, scope string, messageIndex int) string {
	result := text
	for _, pattern := range r.pathPatterns {
		matches := pattern.FindAllString(result, -1)
		usableMatches := 0
		for _, match := range matches {
			if strings.Contains(match, redactedPath) {
				continue
			}
			usableMatches++
			r.addFinding("path", messageIndex, scope, r.opts.RedactPaths, "medium")
		}
		if r.opts.RedactPaths && usableMatches > 0 {
			result = pattern.ReplaceAllString(result, redactedPath)
		}
	}
	return result
}

func (r *redactor) addFinding(kind string, messageIndex int, scope string, redacted bool, severity string) {
	r.findings = append(r.findings, PrivacyFinding{
		Kind:         kind,
		MessageIndex: messageIndex,
		Scope:        scope,
		Redacted:     redacted,
		Severity:     severity,
	})
}

func (r *redactor) report() PrivacyReport {
	report := PrivacyReport{Findings: append([]PrivacyFinding(nil), r.findings...)}
	for _, finding := range r.findings {
		switch finding.Kind {
		case "secret":
			report.SecretFindings++
			if finding.Redacted {
				report.RedactedFindings++
			} else {
				report.UnredactedSecrets++
			}
		case "path":
			report.PathFindings++
			if finding.Redacted {
				report.RedactedFindings++
			} else {
				report.UnredactedPaths++
			}
		default:
			if finding.Redacted {
				report.RedactedFindings++
			}
		}
	}
	return report
}
