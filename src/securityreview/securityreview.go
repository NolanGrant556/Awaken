package securityreview

import (
	"fmt"
	"strings"
)

const minimumConfidence = 0.8

var allowedTools = []string{
	"Bash(git diff:*)",
	"Bash(git status:*)",
	"Bash(git log:*)",
	"Bash(git show:*)",
	"Bash(git remote show:*)",
	"Read",
	"Glob",
	"Grep",
	"LS",
	"Task",
}

type Snapshot struct {
	Status        string
	FilesModified string
	Log           string
	Diff          string
}

type Finding struct {
	ID              string
	File            string
	Line            int
	Severity        string
	Category        string
	Description     string
	ExploitScenario string
	Recommendation  string
	Confidence      float64
}

type Validation struct {
	ID            string
	Confidence    float64
	FalsePositive bool
	Notes         string
}

func BuildPrompt(snapshot Snapshot) string {
	return fmt.Sprintf(`You are a senior security engineer conducting a focused security review of the changes on this branch.

GIT STATUS:

%s

FILES MODIFIED:

%s

COMMITS:

%s

DIFF CONTENT:

%s

Review the complete diff above. This contains all code changes in the PR.

OBJECTIVE:
Perform a security-focused code review to identify HIGH-CONFIDENCE security vulnerabilities that could have real exploitation potential. This is not a general code review. Focus only on security implications newly added by this PR and do not comment on existing security concerns.

CRITICAL INSTRUCTIONS:
1. MINIMIZE FALSE POSITIVES: Only flag issues where you are more than 80%% confident of actual exploitability.
2. AVOID NOISE: Skip theoretical issues, style concerns, defense-in-depth hardening, or low-impact findings.
3. FOCUS ON IMPACT: Prioritize vulnerabilities that could lead to unauthorized access, data breaches, or system compromise.
4. EXCLUSIONS: Do not report denial-of-service-only issues, secrets-on-disk policy-only findings, rate-limit policy-only concerns, resource exhaustion, log spoofing, documentation-only issues, or outdated third-party libraries.

SECURITY CATEGORIES TO EXAMINE:

Input Validation Vulnerabilities:
- SQL injection via unsanitized user input
- Command injection in system calls or subprocesses
- XXE injection in XML parsing
- Template injection in templating engines
- NoSQL injection in database queries
- Path traversal in file operations

Authentication & Authorization Issues:
- Authentication bypass logic
- Privilege escalation paths
- Session management flaws
- JWT token vulnerabilities
- Authorization logic bypasses

Crypto & Secrets Management:
- Hardcoded API keys, passwords, or tokens
- Weak cryptographic algorithms or implementations
- Improper key storage or management
- Cryptographic randomness issues
- Certificate validation bypasses

Injection & Code Execution:
- Remote code execution via deserialization
- Pickle injection in Python
- YAML deserialization vulnerabilities
- Eval injection in dynamic code execution
- XSS vulnerabilities in web applications when unsafe rendering APIs are used

Data Exposure:
- Sensitive data logging or storage
- PII handling violations
- API endpoint data leakage
- Debug information exposure

ANALYSIS METHODOLOGY:

Phase 1 - Repository Context Research:
- Identify existing security frameworks and libraries in use.
- Look for established secure coding, sanitization, validation, permission, and approval patterns.
- Understand the project's security model and threat model before judging the diff.

Phase 2 - Comparative Analysis:
- Compare new code changes against existing security patterns.
- Identify deviations from established secure practices.
- Look for inconsistent security implementations.
- Flag code that introduces new attack surfaces.

Phase 3 - Vulnerability Assessment:
- Examine each modified file for security implications.
- Trace data flow from user inputs to sensitive operations.
- Look for privilege boundaries being crossed unsafely.
- Identify injection points and unsafe deserialization.

FALSE POSITIVE FILTERING:
- Exclude denial of service, resource exhaustion, memory exhaustion, CPU exhaustion, and regex DOS findings.
- Exclude secrets or credentials merely stored on disk when the finding is policy-only and has no concrete exploit path.
- Exclude rate limiting, quota, or service overload concerns.
- Exclude missing hardening, missing audit logs, insecure documentation, log spoofing, regex injection, and outdated dependency reports.
- Exclude client-side authentication or authorization concerns unless the changed server-side trust boundary is clearly bypassed.
- Exclude React or Angular XSS claims unless unsafe APIs such as dangerouslySetInnerHTML or bypassSecurityTrustHtml are used.
- Treat environment variables and CLI flags as trusted values unless the diff creates a concrete untrusted control path.
- Only include MEDIUM findings if they are obvious, concrete, and actionable.

SIGNAL QUALITY CRITERIA:
1. Is there a concrete, exploitable vulnerability with a clear attack path?
2. Does this represent a real security risk rather than a theoretical best-practice gap?
3. Are there specific code locations and reproduction steps?
4. Would this finding be actionable for a security team?

CONFIDENCE SCORING:
- 0.9-1.0 or 9-10/10: Certain exploit path identified, tested if possible.
- 0.8-0.9 or 8-9/10: Clear vulnerability pattern with known exploitation methods.
- 0.7-0.8 or 7-8/10: Suspicious pattern requiring specific conditions to exploit.
- Below 0.8 or below 8/10: Do not report.

REQUIRED OUTPUT FORMAT:
You must output the final findings in markdown. Each finding must contain the file, line number, severity, category, description, exploit scenario, fix recommendation, and confidence score.

Example:

# Vuln 1: XSS: foo.py:42

* Severity: High
* Category: xss
* Confidence: 0.90
* Description: User input from username is directly interpolated into HTML without escaping.
* Exploit Scenario: An attacker crafts a URL that executes JavaScript in the victim's browser.
* Recommendation: Use framework escaping or safe templating for all user-controlled HTML output.

FINAL REMINDER:
Focus on HIGH and MEDIUM findings only. Better to miss some theoretical issues than flood the report with false positives. Filter out any finding where the validated confidence is less than 8/10 or 0.8. Your final reply must contain the markdown report and nothing else.`,
		fenced(snapshot.Status),
		fenced(snapshot.FilesModified),
		fenced(snapshot.Log),
		fenced(snapshot.Diff),
	)
}

