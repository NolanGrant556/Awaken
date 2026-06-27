package truststate

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Decision string

const (
	DecisionTrusted     Decision = "trusted"
	DecisionUntrusted   Decision = "untrusted"
	DecisionNeedsReview Decision = "needs_review"
)

type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityLow      Severity = "low"
	SeverityMedium   Severity = "medium"
	SeverityHigh     Severity = "high"
	SeverityCritical Severity = "critical"
)

type RiskSignalKind string

const (
	SignalUnknown                RiskSignalKind = "unknown"
	SignalProjectSettingsPresent RiskSignalKind = "project_settings_present"
	SignalLocalSettingsPresent   RiskSignalKind = "local_settings_present"
	SignalHooksPresent           RiskSignalKind = "hooks_present"
	SignalBashAllowRule          RiskSignalKind = "bash_allow_rule"
	SignalDangerousEnvVar        RiskSignalKind = "dangerous_env_var"
	SignalLocalExecutionBackend  RiskSignalKind = "local_execution_backend"
	SignalRemoteGatewayCaller    RiskSignalKind = "remote_gateway_caller"
)

type RiskSignal struct {
	Kind     RiskSignalKind
	Severity Severity
	Source   string
	Detail   string
}

type Policy struct {
	NoSignalDecision Decision
}

type WorkspaceInput struct {
	WorkspaceRoot string
	Signals       []RiskSignal
	Acceptance    *Acceptance
	Fingerprint   string
	Revision      string
	Now           time.Time
	Policy        Policy
}

type Acceptance struct {
	AcceptedAt    time.Time
	AcceptedBy    string
	WorkspaceRoot string
	Fingerprint   string
	Revision      string
	ExpiresAt     time.Time
}

type AcceptanceStatus string

const (
	AcceptanceNotPresent        AcceptanceStatus = "not_present"
	AcceptanceValid             AcceptanceStatus = "valid"
	AcceptanceIncomplete        AcceptanceStatus = "incomplete"
	AcceptanceExpired           AcceptanceStatus = "expired"
	AcceptanceWorkspaceMismatch AcceptanceStatus = "workspace_mismatch"
	AcceptanceFingerprintDrift  AcceptanceStatus = "fingerprint_drift"
	AcceptanceRevisionDrift     AcceptanceStatus = "revision_drift"
)

type AcceptanceReport struct {
	Status AcceptanceStatus
	Reason string
}

func (r AcceptanceReport) StillValid() bool {
	return r.Status == AcceptanceValid
}

type State struct {
	WorkspaceRoot string
	Decision      Decision
	Reasons       []string
	Signals       []RiskSignal
	Fingerprint   string
	Revision      string
	Acceptance    AcceptanceReport
}

type Summary struct {
	WorkspaceRoot string
	Decision      Decision
	Reasons       []string
	Signals       []SignalSummary
	Fingerprint   string
	Revision      string
	Acceptance    AcceptanceReport
}

type SignalSummary struct {
	Kind     RiskSignalKind
	Severity Severity
	Source   string
	Detail   string
	Reason   string
}

func Evaluate(input WorkspaceInput) State {
	root := normalizeWorkspaceRoot(input.WorkspaceRoot)
	revision := strings.TrimSpace(input.Revision)
	signals := NormalizeSignals(input.Signals)
	fingerprint := strings.TrimSpace(input.Fingerprint)
	if fingerprint == "" {
		fingerprint = ComputeFingerprint(WorkspaceInput{
			WorkspaceRoot: root,
			Signals:       signals,
			Revision:      revision,
		})
	}
	now := input.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}

	acceptance := evaluateAcceptance(input.Acceptance, root, fingerprint, revision, now)
	decision := decide(input.Policy, signals, acceptance)

	return State{
		WorkspaceRoot: root,
		Decision:      decision,
		Reasons:       buildReasons(input.Policy, signals, acceptance, decision),
		Signals:       signals,
		Fingerprint:   fingerprint,
		Revision:      revision,
		Acceptance:    acceptance,
	}
}

func NormalizeSignals(signals []RiskSignal) []RiskSignal {
	normalized := make([]RiskSignal, 0, len(signals))
	seen := map[string]struct{}{}
	for _, signal := range signals {
		signal = normalizeSignal(signal)
		key := signalKey(signal)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, signal)
	}
	sort.SliceStable(normalized, func(i, j int) bool {
		left := normalized[i]
		right := normalized[j]
		if severityRank(left.Severity) != severityRank(right.Severity) {
			return severityRank(left.Severity) > severityRank(right.Severity)
		}
		if left.Kind != right.Kind {
			return left.Kind < right.Kind
		}
		if left.Source != right.Source {
			return left.Source < right.Source
		}
		return left.Detail < right.Detail
	})
	return normalized
}

