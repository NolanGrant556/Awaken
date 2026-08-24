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

const SkillPackageExportToolName = "skill_package_export"

type skillPackageExportInput struct {
	SkillID     string `json:"skill_id"`
	Destination string `json:"destination"`
}

type skillPackageExportOutput struct {
	SkillID     string `json:"skill_id"`
	Name        string `json:"name"`
	Source      string `json:"source"`
	SourceRoot  string `json:"source_root"`
	Destination string `json:"destination"`
	FileCount   int    `json:"file_count"`
	TotalBytes  int64  `json:"total_bytes"`
	Message     string `json:"message"`
}

type SkillPackageExportTool struct{ registry *skills.SkillRegistry }

func (t *SkillPackageExportTool) Name() string { return SkillPackageExportToolName }

func (t *SkillPackageExportTool) Description() string {
	return "Export a complete installed local Awaken Skill package to a new authoring directory outside the protected live store, preserving scripts, references, and assets for safe modification."
}

func (t *SkillPackageExportTool) Parameters() []tool.ToolParameter { return nil }

func (t *SkillPackageExportTool) Definition() tool.ToolDefinition {
	closed := false
	return tool.BuildBuiltinDefinition(t, tool.ToolDefinition{
		Name: SkillPackageExportToolName, DisplayName: "Skill Package Export", Description: t.Description(),
		Purpose: "Create a complete workspace candidate from an installed Skill before editing, so modification never writes the protected live package in place or silently drops companion resources.",
		WhenToUse: []string{
			"Use before modifying an installed project or global Skill package.",
			"Export to a previously absent authoring directory, edit the candidate with normal file tools, validate it with skill_install validate_only=true, then replace only after review.",
		},
		WhenNotToUse: []string{
			"Do not export MCP-only Skills without a local package root.",
			"Do not choose a destination under ~/.awaken or overwrite an existing candidate directory.",
		},
		InputSchema: tool.ToolParametersSchema{Type: "object", AdditionalProperties: &closed, Required: []string{"skill_id", "destination"}, Properties: map[string]tool.ToolParam{
			"skill_id":    {Type: "string", Description: "Installed local Skill identifier."},
			"destination": {Type: "string", Description: "Previously absent authoring directory; relative paths resolve from the task workspace."},
		}},
		OutputSchema: tool.ToolParam{Type: "object", AdditionalProperties: tool.BoolPtr(false), Required: []string{
			"skill_id", "name", "source", "source_root", "destination", "file_count", "total_bytes", "message",
		}, Properties: map[string]tool.ToolParam{
			"skill_id": {Type: "string"}, "name": {Type: "string"}, "source": {Type: "string"},
			"source_root": {Type: "string"}, "destination": {Type: "string"},
			"file_count": {Type: "integer"}, "total_bytes": {Type: "integer"}, "message": {Type: "string"},
		}},
		SideEffects: []tool.ToolSideEffect{
			{Kind: tool.SideEffectRead, Resources: []string{"installed Skill package"}, Description: "Reads the complete anchored local package without executing its contents.", Reversibility: tool.ReversibilityNotApplicable},
			{Kind: tool.SideEffectCreate, Resources: []string{"workspace Skill authoring candidate"}, Description: "Creates a new complete candidate directory; existing destinations are never overwritten.", Reversibility: tool.ReversibilityReversible},
		},
		Examples: []tool.ToolExample{{Scenario: "Prepare a user Skill for modification", Input: map[string]any{"skill_id": "daily-followup", "destination": "output/skill-authoring/daily-followup"}, Outcome: "Creates a complete candidate copy while leaving the installed package unchanged."}},
		FailureModes: []tool.ToolFailureMode{
			{Code: "skill_export_invalid_input", Cause: "The Skill ID or destination is missing or invalid.", Retryable: false, Recovery: []string{"Choose an installed local Skill and a new authoring destination."}},
			{Code: "skill_export_not_found", Cause: "The installed Skill or its local package root is unavailable.", Retryable: false, Recovery: []string{"Refresh the Skill catalog and choose an existing local package."}},
			{Code: "skill_export_path_denied", Cause: "The source root changed, escaped its discovery anchor, or the destination is protected.", Retryable: false, Recovery: []string{"Choose a normal workspace authoring directory and refresh changed Skill state."}},
			{Code: "skill_export_destination_exists", Cause: "The candidate destination already exists.", Retryable: false, Recovery: []string{"Use a new destination or explicitly continue editing the existing candidate after inspecting it."}},
			{Code: "skill_export_failed", Cause: "The complete package could not be read, validated, or copied atomically.", Retryable: false, Recovery: []string{"Inspect the source package and filesystem error before retrying."}},
		},
		Annotations:  tool.ToolAnnotations{ReadOnly: false, Destructive: false, OpenWorld: false, CapabilityClass: tool.CapabilityArtifactWrite, TrustedOrigin: tool.TrustedOriginBuiltin, FileBridgeScope: tool.FileBridgeTaskScopeWrite, NetworkScope: tool.NetworkScopeNone},
		Capabilities: []tool.Capability{tool.CapabilityFileRead, tool.CapabilityFileWrite},
	})
}

func (t *SkillPackageExportTool) CheckPermission(ctx context.Context, raw string, permission tool.PermissionContext) tool.PermissionDecision {
	req, err := parseSkillPackageExportInput(raw)
	if err != nil {
		return tool.DenyPermission(err.Error())
	}
	if _, err := permission.ResolveWritablePathChecked(req.Destination); err != nil {
		return tool.DenyPermission(err.Error())
	}
	return tool.AllowPermission()
}

