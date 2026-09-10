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
	askuser "awaken/src/tools/ask_user"
)

const (
	DefaultMaxAgentTurns     = 256
	DefaultMaxToolExecutions = 1024

	maxRetryableSameCallAttempts    = 2
	maxNonRetryableSameCallAttempts = 1
	maxSameClassFailures            = 3
	// strategyChangeTurnFailures / strategyChangeRunFailures escalate to a
	// mandatory strategy change directive. They never terminate the run:
	// a single business tool failing repeatedly is a recoverable condition
	// and only run-level budgets (max turns / max tool executions) end a run.
	strategyChangeTurnFailures = 6
	strategyChangeRunFailures  = 12

	ErrorCodeToolRetryBlocked           = "tool_retry_blocked"
	ErrorCodeRepeatedFailure            = "repeated_tool_failure"
	ErrorCodeRepeatedSuccessfulMutation = "repeated_successful_mutation"

	// StatusStrategyChangeRequired marks a recoverable tool result whose exact
	// call must not be replayed; the agent must change tool, arguments, or
	// implementation path. The run continues.
	StatusStrategyChangeRequired            = "strategy_change_required"
	StatusExternalStateConfirmationRequired = "external_state_confirmation_required"
	StatusSuccessfulMutationAlreadyApplied  = "successful_mutation_already_applied"
)

// FailureSupervisor enforces hard budgets and recovery policy for repeated
// tool failures within a single run.
type FailureSupervisor struct {
	workspaceRoot string

	callAttempts             map[string]int
	callFailureMeta          map[string]callFailureMeta
	classFailureMeta         map[string]classFailureMeta
	toolConsecutiveFail      map[string]int
	successfulMutations      map[string]int
	successfulMutationBlocks map[string]int

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
}

type classFailureMeta struct {
	toolName   string
	revision   string
	errorClass string
	errorCode  string
	failures   int
}

// RecoveryDirective is injected into the next model turn after repeated failures.
type RecoveryDirective struct {
	Required                          bool
	ToolName                          string
	ResourceKey                       string
	ResourceRevision                  string
	ErrorCode                         string
	Fingerprint                       string
	AttemptCount                      int
	RecoveryOptions                   []string
	Message                           string
	ExternalStateConfirmationRequired bool
}

type CallAction uint8

const (
	CallExecute CallAction = iota
	CallBlock
	CallStopRun
)

// BeforeCallDecision has one action, so blocking and executing cannot conflict.
type BeforeCallDecision struct {
	Action                            CallAction
	ErrorCode                         string
	ErrorMessage                      string
	RecoveryOptions                   []string
	TerminalReason                    TerminalReason
	CallSignature                     string
	AttemptCount                      int
	RecoveryRequired                  bool
	ExternalStateConfirmationRequired bool
}

// AfterOutcomeDecision is returned after a real tool outcome is observed.
type AfterOutcomeDecision struct {
	Fingerprint      string
	RecoveryRequired bool
	RecoveryOptions  []string
	Directive        *RecoveryDirective
}

