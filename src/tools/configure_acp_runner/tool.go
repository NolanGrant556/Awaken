package configureacprunner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"awaken/src/acp"
	"awaken/src/tool"
)

const ToolName = "configure_acp_runner"

type configureInput struct {
	Action                string            `json:"action"`
	RunnerID              string            `json:"runner_id"`
	Enabled               *bool             `json:"enabled,omitempty"`
	Command               string            `json:"command,omitempty"`
	Args                  []string          `json:"args,omitempty"`
	Env                   map[string]string `json:"env,omitempty"`
	PermissionMode        string            `json:"permission_mode,omitempty"`
	StartupTimeoutSeconds int               `json:"startup_timeout_seconds,omitempty"`
	TurnTimeoutSeconds    int               `json:"turn_timeout_seconds,omitempty"`
	MaxFrameBytes         int               `json:"max_frame_bytes,omitempty"`
}

type configureOutput struct {
	RunnerID     string `json:"runner_id"`
	Action       string `json:"action"`
	ResultStatus string `json:"result_status"`
	Message      string `json:"message"`
}

type configureFailure struct {
	code      string
	category  tool.ErrorCategory
	retryable bool
	recovery  []string
	message   string
}

func (e *configureFailure) Error() string {
	if e == nil {
		return "configure acp runner failed"
	}
	return e.message
}

// ConfigureACPRunnerTool edits the user-scoped ACP runner configuration.
type ConfigureACPRunnerTool struct {
	registry *acp.Registry
}

var defaultTool = &ConfigureACPRunnerTool{}

// SetRegistry injects the ACP registry into the registered tool.
func SetRegistry(r *acp.Registry) {
	defaultTool.registry = r
}

func (t *ConfigureACPRunnerTool) Name() string { return ToolName }

func (t *ConfigureACPRunnerTool) Description() string {
	return "Create, update, upsert or delete a user-scoped ACP (Agent Client Protocol) runner configuration. The tool only edits ~/.awaken/config/acp-agents.json and never touches project-level paths."
}

func (t *ConfigureACPRunnerTool) Parameters() []tool.ToolParameter {
	return []tool.ToolParameter{
		{Name: "action", Type: "string", Required: true, Description: "create | update | upsert | delete"},
		{Name: "runner_id", Type: "string", Required: true, Description: "Stable runner identifier, e.g. codex, claude_code, opencode, qwen_code."},
		{Name: "enabled", Type: "boolean", Required: false, Description: "Whether the runner is enabled. Defaults to true on create."},
		{Name: "command", Type: "string", Required: false, Description: "Executable command for create/update/upsert; required when creating a new runner."},
		{Name: "args", Type: "array", Required: false, Description: "Command arguments for the runner."},
		{Name: "env", Type: "object", Required: false, Description: "Environment variables as key-value pairs."},
		{Name: "permission_mode", Type: "string", Required: false, Description: "ask (default) or deny."},
		{Name: "startup_timeout_seconds", Type: "integer", Required: false, Description: "Startup timeout in seconds; defaults to 15."},
		{Name: "turn_timeout_seconds", Type: "integer", Required: false, Description: "Per-turn timeout in seconds; defaults to 600."},
		{Name: "max_frame_bytes", Type: "integer", Required: false, Description: "Maximum ACP frame size in bytes; defaults to 8 MiB."},
	}
}

func (t *ConfigureACPRunnerTool) InputSchema() tool.ToolParametersSchema {
	noExtra := false
	return tool.ToolParametersSchema{
		Type:                 "object",
		AdditionalProperties: &noExtra,
		Required:             []string{"action", "runner_id"},
		Properties: map[string]tool.ToolParam{
			"action":  {Type: "string", Description: "create | update | upsert | delete", Enum: []any{"create", "update", "upsert", "delete"}},
			"runner_id": {Type: "string", Description: "Runner identifier, e.g. codex, claude_code, opencode, qwen_code."},
			"enabled": {Type: "boolean", Description: "Whether the runner is enabled."},
			"command": {Type: "string", Description: "Executable command; required for create."},
			"args": {Type: "array", Description: "Command arguments.", Items: &tool.ToolParam{Type: "string"}},
			"env": {Type: "object", Description: "Environment variables.", AdditionalProperties: tool.BoolPtr(true)},
			"permission_mode": {Type: "string", Description: "ask or deny.", Enum: []any{"ask", "deny"}},
			"startup_timeout_seconds": {Type: "integer", Description: "Startup timeout in seconds."},
			"turn_timeout_seconds": {Type: "integer", Description: "Per-turn timeout in seconds."},
			"max_frame_bytes": {Type: "integer", Description: "Maximum ACP frame size in bytes."},
		},
	}
}

