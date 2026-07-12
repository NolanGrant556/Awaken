package skillruntime

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"awaken/src/skills"
	"awaken/src/tool"
)

const SkillInstallToolName = "skill_install"

type SkillInstallTool struct {
	registry *skills.SkillRegistry
}

type skillInstallInput struct {
	SourceType         string `json:"source_type"`
	FileName           string `json:"file_name,omitempty"`
	Content            string `json:"content,omitempty"`
	ContentBase64      string `json:"content_base64,omitempty"`
	Path               string `json:"path,omitempty"`
	URL                string `json:"url,omitempty"`
	Ref                string `json:"ref,omitempty"`
	Subpath            string `json:"subpath,omitempty"`
	Scope              string `json:"scope,omitempty"`
	ProjectRoot        string `json:"project_root,omitempty"`
	OverwritePolicy    string `json:"overwrite_policy,omitempty"`
	ReloadAfterInstall *bool  `json:"reload_after_install,omitempty"`
}

type skillInstallOutput struct {
	Status                     string `json:"status"`
	ID                         string `json:"id"`
	Name                       string `json:"name"`
	Scope                      string `json:"scope"`
	SourceOrigin               string `json:"source_origin"`
	RootDir                    string `json:"root_dir"`
	SkillFilePath              string `json:"skill_file_path"`
	ProjectPath                string `json:"project_path,omitempty"`
	State                      string `json:"state"`
	Reloaded                   bool   `json:"reloaded"`
	OverwritePolicy            string `json:"overwrite_policy"`
	Message                    string `json:"message"`
	PreviousPackageTrashedPath string `json:"previous_package_trashed_path,omitempty"`
}

type skillInstallFailure struct {
	code       string
	category   tool.ErrorCategory
	retryable  bool
	recovery   []string
	underlying error
}

func (e *skillInstallFailure) Error() string {
	if e == nil || e.underlying == nil {
		return "skill installation failed"
	}
	return e.underlying.Error()
}

func (e *skillInstallFailure) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.underlying
}

type skillInstallSourceError struct {
	unavailable bool
	underlying  error
}

func (e *skillInstallSourceError) Error() string {
	if e == nil || e.underlying == nil {
		return "skill source is invalid"
	}
	return e.underlying.Error()
}

func (e *skillInstallSourceError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.underlying
}

func invalidSkillInstallSource(err error) error {
	return &skillInstallSourceError{underlying: err}
}

func unavailableSkillInstallSource(err error) error {
	return &skillInstallSourceError{unavailable: true, underlying: err}
}

func (t *SkillInstallTool) Name() string { return SkillInstallToolName }

func (t *SkillInstallTool) Description() string {
	return "Install a validated Awaken Skill package from content, a local path, an HTTP(S) archive/file, or an HTTPS Git repository. The tool resolves project/global skill roots deterministically; do not guess or write .awaken/skills paths manually."
}

func (t *SkillInstallTool) Parameters() []tool.ToolParameter {
	return []tool.ToolParameter{
		{Name: "source_type", Type: "string", Required: true, Description: "Skill source adapter: content, content_base64, local_path, remote_url, or remote_git."},
		{Name: "file_name", Type: "string", Required: false, Description: "Source file name for content/content_base64/remote_url when needed; use SKILL.md or a .zip file name."},
		{Name: "content", Type: "string", Required: false, Description: "Raw SKILL.md or zip bytes represented as text when source_type is content."},
		{Name: "content_base64", Type: "string", Required: false, Description: "Base64 encoded SKILL.md or zip bytes when source_type is content_base64."},
		{Name: "path", Type: "string", Required: false, Description: "Local file or directory path when source_type is local_path. Directories must contain exactly one SKILL.md unless the root has SKILL.md."},
		{Name: "url", Type: "string", Required: false, Description: "HTTP(S) URL for remote_url, or an absolute HTTPS repository URL without inline credentials for remote_git."},
		{Name: "ref", Type: "string", Required: false, Description: "Optional branch, tag, or commit-ish for remote_git."},
		{Name: "subpath", Type: "string", Required: false, Description: "Optional relative subdirectory within a remote_git repository that contains the skill package."},
		{Name: "scope", Type: "string", Required: false, Description: "Install scope: project (default) or global. user is accepted as a global alias."},
		{Name: "project_root", Type: "string", Required: false, Description: "Project root for project scope; defaults to the current workspace and is resolved by the runtime."},
		{Name: "overwrite_policy", Type: "string", Required: false, Description: "Existing skill policy: abort (default) or replace. Replace atomically installs the new package and moves the previous package to the operating-system Trash."},
		{Name: "reload_after_install", Type: "boolean", Required: false, Description: "Reload the skill registry after installation. Defaults to true."},
	}
}