func NewFailureSupervisor(workspaceRoot string, maxAgentTurns, maxToolExecutions int) *FailureSupervisor {
	return &FailureSupervisor{
		workspaceRoot:            strings.TrimSpace(workspaceRoot),
		callAttempts:             map[string]int{},
		callFailureMeta:          map[string]callFailureMeta{},
		classFailureMeta:         map[string]classFailureMeta{},
		toolConsecutiveFail:      map[string]int{},
		successfulMutations:      map[string]int{},
		successfulMutationBlocks: map[string]int{},
		maxAgentTurns:            EffectiveMaxAgentTurns(maxAgentTurns),
		maxToolExecutions:        EffectiveMaxToolExecutions(maxToolExecutions),
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
		return BeforeCallDecision{Action: CallExecute}
	}
	toolName := strings.TrimSpace(call.Function.Name)
	signature := CallSignature(toolName, call.Function.Arguments)
	decision := BeforeCallDecision{
		Action:        CallExecute,
		CallSignature: signature,
	}

	mutationSignature := SuccessfulMutationSignature(toolName, call.Function.Arguments)
	if s.successfulMutations[mutationSignature] > 0 {
		s.successfulMutationBlocks[mutationSignature]++
		decision.Action = CallBlock
		decision.ErrorCode = ErrorCodeRepeatedSuccessfulMutation
		decision.ErrorMessage = fmt.Sprintf(
			"blocked repeated successful mutation %s: the same business arguments were already applied; changing only idempotency_key does not create a new operation",
			toolName,
		)
		decision.RecoveryOptions = []string{
			"continue_to_the_next_task_step",
			"read_back_external_state_if_verification_is_needed",
			"change_business_arguments_only_if_a_correction_is_required",
		}
		decision.RecoveryRequired = true
		decision.AttemptCount = s.successfulMutationBlocks[mutationSignature]
		if decision.AttemptCount >= 2 {
			decision.Action = CallStopRun
			decision.TerminalReason = TerminalReasonRepeatedToolFailure
			decision.ErrorMessage += "; stopping run after the model ignored the first no-progress block"
		}
		return decision
	}

	if s.toolExecutions >= s.maxToolExecutions {
		decision.Action = CallBlock
		decision.Action = CallStopRun
		decision.TerminalReason = TerminalReasonRepeatedToolFailure
		decision.ErrorCode = ErrorCodeRepeatedFailure
		decision.ErrorMessage = fmt.Sprintf("tool execution budget exceeded (%d)", s.maxToolExecutions)
		decision.RecoveryOptions = []string{"stop_and_summarize", "change_strategy"}
		return decision
	}

	if s.pendingRecovery != nil && s.pendingRecovery.ExternalStateConfirmationRequired && s.pendingRecovery.ToolName == toolName {
		decision.Action = CallBlock
		decision.ErrorCode = firstNonEmptyString(s.pendingRecovery.ErrorCode, ErrorCodeToolRetryBlocked)
		decision.ErrorMessage = firstNonEmptyString(
			s.pendingRecovery.Message,
			fmt.Sprintf("blocked %s until the user confirms external provider state", toolName),
		)
		decision.RecoveryOptions = append([]string(nil), s.pendingRecovery.RecoveryOptions...)
		decision.RecoveryRequired = true
		decision.ExternalStateConfirmationRequired = true
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
			decision.Action = CallBlock
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
			// ask_user is an interactive gate rather than a business operation.
			// Once a non-retryable invalid call has already received corrective
			// feedback, replaying the identical arguments cannot make progress and
			// must not consume the remainder of the run budget.
			if toolName == askuser.ToolName && !meta.retryable {
				decision.Action = CallStopRun
				decision.TerminalReason = TerminalReasonRepeatedToolFailure
				decision.ErrorMessage += "; stopping run because ask_user repeated identical invalid arguments after corrective feedback"
				decision.RecoveryOptions = mergeUniqueStrings(decision.RecoveryOptions, []string{
					"start_a_new_run_with_correct_questions",
					"choose_a_model_with_compatible_structured_tool_calling",
				})
			}
			return decision
		}
	}

	// The class budget is intentionally checked before executing the next
	// call. After three real failures for the same tool/resource/error class,
	// a fourth variant must not reach the tool merely because its exact JSON
	// arguments (and therefore call_signature) changed.
	revision := ResourceRevision(call.Function.Arguments, tool.ExecutionOutcome{})
	if exhausted, ok := s.exhaustedClass(toolName, revision); ok {
		decision.Action = CallBlock
		decision.ErrorCode = ErrorCodeRepeatedFailure
		decision.ErrorMessage = fmt.Sprintf(
			"blocked %s before execution: tool/resource/error class %s failed %d times (revision=%s)",
			toolName, exhausted.errorClass, exhausted.failures, exhausted.revision,
		)
		decision.RecoveryOptions = mergeUniqueStrings(
			RecoveryOptionsForError(exhausted.errorCode),
			[]string{"change_tool_or_refresh_resource_revision", "stop_repeating_same_error_class"},
		)
		decision.RecoveryRequired = true
		return decision
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
	resourceKey := ResourceIdentity(call.Function.Arguments, outcome)
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
		if successfulOutcomeRepresentsMutation(toolName, outcome) {
			s.successfulMutations[SuccessfulMutationSignature(toolName, call.Function.Arguments)]++
		}
		recoveryMutation := outcomeMayHaveRevisedShellInputs(toolName, call.Function.Arguments, outcome)
		s.resetFailureHistoryAfterSuccess(toolName, signature, revision, recoveryMutation)
		s.toolConsecutiveFail[toolName] = 0
		s.lastFingerprint = ""
		s.sameFingerprintStreak = 0
		// Only a success for the failed tool or the same resource satisfies a
		// recovery directive. An unrelated calculator/shell success must not
		// silently clear guidance for a still-broken file operation.
		if recoveryDirectiveSatisfied(s.pendingRecovery, toolName, resourceKey, revision, recoveryMutation) {
			s.pendingRecovery = nil
		}
		return decision
	}

	s.turnFailures++
	s.runFailures++
	s.toolConsecutiveFail[toolName] = s.toolConsecutiveFail[toolName] + 1
	s.callFailureMeta[signature] = callFailureMeta{
		retryable: outcome.Retryable,
		errorCode: errorCode,
	}

	classKey := toolName + "|" + revision + "|" + errorClass
	inputRejectedBeforeExecution := outcome.TransportStatus == tool.TransportNotStarted &&
		outcome.ErrorCategory == tool.ErrorCategoryValidation
	if !inputRejectedBeforeExecution {
		s.classFailureMeta[classKey] = classFailureMeta{
			toolName:   toolName,
			revision:   revision,
			errorClass: errorClass,
			errorCode:  errorCode,
			failures:   s.classFailureMeta[classKey].failures + 1,
		}
	}

	if fingerprint != "" && fingerprint == s.lastFingerprint {
		s.sameFingerprintStreak++
	} else {
		s.lastFingerprint = fingerprint
		s.sameFingerprintStreak = 1
	}

	if outcomeRequiresExternalStateConfirmation(outcome) {
		decision.RecoveryRequired = true
		directive := &RecoveryDirective{
			Required:                          true,
			ToolName:                          toolName,
			ResourceKey:                       resourceKey,
			ResourceRevision:                  revision,
			ErrorCode:                         errorCode,
			Fingerprint:                       fingerprint,
			AttemptCount:                      s.callAttempts[signature],
			RecoveryOptions:                   decision.RecoveryOptions,
			ExternalStateConfirmationRequired: true,
			Message: fmt.Sprintf(
				"%s: tool=%s error_code=%s; do not submit another %s call until the user confirms provider state",
				StatusExternalStateConfirmationRequired, toolName, errorCode, toolName,
			),
		}
		s.pendingRecovery = directive
		decision.Directive = directive
		return decision
	}

	if s.sameFingerprintStreak >= 2 {
		decision.RecoveryRequired = true
		directive := &RecoveryDirective{
			Required:         true,
			ToolName:         toolName,
			ResourceKey:      resourceKey,
			ResourceRevision: revision,
			ErrorCode:        errorCode,
			Fingerprint:      fingerprint,
			AttemptCount:     s.callAttempts[signature],
			RecoveryOptions:  decision.RecoveryOptions,
			Message: fmt.Sprintf(
				"recovery_required: tool=%s error_code=%s attempts=%d; change strategy before retrying the same call",
				toolName, errorCode, s.callAttempts[signature],
			),
		}
		s.pendingRecovery = directive
		decision.Directive = directive
	}

	if !inputRejectedBeforeExecution && s.classFailureMeta[classKey].failures >= maxSameClassFailures {
		decision.RecoveryRequired = true
		decision.RecoveryOptions = mergeUniqueStrings(decision.RecoveryOptions, []string{
			"change_tool_or_refresh_resource_revision",
			"stop_repeating_same_error_class",
		})
		if decision.Directive == nil {
			decision.Directive = &RecoveryDirective{
				Required:         true,
				ToolName:         toolName,
				ResourceKey:      resourceKey,
				ResourceRevision: revision,
				ErrorCode:        errorCode,
				Fingerprint:      fingerprint,
				AttemptCount:     s.classFailureMeta[classKey].failures,
				RecoveryOptions:  decision.RecoveryOptions,
				Message: fmt.Sprintf(
					"recovery_required: same tool/resource/error class failed %d times",
					s.classFailureMeta[classKey].failures,
				),
			}
			s.pendingRecovery = decision.Directive
		}
	}

	// Accumulated business-tool failures never terminate the run on their
	// own. They escalate to a mandatory strategy change: the agent must pick
	// a different tool, arguments, or implementation path (or stop the failing
	// strategy and explain). Run-level protection stays with the max turns
	// and max tool execution budgets.
	if s.turnFailures >= strategyChangeTurnFailures || s.runFailures >= strategyChangeRunFailures {
		decision.RecoveryRequired = true
		decision.RecoveryOptions = mergeUniqueStrings(decision.RecoveryOptions, strategyChangeNextActions())
		directive := &RecoveryDirective{
			Required:         true,
			ToolName:         toolName,
			ResourceKey:      resourceKey,
			ResourceRevision: revision,
			ErrorCode:        errorCode,
			Fingerprint:      fingerprint,
			AttemptCount:     s.runFailures,
			RecoveryOptions:  decision.RecoveryOptions,
			Message: fmt.Sprintf(
				"%s: %d tool failures accumulated (turn=%d run=%d); do not repeat the failing approach — change tool, arguments, or implementation path, or stop this strategy and report",
				StatusStrategyChangeRequired, s.runFailures, s.turnFailures, s.runFailures,
			),
		}
		s.pendingRecovery = directive
		decision.Directive = directive
	}
	return decision
}

