package agentcore

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"awaken/src/context"
	"awaken/src/tool"
)

const (
	DefaultMaxAgentTurns     = 128
	DefaultMaxToolExecutions = 512

	maxRetryableSameCallAttempts    = 2
	maxNonRetryableSameCallAttempts = 1
	maxSameClassFailures            = 3
	maxTurnToolFailures             = 6
	maxRunToolFailures              = 12

	ErrorCodeToolRetryBlocked = "tool_retry_blocked"
	ErrorCodeRepeatedFailure  = "repeated_tool_failure"
)

// FailureSupervisor enforces hard budgets and recovery policy for repeated
// tool failures within a single run.
type FailureSupervisor struct {
	workspaceRoot string

	callAttempts       map[string]int
	callFailureMeta    map[string]callFailureMeta
	classFailures      map[string]int
	toolConsecutiveFail map[string]int

	lastFingerprint       string
	sameFingerprintStreak int
	pendingRecovery       *RecoveryDirective

	turnFailures   int
	runFailures    int
	toolExecutions int

	maxAgentTurns     int
	maxToolExecutions int
}

type callFailureMeta struct {
	retryable bool
	errorCode string
	attempts  int
}

// RecoveryDirective is injected into the next model turn after repeated failures.
type RecoveryDirective struct {
	Required        bool
	ToolName        string
	ErrorCode       string
	Fingerprint     string
	AttemptCount    int
	RecoveryOptions []string
	Message         string
}

// BeforeCallDecision is returned before a real tool execution.
type BeforeCallDecision struct {
	Allow            bool
	Block            bool
	ErrorCode        string
	ErrorMessage     string
	RecoveryOptions  []string
	TerminateTurn    bool
	TerminateRun     bool
	TerminalReason   TerminalReason
	CallSignature    string
	AttemptCount     int
	RecoveryRequired bool
}

// AfterOutcomeDecision is returned after a real tool outcome is observed.
type AfterOutcomeDecision struct {
	Fingerprint      string
	RecoveryRequired bool
	RecoveryOptions  []string
	TerminateTurn    bool
	TerminateRun     bool
	TerminalReason   TerminalReason
	Directive        *RecoveryDirective
}

func NewFailureSupervisor(workspaceRoot string, maxAgentTurns, maxToolExecutions int) *FailureSupervisor {
	return &FailureSupervisor{
		workspaceRoot:       strings.TrimSpace(workspaceRoot),
		callAttempts:        map[string]int{},
		callFailureMeta:     map[string]callFailureMeta{},
		classFailures:       map[string]int{},
		toolConsecutiveFail: map[string]int{},
		maxAgentTurns:       EffectiveMaxAgentTurns(maxAgentTurns),
		maxToolExecutions:   EffectiveMaxToolExecutions(maxToolExecutions),
	}
}

// EffectiveMaxAgentTurns maps config 0 to the product default.
func EffectiveMaxAgentTurns(configured int) int {
	if configured <= 0 {
		return DefaultMaxAgentTurns
	}
	return configured
}

// EffectiveMaxToolExecutions maps config 0 to the product default.
func EffectiveMaxToolExecutions(configured int) int {
	if configured <= 0 {
		return DefaultMaxToolExecutions
	}
	return configured
}

func (s *FailureSupervisor) BeginTurn() {
	if s == nil {
		return
	}
	s.turnFailures = 0
}

func (s *FailureSupervisor) PendingRecovery() *RecoveryDirective {
	if s == nil {
		return nil
	}
	return s.pendingRecovery
}

func (s *FailureSupervisor) ClearPendingRecovery() {
	if s == nil {
		return
	}
	s.pendingRecovery = nil
}

func (s *FailureSupervisor) MaxAgentTurns() int {
	if s == nil {
		return DefaultMaxAgentTurns
	}
	return s.maxAgentTurns
}

func (s *FailureSupervisor) MaxToolExecutions() int {
	if s == nil {
		return DefaultMaxToolExecutions
	}
	return s.maxToolExecutions
}

func (s *FailureSupervisor) ToolExecutions() int {
	if s == nil {
		return 0
	}
	return s.toolExecutions
}

