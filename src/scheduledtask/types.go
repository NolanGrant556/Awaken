// Package scheduledtask owns the scheduled-task domain of Awaken Core:
// task/schedule/action/execution schema, five-field cron and calendar time
// math, validation and the journal-first repository. The authoritative
// contract is docs/12_定时任务/定时任务设计契约.md.
//
// Slice A scope: model, time calculation and repository only. No HTTP
// surface and no real timers are registered here.
//
// Task definitions never persist API keys, Authorization headers, resolved
// model profiles or absolute workspace-root snapshots; task content is user
// business data and must never appear in error messages or logs produced by
// this package.
package scheduledtask

import (
	"errors"
	"strings"
	"time"
)

const (
	// TaskIDPrefix prefixes Core-generated task IDs.
	TaskIDPrefix = "sched_"
	// ExecutionIDPrefix prefixes Core-generated execution IDs.
	ExecutionIDPrefix = "exec_"
)

// Domain errors. Callers map them to transport codes with errors.Is; the
// scheduler/service slices rely on them for control flow.
var (
	ErrTaskNotFound       = errors.New("scheduledtask: task not found")
	ErrExecutionNotFound  = errors.New("scheduledtask: execution not found")
	ErrVersionConflict    = errors.New("scheduledtask: version conflict")
	ErrTaskAlreadyRunning = errors.New("scheduledtask: task already running")
	ErrValidationFailed   = errors.New("scheduledtask: validation failed")
	ErrInvalidRequest     = errors.New("scheduledtask: invalid request")
	ErrInvalidTransition  = errors.New("scheduledtask: invalid execution status transition")
	ErrCorruptState       = errors.New("scheduledtask: corrupt mutation stream")
)

// ScheduleKind selects the schedule branch.
type ScheduleKind string

const (
	ScheduleKindCron     ScheduleKind = "cron"
	ScheduleKindCalendar ScheduleKind = "calendar"
)

// ActionKind selects what a trigger does.
type ActionKind string

const (
	ActionKindReminder ActionKind = "reminder"
	ActionKindAgent    ActionKind = "agent"
)

// Trigger records why an execution exists.
type Trigger string

const (
	TriggerScheduled Trigger = "scheduled"
	TriggerManual    Trigger = "manual"
	TriggerMisfire   Trigger = "misfire"
)

// ExecutionStatus is the execution state machine:
// queued|running -> success|error|skipped|cancelled|interrupted.
type ExecutionStatus string

const (
	ExecutionStatusQueued      ExecutionStatus = "queued"
	ExecutionStatusRunning     ExecutionStatus = "running"
	ExecutionStatusSuccess     ExecutionStatus = "success"
	ExecutionStatusError       ExecutionStatus = "error"
	ExecutionStatusSkipped     ExecutionStatus = "skipped"
	ExecutionStatusCancelled   ExecutionStatus = "cancelled"
	ExecutionStatusInterrupted ExecutionStatus = "interrupted"
)

// Terminal reports whether the status ends the execution state machine.
func (s ExecutionStatus) Terminal() bool {
	switch s {
	case ExecutionStatusSuccess, ExecutionStatusError, ExecutionStatusSkipped,
		ExecutionStatusCancelled, ExecutionStatusInterrupted:
		return true
	}
	return false
}

// TaskState is the materialized task state (contract §6.2). It is computed,
// never stored.
type TaskState string

const (
	TaskStateEnabled   TaskState = "enabled"
	TaskStatePaused    TaskState = "paused"
	TaskStateRunning   TaskState = "running"
	TaskStateCompleted TaskState = "completed"
	// TaskStateInvalid is assigned by the scheduler at startup when a
	// persisted definition no longer validates; the repository never emits it.
	TaskStateInvalid TaskState = "invalid"
)

// RepeatEndKind bounds a repeating calendar schedule.
type RepeatEndKind string

const (
	RepeatEndNever RepeatEndKind = "never"
	RepeatEndUntil RepeatEndKind = "until"
	RepeatEndCount RepeatEndKind = "count"
)

// RepeatEnd bounds a repeating calendar schedule. Until is required for
// kind=until, Count for kind=count.
type RepeatEnd struct {
	Kind  RepeatEndKind `json:"kind"`
	Until *time.Time    `json:"until,omitempty"`
	Count int           `json:"count,omitempty"`
}

// Schedule is the task schedule. The cron branch carries Cron+Timezone; the
// calendar branch carries RunAt+Timezone and optional repeat fields. A
// non-repeating calendar schedule omits RepeatEveryDays/RepeatEnd entirely.
type Schedule struct {
	Kind     ScheduleKind `json:"kind"`
	Timezone string       `json:"timezone"`

	// cron branch: five fields "minute hour day-of-month month day-of-week".
	Cron string `json:"cron,omitempty"`

	// calendar branch.
	RunAt           *time.Time `json:"run_at,omitempty"`
	RepeatEveryDays int        `json:"repeat_every_days,omitempty"`
	RepeatEnd       *RepeatEnd `json:"repeat_end,omitempty"`
}

