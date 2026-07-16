package skillruntime

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"awaken/src/skills"
	"awaken/src/tool"
)

const (
	ResourceReadToolName     = "skill_resource_read"
	maxSkillResourceReadSize = 2 << 20
)

// ResourceReadTool exposes read-only access to files bundled inside a local
// skill package. Paths are always resolved below the selected skill RootDir.
type ResourceReadTool struct {
	registry *skills.SkillRegistry
}

type resourceReadInput struct {
	SkillID string `json:"skill_id"`
	Path    string `json:"path"`
}

func (t *ResourceReadTool) Name() string { return ResourceReadToolName }

func (t *ResourceReadTool) Description() string {
	return "Read a file bundled inside a local skill package by skill_id and relative path."
}

func (t *ResourceReadTool) Parameters() []tool.ToolParameter {
	return []tool.ToolParameter{
		{Name: "skill_id", Type: "string", Description: "Skill ID whose package should be read.", Required: true},
		{Name: "path", Type: "string", Description: "Relative file path inside the skill package.", Required: true},
	}
}

func (t *ResourceReadTool) Definition() tool.ToolDefinition {
	return tool.ToolDefinition{
		SchemaVersion: tool.ToolDefinitionSchemaVersion,
		Name:          ResourceReadToolName,
		DisplayName:   "Skill Resource Read",
		Source:        tool.SourceSkill,
		Description:   t.Description(),
		Purpose:       "Read one bounded file from an installed local Skill package without allowing paths to escape that package root.",
		WhenToUse: []string{
			"Use after an installed Skill references a bundled template, example, script, or instruction file needed for the active task.",
			"Use when skill_id and the package-relative path are known and only file contents or metadata are required.",
		},
		WhenNotToUse: []string{
			"Do not use for workspace files outside an installed Skill package; use Read.",
			"Do not use to install or execute a Skill, enumerate arbitrary directories, or read files larger than the declared bound.",
		},
		InputSchema: tool.ToolParametersSchema{
			Type: "object", Properties: map[string]tool.ToolParam{
				"skill_id": {Type: "string", Description: "Installed Skill identifier."},
				"path":     {Type: "string", Description: "Package-relative regular-file path that must stay below the Skill root."},
			},
			Required: []string{"skill_id", "path"}, AdditionalProperties: tool.BoolPtr(false),
		},
		OutputSchema: tool.ToolParam{Type: "object", Properties: map[string]tool.ToolParam{
			"skill_id":      {Type: "string", Description: "Resolved installed Skill identifier."},
			"path":          {Type: "string", Description: "Normalized package-relative resource path."},
			"resolved_path": {Type: "string", Description: "Observed local resource path inside the Skill package."},
			"size_bytes":    {Type: "integer", Description: "Observed resource size in bytes."},
			"sha256":        {Type: "string", Description: "SHA-256 digest of the returned bytes."},
		}, Required: []string{"skill_id", "path", "resolved_path", "size_bytes", "sha256"}, AdditionalProperties: tool.BoolPtr(false)},
		SideEffects: []tool.ToolSideEffect{{
			Kind: tool.SideEffectRead, Resources: []string{"installed Skill package file"},
			Description: "Reads one bounded regular file inside the selected installed Skill package.", Reversibility: tool.ReversibilityNotApplicable,
		}},
		Permissions: tool.ToolPermissionSpec{
			CapabilityClass: tool.CapabilityArtifactRead, WorkspaceScope: tool.WorkspaceScopeRead,
			FileBridgeScope: tool.FileBridgeTaskScopeRead, NetworkScope: tool.NetworkScopeNone,
		},
		Examples: []tool.ToolExample{{
			Scenario: "Read a bundled Skill template", Input: map[string]any{"skill_id": "report-writer", "path": "templates/report.md"},
			Outcome: "Returns bounded content plus the normalized path, byte size, and SHA-256 digest.",
		}},
		FailureModes: []tool.ToolFailureMode{
			{Code: "skill_resource_cancelled", Cause: "The read context was cancelled.", Retryable: true, Recovery: []string{"Retry when the operation context is available."}},
			{Code: "skill_resource_invalid_input", Cause: "Arguments, skill_id, or the relative path are invalid.", Retryable: false, Recovery: []string{"Correct skill_id and path."}},
			{Code: "skill_resource_runtime_unavailable", Cause: "The live Skill registry is unavailable.", Retryable: true, Recovery: []string{"Restore or reload the Skill runtime."}},
			{Code: "skill_resource_not_found", Cause: "The Skill or requested resource does not exist.", Retryable: false, Recovery: []string{"Refresh installed Skill state and select an existing resource."}},
			{Code: "skill_resource_path_denied", Cause: "The path is absolute, escapes the package, or resolves to a directory.", Retryable: false, Recovery: []string{"Use a package-relative regular-file path."}},
			{Code: "skill_resource_too_large", Cause: "The resource exceeds the bounded read limit.", Retryable: false, Recovery: []string{"Read a smaller package resource."}},
			{Code: "skill_resource_binary", Cause: "The resource is not UTF-8 text.", Retryable: false, Recovery: []string{"Use the matching file or artifact tool for binary assets."}},
			{Code: "skill_resource_read_failed", Cause: "The local resource could not be statted or read.", Retryable: true, Recovery: []string{"Verify the installed package and retry once if the failure is transient."}},
		},
		Annotations: tool.ToolAnnotations{
			ReadOnly:        true,
			Destructive:     false,
			OpenWorld:       false,
			CapabilityClass: tool.CapabilityArtifactRead,
			TrustedOrigin:   tool.TrustedOriginBuiltin,
			FileBridgeScope: tool.FileBridgeTaskScopeRead,
			NetworkScope:    tool.NetworkScopeNone,
		},
	}
}