func (t *SkillInstallTool) Definition() tool.ToolDefinition {
	return tool.BuildBuiltinDefinition(t, tool.ToolDefinition{
		Name:        SkillInstallToolName,
		DisplayName: "Skill Install",
		Description: t.Description(),
		Purpose:     "Validate and install an Awaken Skill package into the deterministic project or global skill root, then optionally reload the live registry.",
		WhenToUse: []string{
			"Use when the user explicitly wants to install a Skill from supplied content, an allowed local path, an HTTP(S) source, or a Git repository.",
			"Use when installation must follow Awaken's package validation, scope resolution, overwrite policy, and registry reload contract.",
		},
		WhenNotToUse: []string{
			"Do not use to read or execute an already installed Skill; use skill_resource_read for package resources or skill for invocation.",
			"Do not write .awaken/skills manually or permanently delete an existing package during replacement.",
		},
		InputSchema: skillInstallInputSchema(),
		OutputSchema: tool.ToolParam{Type: "object", Properties: map[string]tool.ToolParam{
			"install_status":                {Type: "string", Enum: []any{"installed"}, Description: "Installation completion state."},
			"id":                            {Type: "string", Description: "Stable installed Skill identifier."},
			"name":                          {Type: "string", Description: "Installed Skill display name."},
			"scope":                         {Type: "string", Enum: []any{"project", "global"}, Description: "Resolved installation scope."},
			"source_origin":                 {Type: "string", Description: "Resolved project or user source origin."},
			"root_dir":                      {Type: "string", Description: "Observed installed package directory."},
			"skill_file_path":               {Type: "string", Description: "Observed installed SKILL.md path."},
			"project_path":                  {Type: "string", Description: "Associated project path when applicable."},
			"state":                         {Type: "string", Description: "Installed Skill availability state."},
			"reloaded":                      {Type: "boolean", Description: "Whether the live Skill registry was reloaded."},
			"overwrite_policy":              {Type: "string", Enum: []any{"abort", "replace"}, Description: "Effective existing-package policy used by the installer."},
			"message":                       {Type: "string", Description: "Bounded installation result message."},
			"previous_package_trashed_path": {Type: "string", Description: "Observed Trash destination for the previous package when overwrite_policy=replace."},
		}, Required: []string{"install_status", "id", "name", "scope", "source_origin", "root_dir", "skill_file_path", "state", "reloaded", "overwrite_policy", "message"}, AdditionalProperties: tool.BoolPtr(false)},
		SideEffects: []tool.ToolSideEffect{
			{Kind: tool.SideEffectCreate, Resources: []string{"project or global Skill package"}, Description: "Creates a validated Skill package under the resolved Awaken skill root.", Reversibility: tool.ReversibilityReversible},
			{Kind: tool.SideEffectUpdate, Resources: []string{"existing Skill package", "live Skill registry"}, Description: "Registry reload may update live Skill state; package replacement moves the prior version to Trash.", Reversibility: tool.ReversibilityConditional},
			{Kind: tool.SideEffectDelete, Resources: []string{"previous Skill package"}, Description: "Replacement removes the prior package from the live catalog only after moving it to operating-system Trash.", Reversibility: tool.ReversibilityReversible},
			{Kind: tool.SideEffectNetwork, Resources: []string{"declared HTTP(S) or Git source"}, Description: "Remote source modes read the declared network location before installation.", Reversibility: tool.ReversibilityNotApplicable},
		},
		Examples: []tool.ToolExample{{
			Scenario: "Install a supplied project Skill and reload the registry",
			Input: map[string]any{
				"source_type": "content", "file_name": "SKILL.md",
				"content": "---\nname: daily-followup\ndescription: Daily followup.\n---\nDo the followup.",
				"scope":   "project", "overwrite_policy": "abort", "reload_after_install": true,
			},
			Outcome: "Returns the observed Skill identity and install paths with an installed package change.",
		}},
		FailureModes: []tool.ToolFailureMode{
			{Code: "skill_install_cancelled", Cause: "The operation context was cancelled or exceeded its deadline.", Retryable: true, Recovery: []string{"Retry when the operation context is available."}},
			{Code: "skill_install_invalid_input", Cause: "Arguments or source-specific fields are invalid.", Retryable: false, Recovery: []string{"Correct source_type and its required fields."}},
			{Code: "skill_install_target_invalid", Cause: "The requested project/global target violates the workspace or scope boundary.", Retryable: false, Recovery: []string{"Choose an allowed scope and project root."}},
			{Code: "skill_install_runtime_unavailable", Cause: "The live Skill registry is unavailable.", Retryable: true, Recovery: []string{"Restore the Skill runtime before retrying."}},
			{Code: "skill_install_source_invalid", Cause: "The declared content or local source could not produce a valid package.", Retryable: false, Recovery: []string{"Correct the package content or choose a valid readable local path."}},
			{Code: "skill_install_source_unavailable", Cause: "The declared HTTP(S) or Git source could not be retrieved.", Retryable: true, Recovery: []string{"Inspect the remote source and retry with backoff or choose a different source."}},
			{Code: "skill_install_already_exists", Cause: "A package with the same Skill ID exists while overwrite_policy is abort.", Retryable: false, Recovery: []string{"Keep the existing Skill or explicitly request reviewed replacement."}},
			{Code: "skill_install_trash_unavailable", Cause: "The previous package could not be moved to operating-system Trash, so replacement was rolled back or stopped.", Retryable: false, Recovery: []string{"Inspect the existing and staged package state, restore Trash availability, then issue a fresh replacement; do not retry blindly."}},
			{Code: "skill_install_failed", Cause: "Package validation or the atomic installation step failed.", Retryable: false, Recovery: []string{"Fix the package or target permissions before retrying."}},
			{Code: "skill_install_output_invalid", Cause: "The installed result could not be encoded into the declared output contract.", Retryable: false, Recovery: []string{"Report an implementation failure; do not reinstall blindly."}},
		},
		Annotations: tool.ToolAnnotations{
			ReadOnly:        false,
			Destructive:     false,
			OpenWorld:       true,
			CapabilityClass: tool.CapabilityArtifactWrite,
			TrustedOrigin:   tool.TrustedOriginBuiltin,
			FileBridgeScope: tool.FileBridgeTaskScopeWrite,
			NetworkScope:    tool.NetworkScopeOpen,
		},
		Capabilities: []tool.Capability{tool.CapabilityFileWrite, tool.CapabilityFileDelete},
	})
}

