package skillruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"awaken/src/skills"
	"awaken/src/tool"
)

const ResourceListToolName = "skill_resource_list"

type ResourceListTool struct {
	registry *skills.SkillRegistry
}

type resourceListInput struct {
	SkillID string `json:"skill_id"`
	Prefix  string `json:"prefix,omitempty"`
}

type resourceListEntry struct {
	Path      string `json:"path"`
	Kind      string `json:"kind"`
	SizeBytes int64  `json:"size_bytes"`
}

type resourceListResult struct {
	SkillID            string              `json:"skill_id"`
	Prefix             string              `json:"prefix"`
	Entries            []resourceListEntry `json:"entries"`
	InventoryTruncated bool                `json:"inventory_truncated"`
}

func (t *ResourceListTool) Name() string { return ResourceListToolName }

func (t *ResourceListTool) Description() string {
	return "List the bounded package-relative file tree of an installed local Skill before reading a supporting resource."
}

func (t *ResourceListTool) Parameters() []tool.ToolParameter {
	return []tool.ToolParameter{
		{Name: "skill_id", Type: "string", Description: "Installed Skill identifier.", Required: true},
		{Name: "prefix", Type: "string", Description: "Optional package-relative directory or path prefix.", Required: false},
	}
}

func (t *ResourceListTool) Definition() tool.ToolDefinition {
	noExtra := tool.BoolPtr(false)
	entry := tool.ToolParam{
		Type: "object", AdditionalProperties: noExtra,
		Properties: map[string]tool.ToolParam{
			"path":       {Type: "string", Description: "Package-relative path."},
			"kind":       {Type: "string", Description: "file, directory, symlink, or other.", Enum: []any{"file", "directory", "symlink", "other"}},
			"size_bytes": {Type: "integer", Description: "Regular-file size, otherwise zero."},
		},
		Required: []string{"path", "kind", "size_bytes"},
	}
	return tool.ToolDefinition{
		SchemaVersion: tool.ToolDefinitionSchemaVersion,
		Name:          ResourceListToolName,
		DisplayName:   "Skill Resource List",
		Source:        tool.SourceSkill,
		Description:   t.Description(),
		Purpose:       "Discover the real files and directories inside one installed local Skill package without reading contents or exposing arbitrary host paths.",
		WhenToUse: []string{
			"Use after a Skill is loaded and before reading references, scripts, assets, templates, or other package resources whose exact paths are not yet known.",
			"Use to verify that an installed or plugin-provided Skill retained its complete package tree.",
		},
		WhenNotToUse: []string{
			"Do not use for workspace directories, MCP dynamic Skills without a local package, or file contents; use the matching file tool or skill_resource_read.",
		},
		InputSchema: tool.ToolParametersSchema{
			Type: "object", AdditionalProperties: noExtra,
			Properties: map[string]tool.ToolParam{
				"skill_id": {Type: "string", Description: "Installed Skill identifier."},
				"prefix":   {Type: "string", Description: "Optional package-relative directory or path prefix."},
			},
			Required: []string{"skill_id"},
		},
		OutputSchema: tool.ToolParam{
			Type: "object", AdditionalProperties: noExtra,
			Properties: map[string]tool.ToolParam{
				"skill_id":            {Type: "string"},
				"prefix":              {Type: "string"},
				"entries":             {Type: "array", Items: &entry},
				"inventory_truncated": {Type: "boolean"},
			},
			Required: []string{"skill_id", "prefix", "entries", "inventory_truncated"},
		},
		SideEffects: []tool.ToolSideEffect{{
			Kind: tool.SideEffectRead, Resources: []string{"installed Skill package tree"},
			Description: "Lists bounded package-relative metadata without reading resource contents.", Reversibility: tool.ReversibilityNotApplicable,
		}},
		Permissions: tool.ToolPermissionSpec{
			CapabilityClass: tool.CapabilityArtifactRead, WorkspaceScope: tool.WorkspaceScopeRead,
			FileBridgeScope: tool.FileBridgeTaskScopeRead, NetworkScope: tool.NetworkScopeNone,
		},
		Examples: []tool.ToolExample{{
			Scenario: "Discover a Skill's supporting references",
			Input:    map[string]any{"skill_id": "report-writer", "prefix": "references"},
			Outcome:  "Returns only package-relative resource metadata below references/.",
		}},
		FailureModes: []tool.ToolFailureMode{
			{Code: "skill_resource_list_cancelled", Cause: "The list context was cancelled.", Retryable: true, Recovery: []string{"Retry when the operation context is available."}},
			{Code: "skill_resource_list_invalid_input", Cause: "Arguments, skill_id, or prefix are invalid.", Retryable: false, Recovery: []string{"Correct skill_id and use a package-relative prefix."}},
			{Code: "skill_resource_list_runtime_unavailable", Cause: "The live Skill registry is unavailable.", Retryable: true, Recovery: []string{"Restore or reload the Skill runtime."}},
			{Code: "skill_resource_list_not_found", Cause: "The Skill has no available local package.", Retryable: false, Recovery: []string{"Refresh installed Skills and choose a local package."}},
			{Code: "skill_resource_list_path_denied", Cause: "The Skill root or prefix violates the package boundary.", Retryable: false, Recovery: []string{"Use a package-relative prefix and an authorized Skill."}},
			{Code: "skill_resource_list_failed", Cause: "The bounded package inventory could not be read.", Retryable: true, Recovery: []string{"Refresh the Skill registry and retry once."}},
		},
		Annotations: tool.ToolAnnotations{
			ReadOnly: true, Destructive: false, OpenWorld: false,
			CapabilityClass: tool.CapabilityArtifactRead, TrustedOrigin: tool.TrustedOriginBuiltin,
			FileBridgeScope: tool.FileBridgeTaskScopeRead, NetworkScope: tool.NetworkScopeNone,
		},
	}
}