func strategyChangeNextActions() []string {
	return []string{
		"use_specialized_artifact_tool",
		"change_tool_or_library",
		"inspect_file_structure",
		"stop_if_non_retryable",
	}
}

func outcomeRequiresExternalStateConfirmation(outcome tool.ExecutionOutcome) bool {
	if outcome.Diagnostic == nil {
		return false
	}
	required, _ := outcome.Diagnostic["requires_external_state_confirmation"].(bool)
	return required
}

func recoveryDirectiveSatisfied(
	directive *RecoveryDirective,
	toolName, resourceKey, resourceRevision string,
	recoveryMutation bool,
) bool {
	if directive == nil {
		return false
	}
	toolName = strings.TrimSpace(toolName)
	if toolName == "Bash" && directive.ToolName == "Bash" {
		// Shell is a broad process surface, not one resource. A successful
		// unrelated Shell call (for example node --version or curl) cannot
		// satisfy recovery merely because its tool name matches. The exact
		// formerly failing command can satisfy its own directive, while a
		// host-classified mutation may have deliberately repaired its inputs.
		return resourceRevision == directive.ResourceRevision || recoveryMutation
	}
	if toolName != "" && toolName == directive.ToolName {
		return true
	}
	return directive.ResourceKey != "" && directive.ResourceKey != "unknown" && directive.ResourceKey == resourceKey
}