func skillInstallInputSchema() tool.ToolParametersSchema {
	return tool.ToolParametersSchema{
		Type:        "object",
		Description: "Validated Skill installation request. Source-specific required fields are checked by the executor.",
		Properties: map[string]tool.ToolParam{
			"source_type":          {Type: "string", Enum: []any{"content", "content_base64", "local_path", "remote_url", "remote_git"}, Description: "Source adapter used to obtain the Skill package."},
			"file_name":            {Type: "string", Description: "SKILL.md or zip file name for content and URL sources."},
			"content":              {Type: "string", Description: "Raw SKILL.md text or textual package bytes for source_type=content."},
			"content_base64":       {Type: "string", Description: "Base64 encoded SKILL.md or zip bytes for source_type=content_base64."},
			"path":                 {Type: "string", Description: "Allowed local file or directory for source_type=local_path."},
			"url":                  {Type: "string", Description: "HTTP(S) package URL, or an absolute HTTPS repository URL without inline credentials for remote_git."},
			"ref":                  {Type: "string", Description: "Optional branch, tag, or commit for source_type=remote_git."},
			"subpath":              {Type: "string", Description: "Optional relative Skill directory within a Git repository."},
			"scope":                {Type: "string", Enum: []any{"project", "global", "user"}, Description: "Installation scope; empty defaults to project."},
			"project_root":         {Type: "string", Description: "Project root inside the current workspace; runtime supplies the default."},
			"overwrite_policy":     {Type: "string", Enum: []any{"abort", "replace"}, Description: "Existing package policy; empty defaults to abort, while replace moves the prior package to Trash."},
			"reload_after_install": {Type: "boolean", Description: "Reload the live registry after installation; defaults to true."},
		},
		Required:             []string{"source_type"},
		AdditionalProperties: tool.BoolPtr(false),
	}
}

