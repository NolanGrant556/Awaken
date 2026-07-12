package tool

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

// Execution status / transport / semantic / process / error-category enums are
// the single source of truth for ToolOutcome. Do not scatter string literals.

type OutcomeStatus string

const (
	OutcomeSucceeded      OutcomeStatus = "succeeded"
	OutcomeFailed         OutcomeStatus = "failed"
	OutcomeDenied         OutcomeStatus = "denied"
	OutcomeCancelled      OutcomeStatus = "cancelled"
	OutcomeInterrupted    OutcomeStatus = "interrupted"
	OutcomeNeedsAuth      OutcomeStatus = "needs_auth"
	OutcomeNeedsRuntime   OutcomeStatus = "needs_runtime"
	OutcomeUnsupported    OutcomeStatus = "unsupported"
	OutcomeReviewRequired OutcomeStatus = "review_required"
)

type TransportStatus string

const (
	TransportNotStarted TransportStatus = "not_started"
	TransportCompleted  TransportStatus = "completed"
	TransportFailed     TransportStatus = "failed"
	TransportCancelled  TransportStatus = "cancelled"
	TransportUnknown    TransportStatus = "unknown"
)

type SemanticStatus string

const (
	SemanticSuccess SemanticStatus = "success"
	SemanticFailure SemanticStatus = "failure"
	SemanticNeutral SemanticStatus = "neutral"
	SemanticUnknown SemanticStatus = "unknown"
)

type ProcessStatus string

const (
	ProcessNotApplicable ProcessStatus = "not_applicable"
	ProcessExited        ProcessStatus = "exited"
	ProcessTimedOut      ProcessStatus = "timed_out"
	ProcessSignaled      ProcessStatus = "signaled"
	ProcessSpawnFailed   ProcessStatus = "spawn_failed"
	ProcessUnknown       ProcessStatus = "unknown"
)

type ErrorCategory string

const (
	ErrorCategoryValidation    ErrorCategory = "validation"
	ErrorCategoryPolicy        ErrorCategory = "policy"
	ErrorCategoryAuthorization ErrorCategory = "authorization"
	ErrorCategoryRuntime       ErrorCategory = "runtime"
	ErrorCategoryProcess       ErrorCategory = "process"
	ErrorCategoryTool          ErrorCategory = "tool"
	ErrorCategoryPersistence   ErrorCategory = "persistence"
	ErrorCategoryUnsupported   ErrorCategory = "unsupported"
	ErrorCategoryCancelled     ErrorCategory = "cancelled"
	ErrorCategoryDomain        ErrorCategory = "domain"
)

// Common machine-readable error codes.
const (
	ErrorCodeProcessExitNonzero          = "process_exit_nonzero"
	ErrorCodeProcessTimeout              = "process_timeout"
	ErrorCodeProcessSpawnFailed          = "process_spawn_failed"
	ErrorCodeValidationError             = "validation_error"
	ErrorCodeToolDenied                  = "tool_denied"
	ErrorCodeToolCancelled               = "tool_cancelled"
	ErrorCodeNeedsAuth                   = "needs_auth"
	ErrorCodeNeedsRuntime                = "needs_runtime"
	ErrorCodeUnsupported                 = "unsupported"
	ErrorCodeToolExecutionFailed         = "tool_execution_failed"
	ErrorCodeExecutionBrokerUnavailable  = "execution_broker_unavailable"
	ErrorCodeSupervisedExecutionRequired = "supervised_execution_required"
	ErrorCodeRecoverableDeleteTransform  = "recoverable_delete_transform_required"
	ErrorCodeUnsafeRecoverableDeletePath = "unsafe_recoverable_delete_path"
)

