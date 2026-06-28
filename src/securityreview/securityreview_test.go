package securityreview

import (
	"reflect"
	"strings"
	"testing"
)

func TestBuildPromptIncludesSnapshotContextAndReviewRules(t *testing.T) {
	prompt := BuildPrompt(Snapshot{
		Status:        "On branch feature/security-review",
		FilesModified: "src/server/handlers.go",
		Log:           "abc123 add unsafe handler",
		Diff:          "@@ -1 +1 @@\n+exec(userInput)",
	})

	expected := []string{
		"GIT STATUS:",
		"On branch feature/security-review",
		"FILES MODIFIED:",
		"src/server/handlers.go",
		"COMMITS:",
		"abc123 add unsafe handler",
		"DIFF CONTENT:",
		"exec(userInput)",
		"Input Validation Vulnerabilities",
		"Authentication & Authorization Issues",
		"Crypto & Secrets Management",
		"Injection & Code Execution",
		"Data Exposure",
		"Phase 1 - Repository Context Research",
		"Phase 2 - Comparative Analysis",
		"Phase 3 - Vulnerability Assessment",
		"denial-of-service-only",
		"secrets-on-disk policy-only",
		"rate-limit policy-only",
		"less than 8/10 or 0.8",
	}

	for _, value := range expected {
		if !strings.Contains(prompt, value) {
			t.Fatalf("expected prompt to contain %q", value)
		}
	}
}

func TestAllowedToolsMatchesSecurityReviewScope(t *testing.T) {
	expected := []string{
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

	if got := AllowedTools(); !reflect.DeepEqual(got, expected) {
		t.Fatalf("unexpected allowed tools:\nwant: %#v\n got: %#v", expected, got)
	}

	tools := AllowedTools()
	tools[0] = "Write"
	if got := AllowedTools()[0]; got != "Bash(git diff:*)" {
		t.Fatalf("expected AllowedTools to return a defensive copy, got %q", got)
	}
}

func TestFilterFindingsDropsLowConfidence(t *testing.T) {
	findings := []Finding{
		{ID: "low-decimal", Category: "sql_injection", Confidence: 0.79},
		{ID: "low-ten-point", Category: "command_injection", Confidence: 7.9},
		{ID: "validation-low", Category: "xss", Confidence: 0.95},
	}
	validations := []Validation{
		{ID: "validation-low", Confidence: 7},
	}

	if got := FilterFindings(findings, validations); len(got) != 0 {
		t.Fatalf("expected low-confidence findings to be dropped, got %#v", got)
	}
}

func TestFilterFindingsKeepsHighConfidenceValidatedFindings(t *testing.T) {
	findings := []Finding{
		{
			ID:              "high",
			File:            "src/server/handlers.go",
			Line:            42,
			Severity:        "High",
			Category:        "command_injection",
			Description:     "Untrusted request input reaches a shell command.",
			ExploitScenario: "An attacker controls the command argument.",
			Recommendation:  "Pass arguments without shell interpolation.",
			Confidence:      8,
		},
	}
	validations := []Validation{
		{ID: "high", Confidence: 0.8},
	}

	got := FilterFindings(findings, validations)
	if len(got) != 1 {
		t.Fatalf("expected one high-confidence finding, got %#v", got)
	}
	if got[0].ID != "high" {
		t.Fatalf("unexpected finding retained: %#v", got[0])
	}
}

func TestFilterFindingsDropsHardExclusions(t *testing.T) {
	findings := []Finding{
		{
			ID:              "dos",
			Category:        "denial-of-service",
			Description:     "Large payloads create resource-exhaustion.",
			ExploitScenario: "Attacker can disrupt service.",
			Confidence:      0.95,
		},
		{
			ID:              "secrets",
			Category:        "secrets_on_disk",
			Description:     "Credentials are stored on disk as a policy-only concern.",
			ExploitScenario: "No concrete exploit path beyond local disk access.",
			Confidence:      0.95,
		},
		{
			ID:              "rate-limit",
			Category:        "rate_limit",
			Description:     "Endpoint has no rate limit.",
			ExploitScenario: "Attacker can overload the service.",
			Confidence:      0.95,
		},
	}
	validations := []Validation{
		{ID: "dos", Confidence: 10},
		{ID: "secrets", Confidence: 10},
		{ID: "rate-limit", Confidence: 10},
	}

	if got := FilterFindings(findings, validations); len(got) != 0 {
		t.Fatalf("expected hard exclusions to be dropped, got %#v", got)
	}
}

func TestFilterFindingsDropsFalsePositiveValidations(t *testing.T) {
	findings := []Finding{
		{ID: "fp", Category: "path_traversal", Confidence: 0.95},
	}
	validations := []Validation{
		{ID: "fp", Confidence: 10, FalsePositive: true},
	}

	if got := FilterFindings(findings, validations); len(got) != 0 {
		t.Fatalf("expected false positive validation to drop finding, got %#v", got)
	}
}

func TestFormatReportUsesMarkdownFindingFormat(t *testing.T) {
	report := FormatReport([]Finding{
		{
			ID:              "xss-1",
			File:            "src/server/view.go",
			Line:            17,
			Severity:        "High",
			Category:        "xss",
			Description:     "User-controlled HTML is rendered with an unsafe API.",
			ExploitScenario: "An attacker injects JavaScript that runs in another user's browser.",
			Recommendation:  "Escape user input before rendering HTML.",
			Confidence:      9,
		},
	})

	expected := []string{
		"# Security Review Report",
		"# Vuln 1: Xss: `src/server/view.go:17`",
		"* Severity: High",
		"* Category: `xss`",
		"* Confidence: 0.90",
		"* Description: User-controlled HTML is rendered with an unsafe API.",
		"* Exploit Scenario: An attacker injects JavaScript that runs in another user's browser.",
		"* Recommendation: Escape user input before rendering HTML.",
	}

	for _, value := range expected {
		if !strings.Contains(report, value) {
			t.Fatalf("expected report to contain %q\nreport:\n%s", value, report)
		}
	}
}

func TestFormatReportHandlesEmptyFindings(t *testing.T) {
	report := FormatReport(nil)
	if !strings.Contains(report, "# Security Review Report") {
		t.Fatalf("expected markdown heading, got %q", report)
	}
	if !strings.Contains(report, "No high-confidence security vulnerabilities found.") {
		t.Fatalf("expected empty report message, got %q", report)
	}
}
