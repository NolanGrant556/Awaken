package hookruntime

import (
	"context"
	"encoding/json"
	"time"
)

const (
	ConfigVersion  = 1
	MaxConfigBytes = 1 << 20
)

type Event string

const (
	EventRunStart           Event = "RunStart"
	EventUserPromptSubmit   Event = "UserPromptSubmit"
	EventPreToolUse         Event = "PreToolUse"
	EventPostToolUse        Event = "PostToolUse"
	EventPostToolUseFailure Event = "PostToolUseFailure"
	EventPreCompact         Event = "PreCompact"
	EventPostCompact        Event = "PostCompact"
	EventSubagentStart      Event = "SubagentStart"
	EventSubagentStop       Event = "SubagentStop"
	EventBeforeFinish       Event = "BeforeFinish"
	EventRunEnd             Event = "RunEnd"
)

type SourceKind string

const (
	SourceUser         SourceKind = "user"
	SourceProject      SourceKind = "project"
	SourceProjectLocal SourceKind = "project_local"
	SourcePlugin       SourceKind = "plugin"
)

func (s SourceKind) RequiresProjectTrust() bool {
	return s == SourceProject || s == SourceProjectLocal
}

type SourceStatus string

const (
	StatusEnabled   SourceStatus = "enabled"
	StatusDisabled  SourceStatus = "disabled"
	StatusUntrusted SourceStatus = "untrusted"
	StatusInvalid   SourceStatus = "invalid"
)

type Matcher struct {
	ToolNames    []string `json:"toolNames"`
	InputPattern string   `json:"inputPattern,omitempty"`
}

type CommandHook struct {
	ID             string            `json:"id"`
	Type           string            `json:"type"`
	Command        []string          `json:"command,omitempty"`
	CWD            string            `json:"cwd,omitempty"`
	URL            string            `json:"url,omitempty"`
	AllowLoopback  bool              `json:"allowLoopback,omitempty"`
	SecretHeaders  map[string]string `json:"secretHeaders,omitempty"`
	Prompt         string            `json:"prompt,omitempty"`
	Model          string            `json:"model,omitempty"`
	MaxTurns       int               `json:"maxTurns,omitempty"`
	MCPServer      string            `json:"mcpServer,omitempty"`
	MCPTool        string            `json:"mcpTool,omitempty"`
	Arguments      json.RawMessage   `json:"arguments,omitempty"`
	TimeoutMS      int               `json:"timeoutMs"`
	MaxOutputBytes int               `json:"maxOutputBytes"`
	OnFailure      string            `json:"onFailure"`
	Async          bool              `json:"async,omitempty"`
	AsyncRewake    bool              `json:"asyncRewake,omitempty"`
	RewakeMessage  string            `json:"rewakeMessage,omitempty"`
}

type MatcherGroup struct {
	Matcher Matcher       `json:"matcher"`
	Hooks   []CommandHook `json:"hooks"`
}

type Config struct {
	Version int                      `json:"version"`
	Enabled bool                     `json:"enabled"`
	Hooks   map[Event][]MatcherGroup `json:"hooks"`
}

type CompiledHook struct {
	StableID          string            `json:"stableId"`
	Event             Event             `json:"event"`
	Matcher           Matcher           `json:"matcher"`
	Command           CommandHook       `json:"command"`
	SourceKind        SourceKind        `json:"sourceKind"`
	SourcePath        string            `json:"sourcePath"`
	ProjectRoot       string            `json:"projectRoot,omitempty"`
	PluginName        string            `json:"pluginName,omitempty"`
	PluginDigest      string            `json:"pluginDigest,omitempty"`
	LogicalID         string            `json:"logicalId,omitempty"`
	RevisionID        string            `json:"revisionId,omitempty"`
	SourceEvent       string            `json:"sourceEvent,omitempty"`
	Protocol          string            `json:"protocol,omitempty"`
	Environment       map[string]string `json:"-"`
	SecretEnvironment map[string]string `json:"-"`
}

type Diagnostic struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	SourcePath string `json:"sourcePath,omitempty"`
}

