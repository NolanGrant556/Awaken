package configurehook

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"awaken/src/hookruntime"
	"awaken/src/tool"
)

const ToolName = "configure_hook"

type hookInput struct {
	ID             string            `json:"id"`
	Type           string            `json:"type"`
	Command        []string          `json:"command,omitempty"`
	CWD            string            `json:"cwd,omitempty"`
	URL            string            `json:"url,omitempty"`
	AllowLoopback  bool              `json:"allow_loopback,omitempty"`
	SecretHeaders  map[string]string `json:"secret_headers,omitempty"`
	Prompt         string            `json:"prompt,omitempty"`
	Model          string            `json:"model,omitempty"`
	MaxTurns       int               `json:"max_turns,omitempty"`
	TimeoutMS      int               `json:"timeout_ms,omitempty"`
	MaxOutputBytes int               `json:"max_output_bytes,omitempty"`
	OnFailure      string            `json:"on_failure,omitempty"`
}

type input struct {
	Action      string     `json:"action"`
	Scope       string     `json:"scope"`
	ProjectRoot string     `json:"project_root,omitempty"`
	Event       string     `json:"event,omitempty"`
	ToolNames   []string   `json:"tool_names,omitempty"`
	Hook        *hookInput `json:"hook,omitempty"`
	HookID      string     `json:"hook_id,omitempty"`
	Enabled     *bool      `json:"enabled,omitempty"`
}

type Tool struct{ store *hookruntime.Store }

var defaultTool = &Tool{}

func SetStore(store *hookruntime.Store) { defaultTool.store = store }

func (t *Tool) Name() string { return ToolName }

func (t *Tool) Description() string {
	return "Inspect or safely configure persistent Awaken Hooks from natural-language user requests. Supports user, shared project, and project-local scopes; validates the complete version-1 document and writes it atomically. Project files still require explicit trust in Settings and changes only affect a later run."
}

func (t *Tool) Parameters() []tool.ToolParameter { return nil }

func (t *Tool) InputSchema() tool.ToolParametersSchema {
	closed := false
	hookProperties := map[string]tool.ToolParam{
		"id":               {Type: "string", Description: "Stable Hook identifier."},
		"type":             {Type: "string", Enum: []any{"command", "http", "prompt", "agent"}, Description: "Executor type."},
		"command":          {Type: "array", Items: &tool.ToolParam{Type: "string"}, Description: "Direct argv; never a shell command string."},
		"cwd":              {Type: "string", Enum: []any{"workspace_root", "project_root"}, Description: "Host-owned command working directory selector."},
		"url":              {Type: "string", Description: "HTTP or HTTPS POST target."},
		"allow_loopback":   {Type: "boolean", Description: "Explicitly permit loopback HTTP targets; other private ranges remain blocked."},
		"secret_headers":   {Type: "object", AdditionalProperties: tool.BoolPtr(true), Description: "Header names mapped to Hook secret-store reference IDs; never secret values."},
		"prompt":           {Type: "string", Description: "Prompt for prompt or agent executors."},
		"model":            {Type: "string", Description: "Optional model selector within the active profile."},
		"max_turns":        {Type: "integer", Minimum: numberPtr(1), Maximum: numberPtr(8), Description: "Maximum turns for an agent executor."},
		"timeout_ms":       {Type: "integer", Minimum: numberPtr(100), Maximum: numberPtr(120000), Description: "Executor timeout in milliseconds."},
		"max_output_bytes": {Type: "integer", Minimum: numberPtr(1), Maximum: numberPtr(hookruntime.MaxConfigBytes), Description: "Maximum executor output bytes."},
		"on_failure":       {Type: "string", Enum: []any{"continue", "block"}, Description: "Whether a supported boundary continues or blocks after executor failure."},
	}
	return tool.ToolParametersSchema{
		Type: "object", AdditionalProperties: &closed, Required: []string{"action", "scope"},
		Properties: map[string]tool.ToolParam{
			"action":       {Type: "string", Enum: []any{"list", "upsert", "delete", "set_enabled"}, Description: "Configuration operation."},
			"scope":        {Type: "string", Enum: []any{"user", "project", "project_local"}, Description: "Persistent source to inspect or modify."},
			"project_root": {Type: "string", Description: "Optional exact bound project folder. Defaults to the current primary folder."},
			"event":        {Type: "string", Enum: eventEnum(), Description: "Lifecycle event for upsert."},
			"tool_names":   {Type: "array", Items: &tool.ToolParam{Type: "string"}, Description: "Exact canonical tool names for tool events; use * for every tool."},
			"hook":         {Type: "object", AdditionalProperties: &closed, Required: []string{"id", "type"}, Properties: hookProperties, Description: "Hook executor configuration for upsert."},
			"hook_id":      {Type: "string", Description: "Hook ID to delete."},
			"enabled":      {Type: "boolean", Description: "Source enabled state for set_enabled."},
		},
	}
}