// ExecutionOutcome is the canonical ToolOutcome type.
type ExecutionOutcome struct {
	SchemaVersion   int                     `json:"schema_version"`
	Content         string                  `json:"content"`
	Status          OutcomeStatus           `json:"status"`
	TransportStatus TransportStatus         `json:"transport_status"`
	SemanticStatus  SemanticStatus          `json:"semantic_status"`
	ProcessStatus   ProcessStatus           `json:"process_status"`
	ExitCode        *int                    `json:"exit_code,omitempty"`
	Signal          string                  `json:"signal,omitempty"`
	TimedOut        bool                    `json:"timed_out,omitempty"`
	ErrorCode       string                  `json:"error_code,omitempty"`
	ErrorCategory   ErrorCategory           `json:"error_category,omitempty"`
	ErrorMessage    string                  `json:"error_message,omitempty"`
	Retryable       bool                    `json:"retryable"`
	RecoveryOptions []string                `json:"recovery_options,omitempty"`
	Diagnostic      map[string]any          `json:"diagnostic,omitempty"`
	AffectedPaths   []string                `json:"affected_paths,omitempty"`
	PatchSummary    string                  `json:"patch_summary,omitempty"`
	Artifacts       []ArtifactDescriptor    `json:"artifacts,omitempty"`
	Summary         string                  `json:"summary"`
	Operation       *OperationDescriptor    `json:"operation,omitempty"`
	Changes         []ResourceChange        `json:"changes,omitempty"`
	Environment     *EnvironmentObservation `json:"environment,omitempty"`
	Warnings        []OutcomeNotice         `json:"warnings,omitempty"`
	Errors          []OutcomeError          `json:"errors,omitempty"`
	NextActions     []NextAction            `json:"next_actions,omitempty"`
	Details         map[string]any          `json:"details,omitempty"`
	StartedAt       time.Time               `json:"started_at"`
	FinishedAt      time.Time               `json:"finished_at"`
	DurationMs      int64                   `json:"duration_ms"`
	Truncated       bool                    `json:"truncated,omitempty"`
}

// ToolError is a structured business/process failure carried inside an outcome.
type ToolError struct {
	Code            string
	Category        ErrorCategory
	Message         string
	Retryable       bool
	RecoveryOptions []string
	Diagnostic      map[string]any
}

func (e ToolError) Error() string {
	if strings.TrimSpace(e.Message) != "" {
		return e.Message
	}
	return e.Code
}

// ProcessOutcome captures process-level facts for shell-like tools.
type ProcessOutcome struct {
	Status   ProcessStatus
	ExitCode *int
	Signal   string
	TimedOut bool
}

// OutcomeTool is the structured execution interface. Go error is reserved for
// transport/implementation failures; business outcomes live in ExecutionOutcome.
type OutcomeTool interface {
	ExecuteOutcome(ctx context.Context, input string, observer ToolObserver) (ExecutionOutcome, error)
}

// IsError reports the compatibility IsError bit derived from Outcome.
// Never reverse-derive a full Outcome from IsError.
func (o ExecutionOutcome) IsError() bool {
	switch o.Status {
	case OutcomeFailed, OutcomeDenied, OutcomeCancelled, OutcomeInterrupted,
		OutcomeNeedsAuth, OutcomeNeedsRuntime, OutcomeUnsupported:
		return true
	}
	return o.SemanticStatus == SemanticFailure
}

// WithTiming fills StartedAt/FinishedAt/DurationMs when missing.
func (o ExecutionOutcome) WithTiming(started, finished time.Time) ExecutionOutcome {
	if started.IsZero() {
		started = time.Now().UTC()
	}
	if finished.IsZero() {
		finished = time.Now().UTC()
	}
	o.StartedAt = started.UTC()
	o.FinishedAt = finished.UTC()
	if o.DurationMs <= 0 {
		o.DurationMs = finished.Sub(started).Milliseconds()
		if o.DurationMs < 0 {
			o.DurationMs = 0
		}
	}
	return o
}