// BeforeCall decides whether a tool call may execute.
func (s *FailureSupervisor) BeforeCall(call context.ToolCall) BeforeCallDecision {
	if s == nil {
		return BeforeCallDecision{Allow: true}
	}
	toolName := strings.TrimSpace(call.Function.Name)
	signature := CallSignature(toolName, call.Function.Arguments)
	decision := BeforeCallDecision{
		Allow:         true,
		CallSignature: signature,
	}

	if s.toolExecutions >= s.maxToolExecutions {
		decision.Allow = false
		decision.Block = true
		decision.TerminateRun = true
		decision.TerminalReason = TerminalReasonRepeatedToolFailure
		decision.ErrorCode = ErrorCodeRepeatedFailure
		decision.ErrorMessage = fmt.Sprintf("tool execution budget exceeded (%d)", s.maxToolExecutions)
		decision.RecoveryOptions = []string{"stop_and_summarize", "change_strategy"}
		return decision
	}

	meta, failedBefore := s.callFailureMeta[signature]
	attempts := s.callAttempts[signature]
	decision.AttemptCount = attempts

	if failedBefore {
		limit := maxRetryableSameCallAttempts
		if !meta.retryable {
			limit = maxNonRetryableSameCallAttempts
		}
		if attempts >= limit {
			decision.Allow = false
			decision.Block = true
			decision.ErrorCode = ErrorCodeToolRetryBlocked
			decision.ErrorMessage = fmt.Sprintf(
				"blocked repeated tool call %s after %d failure(s); error_code=%s retryable=%v",
				toolName, attempts, meta.errorCode, meta.retryable,
			)
			decision.RecoveryOptions = RecoveryOptionsForError(meta.errorCode)
			if s.pendingRecovery != nil {
				decision.RecoveryRequired = true
				decision.RecoveryOptions = mergeUniqueStrings(decision.RecoveryOptions, s.pendingRecovery.RecoveryOptions)
			}
			return decision
		}
	}

	if s.pendingRecovery != nil {
		decision.RecoveryRequired = true
	}
	return decision
}

// NoteRealExecution increments the real execution counter after BeforeCall allows.
func (s *FailureSupervisor) NoteRealExecution(signature string) {
	if s == nil {
		return
	}
	s.toolExecutions++
	s.callAttempts[signature] = s.callAttempts[signature] + 1
}