func (t *Tool) Definition() tool.ToolDefinition {
	return tool.BuildBuiltinDefinition(t, tool.ToolDefinition{
		Name: ToolName, DisplayName: "Configure Hook", Description: t.Description(),
		Purpose: "Turn an explicit natural-language request into a strictly validated persistent Hook configuration without using Bash or editing protected Awaken configuration directly.",
		WhenToUse: []string{
			"The user asks to create, change, enable, disable, inspect, or delete a Hook.",
			"The user describes an automation at a run, tool, compact, subagent, or finish boundary.",
		},
		WhenNotToUse: []string{
			"Do not configure a Hook unless the user requested persistent automation.",
			"Do not put secrets in command arguments, URLs, prompts, or headers; secret_headers accepts only existing secret reference IDs.",
			"Do not use this tool from a Hook verifier or read-only subagent.",
		},
		InputSchema: t.InputSchema(),
		OutputSchema: tool.ToolParam{Type: "object", AdditionalProperties: tool.BoolPtr(false), Properties: map[string]tool.ToolParam{
			"action": {Type: "string"}, "source": {Type: "string"}, "path": {Type: "string"},
			"exists": {Type: "boolean"}, "enabled": {Type: "boolean"}, "hookCount": {Type: "integer"},
			"requiresTrust": {Type: "boolean"}, "hooks": {Type: "array", Items: &tool.ToolParam{Type: "object", AdditionalProperties: tool.BoolPtr(true)}},
		}},
		SideEffects: []tool.ToolSideEffect{
			{Kind: tool.SideEffectCreate, Resources: []string{"~/.awaken/config/hooks.json", "<project>/.awaken/hooks.json", "<project>/.awaken/hooks.local.json"}, Description: "Creates a strictly validated Hook configuration.", Reversibility: tool.ReversibilityReversible},
			{Kind: tool.SideEffectUpdate, Resources: []string{"Hook configuration"}, Description: "Atomically changes a Hook source; active runs keep their pinned snapshot.", Reversibility: tool.ReversibilityReversible},
			{Kind: tool.SideEffectDelete, Resources: []string{"Hook entry"}, Description: "Removes one Hook entry without deleting the configuration file.", Reversibility: tool.ReversibilityReversible},
		},
		Examples: []tool.ToolExample{
			{Scenario: "Run a direct validator before Write", Input: map[string]any{"action": "upsert", "scope": "project", "event": "PreToolUse", "tool_names": []string{"Write"}, "hook": map[string]any{"id": "validate-write", "type": "command", "command": []string{"./scripts/validate.sh"}, "cwd": "project_root", "on_failure": "block"}}, Outcome: "The project config is updated and requires explicit Settings trust before a later run can execute it."},
			{Scenario: "Disable all user Hooks", Input: map[string]any{"action": "set_enabled", "scope": "user", "enabled": false}, Outcome: "The user Hook source is disabled for later runs."},
		},
		FailureModes: []tool.ToolFailureMode{
			{Code: "hook_config_invalid_input", Cause: "The action, source, event, matcher, or executor fields are invalid.", Retryable: false, Recovery: []string{"Correct the structured Hook request."}},
			{Code: "hook_config_scope_unavailable", Cause: "A project source was requested outside an attached writable project folder.", Retryable: false, Recovery: []string{"Use user scope or select an exact attached writable project folder."}},
			{Code: "hook_config_existing_invalid", Cause: "The existing file is invalid or unsafe and is never overwritten.", Retryable: false, Recovery: []string{"Inspect and repair the existing configuration manually."}},
			{Code: "hook_config_store_unavailable", Cause: "The Core Hook store is unavailable.", Retryable: true, Recovery: []string{"Restart Awaken and retry."}},
			{Code: "hook_config_write_failed", Cause: "The validated configuration could not be persisted atomically.", Retryable: true, Recovery: []string{"Check file permissions and retry."}},
		},
		Annotations:  tool.ToolAnnotations{ReadOnly: false, Destructive: false, OpenWorld: false, CapabilityClass: tool.CapabilitySystemOperation, TrustedOrigin: tool.TrustedOriginBuiltin, FileBridgeScope: tool.FileBridgeNone, NetworkScope: tool.NetworkScopeNone},
		Capabilities: []tool.Capability{tool.CapabilityFileWrite},
	})
}

func init() { tool.MustRegisterToolWithMode(defaultTool, tool.DefinitionValidationStrictBuiltin) }