func (t *SkillInstallTool) CheckPermission(ctx context.Context, input string, permissionContext tool.PermissionContext) tool.PermissionDecision {
	if err := ctx.Err(); err != nil {
		return tool.DenyPermission(err.Error())
	}
	req, err := parseSkillInstallInput(input)
	if err != nil {
		return tool.DenyPermission(fmt.Sprintf("invalid parameters: %v", err))
	}
	if normalizeSourceType(req.SourceType) == "remote_git" {
		if err := validateRemoteGitRequest(req); err != nil {
			return tool.DenyPermission(fmt.Sprintf("invalid remote_git parameters: %v", err))
		}
	}
	target, err := resolveSkillInstallTargetForTool(req, permissionContext)
	if err != nil {
		return tool.DenyPermission(err.Error())
	}
	if strings.TrimSpace(req.Path) != "" {
		if _, err := permissionContext.ResolveReadablePathChecked(req.Path); err != nil {
			return tool.DenyPermission(err.Error())
		}
	}
	return tool.AllowPermissionWithMetadata(tool.DecisionMetadata{
		Title:         "Install skill",
		Description:   fmt.Sprintf("Install a new skill into %s scope", target.scope),
		RiskTone:      "warning",
		AffectedPaths: []string{target.installDir},
	})
}