func (t *ResourceReadTool) IsReadOnly() bool { return true }

func (t *ResourceReadTool) Execute(ctx context.Context, input string) (string, error) {
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

func (t *ResourceReadTool) ExecuteOutcome(ctx context.Context, input string, _ tool.ToolObserver) (tool.ExecutionOutcome, error) {
	started := time.Now().UTC()
	if err := ctx.Err(); err != nil {
		outcome := resourceReadFailure("skill_resource_cancelled", tool.ErrorCategoryCancelled, err.Error(), true, []string{"retry_when_context_available"})
		outcome.Status = tool.OutcomeCancelled
		outcome.TransportStatus = tool.TransportCancelled
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}
	if t.registry == nil {
		return resourceReadFailure("skill_resource_runtime_unavailable", tool.ErrorCategoryRuntime, "skill registry unavailable", true, []string{"restore_skill_runtime"}).WithTiming(started, time.Now().UTC()), nil
	}
	var req resourceReadInput
	if err := json.Unmarshal([]byte(strings.TrimSpace(input)), &req); err != nil {
		return resourceReadFailure("skill_resource_invalid_input", tool.ErrorCategoryValidation, fmt.Sprintf("arguments must be a JSON object: %v", err), false, []string{"fix_skill_resource_arguments"}).WithTiming(started, time.Now().UTC()), nil
	}
	skillID := strings.TrimSpace(req.SkillID)
	if skillID == "" {
		return resourceReadFailure("skill_resource_invalid_input", tool.ErrorCategoryValidation, "skill_id is required", false, []string{"provide_skill_id"}).WithTiming(started, time.Now().UTC()), nil
	}
	def := t.registry.Get(skillID)
	if def == nil {
		return resourceReadFailure("skill_resource_not_found", tool.ErrorCategoryDomain, fmt.Sprintf("skill %q not found", skillID), false, []string{"refresh_installed_skills", "choose_existing_skill"}).WithTiming(started, time.Now().UTC()), nil
	}
	if strings.TrimSpace(def.RootDir) == "" {
		return resourceReadFailure("skill_resource_not_found", tool.ErrorCategoryDomain, fmt.Sprintf("skill %q has no local package root", skillID), false, []string{"choose_local_installed_skill"}).WithTiming(started, time.Now().UTC()), nil
	}
	target, err := resolveSkillResourcePath(def.RootDir, req.Path)
	if err != nil {
		return resourceReadFailure("skill_resource_path_denied", tool.ErrorCategoryPolicy, err.Error(), false, []string{"use_package_relative_path"}).WithTiming(started, time.Now().UTC()), nil
	}
	rel, _ := filepath.Rel(def.RootDir, target)
	rel = filepath.ToSlash(rel)
	root, file, err := openAnchoredSkillResource(ctx, def, rel)
	if err != nil {
		if os.IsNotExist(err) {
			return resourceReadFailure("skill_resource_not_found", tool.ErrorCategoryDomain, fmt.Sprintf("open skill resource: %v", err), false, []string{"verify_installed_resource_path"}).WithTiming(started, time.Now().UTC()), nil
		}
		if os.IsPermission(err) {
			return resourceReadFailure("skill_resource_read_failed", tool.ErrorCategoryAuthorization, fmt.Sprintf("open skill resource: %v", err), false, []string{"verify_package_permissions"}).WithTiming(started, time.Now().UTC()), nil
		}
		return resourceReadFailure("skill_resource_path_denied", tool.ErrorCategoryPolicy, fmt.Sprintf("open rooted skill resource: %v", err), false, []string{"use_package_relative_path_without_external_symlinks"}).WithTiming(started, time.Now().UTC()), nil
	}
	defer root.Close()
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return resourceReadFailure("skill_resource_read_failed", tool.ErrorCategoryRuntime, fmt.Sprintf("stat opened skill resource: %v", err), true, []string{"verify_installed_resource_path"}).WithTiming(started, time.Now().UTC()), nil
	}
	if info.IsDir() {
		return resourceReadFailure("skill_resource_path_denied", tool.ErrorCategoryPolicy, "skill resource path is a directory", false, []string{"choose_regular_file"}).WithTiming(started, time.Now().UTC()), nil
	}
	if !info.Mode().IsRegular() {
		return resourceReadFailure("skill_resource_path_denied", tool.ErrorCategoryPolicy, "skill resource path is not a regular file", false, []string{"choose_regular_file"}).WithTiming(started, time.Now().UTC()), nil
	}
	if info.Size() > maxSkillResourceReadSize {
		return resourceReadFailure("skill_resource_too_large", tool.ErrorCategoryDomain, fmt.Sprintf("skill resource exceeds %d bytes", maxSkillResourceReadSize), false, []string{"choose_smaller_resource"}).WithTiming(started, time.Now().UTC()), nil
	}
	data, err := io.ReadAll(io.LimitReader(file, maxSkillResourceReadSize+1))
	if err != nil {
		return resourceReadFailure("skill_resource_read_failed", tool.ErrorCategoryRuntime, fmt.Sprintf("read skill resource: %v", err), true, []string{"verify_package_permissions", "retry_once"}).WithTiming(started, time.Now().UTC()), nil
	}
	if len(data) > maxSkillResourceReadSize {
		return resourceReadFailure("skill_resource_too_large", tool.ErrorCategoryDomain, fmt.Sprintf("skill resource exceeds %d bytes", maxSkillResourceReadSize), false, []string{"choose_smaller_resource"}).WithTiming(started, time.Now().UTC()), nil
	}
	if !utf8.Valid(data) {
		return resourceReadFailure("skill_resource_binary", tool.ErrorCategoryValidation, "skill_resource_read only returns UTF-8 text; use a file or artifact tool for binary resources", false, []string{"use_file_or_artifact_tool"}).WithTiming(started, time.Now().UTC()), nil
	}
	content := fmt.Sprintf("[skill resource %q:%s]\n\n%s", skillID, rel, string(data))
	outcome := tool.SucceededOutcome(content)
	outcome.Summary = fmt.Sprintf("Read %d bytes from Skill %s resource %s", len(data), skillID, rel)
	outcome.Operation = &tool.OperationDescriptor{Type: ResourceReadToolName, Target: target, SideEffectState: tool.OutcomeSideEffectNone}
	outcome.Environment = &tool.EnvironmentObservation{
		Observation: tool.ObservationPartial, ObservedAt: time.Now().UTC(), Scope: "skill_package", Source: "executor",
		Files: []string{target}, Unobserved: []string{"git", "processes", "environment_variables"},
	}
	outcome.Details = map[string]any{
		"skill_id": skillID, "path": rel, "resolved_path": target,
		"size_bytes": len(data), "sha256": fmt.Sprintf("%x", sha256.Sum256(data)),
	}
	return outcome.WithTiming(started, time.Now().UTC()), nil
}