func (t *ConfigureACPRunnerTool) Definition() tool.ToolDefinition {
	return tool.BuildBuiltinDefinition(t, tool.ToolDefinition{
		Name:        ToolName,
		DisplayName: "Configure ACP Runner",
		Description: t.Description(),
		Purpose:     "Allow AI to manage the local ACP runner catalog in the user's Awaken home directory, including adding built-in or custom external agents and disabling/removing them.",
		WhenToUse: []string{
			"Use when the user asks to add, change, enable, disable or remove an ACP external agent runner.",
			"Use when a runner command path or timeout needs adjustment.",
		},
		WhenNotToUse: []string{
			"Do not use to read or invoke an ACP session; use delegate_external_agent for that.",
			"Do not use to edit workspace-level or project-level runner configs; ACP runners are user-scoped only.",
		},
		InputSchema: t.InputSchema(),
		OutputSchema: tool.ToolParam{Type: "object", AdditionalProperties: tool.BoolPtr(false), Properties: map[string]tool.ToolParam{
			"runner_id":      {Type: "string", Description: "The affected runner identifier."},
			"action":         {Type: "string", Description: "The action that was performed."},
			"result_status":  {Type: "string", Description: "configured or deleted."},
			"message":        {Type: "string", Description: "Human-readable result."},
		}},
		SideEffects: []tool.ToolSideEffect{
			{Kind: tool.SideEffectCreate, Resources: []string{"~/.awaken/config/acp-agents.json"}, Description: "Creates a new runner entry in the user-scoped ACP config.", Reversibility: tool.ReversibilityReversible},
			{Kind: tool.SideEffectUpdate, Resources: []string{"~/.awaken/config/acp-agents.json"}, Description: "Updates an existing runner entry in the user-scoped ACP config.", Reversibility: tool.ReversibilityReversible},
			{Kind: tool.SideEffectDelete, Resources: []string{"~/.awaken/config/acp-agents.json"}, Description: "Removes a runner entry from the user-scoped ACP config.", Reversibility: tool.ReversibilityReversible},
		},
		Examples: []tool.ToolExample{
			{Scenario: "Add or enable Claude Code", Input: map[string]any{"action": "upsert", "runner_id": "claude_code", "command": "claude", "enabled": true}, Outcome: "Claude Code is configured in the user ACP config."},
			{Scenario: "Disable a runner", Input: map[string]any{"action": "update", "runner_id": "codex", "enabled": false}, Outcome: "The runner is disabled but its configuration remains."},
			{Scenario: "Remove a runner", Input: map[string]any{"action": "delete", "runner_id": "opencode"}, Outcome: "The runner entry is removed from the user ACP config."},
		},
		FailureModes: []tool.ToolFailureMode{
			{Code: "acp_runner_configure_invalid_input", Cause: "Arguments are invalid or action is unsupported.", Retryable: false, Recovery: []string{"Correct action and runner_id."}},
			{Code: "acp_runner_configure_already_exists", Cause: "create was requested for an existing runner_id.", Retryable: false, Recovery: []string{"Use update/upsert, or delete and recreate."}},
			{Code: "acp_runner_configure_not_found", Cause: "update/delete was requested for a runner that does not exist.", Retryable: false, Recovery: []string{"Use upsert to create it, or check the runner_id."}},
			{Code: "acp_runner_configure_registry_unavailable", Cause: "The ACP registry is not initialized.", Retryable: true, Recovery: []string{"Retry when the ACP runtime is available."}},
			{Code: "acp_runner_configure_failed", Cause: "The configuration could not be persisted.", Retryable: true, Recovery: []string{"Check ~/.awaken/config permissions and retry."}},
		},
		Annotations: tool.ToolAnnotations{
			ReadOnly:        false,
			Destructive:     false,
			OpenWorld:       false,
			CapabilityClass: tool.CapabilitySystemOperation,
			TrustedOrigin:   tool.TrustedOriginBuiltin,
			FileBridgeScope: tool.FileBridgeNone,
			NetworkScope:    tool.NetworkScopeNone,
		},
		Capabilities: []tool.Capability{tool.CapabilityFileWrite},
	})
}

