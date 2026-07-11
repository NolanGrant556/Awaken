package tool

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const OutcomeSchemaVersion = 2

type SideEffectState string

const (
	OutcomeSideEffectNotStarted SideEffectState = "not_started"
	OutcomeSideEffectNone       SideEffectState = "none"
	OutcomeSideEffectApplied    SideEffectState = "applied"
	OutcomeSideEffectPartial    SideEffectState = "partial"
	OutcomeSideEffectUnknown    SideEffectState = "unknown"
)

type ChangeCertainty string

const (
	ChangeObserved ChangeCertainty = "observed"
	ChangeInferred ChangeCertainty = "inferred"
	ChangeUnknown  ChangeCertainty = "unknown"
)

type ChangeReversibility string

const (
	ChangeReversible           ChangeReversibility = "reversible"
	ChangeConditional          ChangeReversibility = "conditional"
	ChangeIrreversible         ChangeReversibility = "irreversible"
	ChangeReversibilityUnknown ChangeReversibility = "unknown"
)

type ObservationCompleteness string

const (
	ObservationNone     ObservationCompleteness = "none"
	ObservationPartial  ObservationCompleteness = "partial"
	ObservationComplete ObservationCompleteness = "complete"
)

type ErrorStage string

const (
	ErrorStageInputValidation ErrorStage = "input_validation"
	ErrorStagePolicy          ErrorStage = "policy"
	ErrorStageAuthorization   ErrorStage = "authorization"
	ErrorStagePrecondition    ErrorStage = "precondition"
	ErrorStageExecution       ErrorStage = "execution"
	ErrorStageVerification    ErrorStage = "verification"
)

type RetryStrategy string

const (
	RetryDoNotRetry           RetryStrategy = "do_not_retry"
	RetrySameInput            RetryStrategy = "retry_same_input"
	RetryAfterBackoff         RetryStrategy = "retry_after_backoff"
	RetryRefreshState         RetryStrategy = "refresh_state_then_retry"
	RetryChangeInput          RetryStrategy = "change_input_then_retry"
	RetryRequestAuthorization RetryStrategy = "request_authorization"
	RetryPrepareRuntime       RetryStrategy = "prepare_runtime"
	RetrySwitchTool           RetryStrategy = "switch_tool"
	RetryManualRecovery       RetryStrategy = "manual_recovery_required"
)

type OperationDescriptor struct {
	Type            string          `json:"type"`
	Target          string          `json:"target,omitempty"`
	SideEffectState SideEffectState `json:"side_effect_state"`
}

type ResourceChange struct {
	Kind          string              `json:"kind"`
	Action        string              `json:"action"`
	Resource      string              `json:"resource"`
	Summary       string              `json:"summary,omitempty"`
	Certainty     ChangeCertainty     `json:"certainty"`
	Reversibility ChangeReversibility `json:"reversibility"`
	Details       map[string]any      `json:"details,omitempty"`
}

type GitObservation struct {
	RepositoryRoot string   `json:"repository_root,omitempty"`
	Branch         string   `json:"branch,omitempty"`
	Head           string   `json:"head,omitempty"`
	ModifiedFiles  []string `json:"modified_files,omitempty"`
	UntrackedFiles []string `json:"untracked_files,omitempty"`
}

type ProcessObservation struct {
	ProcessID string        `json:"process_id,omitempty"`
	SessionID string        `json:"session_id,omitempty"`
	Status    ProcessStatus `json:"status"`
	ExitCode  *int          `json:"exit_code,omitempty"`
	Signal    string        `json:"signal,omitempty"`
	CWD       string        `json:"cwd,omitempty"`
}

type EnvironmentVariableChange struct {
	Name   string `json:"name"`
	Action string `json:"action"`
}

type EnvironmentObservation struct {
	Observation      ObservationCompleteness     `json:"observation"`
	ObservedAt       time.Time                   `json:"observed_at"`
	Scope            string                      `json:"scope"`
	Source           string                      `json:"source"`
	WorkspaceRoot    string                      `json:"workspace_root,omitempty"`
	WorkingDirectory string                      `json:"working_directory,omitempty"`
	Files            []string                    `json:"files,omitempty"`
	Git              *GitObservation             `json:"git,omitempty"`
	Processes        []ProcessObservation        `json:"processes,omitempty"`
	Variables        []EnvironmentVariableChange `json:"environment_variables,omitempty"`
	Unobserved       []string                    `json:"unobserved,omitempty"`
}