// AfterOutcome records a real tool outcome and may require recovery or terminate.
func (s *FailureSupervisor) AfterOutcome(call context.ToolCall, outcome tool.ExecutionOutcome) AfterOutcomeDecision {
	if s == nil {
		return AfterOutcomeDecision{}
	}
	toolName := strings.TrimSpace(call.Function.Name)
	signature := CallSignature(toolName, call.Function.Arguments)
	revision := ResourceRevision(call.Function.Arguments, outcome)
	errorCode := strings.TrimSpace(outcome.ErrorCode)
	errorClass := ErrorClass(errorCode, outcome)
	fingerprint := FailureFingerprint(toolName, errorCode, outcome, s.workspaceRoot)

	decision := AfterOutcomeDecision{
		Fingerprint:     fingerprint,
		RecoveryOptions: RecoveryOptionsForError(errorCode),
	}
	if len(outcome.RecoveryOptions) > 0 {
		decision.RecoveryOptions = mergeUniqueStrings(outcome.RecoveryOptions, decision.RecoveryOptions)
	}

	isFailure := outcome.IsError()
	if !isFailure {
		s.toolConsecutiveFail[toolName] = 0
		s.lastFingerprint = ""
		s.sameFingerprintStreak = 0
		return decision
	}

	s.turnFailures++
	s.runFailures++
	s.toolConsecutiveFail[toolName] = s.toolConsecutiveFail[toolName] + 1
	s.callFailureMeta[signature] = callFailureMeta{
		retryable: outcome.Retryable,
		errorCode: errorCode,
		attempts:  s.callAttempts[signature],
	}

	classKey := toolName + "|" + revision + "|" + errorClass
	s.classFailures[classKey] = s.classFailures[classKey] + 1

	if fingerprint != "" && fingerprint == s.lastFingerprint {
		s.sameFingerprintStreak++
	} else {
		s.lastFingerprint = fingerprint
		s.sameFingerprintStreak = 1
	}

	if s.sameFingerprintStreak >= 2 {
		decision.RecoveryRequired = true
		directive := &RecoveryDirective{
			Required:        true,
			ToolName:        toolName,
			ErrorCode:       errorCode,
			Fingerprint:     fingerprint,
			AttemptCount:    s.callAttempts[signature],
			RecoveryOptions: decision.RecoveryOptions,
			Message: fmt.Sprintf(
				"recovery_required: tool=%s error_code=%s attempts=%d; change strategy before retrying the same call",
				toolName, errorCode, s.callAttempts[signature],
			),
		}
		s.pendingRecovery = directive
		decision.Directive = directive
	}

	if s.classFailures[classKey] >= maxSameClassFailures {
		decision.RecoveryRequired = true
		decision.RecoveryOptions = mergeUniqueStrings(decision.RecoveryOptions, []string{
			"change_tool_or_refresh_resource_revision",
			"stop_repeating_same_error_class",
		})
		if decision.Directive == nil {
			decision.Directive = &RecoveryDirective{
				Required:        true,
				ToolName:        toolName,
				ErrorCode:       errorCode,
				Fingerprint:     fingerprint,
				AttemptCount:    s.classFailures[classKey],
				RecoveryOptions: decision.RecoveryOptions,
				Message: fmt.Sprintf(
					"recovery_required: same tool/resource/error class failed %d times",
					s.classFailures[classKey],
				),
			}
			s.pendingRecovery = decision.Directive
		}
	}

	if s.turnFailures >= maxTurnToolFailures {
		decision.TerminateTurn = true
		decision.TerminateRun = true
		decision.TerminalReason = TerminalReasonRepeatedToolFailure
	}
	if s.runFailures >= maxRunToolFailures {
		decision.TerminateRun = true
		decision.TerminalReason = TerminalReasonRepeatedToolFailure
	}
	return decision
}

// ApplyRecoveryToOutcome merges supervisor recovery options into an outcome.
func ApplyRecoveryToOutcome(outcome tool.ExecutionOutcome, options []string, required bool) tool.ExecutionOutcome {
	outcome.RecoveryOptions = mergeUniqueStrings(outcome.RecoveryOptions, options)
	if required {
		if outcome.Diagnostic == nil {
			outcome.Diagnostic = map[string]any{}
		}
		outcome.Diagnostic["recovery_required"] = true
	}
	return outcome
}

// SyntheticBlockedResult builds a ToolExecutionResult for a blocked retry.
func SyntheticBlockedResult(call context.ToolCall, decision BeforeCallDecision) ToolExecutionResult {
	now := time.Now().UTC()
	msg := strings.TrimSpace(decision.ErrorMessage)
	if msg == "" {
		msg = "tool retry blocked by FailureSupervisor"
	}
	outcome := tool.ExecutionOutcome{
		Content:         msg,
		Status:          tool.OutcomeDenied,
		TransportStatus: tool.TransportNotStarted,
		SemanticStatus:  tool.SemanticFailure,
		ProcessStatus:   tool.ProcessNotApplicable,
		ErrorCode:       firstNonEmptyString(decision.ErrorCode, ErrorCodeToolRetryBlocked),
		ErrorCategory:   tool.ErrorCategoryPolicy,
		ErrorMessage:    msg,
		Retryable:       false,
		RecoveryOptions: append([]string(nil), decision.RecoveryOptions...),
		Diagnostic: map[string]any{
			"blocked":          true,
			"call_signature":   decision.CallSignature,
			"attempt_count":    decision.AttemptCount,
			"recovery_required": decision.RecoveryRequired,
		},
		StartedAt:  now,
		FinishedAt: now,
		DurationMs: 0,
	}
	return ToolExecutionResult{
		ToolCallID: call.ID,
		Name:       call.Function.Name,
		Content:    outcome.ModelPayload(),
		IsError:    true,
		Status:     ToolExecutionDenied,
		ErrorCode:  outcome.ErrorCode,
		Outcome:    outcome,
		StartedAt:  now,
		FinishedAt: now,
	}
}