// ModelPayload returns a compact, stable JSON object for LLM tool messages.
// Secret-bearing keys in diagnostic/details/input hints are redacted and
// artifact preview content is omitted. Callers remain responsible for path
// policy and for avoiding secrets in legacy free-text content.
func (o ExecutionOutcome) ModelPayload() string {
	// Runtime-finalized v2 outcomes already carry execution-context facts such
	// as declared mutation capability. Re-normalizing them with an empty context
	// can only discard provenance (and historically downgraded an intentionally
	// unknown mutation state to "none"). Projection must preserve canonical
	// facts; only legacy/unfinalized outcomes need compatibility normalization.
	if o.SchemaVersion != OutcomeSchemaVersion {
		o = o.Normalize(OutcomeNormalizationContext{})
	} else {
		o = cloneExecutionOutcome(o)
		// Projection still completes environment observation bookkeeping, but
		// deliberately avoids full semantic normalization without the original
		// execution context.
		environmentContext := OutcomeNormalizationContext{}
		if o.Environment != nil {
			environmentContext.WorkspaceRoot = o.Environment.WorkspaceRoot
			environmentContext.WorkingDirectory = o.Environment.WorkingDirectory
			environmentContext.ObservationSource = o.Environment.Source
		}
		o.normalizeEnvironment(environmentContext)
	}
	projectionTruncated := false
	putText := func(payload map[string]any, key string, value string, budget int) {
		projected, truncated := boundedModelText(value, budget)
		payload[key] = projected
		projectionTruncated = projectionTruncated || truncated
	}
	putStructured := func(payload map[string]any, key string, value any) {
		projected, truncated := boundedSafeModelValue(value, modelStructuredFieldBudgetBytes)
		payload[key] = projected
		projectionTruncated = projectionTruncated || truncated
	}
	payload := map[string]any{
		"schema_version":   o.SchemaVersion,
		"status":           o.Status,
		"transport_status": o.TransportStatus,
		"semantic_status":  o.SemanticStatus,
		"process_status":   o.ProcessStatus,
		"retryable":        o.Retryable,
		"duration_ms":      o.DurationMs,
		"truncated":        o.Truncated,
	}
	putText(payload, "content", o.Content, modelContentBudgetRunes)
	putText(payload, "summary", o.Summary, modelSummaryBudgetRunes)
	if o.Operation != nil {
		operation := *o.Operation
		var truncated bool
		operation.Type, truncated = boundedModelText(operation.Type, modelOperationTypeBudgetRunes)
		projectionTruncated = projectionTruncated || truncated
		operation.Target, truncated = boundedModelText(operation.Target, modelOperationTargetBudgetRunes)
		projectionTruncated = projectionTruncated || truncated
		payload["operation"] = &operation
	}
	if o.ExitCode != nil {
		payload["exit_code"] = *o.ExitCode
	}
	if o.Signal != "" {
		payload["signal"] = o.Signal
	}
	if o.TimedOut {
		payload["timed_out"] = true
	}
	if o.ErrorCode != "" {
		payload["error_code"] = o.ErrorCode
	}
	if o.ErrorCategory != "" {
		payload["error_category"] = o.ErrorCategory
	}
	if o.ErrorMessage != "" {
		putText(payload, "error_message", o.ErrorMessage, modelErrorMessageBudgetRunes)
	}
	if len(o.RecoveryOptions) > 0 {
		putStructured(payload, "recovery_options", o.RecoveryOptions)
	}
	if len(o.Diagnostic) > 0 {
		putStructured(payload, "diagnostic", o.Diagnostic)
	}
	if len(o.AffectedPaths) > 0 {
		putStructured(payload, "affected_paths", o.AffectedPaths)
	}
	if o.PatchSummary != "" {
		putText(payload, "patch_summary", o.PatchSummary, modelPatchSummaryBudgetRunes)
	}
	if len(o.Changes) > 0 {
		putStructured(payload, "changes", safeResourceChanges(o.Changes))
	}
	if len(o.Artifacts) > 0 {
		putStructured(payload, "artifacts", compactModelArtifacts(o.Artifacts))
	}
	if o.Environment != nil {
		putStructured(payload, "environment", safeEnvironmentObservation(*o.Environment))
	}
	if len(o.Warnings) > 0 {
		putStructured(payload, "warnings", o.Warnings)
	}
	if len(o.Errors) > 0 {
		putStructured(payload, "errors", o.Errors)
	}
	if len(o.NextActions) > 0 {
		putStructured(payload, "next_actions", safeNextActions(o.NextActions))
	}
	if len(o.Details) > 0 {
		putStructured(payload, "details", o.Details)
	}
	payload["truncated"] = o.Truncated || projectionTruncated
	data, err := json.Marshal(payload)
	if err != nil {
		return o.Content
	}
	return string(data)
}