type OutcomeNotice struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type OutcomeError struct {
	Type          string        `json:"type"`
	Category      ErrorCategory `json:"category"`
	Stage         ErrorStage    `json:"stage,omitempty"`
	Cause         string        `json:"cause"`
	Context       string        `json:"context,omitempty"`
	PossibleFixes []string      `json:"possible_fixes,omitempty"`
	RetryStrategy RetryStrategy `json:"retry_strategy,omitempty"`
}

type NextAction struct {
	ID            string         `json:"id"`
	Description   string         `json:"description"`
	Tool          string         `json:"tool,omitempty"`
	Preconditions []string       `json:"preconditions,omitempty"`
	InputHint     map[string]any `json:"input_hint,omitempty"`
}

// OutcomeNormalizationContext carries facts observed outside the tool itself.
// ActualAffectedPaths must contain executor-observed paths only. Permission
// predictions intentionally have no field here, so they cannot be promoted to
// completed changes by accident.
type OutcomeNormalizationContext struct {
	OperationType       string
	ActualAffectedPaths []string
	// ReferencedPaths are targets observed or reported by the executor but
	// not proven to have changed (for example, a stale edit target).
	ReferencedPaths   []string
	PatchSummary      string
	Artifacts         []ArtifactDescriptor
	Changes           []ResourceChange
	WorkspaceRoot     string
	WorkingDirectory  string
	ObservationSource string
	// DeclaredSideEffects are capability facts from the exact registered
	// definition shown to the model. They prevent a successful but unobserved
	// mutation-capable call (for example shell or a connector write) from being
	// falsely reported as side-effect-free.
	DeclaredSideEffects []ToolSideEffect
	// OutputSchema is the tool-specific Details contract from the same
	// registered definition used for model selection. StrictOutputSchema turns
	// a successful-but-invalid result into a typed verification failure.
	OutputSchema       ToolParam
	StrictOutputSchema bool
}

// Normalize produces the canonical v2 envelope without discarding legacy
// fields. Contradictory or unknown states are conservatively represented as a
// failed outcome with unknown side effects rather than guessed as success.
func (o ExecutionOutcome) Normalize(ctx OutcomeNormalizationContext) ExecutionOutcome {
	o = cloneExecutionOutcome(o)
	o.SchemaVersion = OutcomeSchemaVersion
	o.normalizeStatus()
	executionSucceeded := o.Status == OutcomeSucceeded
	o.validateOutputContract(ctx)
	ctx.ReferencedPaths = uniqueStrings(append(ctx.ReferencedPaths, o.AffectedPaths...))
	if executionSucceeded {
		ctx.ActualAffectedPaths = uniqueStrings(append(ctx.ActualAffectedPaths, o.AffectedPaths...))
	}

	if strings.TrimSpace(o.Summary) == "" {
		o.Summary = summarizeOutcome(o)
	}
	if o.Operation == nil {
		o.Operation = &OperationDescriptor{}
	}
	if strings.TrimSpace(o.Operation.Type) == "" {
		o.Operation.Type = firstNonBlank(strings.TrimSpace(ctx.OperationType), "unknown")
	}

	o.AffectedPaths = uniqueStrings(append(o.AffectedPaths, ctx.ActualAffectedPaths...))
	if strings.TrimSpace(o.PatchSummary) == "" {
		o.PatchSummary = strings.TrimSpace(ctx.PatchSummary)
	}
	o.Artifacts = mergeArtifactDescriptors(o.Artifacts, ctx.Artifacts)
	o.Changes = mergeResourceChanges(o.Changes, ctx.Changes)
	o.addObservedPathChanges(ctx.ActualAffectedPaths)
	o.addArtifactChanges()
	if strings.TrimSpace(o.Operation.Target) == "" {
		o.Operation.Target = firstOutcomeTarget(o)
		if o.Operation.Target == "" && len(ctx.ReferencedPaths) > 0 {
			o.Operation.Target = ctx.ReferencedPaths[0]
		}
	}
	o.Operation.SideEffectState = normalizeSideEffectState(o.Operation.SideEffectState, o, ctx.DeclaredSideEffects)

	o.normalizeEnvironment(ctx)
	o.normalizeErrorsAndRecovery()
	if o.Truncated {
		o.Warnings = mergeNotices(o.Warnings, []OutcomeNotice{{
			Code: "output_truncated", Message: "Tool output was truncated; use a narrower query or continuation mechanism when available.",
		}})
	}
	o.Changes = normalizeChanges(o.Changes)
	o.NextActions = normalizeNextActions(o.NextActions)
	return o
}