func (t *Tool) Execute(ctx context.Context, raw string) (string, error) {
	result, err := t.execute(ctx, raw)
	if err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(result, "", "  ")
	return string(data), err
}

func (t *Tool) ExecuteOutcome(ctx context.Context, raw string, _ tool.ToolObserver) (tool.ExecutionOutcome, error) {
	started := time.Now().UTC()
	result, err := t.execute(ctx, raw)
	if err != nil {
		code, category, retryable := classifyError(err)
		outcome := tool.FailedOutcome(err.Error(), code, category, retryable)
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		outcome := tool.FailedOutcome(err.Error(), "hook_config_output_invalid", tool.ErrorCategoryTool, false)
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}
	outcome := tool.SucceededOutcome(string(data))
	outcome.Summary = fmt.Sprintf("Hook configuration %s: %s", result.Action, result.Path)
	sideEffectState := tool.OutcomeSideEffectApplied
	if result.Action == "list" {
		sideEffectState = tool.OutcomeSideEffectNone
	}
	outcome.Operation = &tool.OperationDescriptor{Type: ToolName, Target: result.Path, SideEffectState: sideEffectState}
	// Details is validated against the public output schema. Keep its field
	// names identical to ConfigMutationResult's JSON contract; durable journal
	// receipts use their own intentionally smaller snake_case shape.
	outcome.Details = map[string]any{
		"action": result.Action, "source": result.Source, "path": result.Path,
		"exists": result.Exists, "enabled": result.Enabled, "hookCount": result.HookCount,
		"requiresTrust": result.RequiresTrust, "hooks": result.Hooks,
	}
	if result.Action != "list" {
		outcome.Changes = []tool.ResourceChange{{Kind: "hook_config", Action: "updated", Resource: result.Path, Summary: "Updated a strictly validated Hook configuration for later runs.", Certainty: tool.ChangeObserved, Reversibility: tool.ChangeReversible}}
	}
	return outcome.WithTiming(started, time.Now().UTC()), nil
}

func (t *Tool) CheckPermission(ctx context.Context, raw string, permission tool.PermissionContext) tool.PermissionDecision {
	request, err := parseInput(raw)
	if err != nil {
		return tool.DenyPermission(err.Error())
	}
	if _, _, err := resolveSource(request, permission); err != nil {
		return tool.DenyPermission(err.Error())
	}
	return tool.AllowPermission()
}

// SemanticJournalArguments keeps configuration bodies, commands, prompts and
// secret reference IDs out of durable replay while preserving the operation
// identity needed to explain what happened.
func (t *Tool) SemanticJournalArguments(raw string) string {
	request, err := parseInput(raw)
	if err != nil {
		return `{}`
	}
	receipt := map[string]any{"action": request.Action, "scope": request.Scope}
	if request.Event != "" {
		receipt["event"] = request.Event
	}
	if request.HookID != "" {
		receipt["hook_id"] = request.HookID
	}
	if request.Hook != nil {
		receipt["hook_id"] = request.Hook.ID
		receipt["hook_type"] = request.Hook.Type
	}
	data, _ := json.Marshal(receipt)
	return string(data)
}

// SemanticJournalResult strips normalized review bodies from durable replay.
// The current model may inspect them during this run, but a restart receives
// only a bounded mutation receipt.
func (t *Tool) SemanticJournalResult(raw string) string {
	var result hookruntime.ConfigMutationResult
	if json.Unmarshal([]byte(raw), &result) != nil {
		return `{"status":"completed"}`
	}
	receipt := map[string]any{
		"status": "completed", "action": result.Action, "source": result.Source,
		"exists": result.Exists, "enabled": result.Enabled, "hook_count": result.HookCount,
		"requires_trust": result.RequiresTrust,
	}
	data, _ := json.Marshal(receipt)
	return string(data)
}

