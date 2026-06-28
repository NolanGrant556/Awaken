package pluginruntime

import "context"

type PluginSource string

const (
	SourceBundled    PluginSource = "bundled"
	SourceUser       PluginSource = "user"
	SourceProject    PluginSource = "project"
	SourceEntrypoint PluginSource = "entrypoint"
)

type PluginManifest struct {
	Name         string
	Key          string
	Version      string
	Description  string
	Author       string
	Source       PluginSource
	Path         string
	Capabilities ManifestCapabilities
}

type ManifestCapabilities struct {
	Hooks              []HookEvent
	SlashCommands      []string
	CLICommands        []string
	Tools              []string
	ResultTransforms   bool
	TerminalTransforms bool
	LLMContext         bool
	SessionLifecycle   bool
}

type LoadedPlugin struct {
	Manifest     PluginManifest
	Registration Registration
	Enabled      bool
	Error        string
	Sequence     int
}

type Registration struct {
	Hooks         []HookRegistration
	SlashCommands []CommandRegistration
	CLICommands   []CommandRegistration
	Tools         []ToolRegistration
}

type PluginRef struct {
	Name   string
	Key    string
	Source PluginSource
}

type CapabilityKind string

const (
	CapabilityHook         CapabilityKind = "hook"
	CapabilitySlashCommand CapabilityKind = "slash_command"
	CapabilityCLICommand   CapabilityKind = "cli_command"
	CapabilityTool         CapabilityKind = "tool"
)

type ExtensionCapability struct {
	Plugin      PluginRef
	Kind        CapabilityKind
	Name        string
	Description string
}

type HookEvent string

const (
	HookPreToolCall             HookEvent = "pre_tool_call"
	HookPostToolCall            HookEvent = "post_tool_call"
	HookTransformToolResult     HookEvent = "transform_tool_result"
	HookTransformTerminalOutput HookEvent = "transform_terminal_output"
	HookPreLLMCall              HookEvent = "pre_llm_call"
	HookPostLLMCall             HookEvent = "post_llm_call"
	HookOnSessionStart          HookEvent = "on_session_start"
	HookOnSessionEnd            HookEvent = "on_session_end"
	HookOnSessionFinalize       HookEvent = "on_session_finalize"
	HookOnSessionReset          HookEvent = "on_session_reset"
)

type HookHandler func(context.Context, HookInvocation) (HookResult, error)

type HookRegistration struct {
	Event       HookEvent
	Description string
	Handler     HookHandler
}

type HookInvocation struct {
	Event          HookEvent
	Plugin         PluginRef
	ToolCall       ToolCallEvent
	ToolResult     ToolResultEvent
	TerminalOutput TerminalOutputEvent
	LLMCall        LLMCallEvent
	LLMResponse    LLMResponseEvent
	Session        SessionEvent
}

type HookResult struct {
	PreToolCall       *PreToolCallDecision
	ToolResult        *string
	TerminalOutput    *string
	ContextInjections []ContextInjection
}

type PreToolCallAction string

const (
	PreToolCallAllow PreToolCallAction = "allow"
	PreToolCallBlock PreToolCallAction = "block"
)

type PreToolCallDecision struct {
	Action     PreToolCallAction
	Message    string
	PluginName string
	PluginKey  string
}

func (d PreToolCallDecision) Allowed() bool {
	return d.Action != PreToolCallBlock
}

type HookError struct {
	PluginName string
	PluginKey  string
	Event      HookEvent
	Message    string
}

type HookReport struct {
	Errors []HookError
}

func (r HookReport) HasErrors() bool {
	return len(r.Errors) > 0
}

type ToolCallEvent struct {
	ID        string
	Name      string
	Input     string
	Arguments map[string]any
	TaskID    string
	SessionID string
	Metadata  map[string]string
}

type ToolResultEvent struct {
	Call    ToolCallEvent
	Result  string
	IsError bool
}

type TerminalOutputEvent struct {
	Call      ToolCallEvent
	Stream    string
	Output    string
	Truncated bool
}

type LLMCallEvent struct {
	SessionID    string
	Model        string
	SystemPrompt string
	UserPrompt   string
	Messages     []Message
	Metadata     map[string]string
}

type LLMResponseEvent struct {
	Content   string
	ToolCalls []ToolCallEvent
	IsError   bool
}

type Message struct {
	Role    string
	Content string
}

type ContextInjection struct {
	PluginName string
	PluginKey  string
	Role       string
	Content    string
	Metadata   map[string]string
}

type SessionEvent struct {
	SessionID string
	Reason    string
	Metadata  map[string]string
}

type CommandHandler func(context.Context, CommandInvocation) (CommandResult, error)

type CommandRegistration struct {
	Name        string
	Description string
	Usage       string
	Handler     CommandHandler
}

type CommandInvocation struct {
	Name      string
	RawArgs   string
	Arguments []string
	SessionID string
}

type CommandResult struct {
	Output string
}

type ToolHandler func(context.Context, ToolCallEvent) (string, error)

type ToolRegistration struct {
	Name        string
	Description string
	InputSchema map[string]any
	Handler     ToolHandler
}

func BlockToolCall(message string) HookResult {
	decision := PreToolCallDecision{Action: PreToolCallBlock, Message: message}
	return HookResult{PreToolCall: &decision}
}

func ReplaceToolResult(result string) HookResult {
	return HookResult{ToolResult: &result}
}

func ReplaceTerminalOutput(output string) HookResult {
	return HookResult{TerminalOutput: &output}
}

func InjectContext(content string) HookResult {
	return HookResult{
		ContextInjections: []ContextInjection{{Content: content}},
	}
}