func (t *SkillInstallTool) Execute(ctx context.Context, input string) (string, error) {
	out, err := t.install(ctx, input)
	if err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (t *SkillInstallTool) ExecuteOutcome(ctx context.Context, input string, _ tool.ToolObserver) (tool.ExecutionOutcome, error) {
	started := time.Now().UTC()
	out, err := t.install(ctx, input)
	if err != nil {
		return skillInstallFailureOutcome(err).WithTiming(started, time.Now().UTC()), nil
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		failure := &skillInstallFailure{
			code: "skill_install_output_invalid", category: tool.ErrorCategoryTool,
			retryable: false, recovery: []string{"report_tool_implementation_failure"}, underlying: err,
		}
		return skillInstallFailureOutcome(failure).WithTiming(started, time.Now().UTC()), nil
	}
	details := map[string]any{
		"install_status": out.Status, "id": out.ID, "name": out.Name, "scope": out.Scope,
		"source_origin": out.SourceOrigin, "root_dir": out.RootDir, "skill_file_path": out.SkillFilePath,
		"project_path": out.ProjectPath, "state": out.State, "reloaded": out.Reloaded,
		"overwrite_policy": out.OverwritePolicy, "message": out.Message,
		"previous_package_trashed_path": out.PreviousPackageTrashedPath,
	}
	outcome := tool.SucceededOutcome(string(data))
	outcome.Summary = fmt.Sprintf("Installed Skill %s in %s scope", out.ID, out.Scope)
	outcome.Operation = &tool.OperationDescriptor{
		Type: SkillInstallToolName, Target: out.RootDir, SideEffectState: tool.OutcomeSideEffectApplied,
	}
	outcome.AffectedPaths = []string{out.RootDir}
	changeAction := "created"
	changeSummary := "Created a validated Skill package at a previously absent target."
	changeDetails := map[string]any{
		"skill_id": out.ID, "skill_file_path": out.SkillFilePath,
		"reloaded": out.Reloaded, "overwrite_policy": out.OverwritePolicy,
	}
	if out.OverwritePolicy == "replace" {
		changeAction = "updated"
		changeSummary = "Installed a validated Skill package and moved the previous package to operating-system Trash."
		changeDetails["previous_package_trashed_path"] = out.PreviousPackageTrashedPath
	}
	outcome.Changes = []tool.ResourceChange{{
		Kind: "skill_package", Action: changeAction, Resource: out.RootDir,
		Summary:   changeSummary,
		Certainty: tool.ChangeObserved, Reversibility: tool.ChangeConditional,
		Details: changeDetails,
	}}
	if out.PreviousPackageTrashedPath != "" {
		outcome.Changes = append(outcome.Changes, tool.ResourceChange{
			Kind: "skill_package", Action: "trashed", Resource: out.RootDir,
			Summary:   "Previous Skill package moved to operating-system Trash before replacement completed.",
			Certainty: tool.ChangeObserved, Reversibility: tool.ChangeReversible,
			Details: map[string]any{"trashed_path": out.PreviousPackageTrashedPath},
		})
	}
	outcome.Details = details
	if out.Reloaded {
		outcome.NextActions = []tool.NextAction{{
			ID: "use_installed_skill", Description: "Invoke the installed Skill when its declared use boundary matches the task.",
			Tool: CatalogToolName, Preconditions: []string{"The reloaded Skill is available and matches the user's request."},
			InputHint: map[string]any{"skill": out.ID},
		}}
	} else {
		outcome.NextActions = []tool.NextAction{{
			ID: "reload_skill_registry", Description: "Reload the Skill registry before trying to invoke the newly installed Skill.",
			Preconditions: []string{"The package is installed, but reloaded=false means live availability has not been confirmed."},
		}}
	}
	return outcome.WithTiming(started, time.Now().UTC()), nil
}

func (t *SkillInstallTool) install(ctx context.Context, input string) (skillInstallOutput, error) {
	if err := ctx.Err(); err != nil {
		return skillInstallOutput{}, &skillInstallFailure{
			code: "skill_install_cancelled", category: tool.ErrorCategoryCancelled,
			retryable: true, recovery: []string{"retry_when_context_available"}, underlying: err,
		}
	}
	if t.registry == nil {
		return skillInstallOutput{}, &skillInstallFailure{
			code: "skill_install_runtime_unavailable", category: tool.ErrorCategoryRuntime,
			retryable: true, recovery: []string{"restore_skill_runtime"}, underlying: fmt.Errorf("skill registry unavailable"),
		}
	}
	req, err := parseSkillInstallInput(input)
	if err != nil {
		return skillInstallOutput{}, &skillInstallFailure{
			code: "skill_install_invalid_input", category: tool.ErrorCategoryValidation,
			retryable: false, recovery: []string{"fix_skill_install_arguments"}, underlying: err,
		}
	}
	target, err := resolveSkillInstallTargetForTool(req, tool.PermissionContextFromContext(ctx))
	if err != nil {
		return skillInstallOutput{}, &skillInstallFailure{
			code: "skill_install_target_invalid", category: tool.ErrorCategoryPolicy,
			retryable: false, recovery: []string{"choose_allowed_skill_scope_and_project_root"}, underlying: err,
		}
	}
	pkg, err := t.packageFromInput(ctx, req)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return skillInstallOutput{}, &skillInstallFailure{
				code: "skill_install_cancelled", category: tool.ErrorCategoryCancelled,
				retryable: true, recovery: []string{"retry_when_context_available"}, underlying: err,
			}
		}
		failure := &skillInstallFailure{
			code: "skill_install_source_invalid", category: tool.ErrorCategoryValidation,
			retryable: false, recovery: []string{"correct_skill_package_source"}, underlying: err,
		}
		var sourceFailure *skillInstallSourceError
		if errors.As(err, &sourceFailure) && sourceFailure.unavailable {
			failure.code = "skill_install_source_unavailable"
			failure.category = tool.ErrorCategoryRuntime
			failure.retryable = true
			failure.recovery = []string{"inspect_remote_source", "retry_with_backoff", "choose_different_source"}
		}
		return skillInstallOutput{}, failure
	}
	result, err := skills.InstallSkillPackage(skills.InstallRequest{
		Package:          pkg,
		Scope:            target.scope,
		ProjectRoot:      target.projectRoot,
		DefaultWorkspace: target.defaultWorkspace,
		OverwritePolicy:  req.OverwritePolicy,
	})
	if err != nil {
		if errors.Is(err, skills.ErrSkillAlreadyExists) {
			return skillInstallOutput{}, &skillInstallFailure{
				code: "skill_install_already_exists", category: tool.ErrorCategoryDomain,
				retryable: false, recovery: []string{"keep_existing_skill", "request_reviewed_replacement"},
				underlying: fmt.Errorf("skill already exists; set overwrite_policy to replace to replace it: %w", err),
			}
		}
		if errors.Is(err, skills.ErrSkillReplaceTrashUnavailable) {
			return skillInstallOutput{}, &skillInstallFailure{
				code: "skill_install_trash_unavailable", category: tool.ErrorCategoryUnsupported,
				retryable: false, recovery: []string{"inspect_existing_and_staged_packages", "restore_operating_system_trash", "issue_fresh_replacement", "do_not_retry_blindly"}, underlying: err,
			}
		}
		return skillInstallOutput{}, &skillInstallFailure{
			code: "skill_install_failed", category: tool.ErrorCategoryTool,
			retryable: false, recovery: []string{"fix_skill_package", "verify_target_permissions"}, underlying: err,
		}
	}

	reload := true
	if req.ReloadAfterInstall != nil {
		reload = *req.ReloadAfterInstall
	}
	if reload {
		t.registry.ReloadLocalSources(skills.LocalSourceDirs(target.projectRoot))
	}
	overwritePolicy := strings.TrimSpace(req.OverwritePolicy)
	if overwritePolicy == "" {
		overwritePolicy = "abort"
	}

	return skillInstallOutput{
		Status:          "installed",
		ID:              result.Skill.ID,
		Name:            result.Skill.Name,
		Scope:           result.Scope,
		SourceOrigin:    string(result.Skill.Source),
		RootDir:         result.Skill.RootDir,
		SkillFilePath:   result.Skill.SkillFilePath,
		ProjectPath:     result.Skill.ProjectPath,
		State:           string(result.Skill.State),
		Reloaded:        reload,
		OverwritePolicy: overwritePolicy,
		Message:         result.Message,
		PreviousPackageTrashedPath: func() string {
			if result.PreviousPackageTrash == nil {
				return ""
			}
			return result.PreviousPackageTrash.TrashedPath
		}(),
	}, nil
}