func (o *ExecutionOutcome) validateOutputContract(ctx OutcomeNormalizationContext) {
	if o.Status != OutcomeSucceeded || !hasToolParamSchema(ctx.OutputSchema) {
		return
	}
	schema := ToolParametersSchema{
		Type:                 firstNonBlank(ctx.OutputSchema.Type, "object"),
		Description:          ctx.OutputSchema.Description,
		Properties:           ctx.OutputSchema.Properties,
		Required:             ctx.OutputSchema.Required,
		AdditionalProperties: ctx.OutputSchema.AdditionalProperties,
		OneOf:                ctx.OutputSchema.OneOf,
	}
	data, err := json.Marshal(firstNonNilMap(o.Details))
	if err == nil {
		err = ValidateInputAgainstSchema(string(data), schema)
	}
	if err == nil {
		return
	}
	message := "tool result details did not match the registered output schema: " + err.Error()
	if !ctx.StrictOutputSchema {
		o.Warnings = mergeNotices(o.Warnings, []OutcomeNotice{{Code: "tool_output_schema_mismatch", Message: message}})
		return
	}
	o.Status = OutcomeFailed
	o.TransportStatus = TransportCompleted
	o.SemanticStatus = SemanticFailure
	o.ErrorCode = "tool_output_schema_mismatch"
	o.ErrorCategory = ErrorCategoryTool
	o.ErrorMessage = message
	o.Retryable = false
	o.RecoveryOptions = uniqueStrings(append(o.RecoveryOptions,
		"inspect_tool_output_contract", "do_not_blindly_retry"))
}

func firstNonNilMap(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	return value
}

func (o *ExecutionOutcome) normalizeStatus() {
	if !validOutcomeStatus(o.Status) {
		original := strings.TrimSpace(string(o.Status))
		o.Status = OutcomeFailed
		o.TransportStatus = TransportUnknown
		o.SemanticStatus = SemanticFailure
		o.ProcessStatus = ProcessUnknown
		o.ErrorCode = "invalid_outcome_status"
		o.ErrorCategory = ErrorCategoryTool
		o.ErrorMessage = firstNonBlank(o.ErrorMessage, fmt.Sprintf("tool returned invalid outcome status %q", original))
		o.Retryable = false
		return
	}

	if !validTransportStatus(o.TransportStatus) {
		switch o.Status {
		case OutcomeSucceeded:
			o.TransportStatus = TransportCompleted
		case OutcomeDenied, OutcomeNeedsAuth, OutcomeNeedsRuntime, OutcomeUnsupported, OutcomeReviewRequired:
			o.TransportStatus = TransportNotStarted
		case OutcomeCancelled, OutcomeInterrupted:
			o.TransportStatus = TransportCancelled
		default:
			o.TransportStatus = TransportFailed
		}
	}
	if !validSemanticStatus(o.SemanticStatus) {
		if o.Status == OutcomeSucceeded {
			o.SemanticStatus = SemanticSuccess
		} else {
			o.SemanticStatus = SemanticFailure
		}
	}
	switch o.Status {
	case OutcomeSucceeded:
		if o.SemanticStatus == SemanticUnknown {
			o.SemanticStatus = SemanticSuccess
		}
		if o.SemanticStatus == SemanticFailure || o.TransportStatus != TransportCompleted {
			o.Status = OutcomeFailed
			o.TransportStatus = TransportUnknown
			o.SemanticStatus = SemanticFailure
			o.ErrorCode = firstNonBlank(o.ErrorCode, "invalid_outcome_combination")
			o.ErrorCategory = firstNonErrorCategory(o.ErrorCategory, ErrorCategoryTool)
			o.ErrorMessage = firstNonBlank(o.ErrorMessage, "succeeded outcome reported contradictory transport or semantic state")
			o.Retryable = false
		}
	case OutcomeDenied, OutcomeNeedsAuth, OutcomeNeedsRuntime, OutcomeUnsupported, OutcomeReviewRequired:
		o.TransportStatus = TransportNotStarted
		if o.SemanticStatus == SemanticUnknown || o.SemanticStatus == SemanticSuccess {
			o.SemanticStatus = SemanticFailure
		}
	default:
		if o.SemanticStatus == SemanticUnknown || o.SemanticStatus == SemanticSuccess {
			o.SemanticStatus = SemanticFailure
		}
	}
	if !validProcessStatus(o.ProcessStatus) {
		o.ProcessStatus = ProcessNotApplicable
	}
}