func (t *SkillPackageExportTool) Execute(ctx context.Context, raw string) (string, error) {
	value, err := t.execute(ctx, raw)
	if err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	return string(data), err
}

func (t *SkillPackageExportTool) ExecuteOutcome(ctx context.Context, raw string, _ tool.ToolObserver) (tool.ExecutionOutcome, error) {
	started := time.Now().UTC()
	value, err := t.execute(ctx, raw)
	if err != nil {
		code := "skill_export_failed"
		if strings.Contains(err.Error(), "invalid input") {
			code = "skill_export_invalid_input"
		} else if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "no local package") {
			code = "skill_export_not_found"
		} else if strings.Contains(err.Error(), "denied") || strings.Contains(err.Error(), "escapes") || strings.Contains(err.Error(), "changed after registry") {
			code = "skill_export_path_denied"
		} else if strings.Contains(err.Error(), "destination already exists") {
			code = "skill_export_destination_exists"
		}
		outcome := tool.FailedOutcome(err.Error(), code, tool.ErrorCategoryValidation, false)
		outcome.Operation = &tool.OperationDescriptor{Type: SkillPackageExportToolName, SideEffectState: tool.OutcomeSideEffectNotStarted}
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return tool.FailedOutcome(err.Error(), "skill_export_failed", tool.ErrorCategoryTool, false).WithTiming(started, time.Now().UTC()), nil
	}
	outcome := tool.SucceededOutcome(string(data))
	outcome.Summary = value.Message
	outcome.Operation = &tool.OperationDescriptor{Type: SkillPackageExportToolName, Target: value.Destination, SideEffectState: tool.OutcomeSideEffectApplied}
	outcome.AffectedPaths = []string{value.Destination}
	outcome.Changes = []tool.ResourceChange{{Kind: "skill_authoring_candidate", Action: "created", Resource: value.Destination, Summary: value.Message, Certainty: tool.ChangeObserved, Reversibility: tool.ChangeReversible}}
	outcome.Details = map[string]any{"skill_id": value.SkillID, "name": value.Name, "source": value.Source, "source_root": value.SourceRoot, "destination": value.Destination, "file_count": value.FileCount, "total_bytes": value.TotalBytes, "message": value.Message}
	outcome.NextActions = []tool.NextAction{{ID: "edit_exported_skill", Description: "Edit the complete candidate, then call skill_install with validate_only=true before any replacement.", Tool: SkillInstallToolName, Preconditions: []string{"The candidate has been reviewed and retains every required package resource."}, InputHint: map[string]any{"source_type": "local_path", "path": value.Destination, "validate_only": true}}}
	return outcome.WithTiming(started, time.Now().UTC()), nil
}

func (t *SkillPackageExportTool) execute(ctx context.Context, raw string) (skillPackageExportOutput, error) {
	if err := ctx.Err(); err != nil {
		return skillPackageExportOutput{}, err
	}
	req, err := parseSkillPackageExportInput(raw)
	if err != nil {
		return skillPackageExportOutput{}, fmt.Errorf("invalid input: %w", err)
	}
	if t == nil || t.registry == nil {
		return skillPackageExportOutput{}, fmt.Errorf("Skill registry is unavailable")
	}
	def := t.registry.Get(req.SkillID)
	if def == nil {
		return skillPackageExportOutput{}, fmt.Errorf("Skill %q not found", req.SkillID)
	}
	if strings.TrimSpace(def.RootDir) == "" {
		return skillPackageExportOutput{}, fmt.Errorf("Skill %q has no local package root", req.SkillID)
	}
	root, file, err := openAnchoredSkillResource(ctx, def, "SKILL.md")
	if err != nil {
		return skillPackageExportOutput{}, err
	}
	file.Close()
	root.Close()
	currentRoot, err := filepath.EvalSymlinks(def.RootDir)
	if err != nil {
		return skillPackageExportOutput{}, err
	}
	pkg, err := skills.PackageFromPath(currentRoot)
	if err != nil {
		return skillPackageExportOutput{}, err
	}
	if after, err := filepath.EvalSymlinks(def.RootDir); err != nil || filepath.Clean(after) != filepath.Clean(currentRoot) {
		return skillPackageExportOutput{}, fmt.Errorf("Skill package root changed after registry validation")
	}
	destination, err := tool.PermissionContextFromContext(ctx).ResolveWritablePathChecked(req.Destination)
	if err != nil {
		return skillPackageExportOutput{}, fmt.Errorf("destination denied: %w", err)
	}
	validation, err := skills.ExportSkillPackage(pkg, destination)
	if err != nil {
		return skillPackageExportOutput{}, err
	}
	return skillPackageExportOutput{SkillID: def.ID, Name: def.Name, Source: string(def.Source), SourceRoot: currentRoot, Destination: destination, FileCount: validation.FileCount, TotalBytes: validation.TotalBytes, Message: "Exported a complete Skill authoring candidate without changing the installed package"}, nil
}

func parseSkillPackageExportInput(raw string) (skillPackageExportInput, error) {
	var req skillPackageExportInput
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &req); err != nil {
		return req, err
	}
	req.SkillID = strings.TrimSpace(req.SkillID)
	req.Destination = strings.TrimSpace(req.Destination)
	if req.SkillID == "" || req.Destination == "" {
		return req, fmt.Errorf("skill_id and destination are required")
	}
	return req, nil
}