func skillInstallFailureOutcome(err error) tool.ExecutionOutcome {
	var failure *skillInstallFailure
	if !errors.As(err, &failure) || failure == nil {
		failure = &skillInstallFailure{
			code: "skill_install_failed", category: tool.ErrorCategoryTool,
			retryable: false, recovery: []string{"inspect_skill_install_failure"}, underlying: err,
		}
	}
	outcome := tool.FailedOutcome(failure.Error(), failure.code, failure.category, failure.retryable)
	outcome.RecoveryOptions = append([]string(nil), failure.recovery...)
	outcome.Operation = &tool.OperationDescriptor{Type: SkillInstallToolName, SideEffectState: tool.OutcomeSideEffectUnknown}
	switch failure.code {
	case "skill_install_cancelled":
		outcome.Status = tool.OutcomeCancelled
		outcome.TransportStatus = tool.TransportCancelled
		outcome.Operation.SideEffectState = tool.OutcomeSideEffectNone
	case "skill_install_invalid_input", "skill_install_source_invalid", "skill_install_already_exists":
		outcome.TransportStatus = tool.TransportNotStarted
		outcome.Operation.SideEffectState = tool.OutcomeSideEffectNotStarted
	case "skill_install_target_invalid":
		outcome.Status = tool.OutcomeDenied
		outcome.TransportStatus = tool.TransportNotStarted
		outcome.Operation.SideEffectState = tool.OutcomeSideEffectNotStarted
	case "skill_install_runtime_unavailable":
		outcome.Status = tool.OutcomeNeedsRuntime
		outcome.TransportStatus = tool.TransportNotStarted
		outcome.Operation.SideEffectState = tool.OutcomeSideEffectNotStarted
	case "skill_install_source_unavailable":
		// A remote read may have been attempted, but installation never began.
		outcome.Operation.SideEffectState = tool.OutcomeSideEffectNone
	case "skill_install_trash_unavailable":
		outcome.Status = tool.OutcomeUnsupported
	}
	return outcome
}