func CallSignature(toolName, arguments string) string {
	return strings.TrimSpace(toolName) + "|" + hashCanonicalArguments(arguments)
}

func FailureFingerprint(toolName, errorCode string, outcome tool.ExecutionOutcome, workspaceRoot string) string {
	primary := primaryDiagnostic(outcome)
	normalized := normalizeDiagnostic(primary, workspaceRoot)
	raw := strings.TrimSpace(toolName) + "|" + strings.TrimSpace(errorCode) + "|" + normalized
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func ResourceRevision(arguments string, outcome tool.ExecutionOutcome) string {
	if outcome.Diagnostic != nil {
		for _, key := range []string{"sha256", "expected_sha256", "after_sha256", "before_sha256", "resource_revision"} {
			if v, ok := outcome.Diagnostic[key]; ok {
				if s := strings.TrimSpace(fmt.Sprint(v)); s != "" && s != "<nil>" {
					return s
				}
			}
		}
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(arguments), &payload); err == nil {
		for _, key := range []string{"expected_sha256", "sha256", "resource_revision"} {
			if v, ok := payload[key]; ok {
				if s := strings.TrimSpace(fmt.Sprint(v)); s != "" && s != "<nil>" {
					return s
				}
			}
		}
		if path, ok := payload["path"]; ok {
			if s := strings.TrimSpace(fmt.Sprint(path)); s != "" {
				return "path:" + s
			}
		}
	}
	return "unknown"
}

func ErrorClass(errorCode string, outcome tool.ExecutionOutcome) string {
	code := strings.TrimSpace(errorCode)
	switch {
	case code == "" && outcome.ErrorCategory != "":
		return string(outcome.ErrorCategory)
	case isInvalidPatchFormatCode(code):
		return "invalid_patch_format"
	case isPatchContextMismatchCode(code):
		return "patch_context_mismatch"
	case code != "":
		return code
	default:
		return "unknown"
	}
}

func RecoveryOptionsForError(errorCode string) []string {
	code := strings.TrimSpace(errorCode)
	switch {
	case isInvalidPatchFormatCode(code):
		return []string{"use_text_edit_or_text_file_create", "re_read_file", "do_not_replay_same_patch"}
	case isPatchContextMismatchCode(code):
		return []string{"read_file", "use_text_edit", "do_not_replay_same_patch"}
	case code == "text_not_unique":
		return []string{"expand_old_string_context", "set_replace_all_true"}
	case code == "text_not_found":
		return []string{"read_file", "retry_text_edit_with_current_content"}
	case code == "stale_file_revision":
		return []string{"read_file", "retry_text_edit_with_fresh_digest"}
	case code == tool.ErrorCodeProcessExitNonzero:
		return []string{"inspect_exit_code_and_stderr", "change_command_or_stop"}
	case code == tool.ErrorCodeNeedsRuntime || code == "needs_runtime":
		return []string{"use_host_runtime_manager", "do_not_shell_install_for_artifact_skill"}
	case code == tool.ErrorCodeSkillToolNotAllowed:
		return []string{"recompile_skill_activation_plan", "do_not_shell_around_denied_tool"}
	case code == "skill_platform_contract_conflict":
		return []string{"exit_skill", "plan_typed_platform_tool_outside_skill_scope"}
	case code == "persistence_error" || code == "persistence_unavailable":
		return []string{"stop_new_side_effects", "wait_for_journal_recovery_or_terminate"}
	case code == ErrorCodeToolRetryBlocked:
		return []string{"change_strategy", "do_not_repeat_identical_call"}
	default:
		if code != "" {
			return []string{"change_strategy", "inspect_error_and_stop_if_non_retryable"}
		}
		return nil
	}
}

func isInvalidPatchFormatCode(code string) bool {
	switch code {
	case "invalid_patch_format",
		"patch_missing_begin",
		"patch_missing_end",
		"patch_invalid_hunk_header",
		"patch_invalid_line_prefix":
		return true
	default:
		return false
	}
}

func isPatchContextMismatchCode(code string) bool {
	switch code {
	case "patch_context_mismatch",
		"patch_context_not_found",
		"patch_context_ambiguous",
		"patch_stale_file":
		return true
	default:
		return false
	}
}

func hashCanonicalArguments(arguments string) string {
	canonical := canonicalizeArguments(arguments)
	sum := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(sum[:8])
}

func canonicalizeArguments(arguments string) string {
	trimmed := strings.TrimSpace(arguments)
	if trimmed == "" {
		return ""
	}
	var raw any
	if err := json.Unmarshal([]byte(trimmed), &raw); err != nil {
		return normalizePatchText(trimmed)
	}
	normalized := normalizeJSONValue(raw)
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return normalizePatchText(trimmed)
	}
	return string(encoded)
}