func init() {
	tool.MustRegisterToolWithMode(defaultTool, tool.DefinitionValidationStrictBuiltin)
}

func (t *ConfigureACPRunnerTool) Execute(ctx context.Context, input string) (string, error) {
	out, err := t.configure(ctx, input)
	if err != nil {
		return "", err
	}
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (t *ConfigureACPRunnerTool) ExecuteOutcome(ctx context.Context, input string, _ tool.ToolObserver) (tool.ExecutionOutcome, error) {
	started := time.Now().UTC()
	out, err := t.configure(ctx, input)
	if err != nil {
		return configureFailureOutcome(err).WithTiming(started, time.Now().UTC()), nil
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return configureFailureOutcome(&configureFailure{
			code: "acp_runner_configure_output_invalid", category: tool.ErrorCategoryTool,
			retryable: false, message: fmt.Sprintf("encode output: %v", err),
		}).WithTiming(started, time.Now().UTC()), nil
	}
	outcome := tool.SucceededOutcome(string(data))
	outcome.Summary = out.Message
	outcome.Operation = &tool.OperationDescriptor{
		Type: ToolName, Target: out.RunnerID, SideEffectState: tool.OutcomeSideEffectApplied,
	}
	outcome.Details = map[string]any{"runner_id": out.RunnerID, "action": out.Action, "result_status": out.ResultStatus}
	changeAction := "updated"
	changeSummary := "Updated an ACP runner entry in the user-scoped config."
	switch out.Action {
	case "create":
		changeAction = "created"
		changeSummary = "Created a new ACP runner entry in the user-scoped config."
	case "delete":
		changeAction = "deleted"
		changeSummary = "Removed an ACP runner entry from the user-scoped config."
	}
	outcome.Changes = []tool.ResourceChange{{
		Kind: "acp_runner_config", Action: changeAction, Resource: out.RunnerID,
		Summary: changeSummary, Certainty: tool.ChangeObserved, Reversibility: tool.ChangeReversible,
		Details: map[string]any{"runner_id": out.RunnerID, "action": out.Action},
	}}
	return outcome.WithTiming(started, time.Now().UTC()), nil
}

func (t *ConfigureACPRunnerTool) CheckPermission(_ context.Context, _ string, _ tool.PermissionContext) tool.PermissionDecision {
	// This tool only writes to the user-scoped ACP config (~/.awaken/config/acp-agents.json)
	// through the registry, never through the task-scope file bridge.
	return tool.AllowPermission()
}

func (t *ConfigureACPRunnerTool) configure(_ context.Context, input string) (configureOutput, error) {
	if t.registry == nil {
		return configureOutput{}, &configureFailure{
			code: "acp_runner_configure_registry_unavailable", category: tool.ErrorCategoryRuntime,
			retryable: true, recovery: []string{"retry_when_acp_runtime_available"},
			message: "ACP registry is not available",
		}
	}
	req, err := parseConfigureInput(input)
	if err != nil {
		return configureOutput{}, &configureFailure{
			code: "acp_runner_configure_invalid_input", category: tool.ErrorCategoryValidation,
			retryable: false, recovery: []string{"fix_acp_runner_configure_arguments"},
			message: err.Error(),
		}
	}
	action := strings.ToLower(strings.TrimSpace(req.Action))
	runnerID := strings.TrimSpace(req.RunnerID)
	if runnerID == "" {
		return configureOutput{}, &configureFailure{
			code: "acp_runner_configure_invalid_input", category: tool.ErrorCategoryValidation,
			retryable: false, recovery: []string{"provide_a_runner_id"},
			message: "runner_id is required",
		}
	}

	existing, exists := t.registry.Get(runnerID)

	switch action {
	case "delete":
		if !exists {
			return configureOutput{}, &configureFailure{
				code: "acp_runner_configure_not_found", category: tool.ErrorCategoryDomain,
				retryable: false, recovery: []string{"check_runner_id", "use_upsert_to_create"},
				message: fmt.Sprintf("runner %q does not exist", runnerID),
			}
		}
		if err := t.registry.DeleteUser(runnerID); err != nil {
			return configureOutput{}, configurePersistenceFailure(err)
		}
		return configureOutput{RunnerID: runnerID, Action: "delete", ResultStatus: "deleted", Message: fmt.Sprintf("已删除 ACP runner %s", runnerID)}, nil

	case "create":
		if exists {
			return configureOutput{}, &configureFailure{
				code: "acp_runner_configure_already_exists", category: tool.ErrorCategoryDomain,
				retryable: false, recovery: []string{"use_update_or_upsert", "delete_then_recreate"},
				message: fmt.Sprintf("runner %q already exists; use update or upsert", runnerID),
			}
		}
		cfg, err := runnerConfigFromCreate(req)
		if err != nil {
			return configureOutput{}, &configureFailure{
				code: "acp_runner_configure_invalid_input", category: tool.ErrorCategoryValidation,
				retryable: false, recovery: []string{"provide_required_runner_fields"},
				message: err.Error(),
			}
		}
		if err := t.registry.UpsertUser(cfg); err != nil {
			return configureOutput{}, configurePersistenceFailure(err)
		}
		return configureOutput{RunnerID: runnerID, Action: "create", ResultStatus: "configured", Message: fmt.Sprintf("已创建 ACP runner %s", runnerID)}, nil

	case "update":
		if !exists {
			return configureOutput{}, &configureFailure{
				code: "acp_runner_configure_not_found", category: tool.ErrorCategoryDomain,
				retryable: false, recovery: []string{"check_runner_id", "use_upsert_to_create"},
				message: fmt.Sprintf("runner %q does not exist; use upsert to create it", runnerID),
			}
		}
		cfg := applyUpdates(existing, req)
		if err := t.registry.UpsertUser(cfg); err != nil {
			return configureOutput{}, configurePersistenceFailure(err)
		}
		return configureOutput{RunnerID: runnerID, Action: "update", ResultStatus: "configured", Message: fmt.Sprintf("已更新 ACP runner %s", runnerID)}, nil

	case "upsert":
		var cfg acp.RunnerConfig
		if exists {
			cfg = applyUpdates(existing, req)
		} else {
			cfg, err = runnerConfigFromCreate(req)
			if err != nil {
				return configureOutput{}, &configureFailure{
					code: "acp_runner_configure_invalid_input", category: tool.ErrorCategoryValidation,
					retryable: false, recovery: []string{"provide_required_runner_fields"},
					message: err.Error(),
				}
			}
		}
		if err := t.registry.UpsertUser(cfg); err != nil {
			return configureOutput{}, configurePersistenceFailure(err)
		}
		actionLabel := "update"
		verb := "已更新"
		if !exists {
			actionLabel = "create"
			verb = "已创建"
		}
		return configureOutput{RunnerID: runnerID, Action: actionLabel, ResultStatus: "configured", Message: fmt.Sprintf("%s ACP runner %s", verb, runnerID)}, nil

	default:
		return configureOutput{}, &configureFailure{
			code: "acp_runner_configure_invalid_input", category: tool.ErrorCategoryValidation,
			retryable: false, recovery: []string{"choose_create_update_upsert_or_delete"},
			message: fmt.Sprintf("unsupported action %q", req.Action),
		}
	}
}

func parseConfigureInput(input string) (configureInput, error) {
	var req configureInput
	if err := json.Unmarshal([]byte(strings.TrimSpace(input)), &req); err != nil {
		return req, fmt.Errorf("arguments must be a JSON object: %w", err)
	}
	return req, nil
}

func runnerConfigFromCreate(req configureInput) (acp.RunnerConfig, error) {
	command := strings.TrimSpace(req.Command)
	if command == "" {
		return acp.RunnerConfig{}, fmt.Errorf("command is required when creating a runner")
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	mode := acp.PermissionModeAsk
	if strings.TrimSpace(req.PermissionMode) != "" {
		mode = acp.PermissionMode(strings.ToLower(strings.TrimSpace(req.PermissionMode)))
	}
	cfg := acp.RunnerConfig{
		ID:                    req.RunnerID,
		Enabled:               enabled,
		Command:               command,
		Args:                  append([]string(nil), req.Args...),
		Env:                   cloneStringMap(req.Env),
		PermissionMode:        mode,
		StartupTimeoutSeconds: req.StartupTimeoutSeconds,
		TurnTimeoutSeconds:    req.TurnTimeoutSeconds,
		MaxFrameBytes:         req.MaxFrameBytes,
	}
	if err := cfg.Normalize(); err != nil {
		return acp.RunnerConfig{}, err
	}
	return cfg, nil
}

func applyUpdates(existing acp.RunnerConfig, req configureInput) acp.RunnerConfig {
	cfg := existing
	if req.Enabled != nil {
		cfg.Enabled = *req.Enabled
	}
	if strings.TrimSpace(req.Command) != "" {
		cfg.Command = strings.TrimSpace(req.Command)
	}
	if req.Args != nil {
		cfg.Args = append([]string(nil), req.Args...)
	}
	if req.Env != nil {
		cfg.Env = cloneStringMap(req.Env)
	}
	if strings.TrimSpace(req.PermissionMode) != "" {
		cfg.PermissionMode = acp.PermissionMode(strings.ToLower(strings.TrimSpace(req.PermissionMode)))
	}
	if req.StartupTimeoutSeconds > 0 {
		cfg.StartupTimeoutSeconds = req.StartupTimeoutSeconds
	}
	if req.TurnTimeoutSeconds > 0 {
		cfg.TurnTimeoutSeconds = req.TurnTimeoutSeconds
	}
	if req.MaxFrameBytes > 0 {
		cfg.MaxFrameBytes = req.MaxFrameBytes
	}
	// Re-apply defaults for untouched fields in case the existing record predates them.
	_ = cfg.Normalize()
	return cfg
}

func cloneStringMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func configurePersistenceFailure(err error) error {
	return &configureFailure{
		code: "acp_runner_configure_failed", category: tool.ErrorCategoryPersistence,
		retryable: true, recovery: []string{"check_awaken_home_permissions", "retry"},
		message: fmt.Sprintf("failed to persist ACP runner config: %v", err),
	}
}

func configureFailureOutcome(err error) tool.ExecutionOutcome {
	var failure *configureFailure
	if !errors.As(err, &failure) || failure == nil {
		failure = &configureFailure{
			code: "acp_runner_configure_failed", category: tool.ErrorCategoryTool,
			retryable: false, message: err.Error(),
		}
	}
	outcome := tool.FailedOutcome(failure.message, failure.code, failure.category, failure.retryable)
	outcome.RecoveryOptions = append([]string(nil), failure.recovery...)
	outcome.Operation = &tool.OperationDescriptor{Type: ToolName, SideEffectState: tool.OutcomeSideEffectUnknown}
	switch failure.code {
	case "acp_runner_configure_registry_unavailable":
		outcome.Status = tool.OutcomeNeedsRuntime
		outcome.TransportStatus = tool.TransportNotStarted
		outcome.Operation.SideEffectState = tool.OutcomeSideEffectNotStarted
	case "acp_runner_configure_invalid_input", "acp_runner_configure_already_exists", "acp_runner_configure_not_found":
		outcome.TransportStatus = tool.TransportNotStarted
		outcome.Operation.SideEffectState = tool.OutcomeSideEffectNotStarted
	}
	return outcome
}