func (t *SkillInstallTool) packageFromInput(ctx context.Context, req skillInstallInput) (skills.SkillPackage, error) {
	switch normalizeSourceType(req.SourceType) {
	case "content":
		if req.Content == "" {
			return skills.SkillPackage{}, invalidSkillInstallSource(fmt.Errorf("content is required"))
		}
		pkg, err := skills.PackageFromBytes(defaultSkillFileName(req.FileName), []byte(req.Content))
		if err != nil {
			return skills.SkillPackage{}, invalidSkillInstallSource(err)
		}
		return pkg, nil
	case "content_base64":
		if strings.TrimSpace(req.ContentBase64) == "" {
			return skills.SkillPackage{}, invalidSkillInstallSource(fmt.Errorf("content_base64 is required"))
		}
		data, err := base64.StdEncoding.DecodeString(req.ContentBase64)
		if err != nil {
			return skills.SkillPackage{}, invalidSkillInstallSource(fmt.Errorf("decode content_base64: %w", err))
		}
		pkg, err := skills.PackageFromBytes(defaultSkillFileName(req.FileName), data)
		if err != nil {
			return skills.SkillPackage{}, invalidSkillInstallSource(err)
		}
		return pkg, nil
	case "local_path":
		if strings.TrimSpace(req.Path) == "" {
			return skills.SkillPackage{}, invalidSkillInstallSource(fmt.Errorf("path is required"))
		}
		resolved, err := tool.PermissionContextFromContext(ctx).ResolveReadablePathChecked(req.Path)
		if err != nil {
			return skills.SkillPackage{}, invalidSkillInstallSource(err)
		}
		pkg, err := skills.PackageFromPath(resolved)
		if err != nil {
			return skills.SkillPackage{}, invalidSkillInstallSource(err)
		}
		return pkg, nil
	case "remote_url":
		return fetchRemoteSkillPackage(ctx, req)
	case "remote_git":
		return cloneRemoteGitSkillPackage(ctx, req)
	default:
		return skills.SkillPackage{}, invalidSkillInstallSource(fmt.Errorf("unsupported source_type %q", req.SourceType))
	}
}

type skillInstallTarget struct {
	scope            string
	projectRoot      string
	defaultWorkspace string
	installDir       string
}

func resolveSkillInstallTargetForTool(req skillInstallInput, permissionContext tool.PermissionContext) (skillInstallTarget, error) {
	normalized := permissionContext.Normalize()
	defaultWorkspace := normalized.WorkspaceRootOrCWD()
	projectRoot := strings.TrimSpace(req.ProjectRoot)
	if projectRoot != "" {
		projectRoot = normalized.ResolvePath(projectRoot)
		resolvedProjectRoot, err := filepath.EvalSymlinks(projectRoot)
		if err != nil || !normalized.IsWithinWorkspaceResolved(resolvedProjectRoot) {
			return skillInstallTarget{}, fmt.Errorf("project_root must be inside the current workspace")
		}
		// Pass the authorized real path downstream so a later change to the
		// caller-supplied symlink cannot redirect the install target.
		projectRoot = resolvedProjectRoot
	}
	scope := normalizeSkillInstallScope(req.Scope)
	resolvedScope, resolvedProjectRoot, installDir, _, err := skills.ResolveInstallTarget(skills.InstallTargetRequest{
		Scope:            scope,
		ProjectRoot:      projectRoot,
		DefaultWorkspace: defaultWorkspace,
	})
	if err != nil {
		return skillInstallTarget{}, err
	}
	if resolvedScope == string(skills.SourceProject) && !projectSkillInstallDirAllowed(installDir, normalized) {
		return skillInstallTarget{}, fmt.Errorf("skill install directory must remain inside the current workspace and must not escape through symlinks")
	}
	return skillInstallTarget{
		scope:            resolvedScope,
		projectRoot:      resolvedProjectRoot,
		defaultWorkspace: defaultWorkspace,
		installDir:       installDir,
	}, nil
}