func (s *FailureSupervisor) exhaustedClass(toolName, revision string) (classFailureMeta, bool) {
	if s == nil {
		return classFailureMeta{}, false
	}
	var selected classFailureMeta
	for _, meta := range s.classFailureMeta {
		if meta.toolName != toolName || meta.revision != revision || meta.failures < maxSameClassFailures {
			continue
		}
		if selected.failures == 0 || meta.failures > selected.failures ||
			(meta.failures == selected.failures && meta.errorClass < selected.errorClass) {
			selected = meta
		}
	}
	return selected, selected.failures >= maxSameClassFailures
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
// The result is recoverable: only the identical call is refused, the run
// continues, and the agent is told explicitly which strategy changes count
// as a fresh attempt.
func SyntheticBlockedResult(call context.ToolCall, decision BeforeCallDecision) ToolExecutionResult {
	now := time.Now().UTC()
	msg := strings.TrimSpace(decision.ErrorMessage)
	if msg == "" {
		msg = "tool retry blocked by FailureSupervisor"
	}
	status := StatusStrategyChangeRequired
	nextActions := mergeUniqueStrings(decision.RecoveryOptions, strategyChangeNextActions())
	changedCallAllowed := true
	if decision.ExternalStateConfirmationRequired {
		status = StatusExternalStateConfirmationRequired
		nextActions = append([]string(nil), decision.RecoveryOptions...)
		changedCallAllowed = false
	} else if decision.ErrorCode == ErrorCodeRepeatedSuccessfulMutation {
		status = StatusSuccessfulMutationAlreadyApplied
	}
	guidance, _ := json.Marshal(map[string]any{
		"status":               status,
		"same_call_retryable":  false,
		"changed_call_allowed": changedCallAllowed,
		"run_will_terminate":   decision.Action == CallStopRun,
		"next_actions":         nextActions,
	})
	content := msg + "\n" + string(guidance)
	outcome := tool.ExecutionOutcome{
		Content:         content,
		Status:          tool.OutcomeDenied,
		TransportStatus: tool.TransportNotStarted,
		SemanticStatus:  tool.SemanticFailure,
		ProcessStatus:   tool.ProcessNotApplicable,
		ErrorCode:       firstNonEmptyString(decision.ErrorCode, ErrorCodeToolRetryBlocked),
		ErrorCategory:   tool.ErrorCategoryPolicy,
		ErrorMessage:    msg,
		Retryable:       false,
		RecoveryOptions: nextActions,
		Diagnostic: map[string]any{
			"blocked":                              true,
			"status":                               status,
			"same_call_retryable":                  false,
			"retryable":                            false,
			"changed_call_allowed":                 changedCallAllowed,
			"run_will_terminate":                   decision.Action == CallStopRun,
			"call_signature":                       decision.CallSignature,
			"attempt_count":                        decision.AttemptCount,
			"recovery_required":                    decision.RecoveryRequired,
			"requires_external_state_confirmation": decision.ExternalStateConfirmationRequired,
		},
		StartedAt:  now,
		FinishedAt: now,
		DurationMs: 0,
	}
	return ToolExecutionResult{
		ToolCallID: call.ID,
		Name:       call.Function.Name,
		Content:    content,
		IsError:    true,
		Status:     ToolExecutionDenied,
		ErrorCode:  outcome.ErrorCode,
		Outcome:    outcome,
		StartedAt:  now,
		FinishedAt: now,
	}
}

// SuccessfulMutationSignature identifies a business mutation independently of
// a caller-generated idempotency token. The token protects provider replay; it
// must not let a model disguise the same already-successful write as new work.
func SuccessfulMutationSignature(toolName, arguments string) string {
	var payload any
	if err := json.Unmarshal([]byte(arguments), &payload); err != nil {
		return CallSignature(toolName, arguments)
	}
	stripMutationTransportFields(payload)
	canonical, err := json.Marshal(payload)
	if err != nil {
		return CallSignature(toolName, arguments)
	}
	return strings.TrimSpace(toolName) + "|mutation|" + hashCanonicalArguments(string(canonical))
}

func stripMutationTransportFields(value any) {
	switch typed := value.(type) {
	case map[string]any:
		delete(typed, "idempotency_key")
		delete(typed, "idempotencyKey")
		for _, child := range typed {
			stripMutationTransportFields(child)
		}
	case []any:
		for _, child := range typed {
			stripMutationTransportFields(child)
		}
	}
}

func successfulOutcomeRepresentsMutation(toolName string, outcome tool.ExecutionOutcome) bool {
	if outcome.IsError() || outcome.Operation == nil {
		return false
	}
	switch outcome.Operation.SideEffectState {
	case tool.OutcomeSideEffectApplied:
		return true
	case tool.OutcomeSideEffectUnknown:
		// MCP mutation tools currently cannot surface a host-observed filesystem
		// delta, so their successful write annotation normalizes to unknown. MCP
		// read tools normalize to none. Broad Bash remains excluded because an
		// identical successful command can be a legitimate probe or test rerun.
		return strings.HasPrefix(strings.TrimSpace(toolName), "mcp__")
	default:
		return false
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
	var payload map[string]any
	if err := json.Unmarshal([]byte(arguments), &payload); err == nil {
		for _, key := range []string{"expected_sha256", "sha256", "resource_revision"} {
			if v, ok := payload[key]; ok {
				if s := strings.TrimSpace(fmt.Sprint(v)); s != "" && s != "<nil>" {
					return s
				}
			}
		}
		for _, key := range []string{"path", "file_path"} {
			if path, ok := payload[key]; ok {
				if s := strings.TrimSpace(fmt.Sprint(path)); s != "" {
					return "path:" + s
				}
			}
		}
		// Shell does not expose a typed path/revision field. Treating every
		// command as revision=unknown merges unrelated compiler, installer and
		// recovery failures into one class budget. Bind the revision to the
		// command and cwd without retaining the command text itself.
		if command, ok := payload["command"]; ok {
			if normalized := strings.TrimSpace(fmt.Sprint(command)); normalized != "" {
				workdir := strings.TrimSpace(fmt.Sprint(payload["workdir"]))
				digest := sha256.Sum256([]byte(normalized + "\x00" + workdir))
				return "command:" + hex.EncodeToString(digest[:8])
			}
		}
	}
	if outcome.Diagnostic != nil {
		for _, key := range []string{"expected_sha256", "resource_revision", "sha256", "after_sha256", "before_sha256"} {
			if v, ok := outcome.Diagnostic[key]; ok {
				if s := strings.TrimSpace(fmt.Sprint(v)); s != "" && s != "<nil>" {
					return s
				}
			}
		}
	}
	return "unknown"
}

func (s *FailureSupervisor) resetFailureHistoryAfterSuccess(toolName, signature, revision string, recoveryMutation bool) {
	if s == nil {
		return
	}
	delete(s.callAttempts, signature)
	delete(s.callFailureMeta, signature)
	for key, meta := range s.classFailureMeta {
		if meta.toolName == toolName && meta.revision == revision {
			delete(s.classFailureMeta, key)
		}
	}

	// A verified or host-classified mutation can intentionally repair the input
	// consumed by a previously failing Shell command. Do not treat every direct
	// Shell success with unknown side effects as recovery: node --version, curl
	// and arbitrary local CLIs are unrelated even though their direct broker
	// cannot observe child-process side effects.
	if !recoveryMutation {
		return
	}
	for key := range s.callFailureMeta {
		if strings.HasPrefix(key, "Bash|") {
			delete(s.callFailureMeta, key)
			delete(s.callAttempts, key)
		}
	}
	for key, meta := range s.classFailureMeta {
		if meta.toolName == "Bash" {
			delete(s.classFailureMeta, key)
		}
	}
	s.toolConsecutiveFail["Bash"] = 0
}

func outcomeMayHaveRevisedShellInputs(toolName, arguments string, outcome tool.ExecutionOutcome) bool {
	if outcomeIncludesObservedMutation(outcome) {
		return true
	}
	if strings.TrimSpace(toolName) != "Bash" || outcome.Operation == nil ||
		outcome.Operation.SideEffectState != tool.OutcomeSideEffectUnknown {
		return false
	}
	return shellCallHasHostClassifiedRecoveryMutation(arguments)
}

// shellCallHasHostClassifiedRecoveryMutation derives recovery semantics from
// the host-owned Shell classifier and canonical call arguments. It never
// trusts prose or a model-supplied side-effect claim. Commands the shared
// classifier proves to be file/workspace mutations are recovery candidates.
// The narrow fallback covers package operations whose network-capable wrapper
// is conservatively classified as local/open-world (notably python -m pip).
func shellCallHasHostClassifiedRecoveryMutation(arguments string) bool {
	var payload struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal([]byte(arguments), &payload); err != nil {
		return false
	}
	command := strings.TrimSpace(payload.Command)
	if command == "" {
		return false
	}
	return reExplicitDependencyRecoveryCommand.MatchString(command) ||
		reExplicitPythonEnvironmentRecoveryCommand.MatchString(command)
}

var (
	reExplicitDependencyRecoveryCommand = regexp.MustCompile(
		`(?i)(^|[;&|][&|]?[[:space:]]*)(env[[:space:]]+)?([A-Z_][A-Z0-9_]*=[^[:space:]]+[[:space:]]+)*` +
			`((python([0-9.]*)?|py)[[:space:]]+-m[[:space:]]+)?` +
			`(npm|pnpm|yarn|pip([0-9.]*)?|pipx|brew|apt|apt-get|yum|dnf|conda)[[:space:]]+` +
			`(-[^[:space:];|&]+[[:space:]]+)*(install|i|ci|add|create|update|upgrade|remove|uninstall)([[:space:]]|$)`,
	)
	reExplicitPythonEnvironmentRecoveryCommand = regexp.MustCompile(
		`(?i)(^|[;&|][&|]?[[:space:]]*)(env[[:space:]]+)?([A-Z_][A-Z0-9_]*=[^[:space:]]+[[:space:]]+)*` +
			`(python([0-9.]*)?|py)[[:space:]]+-m[[:space:]]+(venv|virtualenv)([[:space:]]|$)`,
	)
)

func outcomeIncludesObservedMutation(outcome tool.ExecutionOutcome) bool {
	if outcome.Operation != nil && outcome.Operation.SideEffectState == tool.OutcomeSideEffectApplied {
		return true
	}
	if len(outcome.AffectedPaths) > 0 {
		return true
	}
	for _, change := range outcome.Changes {
		if change.Certainty == tool.ChangeObserved {
			return true
		}
	}
	return false
}

func ResourceIdentity(arguments string, outcome tool.ExecutionOutcome) string {
	var payload map[string]any
	if err := json.Unmarshal([]byte(arguments), &payload); err == nil {
		for _, key := range []string{"path", "file_path"} {
			if value, ok := payload[key]; ok {
				if path := strings.TrimSpace(fmt.Sprint(value)); path != "" {
					return "path:" + filepath.ToSlash(filepath.Clean(path))
				}
			}
		}
	}
	if outcome.Diagnostic != nil {
		if value, ok := outcome.Diagnostic["path"]; ok {
			if path := strings.TrimSpace(fmt.Sprint(value)); path != "" && path != "<nil>" {
				return "path:" + filepath.ToSlash(filepath.Clean(path))
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
	case code != "":
		return code
	default:
		return "unknown"
	}
}

func RecoveryOptionsForError(errorCode string) []string {
	code := strings.TrimSpace(errorCode)
	switch {
	case code == "edit_match_not_unique":
		return []string{"expand_old_string_context", "set_replace_all_true"}
	case code == "edit_match_not_found":
		return []string{"Read", "retry_Edit_with_current_content"}
	case code == "file_changed_since_read" || code == "file_not_read":
		return []string{"Read", "retry_Edit_with_fresh_snapshot"}
	case code == tool.ErrorCodeProcessExitNonzero:
		return []string{"inspect_exit_code_and_stderr", "change_command_or_stop"}
	case code == tool.ErrorCodeNeedsRuntime || code == "needs_runtime":
		return []string{"follow_selected_skill_initialization_or_use_matching_provider_runtime", "inspect_runtime_requirement"}
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