func ComputeFingerprint(input WorkspaceInput) string {
	root := normalizeWorkspaceRoot(input.WorkspaceRoot)
	revision := strings.TrimSpace(input.Revision)
	signals := NormalizeSignals(input.Signals)

	hasher := sha256.New()
	writeHashPart(hasher, root)
	writeHashPart(hasher, revision)
	for _, signal := range signals {
		writeHashPart(hasher, string(signal.Kind))
		writeHashPart(hasher, string(signal.Severity))
		writeHashPart(hasher, signal.Source)
		writeHashPart(hasher, signal.Detail)
	}
	return "sha256:" + hex.EncodeToString(hasher.Sum(nil))
}

func (s State) Summary() Summary {
	signals := make([]SignalSummary, 0, len(s.Signals))
	for _, signal := range s.Signals {
		signals = append(signals, SignalSummary{
			Kind:     signal.Kind,
			Severity: signal.Severity,
			Source:   redactSecretAssignments(signal.Source, false),
			Detail:   redactedDetail(signal),
			Reason:   signalReason(signal),
		})
	}
	reasons := append([]string(nil), s.Reasons...)
	return Summary{
		WorkspaceRoot: s.WorkspaceRoot,
		Decision:      s.Decision,
		Reasons:       reasons,
		Signals:       signals,
		Fingerprint:   s.Fingerprint,
		Revision:      s.Revision,
		Acceptance:    s.Acceptance,
	}
}

func normalizeSignal(signal RiskSignal) RiskSignal {
	kind := RiskSignalKind(strings.TrimSpace(string(signal.Kind)))
	if kind == "" {
		kind = SignalUnknown
	}
	severity := Severity(strings.TrimSpace(string(signal.Severity)))
	if !isKnownSeverity(severity) {
		severity = defaultSeverity(kind)
	}
	return RiskSignal{
		Kind:     kind,
		Severity: severity,
		Source:   strings.TrimSpace(signal.Source),
		Detail:   strings.TrimSpace(signal.Detail),
	}
}

func decide(policy Policy, signals []RiskSignal, acceptance AcceptanceReport) Decision {
	if acceptance.StillValid() {
		return DecisionTrusted
	}
	if len(signals) == 0 {
		return noSignalDecision(policy)
	}
	for _, signal := range signals {
		if signal.Severity == SeverityCritical {
			return DecisionUntrusted
		}
	}
	return DecisionNeedsReview
}

func buildReasons(policy Policy, signals []RiskSignal, acceptance AcceptanceReport, decision Decision) []string {
	if acceptance.StillValid() {
		return []string{"operator acceptance matches the current workspace trust fingerprint"}
	}
	if len(signals) == 0 {
		if noSignalDecision(policy) == DecisionNeedsReview {
			return []string{"policy requires workspace trust review even though no risk signals were reported"}
		}
		if noSignalDecision(policy) == DecisionUntrusted {
			return []string{"policy marks workspaces untrusted until reviewed"}
		}
		return []string{"no workspace trust risk signals were reported"}
	}

	reasons := make([]string, 0, len(signals)+2)
	for _, signal := range signals {
		reasons = append(reasons, signalReason(signal))
	}
	if acceptance.Status != AcceptanceNotPresent {
		reasons = append(reasons, "previous operator acceptance is not valid: "+acceptance.Reason)
	}
	if decision == DecisionUntrusted {
		reasons = append(reasons, "critical workspace trust risk signals cannot be trusted without matching operator acceptance")
	}
	return reasons
}

func evaluateAcceptance(acceptance *Acceptance, root, fingerprint, revision string, now time.Time) AcceptanceReport {
	if acceptance == nil {
		return AcceptanceReport{
			Status: AcceptanceNotPresent,
			Reason: "no operator acceptance was provided",
		}
	}
	acceptedBy := strings.TrimSpace(acceptance.AcceptedBy)
	acceptedRoot := normalizeWorkspaceRoot(acceptance.WorkspaceRoot)
	acceptedFingerprint := strings.TrimSpace(acceptance.Fingerprint)
	acceptedRevision := strings.TrimSpace(acceptance.Revision)

	if acceptedBy == "" || acceptance.AcceptedAt.IsZero() || acceptedRoot == "" {
		return AcceptanceReport{
			Status: AcceptanceIncomplete,
			Reason: "operator acceptance is missing acceptedAt, acceptedBy, or workspaceRoot",
		}
	}
	if acceptedRoot != root {
		return AcceptanceReport{
			Status: AcceptanceWorkspaceMismatch,
			Reason: fmt.Sprintf("accepted workspace %q does not match current workspace %q", acceptedRoot, root),
		}
	}
	if !acceptance.ExpiresAt.IsZero() && !now.Before(acceptance.ExpiresAt) {
		return AcceptanceReport{
			Status: AcceptanceExpired,
			Reason: "operator acceptance has expired",
		}
	}
	if acceptedFingerprint == "" && acceptedRevision == "" {
		return AcceptanceReport{
			Status: AcceptanceIncomplete,
			Reason: "operator acceptance is missing fingerprint or revision",
		}
	}
	if acceptedFingerprint != "" && acceptedFingerprint != fingerprint {
		return AcceptanceReport{
			Status: AcceptanceFingerprintDrift,
			Reason: "workspace trust fingerprint changed since acceptance",
		}
	}
	if acceptedRevision != "" && revision != "" && acceptedRevision != revision {
		return AcceptanceReport{
			Status: AcceptanceRevisionDrift,
			Reason: "workspace revision changed since acceptance",
		}
	}
	if acceptedRevision != "" && revision == "" && acceptedFingerprint == "" {
		return AcceptanceReport{
			Status: AcceptanceIncomplete,
			Reason: "current workspace revision is unavailable",
		}
	}
	return AcceptanceReport{
		Status: AcceptanceValid,
		Reason: "operator acceptance is still valid",
	}
}