func projectSkillInstallDirAllowed(installDir string, permissionContext tool.PermissionContext) bool {
	candidate := filepath.Clean(installDir)
	for {
		if _, err := os.Lstat(candidate); err == nil {
			resolved, err := filepath.EvalSymlinks(candidate)
			return err == nil && permissionContext.IsWithinWorkspaceResolved(resolved)
		} else if !os.IsNotExist(err) {
			return false
		}
		parent := filepath.Dir(candidate)
		if parent == candidate {
			return false
		}
		candidate = parent
	}
}

func parseSkillInstallInput(input string) (skillInstallInput, error) {
	var req skillInstallInput
	if err := json.Unmarshal([]byte(strings.TrimSpace(input)), &req); err != nil {
		return req, fmt.Errorf("arguments must be a JSON object: %w", err)
	}
	if normalizeSourceType(req.SourceType) == "" {
		return req, fmt.Errorf("source_type is required")
	}
	return req, nil
}

func normalizeSourceType(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func normalizeSkillInstallScope(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", string(skills.SourceProject):
		return string(skills.SourceProject)
	case "global", string(skills.SourceUser):
		return "global"
	default:
		return strings.TrimSpace(raw)
	}
}

func defaultSkillFileName(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return "SKILL.md"
	}
	return strings.TrimSpace(raw)
}

func fetchRemoteSkillPackage(ctx context.Context, req skillInstallInput) (skills.SkillPackage, error) {
	rawURL := strings.TrimSpace(req.URL)
	if rawURL == "" {
		return skills.SkillPackage{}, invalidSkillInstallSource(fmt.Errorf("url is required"))
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return skills.SkillPackage{}, invalidSkillInstallSource(fmt.Errorf("parse url: %w", err))
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return skills.SkillPackage{}, invalidSkillInstallSource(fmt.Errorf("remote_url only supports http or https URLs"))
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return skills.SkillPackage{}, invalidSkillInstallSource(err)
	}
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return skills.SkillPackage{}, unavailableSkillInstallSource(fmt.Errorf("download skill package: %w", err))
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		err := fmt.Errorf("download skill package: unexpected HTTP status %s", resp.Status)
		if resp.StatusCode == http.StatusRequestTimeout || resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			return skills.SkillPackage{}, unavailableSkillInstallSource(err)
		}
		return skills.SkillPackage{}, invalidSkillInstallSource(err)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, skills.MaxSkillPackageBytes+1))
	if err != nil {
		return skills.SkillPackage{}, unavailableSkillInstallSource(fmt.Errorf("read remote package: %w", err))
	}
	if len(data) > skills.MaxSkillPackageBytes {
		return skills.SkillPackage{}, invalidSkillInstallSource(fmt.Errorf("remote package exceeds %d bytes", skills.MaxSkillPackageBytes))
	}
	fileName := defaultRemoteFileName(req.FileName, parsed.Path)
	pkg, err := skills.PackageFromBytes(fileName, data)
	if err != nil {
		return skills.SkillPackage{}, invalidSkillInstallSource(err)
	}
	return pkg, nil
}

func defaultRemoteFileName(rawFileName string, rawPath string) string {
	if strings.TrimSpace(rawFileName) != "" {
		return strings.TrimSpace(rawFileName)
	}
	base := path.Base(rawPath)
	if base == "." || base == "/" || strings.TrimSpace(base) == "" {
		return "SKILL.md"
	}
	return base
}