type SourceReceipt struct {
	Kind         SourceKind   `json:"kind"`
	Path         string       `json:"path"`
	ProjectRoot  string       `json:"projectRoot,omitempty"`
	Digest       string       `json:"digest,omitempty"`
	Status       SourceStatus `json:"status"`
	HookCount    int          `json:"hookCount"`
	Diagnostic   string       `json:"diagnostic,omitempty"`
	PluginName   string       `json:"pluginName,omitempty"`
	PluginDigest string       `json:"pluginDigest,omitempty"`
	// ReviewHooks is populated only by Store.Inspect for the local Settings
	// surface. It is deliberately excluded from snapshot serialization so a
	// command, URL, or prompt never enters hook_snapshot or snapshot digests.
	ReviewHooks []ReviewHook `json:"-"`
}

// ReviewHook is the normalized, static configuration that a user must be able
// to inspect before trusting a project Hook. Secret values and secret reference
// names are never included; only the configured HTTP header names are exposed.
type ReviewHook struct {
	ID                string   `json:"id"`
	Event             Event    `json:"event"`
	Type              string   `json:"type"`
	ToolNames         []string `json:"toolNames,omitempty"`
	Command           []string `json:"command,omitempty"`
	CWD               string   `json:"cwd,omitempty"`
	URL               string   `json:"url,omitempty"`
	AllowLoopback     bool     `json:"allowLoopback,omitempty"`
	SecretHeaderNames []string `json:"secretHeaderNames,omitempty"`
	Prompt            string   `json:"prompt,omitempty"`
	Model             string   `json:"model,omitempty"`
	MaxTurns          int      `json:"maxTurns,omitempty"`
	TimeoutMS         int      `json:"timeoutMs"`
	MaxOutputBytes    int      `json:"maxOutputBytes"`
	OnFailure         string   `json:"onFailure"`
}

type AvailabilitySnapshot struct {
	SnapshotID      string                   `json:"snapshotId"`
	SchemaVersion   int                      `json:"schemaVersion"`
	CreatedAt       time.Time                `json:"createdAt"`
	Sources         []SourceReceipt          `json:"sources"`
	HooksByEvent    map[Event][]CompiledHook `json:"hooksByEvent"`
	Diagnostics     []Diagnostic             `json:"diagnostics,omitempty"`
	MCPToolExecutor MCPToolExecutor          `json:"-"`
	ReceiptSink     ReceiptSink              `json:"-"`
	AsyncSink       AsyncCompletionSink      `json:"-"`
	AsyncLifecycle  AsyncInvocationLifecycle `json:"-"`
}

func EmptySnapshot() AvailabilitySnapshot {
	return AvailabilitySnapshot{SchemaVersion: ConfigVersion, CreatedAt: time.Now().UTC(), HooksByEvent: map[Event][]CompiledHook{}}
}

type InvocationIdentity struct {
	ThreadID       string
	RunID          string
	TurnID         string
	ProjectID      string
	WorkspaceRoot  string
	WorkspaceRoots []string
	SubagentTaskID string
}

type InputEnvelope struct {
	ProtocolVersion int                 `json:"protocolVersion"`
	InvocationID    string              `json:"invocationId"`
	HookID          string              `json:"hookId"`
	Event           Event               `json:"event"`
	Timestamp       time.Time           `json:"timestamp"`
	ThreadID        string              `json:"threadId"`
	RunID           string              `json:"runId"`
	TurnID          string              `json:"turnId,omitempty"`
	SubagentTaskID  string              `json:"subagentTaskId,omitempty"`
	Workspace       InvocationWorkspace `json:"workspace"`
	Subject         map[string]any      `json:"subject"`
}

type InvocationWorkspace struct {
	ProjectID      string   `json:"projectId,omitempty"`
	CWD            string   `json:"cwd"`
	WorkspaceRoots []string `json:"workspaceRoots"`
}

type Response struct {
	ProtocolVersion   int             `json:"protocolVersion"`
	Action            string          `json:"action"`
	Reason            string          `json:"reason,omitempty"`
	AdditionalContext string          `json:"additionalContext,omitempty"`
	UpdatedInput      json.RawMessage `json:"updatedInput,omitempty"`
}

