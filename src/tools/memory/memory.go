package memorytool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"awaken/src/memory"
	"awaken/src/tool"
)

const ToolName = "memory"

type Tool struct{}

type input struct {
	Action           string        `json:"action"`
	Scope            memory.Scope  `json:"scope,omitempty"`
	Types            []memory.Type `json:"types,omitempty"`
	Type             memory.Type   `json:"type,omitempty"`
	Query            string        `json:"query,omitempty"`
	Limit            int           `json:"limit,omitempty"`
	MemoryID         string        `json:"memory_id,omitempty"`
	ExpectedRevision int           `json:"expected_revision,omitempty"`
	Name             string        `json:"name,omitempty"`
	Description      string        `json:"description,omitempty"`
	Content          string        `json:"content,omitempty"`
	Keywords         []string      `json:"keywords,omitempty"`
	Patch            memory.Patch  `json:"patch,omitempty"`
	Reason           string        `json:"reason,omitempty"`
}

func (t *Tool) Name() string { return ToolName }
func (t *Tool) Description() string {
	return "Search, read, create, update, supersede, or delete durable user/project memories stored as controlled Markdown. You must call this tool for confirmed future-facing naming or collaboration instructions even when the user does not literally say remember; a text acknowledgement alone does not persist them. Never use it for temporary task state, secrets, raw transcripts, or facts directly available from code and project documentation."
}
func (t *Tool) Parameters() []tool.ToolParameter { return nil }
func (t *Tool) InputSchema() tool.ToolParametersSchema {
	return tool.ToolParametersSchema{
		Type: "object", Required: []string{"action"}, AdditionalProperties: tool.BoolPtr(false),
		Properties: map[string]tool.ToolParam{
			"action": {Type: "string", Enum: []any{"search", "get", "create", "update", "supersede", "delete"}, Description: "Memory operation."},
			"scope":  {Type: "string", Enum: []any{"user", "project"}, Description: "Required for create; optional search filter."},
			"types":  {Type: "array", Description: "Optional memory-type filters for search.", Items: &tool.ToolParam{Type: "string", Description: "One memory type.", Enum: []any{"user", "feedback", "project", "reference"}}},
			"type":   {Type: "string", Enum: []any{"user", "feedback", "project", "reference"}, Description: "Required for create."},
			"query":  {Type: "string", Description: "Text query for search."}, "limit": {Type: "integer", Description: "Maximum search results, from 1 through 20.", Minimum: floatPtr(1), Maximum: floatPtr(20)},
			"memory_id": {Type: "string", Description: "Stable identity for get or mutation actions."}, "expected_revision": {Type: "integer", Description: "Last observed revision for optimistic concurrency.", Minimum: floatPtr(1)},
			"name": {Type: "string", Description: "Short topic name for create."}, "description": {Type: "string", Description: "Short, concrete recall description for create."}, "content": {Type: "string", Description: "Markdown topic body for create."},
			"keywords": {Type: "array", Description: "Optional search terms for create.", Items: &tool.ToolParam{Type: "string", Description: "One search keyword."}},
			"patch": {Type: "object", Description: "Fields to replace during update; omitted fields remain unchanged.", AdditionalProperties: tool.BoolPtr(false), Properties: map[string]tool.ToolParam{
				"name": {Type: "string", Description: "Replacement topic name."}, "description": {Type: "string", Description: "Replacement recall description."}, "content": {Type: "string", Description: "Replacement Markdown body."}, "keywords": {Type: "array", Description: "Replacement keywords.", Items: &tool.ToolParam{Type: "string", Description: "One search keyword."}},
			}},
			"reason": {Type: "string", Description: "Required for update, supersede, and delete."},
		},
	}
}