func AllowedTools() []string {
	tools := make([]string, len(allowedTools))
	copy(tools, allowedTools)
	return tools
}

func FilterFindings(findings []Finding, validations []Validation) []Finding {
	validationByID := make(map[string]Validation, len(validations))
	for _, validation := range validations {
		validationByID[validation.ID] = validation
	}

	filtered := make([]Finding, 0, len(findings))
	for _, finding := range findings {
		if normalizedConfidence(finding.Confidence) < minimumConfidence {
			continue
		}
		if isExcluded(finding) {
			continue
		}

		if validation, ok := validationByID[finding.ID]; ok {
			if validation.FalsePositive {
				continue
			}
			if normalizedConfidence(validation.Confidence) < minimumConfidence {
				continue
			}
		}

		filtered = append(filtered, finding)
	}
	return filtered
}

func FormatReport(findings []Finding) string {
	if len(findings) == 0 {
		return "# Security Review Report\n\nNo high-confidence security vulnerabilities found.\n"
	}

	var builder strings.Builder
	builder.WriteString("# Security Review Report\n\n")
	for i, finding := range findings {
		if i > 0 {
			builder.WriteString("\n")
		}

		category := fallback(finding.Category, "uncategorized")
		builder.WriteString(fmt.Sprintf("# Vuln %d: %s: `%s`\n\n", i+1, title(category), location(finding)))
		builder.WriteString(fmt.Sprintf("* Severity: %s\n", fallback(finding.Severity, "Unspecified")))
		builder.WriteString(fmt.Sprintf("* Category: `%s`\n", category))
		builder.WriteString(fmt.Sprintf("* Confidence: %.2f\n", normalizedConfidence(finding.Confidence)))
		builder.WriteString(fmt.Sprintf("* Description: %s\n", fallback(finding.Description, "No description provided.")))
		builder.WriteString(fmt.Sprintf("* Exploit Scenario: %s\n", fallback(finding.ExploitScenario, "No exploit scenario provided.")))
		builder.WriteString(fmt.Sprintf("* Recommendation: %s\n", fallback(finding.Recommendation, "No recommendation provided.")))
	}

	return builder.String()
}

func fenced(value string) string {
	return "```\n" + strings.TrimSpace(value) + "\n```"
}

func normalizedConfidence(confidence float64) float64 {
	if confidence > 1 {
		return confidence / 10
	}
	return confidence
}

func isExcluded(finding Finding) bool {
	category := strings.ToLower(finding.Category)
	text := strings.ToLower(strings.Join([]string{
		finding.Category,
		finding.Description,
		finding.ExploitScenario,
		finding.Recommendation,
	}, " "))

	if containsAny(text, "denial of service", "denial-of-service", "denial_of_service", " dos", "dos ", "dos-only", "resource exhaustion", "resource-exhaustion", "resource_exhaustion", "memory exhaustion", "memory-exhaustion", "cpu exhaustion", "cpu-exhaustion", "regex dos", "regex-dos", "redos", "service disruption") {
		return true
	}
	if containsAny(text, "rate limit", "rate-limit", "quota", "service overload") || containsAny(category, "rate_limit", "rate-limit") {
		return true
	}
	if containsAny(category, "secret_on_disk", "secrets_on_disk") {
		return true
	}
	if containsAny(text, "stored on disk", "saved on disk", "written to disk", "on-disk") && containsAny(text, "secret", "credential", "token", "sensitive data") {
		return true
	}
	if strings.Contains(text, "policy-only") && containsAny(text, "secret", "credential", "rate limit", "rate-limit") {
		return true
	}
	if containsAny(category, "documentation", "outdated_dependency", "outdated_dependencies", "log_spoofing", "regex_injection") {
		return true
	}
	if isDocumentationFile(finding.File) {
		return true
	}

	return false
}

func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func isDocumentationFile(path string) bool {
	lower := strings.ToLower(path)
	return strings.HasSuffix(lower, ".md") || strings.HasSuffix(lower, ".mdx") || strings.HasSuffix(lower, ".txt") || strings.HasPrefix(lower, "docs/")
}

func location(finding Finding) string {
	file := fallback(finding.File, "unknown")
	if finding.Line <= 0 {
		return file
	}
	return fmt.Sprintf("%s:%d", file, finding.Line)
}

func fallback(value string, fallbackValue string) string {
	if strings.TrimSpace(value) == "" {
		return fallbackValue
	}
	return strings.TrimSpace(value)
}

func title(value string) string {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == '_' || r == '-'
	})
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, " ")
}