func normalizeJSONValue(v any) any {
	switch typed := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		out := make(map[string]any, len(typed))
		for _, key := range keys {
			value := typed[key]
			if key == "patch" || key == "old_string" || key == "new_string" || key == "content" {
				if s, ok := value.(string); ok {
					out[key] = normalizePatchText(s)
					continue
				}
			}
			out[key] = normalizeJSONValue(value)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = normalizeJSONValue(item)
		}
		return out
	case string:
		return normalizePatchText(typed)
	default:
		return typed
	}
}

func normalizePatchText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	return strings.Join(lines, "\n")
}

func primaryDiagnostic(outcome tool.ExecutionOutcome) string {
	if outcome.ErrorMessage != "" {
		return outcome.ErrorMessage
	}
	if outcome.Diagnostic != nil {
		for _, key := range []string{"reason", "message", "primary", "stderr", "detail"} {
			if v, ok := outcome.Diagnostic[key]; ok {
				if s := strings.TrimSpace(fmt.Sprint(v)); s != "" && s != "<nil>" {
					return s
				}
			}
		}
	}
	return strings.TrimSpace(outcome.Content)
}

var (
	reTimestamp   = regexp.MustCompile(`\b\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(?:\.\d+)?Z?\b`)
	reTempPath    = regexp.MustCompile(`(?i)(/var/folders/[^\s]+|/tmp/[^\s]+|\\Temp\\[^\s]+|T/[^\s]+)`)
	reRandomID    = regexp.MustCompile(`\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b|\b[a-f0-9]{16,}\b`)
	reLineNumbers = regexp.MustCompile(`(?i)\bline\s+\d+\b`)
)

func normalizeDiagnostic(raw, workspaceRoot string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	if root := strings.TrimSpace(workspaceRoot); root != "" {
		s = strings.ReplaceAll(s, root, "<workspace>")
		s = strings.ReplaceAll(s, filepath.ToSlash(root), "<workspace>")
	}
	s = reTimestamp.ReplaceAllString(s, "<timestamp>")
	s = reTempPath.ReplaceAllString(s, "<tmpdir>")
	s = reRandomID.ReplaceAllString(s, "<id>")
	s = reLineNumbers.ReplaceAllString(s, "line <n>")
	s = strings.Join(strings.Fields(s), " ")
	return s
}

func mergeUniqueStrings(parts ...[]string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0)
	for _, part := range parts {
		for _, item := range part {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			if _, ok := seen[item]; ok {
				continue
			}
			seen[item] = struct{}{}
			out = append(out, item)
		}
	}
	return out
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

// FormatRecoveryContext returns a compact model-facing recovery notice.
func FormatRecoveryContext(directive *RecoveryDirective) string {
	if directive == nil || !directive.Required {
		return ""
	}
	options, _ := json.Marshal(directive.RecoveryOptions)
	return fmt.Sprintf(
		`{"recovery_required":true,"tool":%q,"error_code":%q,"attempt_count":%s,"recovery_options":%s,"message":%q}`,
		directive.ToolName,
		directive.ErrorCode,
		strconv.Itoa(directive.AttemptCount),
		string(options),
		directive.Message,
	)
}