type Outcome struct {
	InvocationID       string     `json:"invocation_id"`
	HookID             string     `json:"hook_id"`
	Event              Event      `json:"event"`
	SourceKind         SourceKind `json:"source_kind"`
	PluginName         string     `json:"plugin_name,omitempty"`
	PluginDigest       string     `json:"plugin_digest,omitempty"`
	LogicalID          string     `json:"logical_id,omitempty"`
	RevisionID         string     `json:"revision_id,omitempty"`
	Status             string     `json:"status"`
	SpawnState         string     `json:"spawn_state"`
	ProcessState       string     `json:"process_state"`
	ExitCode           *int       `json:"exit_code,omitempty"`
	Action             string     `json:"action"`
	ReasonCode         string     `json:"reason_code,omitempty"`
	Reason             string     `json:"reason,omitempty"`
	DurationMS         int64      `json:"duration_ms"`
	StdoutDigest       string     `json:"stdout_digest,omitempty"`
	StderrDigest       string     `json:"stderr_digest,omitempty"`
	OutputPreview      string     `json:"output_preview,omitempty"`
	InputDigest        string     `json:"input_digest,omitempty"`
	UpdatedInputDigest string     `json:"updated_input_digest,omitempty"`
}

type DispatchResult struct {
	Outcomes          []Outcome
	Action            string
	Reason            string
	AdditionalContext []string
	UpdatedInput      json.RawMessage
}

type Recorder interface {
	InvocationStarted(context.Context, InputEnvelope, CompiledHook) error
	InvocationCompleted(context.Context, Outcome) error
}

type Receipt struct {
	ID           string    `json:"id"`
	PluginName   string    `json:"pluginName"`
	PluginDigest string    `json:"pluginDigest"`
	LogicalID    string    `json:"logicalId"`
	RevisionID   string    `json:"revisionId"`
	Event        Event     `json:"event"`
	Status       string    `json:"status"`
	Action       string    `json:"action"`
	ReasonCode   string    `json:"reasonCode,omitempty"`
	DurationMS   int64     `json:"durationMs"`
	CreatedAt    time.Time `json:"createdAt"`
	RetryOf      string    `json:"retryOf,omitempty"`
}

type ReceiptSink interface {
	RecordHookReceipt(Outcome)
}

// AsyncCompletion is model-visible data produced after an asynchronous Hook
// has already released the operation that triggered it. It is kept outside the
// immutable Hook snapshot and can only be drained by the owning thread.
type AsyncCompletion struct {
	InvocationID      string
	Event             Event
	AdditionalContext string
	CompletedAt       time.Time
}

// AsyncCompletionSink is owned by the host runtime. The dispatcher records
// late Hook data here; the next run for the same thread drains it before its
// first model request. Implementations must be bounded.
type AsyncCompletionSink interface {
	RecordAsyncHookCompletion(threadID string, completion AsyncCompletion)
	HasAsyncHookCompletions(threadID string) bool
	DrainAsyncHookCompletions(threadID string) []AsyncCompletion
}

// AsyncInvocationLifecycle keeps host resources such as a Plugin generation
// lease alive until a scheduled Hook reaches a terminal process state.
type AsyncInvocationLifecycle interface {
	RetainAsyncHook() func()
}

// MCPToolExecutor invokes an already registered Plugin MCP tool. The hook
// runtime never resolves arbitrary server/tool strings on its own.
type MCPToolExecutor interface {
	ExecutePluginMCPTool(context.Context, CompiledHook, map[string]any) (string, error)
}

type MCPToolExecutorFunc func(context.Context, CompiledHook, map[string]any) (string, error)

func (f MCPToolExecutorFunc) ExecutePluginMCPTool(ctx context.Context, hook CompiledHook, arguments map[string]any) (string, error) {
	return f(ctx, hook, arguments)
}

// ModelExecutor is optionally implemented by the durable Hook recorder. This
// keeps model execution and accounting in Agent Core while the dispatcher
// remains independent from provider and SemanticJournal packages.
type ModelExecutor interface {
	ExecuteModelHook(context.Context, InputEnvelope, CompiledHook) (Response, Outcome, error)
}

type SecretResolver interface {
	ResolveHookSecret(context.Context, string) (string, error)
}

type SecretResolverFunc func(context.Context, string) (string, error)

func (f SecretResolverFunc) ResolveHookSecret(ctx context.Context, ref string) (string, error) {
	return f(ctx, ref)
}