func openAnchoredSkillResource(ctx context.Context, def *skills.SkillDefinition, resourcePath string) (*os.Root, *os.File, error) {
	if def == nil {
		return nil, nil, fmt.Errorf("Skill definition is unavailable")
	}
	currentRoot, err := filepath.EvalSymlinks(def.RootDir)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve current Skill package root: %w", err)
	}
	trustedRoot := strings.TrimSpace(def.ResolvedRootDir)
	if trustedRoot == "" {
		trustedRoot = currentRoot
	}
	trustedRoot = filepath.Clean(trustedRoot)
	if filepath.Clean(currentRoot) != trustedRoot {
		return nil, nil, fmt.Errorf("Skill package root changed after registry load")
	}

	anchor := strings.TrimSpace(def.SourceRoot)
	switch def.Source {
	case skills.SourceProject:
		projectRoot := strings.TrimSpace(def.ProjectPath)
		if projectRoot == "" {
			return nil, nil, fmt.Errorf("project Skill has no project identity")
		}
		if tool.HasPermissionContext(ctx) && !tool.PermissionContextFromContext(ctx).IsWithinWorkspaceResolved(projectRoot) {
			return nil, nil, fmt.Errorf("project Skill root is outside the active workspace")
		}
		if anchor == "" {
			anchor = filepath.Join(projectRoot, ".awaken", "skills")
		}
	case skills.SourceUser:
		if anchor == "" {
			home, homeErr := os.UserHomeDir()
			if homeErr != nil || strings.TrimSpace(home) == "" {
				return nil, nil, fmt.Errorf("user Skill root is unavailable")
			}
			anchor = filepath.Join(home, ".awaken", "skills")
		}
	case skills.SourceManaged, skills.SourceAdditional, skills.SourcePlugin, skills.SourceBundled:
		if anchor == "" {
			return nil, nil, fmt.Errorf("Skill source %q has no recorded discovery root", def.Source)
		}
	case skills.SourceLegacy:
		if anchor == "" || (filepath.Clean(def.RootDir) == filepath.Clean(anchor) && !strings.EqualFold(filepath.Base(def.SkillFilePath), "SKILL.md")) {
			return nil, nil, fmt.Errorf("flat legacy commands do not expose package resources")
		}
	default:
		return nil, nil, fmt.Errorf("Skill source %q has no local resource boundary", def.Source)
	}
	anchor, err = filepath.EvalSymlinks(anchor)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve Skill resource anchor: %w", err)
	}
	if !tool.PathWithinRoot(trustedRoot, anchor) {
		return nil, nil, fmt.Errorf("Skill package root escapes its authorized anchor")
	}
	packageRel, err := filepath.Rel(anchor, trustedRoot)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve Skill package identity: %w", err)
	}
	parts := strings.Split(filepath.ToSlash(packageRel), "/")
	if len(parts) != 1 || strings.TrimSpace(parts[0]) == "" || parts[0] == "." {
		return nil, nil, fmt.Errorf("Skill package root is not one package below its authorized discovery root")
	}

	root, err := os.OpenRoot(anchor)
	if err != nil {
		return nil, nil, fmt.Errorf("open Skill resource anchor: %w", err)
	}
	file, err := root.Open(filepath.Join(packageRel, filepath.FromSlash(resourcePath)))
	if err != nil {
		root.Close()
		return nil, nil, err
	}
	return root, file, nil
}

func resourceReadFailure(code string, category tool.ErrorCategory, message string, retryable bool, recovery []string) tool.ExecutionOutcome {
	outcome := tool.FailedOutcome(message, code, category, retryable)
	outcome.RecoveryOptions = append([]string(nil), recovery...)
	return outcome
}

func resolveSkillResourcePath(rootDir string, rawPath string) (string, error) {
	root := filepath.Clean(rootDir)
	rel := strings.TrimSpace(rawPath)
	if rel == "" {
		return "", fmt.Errorf("path is required")
	}
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("path must be relative to the skill package")
	}
	cleanRel := filepath.Clean(filepath.FromSlash(strings.ReplaceAll(rel, "\\", "/")))
	if cleanRel == "." || cleanRel == ".." || strings.HasPrefix(cleanRel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes the skill package")
	}
	target := filepath.Join(root, cleanRel)
	back, err := filepath.Rel(root, target)
	if err != nil {
		return "", fmt.Errorf("resolve skill resource: %w", err)
	}
	if back == ".." || strings.HasPrefix(back, ".."+string(filepath.Separator)) || filepath.IsAbs(back) {
		return "", fmt.Errorf("path escapes the skill package")
	}
	return target, nil
}