// ComposerTokenPayload is the durable identity carried by a user-selected
// Composer action. The shape mirrors runtime.ComposerToken so the host can
// reuse StartRun's Skill/Plugin/capability revalidation instead of trusting
// Renderer paths or package metadata.
type ComposerTokenPayload struct {
	CapabilityID  string `json:"capabilityId,omitempty"`
	SkillID       string `json:"skillId,omitempty"`
	PluginName    string `json:"pluginName,omitempty"`
	PluginDigest  string `json:"pluginDigest,omitempty"`
	SourceOrigin  string `json:"sourceOrigin,omitempty"`
	SkillFilePath string `json:"skillFilePath,omitempty"`
	RootDir       string `json:"rootDir,omitempty"`
}

// ComposerToken preserves the structured Skill, Plugin, or capability
// selection and the text that preceded it in the editor. Content stores the
// remaining trailing text.
type ComposerToken struct {
	ID          string               `json:"id"`
	Type        string               `json:"type"`
	LeadingText string               `json:"leadingText,omitempty"`
	Label       string               `json:"label"`
	Payload     ComposerTokenPayload `json:"payload,omitempty"`
}

// Action is what the task executes. Reminder content is the fixed body.
// Agent content is the trailing text after structured composer tokens; the
// effective prompt is ComposerTokens' leading text followed by Content.
// All of it is user business data and stays out of errors/logs.
type Action struct {
	Kind           ActionKind      `json:"kind"`
	Content        string          `json:"content"`
	ComposerTokens []ComposerToken `json:"composer_tokens,omitempty"`
}

// EffectivePrompt reconstructs the same visible text accepted by the home
// Composer while keeping every token identity structured for host-side
// validation and expansion. Token-only actions intentionally return an empty
// prompt.
func (a Action) EffectivePrompt() string {
	var prompt strings.Builder
	for _, token := range a.ComposerTokens {
		prompt.WriteString(token.LeadingText)
	}
	prompt.WriteString(a.Content)
	return strings.TrimSpace(prompt.String())
}

// ExecutionOptions holds per-run parameters. ProfileID is an opaque model
// profile ID; empty resolves the default ready profile at trigger time.
type ExecutionOptions struct {
	ProfileID           string `json:"profile_id"`
	TimeoutSeconds      int    `json:"timeout_seconds"`
	MisfireGraceSeconds int    `json:"misfire_grace_seconds"`
}

// TemplateOrigin records which template prefilled the editor. The task is a
// plain task afterwards; later template changes never rewrite it.
type TemplateOrigin struct {
	TemplateID string `json:"template_id"`
	Source     string `json:"source"`
}

// Task is the persisted scheduled-task definition (contract §6.1). TaskID,
// Version, CreatedAt and UpdatedAt are Core-generated; client-supplied
// identity, state, next-run and timestamps are never trusted.
type Task struct {
	TaskID    string           `json:"task_id"`
	Version   int64            `json:"version"`
	Name      string           `json:"name"`
	Enabled   bool             `json:"enabled"`
	ProjectID string           `json:"project_id"`
	ThreadID  string           `json:"thread_id,omitempty"`
	Schedule  Schedule         `json:"schedule"`
	Action    Action           `json:"action"`
	Execution ExecutionOptions `json:"execution"`

	TemplateOrigin *TemplateOrigin `json:"template_origin,omitempty"`

	// ConsumedOccurrences counts due scheduled/misfire occurrences already
	// consumed, including failed and skipped ones; manual runs never count.
	ConsumedOccurrences int `json:"consumed_occurrences,omitempty"`
	// LastProcessedOccurrence is the newest occurrence already processed; the
	// misfire calculation resumes strictly after it.
	LastProcessedOccurrence *time.Time `json:"last_processed_occurrence,omitempty"`
	// Completed is set when the schedule is exhausted (one-time run/skipped,
	// or repeat count reached); completed tasks are never scheduled again.
	Completed bool `json:"completed,omitempty"`
	// InvalidReason is set by the scheduler when a persisted definition no
	// longer validates at startup (contract §8.1). It carries only the
	// sanitized field/issue label of the validation error, never content.
	// A successful edit clears it.
	InvalidReason string `json:"invalid_reason,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// State materializes the task state. running reports whether an execution is
// currently active (tracked by the scheduler, not the repository).
func (t Task) State(running bool) TaskState {
	if t.Completed {
		return TaskStateCompleted
	}
	if t.InvalidReason != "" {
		return TaskStateInvalid
	}
	if !t.Enabled {
		return TaskStatePaused
	}
	if running {
		return TaskStateRunning
	}
	return TaskStateEnabled
}

// Execution is one execution-history record. Status transitions are appended
// as new records carrying the same ExecutionID; readers fold to the newest
// record per ExecutionID.
type Execution struct {
	ExecutionID  string          `json:"execution_id"`
	TaskID       string          `json:"task_id"`
	TaskVersion  int64           `json:"task_version"`
	Trigger      Trigger         `json:"trigger"`
	ScheduledFor time.Time       `json:"scheduled_for"`
	StartedAt    time.Time       `json:"started_at"`
	FinishedAt   *time.Time      `json:"finished_at,omitempty"`
	Status       ExecutionStatus `json:"status"`
	RunID        string          `json:"run_id,omitempty"`
	ThreadID     string          `json:"thread_id,omitempty"`
	ErrorCode    string          `json:"error_code,omitempty"`
	ErrorMessage string          `json:"error_message,omitempty"`

	// IdempotencyKey deduplicates manual runs across transport retries.
	IdempotencyKey string `json:"idempotency_key,omitempty"`
}