func (t *Tool) Definition() tool.ToolDefinition {
	return tool.BuildBuiltinDefinition(t, tool.ToolDefinition{
		Name: ToolName, DisplayName: "Memory", Description: t.Description(),
		Purpose:      "Maintain bounded cross-thread memory through the host-owned Markdown service.",
		WhenToUse:    []string{"The user explicitly asks to remember or forget something, or gives a clear future-facing instruction", "A stable name, form of address, collaboration preference, correction, project rationale, or external reference will materially help future threads", "Searching or reading an existing durable memory"},
		WhenNotToUse: []string{"Temporary plans, progress, errors, or one-thread continuation", "Secrets, credentials, raw conversation, system prompts, or full external documents", "Facts directly derivable from current code, Git, runtime state, or authoritative project docs"},
		SideEffects:  []tool.ToolSideEffect{{Kind: tool.SideEffectRead, Resources: []string{"user/project Memory Markdown"}, Description: "search/get read controlled Memory.", Reversibility: tool.ReversibilityNotApplicable}, {Kind: tool.SideEffectUpdate, Resources: []string{"user/project Memory Markdown and MEMORY.md"}, Description: "mutation actions atomically update controlled Memory.", Reversibility: tool.ReversibilityConditional}},
		Examples: []tool.ToolExample{
			{Scenario: "Persist a future-facing form of address without waiting for the literal word remember", Input: map[string]any{"action": "create", "scope": "user", "type": "feedback", "name": "Names and forms of address", "description": "Call the user 博士哥 and refer to the assistant as 小田", "content": "Address the user as 博士哥. The user calls the assistant 小田.\n\n**How to apply:** Use these names in future conversations."}, Outcome: "Creates or, if a matching topic exists, updates one user preference topic."},
			{Scenario: "Remember a confirmed testing preference", Input: map[string]any{"action": "create", "scope": "user", "type": "feedback", "name": "Integration test preference", "description": "Prefer real database integration tests", "content": "Use a real database for integration tests when practical."}, Outcome: "Creates one Markdown topic and rebuilds MEMORY.md."},
		},
		FailureModes: []tool.ToolFailureMode{{Code: "memory_invalid_input", Cause: "Action-specific parameters are missing", Retryable: true}, {Code: "memory_revision_conflict", Cause: "The record changed since it was read", Retryable: true}, {Code: "memory_sensitive_content", Cause: "Content contains secrets or forbidden transcript/prompt material", Retryable: false}, {Code: "memory_duplicate", Cause: "An identical memory already exists", Retryable: true}},
		Annotations:  tool.ToolAnnotations{ReadOnly: false, Destructive: true, OpenWorld: false, CapabilityClass: tool.CapabilityArtifactWrite, TrustedOrigin: tool.TrustedOriginBuiltin, FileBridgeScope: tool.FileBridgeNone, NetworkScope: tool.NetworkScopeNone},
	})
}

