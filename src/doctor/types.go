package doctor

// Status describes the outcome of a single diagnostic check.
type Status string

const (
	StatusOK      Status = "ok"
	StatusWarning Status = "warning"
	StatusFailure Status = "failure"
	StatusInfo    Status = "info"
)

// Severity describes how strongly a diagnostic should affect readiness.
type Severity string

const (
	SeverityInfo    Severity = "info"
	SeverityWarning Severity = "warning"
	SeverityError   Severity = "error"
)

// CheckResult is a structured, display-neutral diagnostic result.
type CheckResult struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Category       string   `json:"category"`
	Status         Status   `json:"status"`
	Severity       Severity `json:"severity"`
	Summary        string   `json:"summary"`
	Details        []string `json:"details,omitempty"`
	Remediation    []string `json:"remediation,omitempty"`
	RedactedOutput string   `json:"redacted_output,omitempty"`
}

// ReportSummary is the aggregate status and count breakdown for a report.
type ReportSummary struct {
	Status   Status `json:"status"`
	OK       int    `json:"ok"`
	Warnings int    `json:"warnings"`
	Failures int    `json:"failures"`
	Info     int    `json:"info"`
}

// Report contains all diagnostic checks plus an aggregate summary.
type Report struct {
	Summary ReportSummary `json:"summary"`
	Checks  []CheckResult `json:"checks"`
}

// NewReport aggregates check results into a report.
func NewReport(checks []CheckResult) Report {
	summary := ReportSummary{Status: StatusOK}
	for _, check := range checks {
		switch check.Status {
		case StatusFailure:
			summary.Failures++
		case StatusWarning:
			summary.Warnings++
		case StatusInfo:
			summary.Info++
		default:
			summary.OK++
		}
	}

	switch {
	case summary.Failures > 0:
		summary.Status = StatusFailure
	case summary.Warnings > 0:
		summary.Status = StatusWarning
	default:
		summary.Status = StatusOK
	}

	return Report{Summary: summary, Checks: checks}
}