func (t *Tool) execute(ctx context.Context, raw string) (hookruntime.ConfigMutationResult, error) {
	if t.store == nil {
		return hookruntime.ConfigMutationResult{}, errors.New("hook_config_store_unavailable: Hook store is not configured")
	}
	request, err := parseInput(raw)
	if err != nil {
		return hookruntime.ConfigMutationResult{}, fmt.Errorf("hook_config_invalid_input: %w", err)
	}
	permission := tool.PermissionContextFromContext(ctx)
	source, root, err := resolveSource(request, permission)
	if err != nil {
		return hookruntime.ConfigMutationResult{}, fmt.Errorf("hook_config_scope_unavailable: %w", err)
	}
	mutation := hookruntime.ConfigMutationRequest{
		Action: request.Action, Source: source, ProjectRoot: root, Event: hookruntime.Event(request.Event),
		Matcher: hookruntime.Matcher{ToolNames: append([]string(nil), request.ToolNames...)}, HookID: request.HookID, Enabled: request.Enabled,
	}
	if request.Hook != nil {
		mutation.Hook = hookruntime.CommandHook{
			ID: request.Hook.ID, Type: request.Hook.Type, Command: append([]string(nil), request.Hook.Command...), CWD: request.Hook.CWD,
			URL: request.Hook.URL, AllowLoopback: request.Hook.AllowLoopback, SecretHeaders: cloneMap(request.Hook.SecretHeaders), Prompt: request.Hook.Prompt,
			Model: request.Hook.Model, MaxTurns: request.Hook.MaxTurns, TimeoutMS: request.Hook.TimeoutMS, MaxOutputBytes: request.Hook.MaxOutputBytes, OnFailure: request.Hook.OnFailure,
		}
	}
	result, err := t.store.MutateConfig(mutation)
	if err != nil {
		return hookruntime.ConfigMutationResult{}, fmt.Errorf("hook_config_write_failed: %w", err)
	}
	return result, nil
}

func parseInput(raw string) (input, error) {
	var request input
	decoder := json.NewDecoder(strings.NewReader(strings.TrimSpace(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return request, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return request, errors.New("arguments must contain exactly one JSON object")
	}
	if strings.TrimSpace(request.Action) == "" || strings.TrimSpace(request.Scope) == "" {
		return request, errors.New("action and scope are required")
	}
	return request, nil
}

func resolveSource(request input, permission tool.PermissionContext) (hookruntime.SourceKind, string, error) {
	switch strings.ToLower(strings.TrimSpace(request.Scope)) {
	case "user":
		return hookruntime.SourceUser, "", nil
	case "project", "project_local":
		if permission.SessionWorkspace || strings.TrimSpace(permission.ProjectID) == "" {
			return "", "", errors.New("the current run is not attached to a folder-backed project")
		}
		source := hookruntime.SourceProject
		if strings.EqualFold(request.Scope, "project_local") {
			source = hookruntime.SourceProjectLocal
		}
		root, err := exactWritableProjectRoot(request.ProjectRoot, permission)
		return source, root, err
	default:
		return "", "", fmt.Errorf("unsupported scope %q", request.Scope)
	}
}

func exactWritableProjectRoot(requested string, permission tool.PermissionContext) (string, error) {
	permission = permission.Normalize()
	candidate := strings.TrimSpace(requested)
	if candidate == "" {
		candidate = permission.WorkspaceRoot
	}
	if !filepath.IsAbs(candidate) {
		return "", errors.New("project_root must be an absolute attached folder")
	}
	candidateReal, err := filepath.EvalSymlinks(filepath.Clean(candidate))
	if err != nil {
		return "", fmt.Errorf("resolve project_root: %w", err)
	}
	readable := false
	for _, root := range permission.ReadableRoots {
		rootReal, resolveErr := filepath.EvalSymlinks(root)
		if resolveErr == nil && filepath.Clean(rootReal) == filepath.Clean(candidateReal) {
			readable = true
			break
		}
	}
	if readable {
		for _, root := range permission.WritableRoots {
			rootReal, resolveErr := filepath.EvalSymlinks(root)
			if resolveErr == nil && filepath.Clean(rootReal) == filepath.Clean(candidateReal) {
				return filepath.Clean(rootReal), nil
			}
		}
	}
	return "", errors.New("project_root is not an exact writable project folder")
}

func classifyError(err error) (string, tool.ErrorCategory, bool) {
	message := err.Error()
	switch {
	case strings.Contains(message, "store_unavailable"):
		return "hook_config_store_unavailable", tool.ErrorCategoryRuntime, true
	case strings.Contains(message, "scope_unavailable"):
		return "hook_config_scope_unavailable", tool.ErrorCategoryPolicy, false
	case strings.Contains(message, "invalid_input"):
		return "hook_config_invalid_input", tool.ErrorCategoryValidation, false
	case strings.Contains(message, "permissions") || strings.Contains(message, "invalid config") || strings.Contains(message, "symlink"):
		return "hook_config_existing_invalid", tool.ErrorCategoryPolicy, false
	default:
		return "hook_config_write_failed", tool.ErrorCategoryTool, true
	}
}

func eventEnum() []any {
	return []any{"RunStart", "UserPromptSubmit", "PreToolUse", "PostToolUse", "PostToolUseFailure", "PreCompact", "PostCompact", "SubagentStart", "SubagentStop", "BeforeFinish", "RunEnd"}
}

func numberPtr(value int) *float64 { result := float64(value); return &result }

func cloneMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}