func (t *Tool) Execute(ctx context.Context, raw string) (string, error) {
	var p input
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return "", fmt.Errorf("memory_invalid_input: %w", err)
	}
	permission := tool.PermissionContextFromContext(ctx)
	store, err := memory.NewStore(permission.WorkspaceRootOrCWD())
	if err != nil {
		return "", err
	}
	if settingsStore, settingsErr := memory.NewSettingsStore(); settingsErr == nil {
		if settings, loadErr := settingsStore.Get(); loadErr != nil {
			return "", loadErr
		} else if !settings.Value.Enabled {
			return "", fmt.Errorf("memory_disabled: Memory is disabled in Settings")
		}
	}
	identity, ok := tool.ToolExecutionIdentityFromContext(ctx)
	threadID := tool.ToolThreadIDFromContext(ctx)
	if !ok || threadID == "" {
		return "", fmt.Errorf("memory_host_identity_missing")
	}
	source := memory.Source{ThreadID: threadID, RecordID: identity.ToolCallID, CapturedBy: "main_agent"}
	var output any
	switch strings.ToLower(strings.TrimSpace(p.Action)) {
	case "search":
		var scope *memory.Scope
		if p.Scope != "" {
			scope = &p.Scope
		}
		output, err = store.Search(p.Query, scope, p.Types, p.Limit)
	case "get":
		if p.MemoryID == "" {
			return "", fmt.Errorf("memory_invalid_input: memory_id is required")
		}
		output, err = store.Get(p.MemoryID)
	case "create":
		record, createErr := store.Create(memory.CreateInput{Scope: p.Scope, Type: p.Type, Name: p.Name, Description: p.Description, Content: p.Content, Keywords: p.Keywords, Source: source})
		record.Action, output, err = "create", record, createErr
	case "update":
		if p.MemoryID == "" || p.ExpectedRevision < 1 || strings.TrimSpace(p.Reason) == "" {
			return "", fmt.Errorf("memory_invalid_input: memory_id, expected_revision and reason are required")
		}
		record, updateErr := store.Update(p.MemoryID, p.ExpectedRevision, p.Patch, source)
		record.Action, output, err = "update", record, updateErr
	case "supersede":
		if p.MemoryID == "" || p.ExpectedRevision < 1 || strings.TrimSpace(p.Reason) == "" {
			return "", fmt.Errorf("memory_invalid_input: memory_id, expected_revision and reason are required")
		}
		record, supersedeErr := store.Supersede(p.MemoryID, p.ExpectedRevision, source)
		record.Action, output, err = "supersede", record, supersedeErr
	case "delete":
		if p.MemoryID == "" || p.ExpectedRevision < 1 || strings.TrimSpace(p.Reason) == "" {
			return "", fmt.Errorf("memory_invalid_input: memory_id, expected_revision and reason are required")
		}
		err = store.Delete(p.MemoryID, p.ExpectedRevision)
		output = map[string]any{"action": "delete", "memory_id": p.MemoryID, "revision": p.ExpectedRevision}
	default:
		return "", fmt.Errorf("memory_invalid_input: unsupported action %q", p.Action)
	}
	if err != nil {
		return "", classify(err)
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func (t *Tool) ExecuteOutcome(ctx context.Context, raw string, _ tool.ToolObserver) (tool.ExecutionOutcome, error) {
	started := time.Now().UTC()
	content, err := t.Execute(ctx, raw)
	if err == nil {
		var p input
		_ = json.Unmarshal([]byte(raw), &p)
		action := strings.ToLower(strings.TrimSpace(p.Action))
		outcome := tool.SucceededOutcome(content)
		if isMutationAction(action) {
			outcome.Content = mutationCompletionPayload(action, content)
			outcome.Summary = fmt.Sprintf("Memory %s completed successfully. The durable change is already saved. Stop additional reasoning, do not repeat the mutation, and return one concise final confirmation.", action)
			outcome.NextActions = []tool.NextAction{{
				ID:          "finish_after_memory_write",
				Description: "Stop additional reasoning. Do not call memory again for the same fact. Return exactly one concise final confirmation, then finish if no other work remains.",
			}}
		}
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}
	message := err.Error()
	code, category, retryable := "memory_execution_failed", tool.ErrorCategoryTool, true
	switch {
	case strings.Contains(message, "memory_invalid_input") || strings.Contains(message, "host_identity") || strings.Contains(message, "host source identity"):
		code, category, retryable = "memory_invalid_input", tool.ErrorCategoryValidation, false
	case strings.Contains(message, "memory_revision_conflict"):
		code, category, retryable = "memory_revision_conflict", tool.ErrorCategoryDomain, true
	case strings.Contains(message, "memory_sensitive_content"):
		code, category, retryable = "memory_sensitive_content", tool.ErrorCategoryPolicy, false
	case strings.Contains(message, "memory_duplicate"):
		code, category, retryable = "memory_duplicate", tool.ErrorCategoryDomain, true
	case errors.Is(err, memory.ErrNotFound):
		code, category, retryable = "memory_not_found", tool.ErrorCategoryDomain, false
	default:
		outcome := tool.FailedOutcome(message, code, category, retryable).WithTiming(started, time.Now().UTC())
		return outcome, err
	}
	return tool.FailedOutcome(message, code, category, retryable).WithTiming(started, time.Now().UTC()), nil
}

func isMutationAction(action string) bool {
	switch action {
	case "create", "update", "supersede", "delete":
		return true
	default:
		return false
	}
}

func mutationCompletionPayload(action, raw string) string {
	var source map[string]any
	_ = json.Unmarshal([]byte(raw), &source)
	receipt := map[string]any{
		"status":          "completed",
		"action":          action,
		"write_completed": true,
		"message":         "The durable Memory mutation completed successfully.",
		"next_step":       "Stop additional reasoning. Do not repeat this mutation. Return exactly one concise final confirmation now.",
	}
	for _, key := range []string{"memory_id", "scope", "type", "revision", "updated_at"} {
		if value, ok := source[key]; ok {
			receipt[key] = value
		}
	}
	encoded, _ := json.Marshal(receipt)
	return string(encoded)
}

// SemanticJournalArguments removes Memory bodies from durable tool-call
// replay while retaining the operation identity and concurrency fields.
func (t *Tool) SemanticJournalArguments(raw string) string {
	var p input
	if json.Unmarshal([]byte(raw), &p) != nil {
		return `{}`
	}
	receipt := map[string]any{"action": p.Action, "scope": p.Scope, "type": p.Type, "memory_id": p.MemoryID, "expected_revision": p.ExpectedRevision}
	encoded, _ := json.Marshal(receipt)
	return string(encoded)
}

// SemanticJournalResult returns a body-free receipt. Live model output remains
// available for the current run, but thread persistence cannot resurrect it.
func (t *Tool) SemanticJournalResult(raw string) string {
	var value any
	if json.Unmarshal([]byte(raw), &value) != nil {
		return `{"status":"completed"}`
	}
	return bodyFreeJSON(value)
}

func bodyFreeJSON(value any) string {
	allowed := map[string]bool{"memory_id": true, "scope": true, "type": true, "status": true, "revision": true, "updated_at": true, "action": true}
	var clean func(any) any
	clean = func(item any) any {
		switch typed := item.(type) {
		case []any:
			result := make([]any, 0, len(typed))
			for _, child := range typed {
				result = append(result, clean(child))
			}
			return result
		case map[string]any:
			result := map[string]any{}
			for key, child := range typed {
				if allowed[key] {
					result[key] = clean(child)
				}
			}
			return result
		default:
			return typed
		}
	}
	encoded, _ := json.Marshal(clean(value))
	return string(encoded)
}

func classify(err error) error {
	switch {
	case errors.Is(err, memory.ErrRevisionConflict):
		return fmt.Errorf("memory_revision_conflict: %w", err)
	case errors.Is(err, memory.ErrSensitive):
		return fmt.Errorf("memory_sensitive_content: %w", err)
	case errors.Is(err, memory.ErrDuplicate):
		return fmt.Errorf("memory_duplicate: %w", err)
	default:
		return err
	}
}
func floatPtr(value float64) *float64 { return &value }

func init() { tool.MustRegisterToolWithMode(&Tool{}, tool.DefinitionValidationStrictBuiltin) }