func (t *ResourceListTool) IsReadOnly() bool { return true }

func (t *ResourceListTool) Execute(ctx context.Context, input string) (string, error) {
	outcome, err := t.ExecuteOutcome(ctx, input, nil)
	if err != nil {
		return outcome.Content, err
	}
	if outcome.IsError() {
		return "", tool.ToolError{
			Code: outcome.ErrorCode, Category: outcome.ErrorCategory, Message: outcome.ErrorMessage,
			Retryable: outcome.Retryable, RecoveryOptions: outcome.RecoveryOptions, Diagnostic: outcome.Diagnostic,
		}
	}
	return outcome.Content, nil
}

func (t *ResourceListTool) ExecuteOutcome(ctx context.Context, input string, _ tool.ToolObserver) (tool.ExecutionOutcome, error) {
	started := time.Now().UTC()
	if err := ctx.Err(); err != nil {
		outcome := resourceListFailure("skill_resource_list_cancelled", tool.ErrorCategoryCancelled, err.Error(), true, []string{"retry_when_context_available"})
		outcome.Status = tool.OutcomeCancelled
		outcome.TransportStatus = tool.TransportCancelled
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}
	if t == nil || t.registry == nil {
		return resourceListFailure("skill_resource_list_runtime_unavailable", tool.ErrorCategoryRuntime, "skill registry unavailable", true, []string{"restore_skill_runtime"}).WithTiming(started, time.Now().UTC()), nil
	}

	var req resourceListInput
	if err := json.Unmarshal([]byte(strings.TrimSpace(input)), &req); err != nil {
		return resourceListFailure("skill_resource_list_invalid_input", tool.ErrorCategoryValidation, fmt.Sprintf("arguments must be a JSON object: %v", err), false, []string{"fix_skill_resource_list_arguments"}).WithTiming(started, time.Now().UTC()), nil
	}
	req.SkillID = strings.TrimSpace(req.SkillID)
	if req.SkillID == "" {
		return resourceListFailure("skill_resource_list_invalid_input", tool.ErrorCategoryValidation, "skill_id is required", false, []string{"provide_skill_id"}).WithTiming(started, time.Now().UTC()), nil
	}
	prefix, err := normalizeResourcePrefix(req.Prefix)
	if err != nil {
		return resourceListFailure("skill_resource_list_invalid_input", tool.ErrorCategoryValidation, err.Error(), false, []string{"use_package_relative_prefix"}).WithTiming(started, time.Now().UTC()), nil
	}
	def := t.registry.Get(req.SkillID)
	if def == nil || strings.TrimSpace(def.RootDir) == "" {
		return resourceListFailure("skill_resource_list_not_found", tool.ErrorCategoryDomain, fmt.Sprintf("skill %q has no local package", req.SkillID), false, []string{"refresh_installed_skills", "choose_local_skill"}).WithTiming(started, time.Now().UTC()), nil
	}
	if def.Source == skills.SourceProject && tool.HasPermissionContext(ctx) {
		projectRoot := strings.TrimSpace(def.ProjectPath)
		if projectRoot == "" || !tool.PermissionContextFromContext(ctx).IsWithinWorkspaceResolved(projectRoot) {
			return resourceListFailure("skill_resource_list_path_denied", tool.ErrorCategoryPolicy, "project Skill root is outside the active workspace", false, []string{"choose_skill_in_active_workspace"}).WithTiming(started, time.Now().UTC()), nil
		}
	}

	inventory, err := skills.ListSkillPackageResourcesAt(def, prefix)
	if err != nil {
		code := "skill_resource_list_failed"
		category := tool.ErrorCategoryRuntime
		if strings.Contains(err.Error(), "denied") || strings.Contains(err.Error(), "escape") || strings.Contains(err.Error(), "changed") {
			code, category = "skill_resource_list_path_denied", tool.ErrorCategoryPolicy
		}
		return resourceListFailure(code, category, err.Error(), false, []string{"refresh_installed_skills"}).WithTiming(started, time.Now().UTC()), nil
	}
	entries := make([]resourceListEntry, 0, len(inventory.Entries))
	for _, entry := range inventory.Entries {
		entries = append(entries, resourceListEntry{Path: entry.Path, Kind: entry.Kind, SizeBytes: entry.SizeBytes})
	}
	result := resourceListResult{SkillID: req.SkillID, Prefix: prefix, Entries: entries, InventoryTruncated: inventory.Truncated}
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return resourceListFailure("skill_resource_list_failed", tool.ErrorCategoryRuntime, err.Error(), true, []string{"retry_once"}).WithTiming(started, time.Now().UTC()), nil
	}
	outcome := tool.SucceededOutcome(string(encoded))
	outcome.Summary = fmt.Sprintf("Listed %d entries from Skill %s", len(entries), req.SkillID)
	outcome.Operation = &tool.OperationDescriptor{Type: ResourceListToolName, Target: def.RootDir, SideEffectState: tool.OutcomeSideEffectNone}
	outcome.Environment = &tool.EnvironmentObservation{
		Observation: tool.ObservationPartial, ObservedAt: time.Now().UTC(), Scope: "skill_package", Source: "executor",
		Files: []string{def.RootDir}, Unobserved: []string{"file_contents", "git", "processes", "environment_variables"},
	}
	detailEntries := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		detailEntries = append(detailEntries, map[string]any{"path": entry.Path, "kind": entry.Kind, "size_bytes": entry.SizeBytes})
	}
	outcome.Details = map[string]any{
		"skill_id": result.SkillID, "prefix": result.Prefix, "entries": detailEntries, "inventory_truncated": result.InventoryTruncated,
	}
	return outcome.WithTiming(started, time.Now().UTC()), nil
}

func normalizeResourcePrefix(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	if filepath.IsAbs(raw) {
		return "", fmt.Errorf("prefix must be package-relative")
	}
	clean := filepath.Clean(filepath.FromSlash(raw))
	if clean == "." {
		return "", nil
	}
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("prefix must not escape the Skill package")
	}
	return filepath.ToSlash(clean), nil
}

func resourceListFailure(code string, category tool.ErrorCategory, message string, retryable bool, recovery []string) tool.ExecutionOutcome {
	outcome := tool.FailedOutcome(message, code, category, retryable)
	outcome.RecoveryOptions = append([]string(nil), recovery...)
	return outcome
}