func signalReason(signal RiskSignal) string {
	reason := baseSignalReason(signal.Kind)
	source := redactSecretAssignments(signal.Source, false)
	detail := redactedDetail(signal)
	if source != "" {
		reason += " (" + source + ")"
	}
	if detail != "" {
		reason += ": " + detail
	}
	return reason
}

func baseSignalReason(kind RiskSignalKind) string {
	switch kind {
	case SignalProjectSettingsPresent:
		return "project settings can change agent behavior for this workspace"
	case SignalLocalSettingsPresent:
		return "local settings can grant machine-specific authority"
	case SignalHooksPresent:
		return "hooks can run commands during agent or tool lifecycle events"
	case SignalBashAllowRule:
		return "bash allow rules can permit shell commands without per-command review"
	case SignalDangerousEnvVar:
		return "dangerous environment variables can change tool behavior or expose authority"
	case SignalLocalExecutionBackend:
		return "local execution backends can mutate files and processes on the operator machine"
	case SignalRemoteGatewayCaller:
		return "remote or gateway caller identity is evidence, not an authorization boundary"
	default:
		return "unknown workspace trust risk signal was reported"
	}
}

func defaultSeverity(kind RiskSignalKind) Severity {
	switch kind {
	case SignalProjectSettingsPresent:
		return SeverityLow
	case SignalLocalSettingsPresent, SignalRemoteGatewayCaller:
		return SeverityMedium
	case SignalHooksPresent, SignalBashAllowRule, SignalDangerousEnvVar, SignalLocalExecutionBackend:
		return SeverityHigh
	default:
		return SeverityLow
	}
}

func noSignalDecision(policy Policy) Decision {
	switch policy.NoSignalDecision {
	case DecisionNeedsReview, DecisionUntrusted, DecisionTrusted:
		return policy.NoSignalDecision
	default:
		return DecisionTrusted
	}
}

func isKnownSeverity(severity Severity) bool {
	switch severity {
	case SeverityInfo, SeverityLow, SeverityMedium, SeverityHigh, SeverityCritical:
		return true
	default:
		return false
	}
}

func severityRank(severity Severity) int {
	switch severity {
	case SeverityCritical:
		return 4
	case SeverityHigh:
		return 3
	case SeverityMedium:
		return 2
	case SeverityLow:
		return 1
	case SeverityInfo:
		return 0
	default:
		return -1
	}
}

func signalKey(signal RiskSignal) string {
	return strings.Join([]string{
		string(signal.Kind),
		string(signal.Severity),
		signal.Source,
		signal.Detail,
	}, "\x00")
}

func normalizeWorkspaceRoot(root string) string {
	root = strings.TrimSpace(root)
	if root == "" {
		return ""
	}
	return filepath.Clean(root)
}

func writeHashPart(hasher hash.Hash, value string) {
	hasher.Write([]byte(value))
	hasher.Write([]byte{0})
}

func redactedDetail(signal RiskSignal) string {
	if signal.Detail == "" {
		return ""
	}
	return redactSecretAssignments(signal.Detail, signal.Kind == SignalDangerousEnvVar)
}

func redactSecretAssignments(value string, redactPlainValue bool) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	key, _, ok := strings.Cut(value, "=")
	if ok {
		key = strings.TrimSpace(key)
		if key == "" {
			return "<redacted>"
		}
		if redactPlainValue || looksSensitiveName(key) {
			return key + "=<redacted>"
		}
		return value
	}
	if redactPlainValue {
		return "<redacted>"
	}
	return value
}

func looksSensitiveName(name string) bool {
	upper := strings.ToUpper(name)
	for _, marker := range []string{"KEY", "TOKEN", "SECRET", "PASSWORD", "CREDENTIAL", "AUTH"} {
		if strings.Contains(upper, marker) {
			return true
		}
	}
	return false
}