// SucceededOutcome builds a successful non-process outcome.
func SucceededOutcome(content string) ExecutionOutcome {
	now := time.Now().UTC()
	return ExecutionOutcome{
		Content:         content,
		Status:          OutcomeSucceeded,
		TransportStatus: TransportCompleted,
		SemanticStatus:  SemanticSuccess,
		ProcessStatus:   ProcessNotApplicable,
		StartedAt:       now,
		FinishedAt:      now,
	}
}

// FailedOutcome builds a failed non-process outcome.
func FailedOutcome(content string, code string, category ErrorCategory, retryable bool) ExecutionOutcome {
	now := time.Now().UTC()
	return ExecutionOutcome{
		Content:         content,
		Status:          OutcomeFailed,
		TransportStatus: TransportCompleted,
		SemanticStatus:  SemanticFailure,
		ProcessStatus:   ProcessNotApplicable,
		ErrorCode:       code,
		ErrorCategory:   category,
		ErrorMessage:    content,
		Retryable:       retryable,
		StartedAt:       now,
		FinishedAt:      now,
	}
}

// DeniedOutcome builds a policy/skill denial outcome.
func DeniedOutcome(content, code string) ExecutionOutcome {
	now := time.Now().UTC()
	return ExecutionOutcome{
		Content:         content,
		Status:          OutcomeDenied,
		TransportStatus: TransportNotStarted,
		SemanticStatus:  SemanticFailure,
		ProcessStatus:   ProcessNotApplicable,
		ErrorCode:       code,
		ErrorCategory:   ErrorCategoryPolicy,
		ErrorMessage:    content,
		Retryable:       false,
		StartedAt:       now,
		FinishedAt:      now,
	}
}

// LegacyOutcomeAdapter converts legacy (content, error) into ExecutionOutcome.
// Prefer OutcomeTool for new tools. Business failures encoded only as Go errors
// are treated as tool failures; callers that already know Status should set it.
func LegacyOutcomeAdapter(content string, err error) ExecutionOutcome {
	now := time.Now().UTC()
	if err != nil {
		msg := strings.TrimSpace(err.Error())
		if content == "" {
			content = msg
		}
		return ExecutionOutcome{
			Content:         content,
			Status:          OutcomeFailed,
			TransportStatus: TransportFailed,
			SemanticStatus:  SemanticFailure,
			ProcessStatus:   ProcessNotApplicable,
			ErrorCode:       ErrorCodeToolExecutionFailed,
			ErrorCategory:   ErrorCategoryTool,
			ErrorMessage:    msg,
			Retryable:       true,
			StartedAt:       now,
			FinishedAt:      now,
		}
	}
	return SucceededOutcome(content).WithTiming(now, now)
}

// ExecuteAsOutcome runs an OutcomeTool when available, otherwise adapts Execute.
func ExecuteAsOutcome(ctx context.Context, t Tool, input string, observer ToolObserver) (ExecutionOutcome, error) {
	if outcomeTool, ok := t.(OutcomeTool); ok {
		return outcomeTool.ExecuteOutcome(ctx, input, observer)
	}
	var (
		content string
		err     error
	)
	if observable, ok := t.(ObservableTool); ok && observer != nil {
		content, err = observable.ExecuteWithObserver(ctx, input, observer)
	} else {
		content, err = t.Execute(ctx, input)
	}
	outcome := LegacyOutcomeAdapter(content, err)
	if err != nil {
		// Transport/implementation error: return both outcome and err.
		return outcome, err
	}
	return outcome, nil
}