func firstNonErrorCategory(values ...ErrorCategory) ErrorCategory {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func validOutcomeStatus(status OutcomeStatus) bool {
	switch status {
	case OutcomeSucceeded, OutcomeFailed, OutcomeDenied, OutcomeCancelled, OutcomeInterrupted,
		OutcomeNeedsAuth, OutcomeNeedsRuntime, OutcomeUnsupported, OutcomeReviewRequired:
		return true
	default:
		return false
	}
}

func validTransportStatus(status TransportStatus) bool {
	switch status {
	case TransportNotStarted, TransportCompleted, TransportFailed, TransportCancelled, TransportUnknown:
		return true
	default:
		return false
	}
}

func validSemanticStatus(status SemanticStatus) bool {
	switch status {
	case SemanticSuccess, SemanticFailure, SemanticNeutral, SemanticUnknown:
		return true
	default:
		return false
	}
}

func validProcessStatus(status ProcessStatus) bool {
	switch status {
	case ProcessNotApplicable, ProcessExited, ProcessTimedOut, ProcessSignaled, ProcessSpawnFailed, ProcessUnknown:
		return true
	default:
		return false
	}
}

func normalizeSideEffectState(state SideEffectState, o ExecutionOutcome, declared []ToolSideEffect) SideEffectState {
	switch o.Status {
	case OutcomeDenied, OutcomeNeedsAuth, OutcomeNeedsRuntime, OutcomeUnsupported, OutcomeReviewRequired:
		return OutcomeSideEffectNotStarted
	case OutcomeSucceeded:
		if len(o.Changes) > 0 || len(o.Artifacts) > 0 {
			return OutcomeSideEffectApplied
		}
		// An explicit unknown value is a canonical execution fact: the tool
		// completed but the runtime could not observe whether a mutation occurred.
		// Preserve it across repeated normalization/projection even when the later
		// caller no longer has the original ToolDefinition side-effect context.
		if state == OutcomeSideEffectApplied || state == OutcomeSideEffectNone || state == OutcomeSideEffectUnknown {
			return state
		}
		if declaredSideEffectsMayMutate(declared) {
			return OutcomeSideEffectUnknown
		}
		return OutcomeSideEffectNone
	case OutcomeCancelled, OutcomeInterrupted:
		if o.TransportStatus == TransportNotStarted {
			return OutcomeSideEffectNotStarted
		}
		if len(o.Changes) > 0 || len(o.Artifacts) > 0 {
			return OutcomeSideEffectPartial
		}
		if state == OutcomeSideEffectNone || state == OutcomeSideEffectPartial || state == OutcomeSideEffectUnknown {
			return state
		}
	}
	if o.TransportStatus == TransportNotStarted {
		return OutcomeSideEffectNotStarted
	}
	if len(o.Changes) > 0 || len(o.Artifacts) > 0 {
		return OutcomeSideEffectPartial
	}
	if state == OutcomeSideEffectNone || state == OutcomeSideEffectPartial || state == OutcomeSideEffectUnknown {
		return state
	}
	return OutcomeSideEffectUnknown
}

func declaredSideEffectsMayMutate(effects []ToolSideEffect) bool {
	for _, effect := range effects {
		switch strings.TrimSpace(effect.Kind) {
		case SideEffectCreate, SideEffectUpdate, SideEffectDelete, SideEffectExecute, SideEffectUnknown:
			return true
		}
	}
	return false
}

func (o *ExecutionOutcome) addObservedPathChanges(paths []string) {
	for _, path := range uniqueStrings(paths) {
		if hasChangeResource(o.Changes, path) {
			continue
		}
		action := "modified"
		summary := strings.TrimSpace(o.PatchSummary)
		o.Changes = append(o.Changes, ResourceChange{
			Kind: "file", Action: action, Resource: path, Summary: summary,
			Certainty: ChangeObserved, Reversibility: ChangeConditional,
		})
	}
}

func (o *ExecutionOutcome) addArtifactChanges() {
	for _, artifact := range o.Artifacts {
		path := strings.TrimSpace(artifact.Path)
		if path == "" || hasChangeResource(o.Changes, path) {
			continue
		}
		action := strings.TrimSpace(string(artifact.Action))
		// Publishing exposes an already-existing artifact to the UI/model; it
		// does not create or mutate the underlying resource.
		if action == string(ArtifactActionPublished) {
			continue
		}
		if action == "" {
			action = "produced"
		}
		o.Changes = append(o.Changes, ResourceChange{
			Kind:   firstNonBlank(strings.TrimSpace(artifact.Kind), "artifact"),
			Action: action, Resource: path, Summary: strings.TrimSpace(artifact.Description),
			Certainty: ChangeObserved, Reversibility: ChangeConditional,
		})
	}
}

func (o *ExecutionOutcome) normalizeEnvironment(ctx OutcomeNormalizationContext) {
	if o.Environment != nil {
		if o.Environment.ObservedAt.IsZero() {
			o.Environment.ObservedAt = firstNonZeroTime(o.FinishedAt, time.Now().UTC())
		}
		if o.Environment.Observation != ObservationNone && o.Environment.Observation != ObservationPartial && o.Environment.Observation != ObservationComplete {
			o.Environment.Observation = ObservationNone
			if o.Environment.WorkspaceRoot != "" || o.Environment.WorkingDirectory != "" ||
				len(o.Environment.Files) > 0 || o.Environment.Git != nil ||
				len(o.Environment.Processes) > 0 || len(o.Environment.Variables) > 0 {
				o.Environment.Observation = ObservationPartial
			}
		}
		o.Environment.Source = firstNonBlank(strings.TrimSpace(o.Environment.Source), strings.TrimSpace(ctx.ObservationSource), "executor")
		o.Environment.Scope = firstNonBlank(
			strings.TrimSpace(o.Environment.Scope), strings.TrimSpace(o.Environment.WorkspaceRoot),
			strings.TrimSpace(o.Environment.WorkingDirectory), strings.TrimSpace(ctx.WorkspaceRoot), "tool_call",
		)
		o.Environment.Files = uniqueStrings(o.Environment.Files)
		if o.Environment.Observation != ObservationComplete {
			if o.Environment.Git == nil {
				o.Environment.Unobserved = append(o.Environment.Unobserved, "git")
			}
			if len(o.Environment.Processes) == 0 {
				o.Environment.Unobserved = append(o.Environment.Unobserved, "processes")
			}
			if len(o.Environment.Variables) == 0 {
				o.Environment.Unobserved = append(o.Environment.Unobserved, "environment_variables")
			}
		}
		o.Environment.Unobserved = uniqueStrings(o.Environment.Unobserved)
		return
	}
	source := firstNonBlank(strings.TrimSpace(ctx.ObservationSource), "runtime")
	files := uniqueStrings(append(append([]string(nil), ctx.ReferencedPaths...), ctx.ActualAffectedPaths...))
	observation := ObservationNone
	if ctx.WorkspaceRoot != "" || ctx.WorkingDirectory != "" || len(files) > 0 || o.ProcessStatus != ProcessNotApplicable {
		observation = ObservationPartial
	}
	env := &EnvironmentObservation{
		Observation:      observation,
		ObservedAt:       firstNonZeroTime(o.FinishedAt, time.Now().UTC()),
		Scope:            firstNonBlank(strings.TrimSpace(ctx.WorkspaceRoot), strings.TrimSpace(ctx.WorkingDirectory), "tool_call"),
		Source:           source,
		WorkspaceRoot:    strings.TrimSpace(ctx.WorkspaceRoot),
		WorkingDirectory: strings.TrimSpace(ctx.WorkingDirectory),
		Files:            files,
	}
	if o.ProcessStatus != ProcessNotApplicable {
		env.Processes = []ProcessObservation{{
			Status: o.ProcessStatus, ExitCode: o.ExitCode, Signal: o.Signal, CWD: strings.TrimSpace(ctx.WorkingDirectory),
		}}
	}
	unobserved := []string{"git", "environment_variables"}
	if len(env.Processes) == 0 {
		unobserved = append(unobserved, "processes")
	}
	env.Unobserved = unobserved
	o.Environment = env
}

func (o *ExecutionOutcome) normalizeErrorsAndRecovery() {
	if o.Status == OutcomeSucceeded {
		o.ErrorCode = ""
		o.ErrorCategory = ""
		o.ErrorMessage = ""
		o.Retryable = false
		o.RecoveryOptions = nil
		o.Errors = nil
		return
	}
	code := firstNonBlank(strings.TrimSpace(o.ErrorCode), "tool_execution_failed")
	category := o.ErrorCategory
	if category == "" {
		category = ErrorCategoryTool
	}
	cause := firstNonBlank(strings.TrimSpace(o.ErrorMessage), strings.TrimSpace(o.Content), code)
	o.ErrorCode = code
	o.ErrorCategory = category
	o.ErrorMessage = cause
	strategy := retryStrategyFor(*o)
	canonical := OutcomeError{
		Type: code, Category: category, Stage: errorStageFor(*o), Cause: cause,
		PossibleFixes: uniqueStrings(o.RecoveryOptions), RetryStrategy: strategy,
	}
	if len(o.Errors) == 0 || o.Errors[0].Type != code || o.Errors[0].Category != category {
		o.Errors = append([]OutcomeError{canonical}, o.Errors...)
	} else {
		if o.Errors[0].Cause == "" {
			o.Errors[0].Cause = cause
		}
		if o.Errors[0].Stage == "" {
			o.Errors[0].Stage = canonical.Stage
		}
		if o.Errors[0].RetryStrategy == "" ||
			(!o.Retryable && retryStrategyReplays(o.Errors[0].RetryStrategy)) ||
			(o.Retryable && o.Errors[0].RetryStrategy == RetryDoNotRetry) {
			o.Errors[0].RetryStrategy = strategy
		}
		if len(o.Errors[0].PossibleFixes) == 0 {
			o.Errors[0].PossibleFixes = canonical.PossibleFixes
		}
	}
	if len(o.NextActions) == 0 {
		o.NextActions = nextActionsFor(*o, strategy)
	}
}

func retryStrategyReplays(strategy RetryStrategy) bool {
	switch strategy {
	case RetrySameInput, RetryAfterBackoff, RetryRefreshState, RetryChangeInput,
		RetryRequestAuthorization, RetryPrepareRuntime:
		return true
	default:
		return false
	}
}

func errorStageFor(o ExecutionOutcome) ErrorStage {
	if o.ErrorCode == "tool_output_schema_mismatch" {
		return ErrorStageVerification
	}
	switch o.ErrorCategory {
	case ErrorCategoryValidation:
		return ErrorStageInputValidation
	case ErrorCategoryPolicy, ErrorCategoryUnsupported:
		return ErrorStagePolicy
	case ErrorCategoryAuthorization:
		return ErrorStageAuthorization
	case ErrorCategoryRuntime:
		if o.TransportStatus == TransportNotStarted {
			return ErrorStagePrecondition
		}
		return ErrorStageExecution
	default:
		if o.TransportStatus == TransportNotStarted {
			return ErrorStagePrecondition
		}
		return ErrorStageExecution
	}
}

func retryStrategyFor(o ExecutionOutcome) RetryStrategy {
	switch o.Status {
	case OutcomeNeedsAuth:
		return RetryRequestAuthorization
	case OutcomeNeedsRuntime:
		return RetryPrepareRuntime
	case OutcomeUnsupported:
		if len(o.RecoveryOptions) > 0 {
			return RetrySwitchTool
		}
		return RetryDoNotRetry
	case OutcomeDenied, OutcomeReviewRequired:
		return RetryDoNotRetry
	}
	// Retryable answers whether replaying the same call can succeed. Validation
	// failures are intentionally non-retryable with unchanged input, but they
	// still need an explicit change-input recovery strategy for agent repair.
	if o.ErrorCategory == ErrorCategoryValidation {
		return RetryChangeInput
	}
	if !o.Retryable {
		return RetryDoNotRetry
	}
	switch o.ErrorCategory {
	case ErrorCategoryAuthorization:
		return RetryRequestAuthorization
	case ErrorCategoryRuntime, ErrorCategoryProcess:
		return RetryAfterBackoff
	case ErrorCategoryDomain:
		return RetryRefreshState
	default:
		return RetrySameInput
	}
}

func nextActionsFor(o ExecutionOutcome, strategy RetryStrategy) []NextAction {
	if o.Status == OutcomeReviewRequired {
		return []NextAction{{ID: "complete_required_review", Description: "Wait for or complete the required human review before taking any external action.", Preconditions: []string{"Do not bypass the review decision."}}}
	}
	switch strategy {
	case RetryRequestAuthorization:
		return []NextAction{{ID: "request_authorization", Description: "Authorize the required account or capability before retrying.", Preconditions: []string{"Do not retry until authorization state changes."}}}
	case RetryPrepareRuntime:
		return []NextAction{{ID: "prepare_runtime", Description: "Prepare or repair the required local runtime before retrying.", Preconditions: []string{"Verify the runtime is ready before replaying the call."}}}
	case RetryChangeInput:
		return []NextAction{{ID: "correct_input", Description: "Correct the invalid input using the error details, then retry.", Preconditions: []string{"Do not replay the same invalid input."}}}
	case RetryRefreshState:
		return []NextAction{{ID: "refresh_state", Description: "Refresh the target state and rebuild the call from current facts.", Preconditions: []string{"Do not replay stale assumptions."}}}
	case RetryAfterBackoff:
		return []NextAction{{ID: "retry_after_backoff", Description: "Retry after a bounded delay if the underlying condition is transient."}}
	case RetrySameInput:
		return []NextAction{{ID: "retry_tool_call", Description: "Retry the tool call once if the failure was transient."}}
	case RetrySwitchTool:
		return []NextAction{{ID: "switch_tool", Description: "Use a supported alternative from possible_fixes instead of replaying this tool."}}
	}
	return nil
}

// ObservedTrashedPaths returns typed trash destinations from canonical
// changes. Runtime callers should prefer this over parsing human-readable
// content. The legacy parser remains only for old tool transcripts.
func (o ExecutionOutcome) ObservedTrashedPaths() []string {
	var paths []string
	for _, change := range o.Changes {
		if strings.TrimSpace(change.Action) != "trashed" {
			continue
		}
		if path, ok := change.Details["trashed_path"].(string); ok && strings.TrimSpace(path) != "" {
			paths = append(paths, strings.TrimSpace(path))
		}
	}
	return uniqueStrings(paths)
}

func summarizeOutcome(o ExecutionOutcome) string {
	value := firstNonBlank(strings.TrimSpace(o.ErrorMessage), strings.TrimSpace(o.Content), string(o.Status))
	if idx := strings.IndexByte(value, '\n'); idx >= 0 {
		value = value[:idx]
	}
	runes := []rune(value)
	if len(runes) > 240 {
		value = string(runes[:239]) + "…"
	}
	return value
}

func firstOutcomeTarget(o ExecutionOutcome) string {
	if len(o.Changes) > 0 {
		return strings.TrimSpace(o.Changes[0].Resource)
	}
	if len(o.Artifacts) > 0 {
		return strings.TrimSpace(o.Artifacts[0].Path)
	}
	if len(o.AffectedPaths) > 0 {
		return strings.TrimSpace(o.AffectedPaths[0])
	}
	return ""
}
