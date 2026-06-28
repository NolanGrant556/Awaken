package toolsafety

// Operation describes the file-system action being classified.
type Operation string

const (
	OperationRead   Operation = "read"
	OperationWrite  Operation = "write"
	OperationDelete Operation = "delete"
)

// Decision is the classifier's allow/deny result.
type Decision string

const (
	DecisionAllowed Decision = "allowed"
	DecisionDenied  Decision = "denied"
)

// Severity is a UI-friendly risk level.
type Severity string

const (
	SeverityInfo    Severity = "info"
	SeverityWarning Severity = "warning"
	SeverityDanger  Severity = "danger"
)

// Finding records one path or shell-command safety classification.
type Finding struct {
	Kind         string
	Operation    Operation
	Input        string
	Command      string
	Pattern      string
	Decision     Decision
	Severity     Severity
	Reason       string
	ResolvedPath string
}

// Policy contains path safety settings.
type Policy struct {
	HomeDir          string
	BaseDir          string
	SafeRoot         string
	DeniedExactPaths []string
	DeniedPrefixes   []string
}

// BatchResult summarizes multiple path and command findings for approval UI.
type BatchResult struct {
	Allowed  bool
	Severity Severity
	Findings []Finding
}

// NewBatchResult returns an aggregate result for the provided findings.
func NewBatchResult(findings ...Finding) BatchResult {
	result := BatchResult{Allowed: true, Severity: SeverityInfo}
	result.Add(findings...)
	return result
}

// Add appends findings and refreshes the aggregate decision and severity.
func (b *BatchResult) Add(findings ...Finding) {
	if b.Severity == "" && len(b.Findings) == 0 {
		b.Allowed = true
		b.Severity = SeverityInfo
	}
	for _, finding := range findings {
		b.Findings = append(b.Findings, finding)
		if finding.Decision == DecisionDenied {
			b.Allowed = false
		}
		if severityRank(finding.Severity) > severityRank(b.Severity) {
			b.Severity = finding.Severity
		}
	}
}

// AddPath classifies a path and appends its finding.
func (b *BatchResult) AddPath(operation Operation, path string, policy Policy) Finding {
	finding := ClassifyPath(operation, path, policy)
	b.Add(finding)
	return finding
}

// AddShell classifies a shell command and appends its findings.
func (b *BatchResult) AddShell(command string, policy Policy) []Finding {
	findings := ClassifyShell(command, policy)
	b.Add(findings...)
	return findings
}

func severityRank(severity Severity) int {
	switch severity {
	case SeverityDanger:
		return 3
	case SeverityWarning:
		return 2
	case SeverityInfo:
		return 1
	default:
		return 0
	}
}
