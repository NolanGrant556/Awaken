package plugintool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"awaken/src/plugins"
	"awaken/src/skills"
	"awaken/src/tool"
)

const ToolName = "plugin"

type sourceInput struct {
	Kind    string `json:"kind"`
	Path    string `json:"path,omitempty"`
	URL     string `json:"url,omitempty"`
	Ref     string `json:"ref,omitempty"`
	Subpath string `json:"subpath,omitempty"`
}

type input struct {
	Action         string       `json:"action"`
	Name           string       `json:"name,omitempty"`
	ExpectedDigest string       `json:"expected_digest,omitempty"`
	Destination    string       `json:"destination,omitempty"`
	Source         *sourceInput `json:"source,omitempty"`
	Replace        bool         `json:"replace,omitempty"`
}

type result struct {
	Action              string               `json:"action"`
	Valid               bool                 `json:"valid"`
	Installed           bool                 `json:"installed"`
	Exported            bool                 `json:"exported"`
	Replaced            bool                 `json:"replaced"`
	Name                string               `json:"name"`
	Version             string               `json:"version,omitempty"`
	Description         string               `json:"description,omitempty"`
	RootDir             string               `json:"root_dir,omitempty"`
	Digest              string               `json:"digest,omitempty"`
	PreviousDigest      string               `json:"previous_digest,omitempty"`
	Changed             bool                 `json:"changed"`
	SkillCount          int                  `json:"skill_count"`
	ServerCount         int                  `json:"server_count"`
	Diagnostics         []plugins.Diagnostic `json:"diagnostics"`
	Message             string               `json:"message"`
	PreviousTrashedPath string               `json:"previous_trashed_path,omitempty"`
}

type Tool struct{ store *plugins.Store }

var defaultTool = &Tool{}

func New(store *plugins.Store) *Tool { return &Tool{store: store} }

func SetStore(store *plugins.Store) { defaultTool.store = store }

func (t *Tool) Name() string { return ToolName }

func (t *Tool) Description() string {
	return "Inspect or export installed Awaken Plugins, commit an authorized and validated in-place package edit with digest compare-and-swap, validate authored Agent Plugins 1.0.0 packages, or install a digest-pinned local/HTTPS package with recoverable replacement."
}

func (t *Tool) Parameters() []tool.ToolParameter { return nil }

func (t *Tool) InputSchema() tool.ToolParametersSchema {
	closed := false
	return tool.ToolParametersSchema{
		Type: "object", AdditionalProperties: &closed, Required: []string{"action"},
		Properties: map[string]tool.ToolParam{
			"action":          {Type: "string", Enum: []any{"inspect", "export", "commit", "validate", "install"}, Description: "Plugin package operation."},
			"name":            {Type: "string", Description: "Installed plugin name for inspect, export, or commit."},
			"expected_digest": {Type: "string", Description: "Recorded installed-package digest observed before an authorized in-place edit; required for commit compare-and-swap."},
			"destination":     {Type: "string", Description: "Previously absent authoring directory for export; relative paths resolve from the task workspace."},
			"source": {Type: "object", AdditionalProperties: &closed, Required: []string{"kind"}, Properties: map[string]tool.ToolParam{
				"kind":    {Type: "string", Enum: []any{"local_path", "remote_url", "remote_git"}, Description: "Package source adapter."},
				"path":    {Type: "string", Description: "Local plugin directory or zip path."},
				"url":     {Type: "string", Description: "HTTPS zip URL or HTTPS Git repository URL without inline credentials."},
				"ref":     {Type: "string", Description: "Optional safe Git ref."},
				"subpath": {Type: "string", Description: "Optional safe relative plugin subdirectory inside the Git repository."},
			}, Description: "Source to validate or install."},
			"replace": {Type: "boolean", Description: "Replace the same-name installed package after validation, moving the previous package to operating-system Trash. Defaults to false."},
		},
	}
}

func (t *Tool) Definition() tool.ToolDefinition {
	return tool.BuildBuiltinDefinition(t, tool.ToolDefinition{
		Name: ToolName, DisplayName: "Plugin", Description: t.Description(),
		Purpose: "Give the Agent a typed lifecycle boundary for inspecting, exporting, validating, installing, safely replacing, and atomically accepting authorized in-place changes to Awaken Agent Plugin packages.",
		WhenToUse: []string{
			"The user asks to create, validate, install, or modify an Awaken Plugin.",
			"Use inspect and export before modifying an installed plugin, then edit the complete candidate outside the live store, validate it, and install with replace=true only after review.",
			"When the user authorizes an in-place edit, retain the recorded digest observed before editing and use commit after the complete package edit so the runtime validates, accepts, and reloads the new contents.",
			"Use validate before every creator-authored install so manifest, package paths, embedded Skills, MCP configuration, and digest are checked by the runtime owners.",
		},
		WhenNotToUse: []string{
			"Do not use for an unrelated code package, browser extension, or editor extension.",
			"Do not directly edit plugins.json; recorded state is owned by this tool, the Desktop plugin page, and PluginStore.",
			"Do not commit an in-place edit that the user did not authorize, that is only partially written, or whose recorded digest was not captured before editing.",
			"Do not set replace=true unless the user asked to modify or replace the same installed plugin and the full candidate package was reviewed.",
		},
		InputSchema: t.InputSchema(),
		OutputSchema: tool.ToolParam{Type: "object", AdditionalProperties: tool.BoolPtr(false), Required: []string{
			"action", "valid", "installed", "exported", "replaced", "name", "skill_count", "server_count", "diagnostics", "message",
		}, Properties: map[string]tool.ToolParam{
			"action":                {Type: "string", Enum: []any{"inspect", "export", "commit", "validate", "install"}},
			"valid":                 {Type: "boolean"},
			"installed":             {Type: "boolean"},
			"exported":              {Type: "boolean"},
			"replaced":              {Type: "boolean"},
			"name":                  {Type: "string"},
			"version":               {Type: "string"},
			"description":           {Type: "string"},
			"root_dir":              {Type: "string"},
			"digest":                {Type: "string"},
			"previous_digest":       {Type: "string"},
			"changed":               {Type: "boolean"},
			"skill_count":           {Type: "integer"},
			"server_count":          {Type: "integer"},
			"diagnostics":           {Type: "array", Items: &tool.ToolParam{Type: "object", AdditionalProperties: tool.BoolPtr(true)}},
			"message":               {Type: "string"},
			"previous_trashed_path": {Type: "string"},
		}},
		SideEffects: []tool.ToolSideEffect{
			{Kind: tool.SideEffectRead, Resources: []string{"authored or installed Plugin package"}, Description: "Inspects or dry-run validates a bounded plugin package without executing package code.", Reversibility: tool.ReversibilityNotApplicable},
			{Kind: tool.SideEffectCreate, Resources: []string{"workspace Plugin authoring candidate"}, Description: "Exports a complete installed package to a new candidate directory without changing live state.", Reversibility: tool.ReversibilityReversible},
			{Kind: tool.SideEffectCreate, Resources: []string{"Awaken user Plugin package and plugins.json entry"}, Description: "Installs a validated Plugin into the Core-owned user package store.", Reversibility: tool.ReversibilityReversible},
			{Kind: tool.SideEffectUpdate, Resources: []string{"installed Plugin state and live Skill/MCP roots"}, Description: "Accepts a fully validated in-place package edit or replaces an installed package, preserves Plugin data, and refreshes the existing Skill/MCP runtimes.", Reversibility: tool.ReversibilityConditional},
			{Kind: tool.SideEffectDelete, Resources: []string{"previous Plugin package"}, Description: "Replacement moves the previous package to operating-system Trash after the new package and state are ready.", Reversibility: tool.ReversibilityReversible},
			{Kind: tool.SideEffectNetwork, Resources: []string{"declared HTTPS package source"}, Description: "Remote validation or installation reads only the declared HTTPS source.", Reversibility: tool.ReversibilityNotApplicable},
		},
		Examples: []tool.ToolExample{
			{Scenario: "Validate a creator-authored local plugin", Input: map[string]any{"action": "validate", "source": map[string]any{"kind": "local_path", "path": "output/plugin-authoring/acme-tools"}}, Outcome: "Returns valid, digest, component counts, and diagnostics without installing anything."},
			{Scenario: "Commit an authorized in-place plugin edit", Input: map[string]any{"action": "commit", "name": "acme-tools", "expected_digest": "sha256-v2:recorded-before-edit"}, Outcome: "Validates the complete installed tree, compare-and-swaps the recorded digest, refreshes live roots, and returns the accepted digest and component counts."},
			{Scenario: "Replace a reviewed installed plugin", Input: map[string]any{"action": "install", "source": map[string]any{"kind": "local_path", "path": "output/plugin-authoring/acme-tools"}, "replace": true}, Outcome: "Installs the validated package, preserves Plugin data, moves the previous package to Trash, and refreshes live roots."},
		},
		FailureModes: []tool.ToolFailureMode{
			{Code: "plugin_invalid_input", Cause: "The action, name, or source-specific fields are invalid.", Retryable: false, Recovery: []string{"Correct the structured plugin request."}},
			{Code: "plugin_store_unavailable", Cause: "The protected Plugin store is unavailable or corrupt.", Retryable: true, Recovery: []string{"Restart Awaken or repair the store before retrying."}},
			{Code: "plugin_source_invalid", Cause: "The package source, installed tree, archive, manifest, component, or path is invalid.", Retryable: false, Recovery: []string{"Fix the complete candidate or authorized installed package, then validate or commit again."}},
			{Code: "plugin_source_unavailable", Cause: "A declared remote package source could not be fetched.", Retryable: true, Recovery: []string{"Inspect the remote source and retry with bounded backoff."}},
			{Code: "plugin_package_invalid", Cause: "Dry-run validation found component-level errors; installation did not start.", Retryable: false, Recovery: []string{"Resolve every error diagnostic and validate the candidate again."}},
			{Code: "plugin_source_changed", Cause: "The source or installed tree changed while a validation or commit snapshot was being verified.", Retryable: false, Recovery: []string{"Stop concurrent writes, inspect the complete package again, and retry from a fresh stable snapshot."}},
			{Code: "plugin_digest_changed", Cause: "The installed package no longer matches its recorded digest.", Retryable: false, Recovery: []string{"If the complete change was authorized, commit it with the previously observed recorded digest; otherwise reinstall or replace it from a freshly validated source."}},
			{Code: "plugin_recorded_digest_changed", Cause: "The recorded digest changed after the caller observed it, so commit compare-and-swap was rejected.", Retryable: false, Recovery: []string{"Refresh Plugin state, inspect the new owner of the change, and never retry with the stale expected digest."}},
			{Code: "plugin_not_found", Cause: "The named installed plugin does not exist.", Retryable: false, Recovery: []string{"Refresh installed Plugin state and choose the exact current name."}},
			{Code: "plugin_already_exists", Cause: "A same-name plugin exists and replace was not explicitly requested.", Retryable: false, Recovery: []string{"Keep the existing plugin or request a reviewed replacement."}},
			{Code: "plugin_export_destination_exists", Cause: "The requested authoring candidate already exists.", Retryable: false, Recovery: []string{"Choose a new export destination or inspect the existing candidate before continuing."}},
			{Code: "plugin_trash_unavailable", Cause: "The previous package could not be moved to Trash and replacement was rolled back or stopped.", Retryable: false, Recovery: []string{"Restore operating-system Trash availability and issue a fresh replacement."}},
			{Code: "plugin_operation_failed", Cause: "The inspected or validated package could not be installed, committed, or encoded.", Retryable: false, Recovery: []string{"Inspect the returned error and current Plugin state before retrying."}},
		},
		Annotations: tool.ToolAnnotations{
			ReadOnly: false, Destructive: false, OpenWorld: true,
			CapabilityClass: tool.CapabilitySystemOperation, TrustedOrigin: tool.TrustedOriginBuiltin,
			FileBridgeScope: tool.FileBridgeTaskScopeWrite, NetworkScope: tool.NetworkScopeOpen,
		},
		Capabilities: []tool.Capability{tool.CapabilityFileRead, tool.CapabilityFileWrite, tool.CapabilityFileDelete},
	})
}

func init() { tool.MustRegisterToolWithMode(defaultTool, tool.DefinitionValidationStrictBuiltin) }

func (t *Tool) CheckPermission(ctx context.Context, raw string, permission tool.PermissionContext) tool.PermissionDecision {
	request, err := parseInput(raw)
	if err != nil {
		return tool.DenyPermission(err.Error())
	}
	if request.Source != nil && request.Source.Kind == string(plugins.SourceLocalPath) {
		if _, err := permission.ResolveReadablePathChecked(request.Source.Path); err != nil {
			return tool.DenyPermission(err.Error())
		}
	}
	if request.Action == "export" {
		if _, err := permission.ResolveWritablePathChecked(request.Destination); err != nil {
			return tool.DenyPermission(err.Error())
		}
	}
	return tool.AllowPermission()
}

func (t *Tool) Execute(ctx context.Context, raw string) (string, error) {
	value, err := t.execute(ctx, raw)
	if err != nil {
		return "", err
	}
	encoded, err := json.MarshalIndent(value, "", "  ")
	return string(encoded), err
}

func (t *Tool) ExecuteOutcome(ctx context.Context, raw string, _ tool.ToolObserver) (tool.ExecutionOutcome, error) {
	started := time.Now().UTC()
	value, err := t.execute(ctx, raw)
	if err != nil {
		code, category, retryable := classifyError(err)
		outcome := tool.FailedOutcome(err.Error(), code, category, retryable)
		outcome.Operation = &tool.OperationDescriptor{Type: ToolName, SideEffectState: tool.OutcomeSideEffectNotStarted}
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		outcome := tool.FailedOutcome(err.Error(), "plugin_operation_failed", tool.ErrorCategoryTool, false)
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}
	outcome := tool.SucceededOutcome(string(encoded))
	outcome.Summary = value.Message
	state := tool.OutcomeSideEffectNone
	if value.Action == "install" || value.Action == "export" || value.Action == "commit" {
		if value.Action != "commit" || value.Changed {
			state = tool.OutcomeSideEffectApplied
			outcome.AffectedPaths = []string{value.RootDir}
		}
		action := "created"
		kind := "plugin_package"
		if value.Action == "export" {
			kind = "plugin_authoring_candidate"
			outcome.NextActions = []tool.NextAction{{ID: "validate_exported_plugin", Description: "Edit the complete candidate, then validate it before replacement.", Tool: ToolName, Preconditions: []string{"The candidate retains every required package resource."}, InputHint: map[string]any{"action": "validate", "source": map[string]any{"kind": "local_path", "path": value.RootDir}}}}
		} else {
			if value.Replaced || value.Action == "commit" {
				action = "updated"
			}
			if value.Action != "commit" || value.Changed {
				outcome.NextActions = []tool.NextAction{{
					ID: "use_plugin_in_new_run", Description: "Start a new run before relying on the changed Plugin Skills or MCP tools.",
					Preconditions: []string{"The Plugin lifecycle operation completed and live roots were refreshed."},
				}}
			}
		}
		if value.Action != "commit" || value.Changed {
			outcome.Changes = []tool.ResourceChange{{
				Kind: kind, Action: action, Resource: value.RootDir, Summary: value.Message,
				Certainty: tool.ChangeObserved, Reversibility: tool.ChangeConditional,
			}}
		}
	}
	outcome.Operation = &tool.OperationDescriptor{Type: ToolName, Target: value.RootDir, SideEffectState: state}
	outcome.Details = resultDetails(value)
	return outcome.WithTiming(started, time.Now().UTC()), nil
}

func (t *Tool) execute(ctx context.Context, raw string) (result, error) {
	if err := ctx.Err(); err != nil {
		return result{}, err
	}
	if t == nil || t.store == nil {
		return result{}, fmt.Errorf("plugin_store_unavailable: Plugin store is not configured")
	}
	request, err := parseInput(raw)
	if err != nil {
		return result{}, fmt.Errorf("plugin_invalid_input: %w", err)
	}
	if request.Action == "inspect" {
		installed, err := t.store.Get(request.Name)
		if err != nil {
			return result{}, err
		}
		return resultFromInstalled("inspect", installed, false, false, "Inspected installed Plugin "+installed.Name), nil
	}
	if request.Action == "export" {
		destination, err := tool.PermissionContextFromContext(ctx).ResolveWritablePathChecked(request.Destination)
		if err != nil {
			return result{}, fmt.Errorf("plugin_source_invalid: export destination denied: %w", err)
		}
		installed, err := t.store.Export(request.Name, destination)
		if err != nil {
			return result{}, err
		}
		value := resultFromInstalled("export", installed, false, false, "Exported a complete Plugin authoring candidate without changing the installed package")
		value.Exported = true
		value.RootDir = destination
		return value, nil
	}
	if request.Action == "commit" {
		committed, err := t.store.CommitInstalledChanges(request.Name, request.ExpectedDigest)
		if err != nil {
			return result{}, err
		}
		value := resultFromInstalled("commit", committed.Plugin, false, false, committed.Message)
		value.Installed = true
		value.Changed = committed.Changed
		value.PreviousDigest = committed.PreviousDigest
		value.Digest = committed.Digest
		return value, nil
	}
	source := toSource(request.Source)
	if source.Kind == plugins.SourceLocalPath {
		resolved, err := tool.PermissionContextFromContext(ctx).ResolveReadablePathChecked(source.Path)
		if err != nil {
			return result{}, fmt.Errorf("plugin_source_invalid: %w", err)
		}
		source.Path = resolved
	}
	validation, err := t.store.Validate(ctx, source)
	if err != nil {
		return result{}, err
	}
	if request.Action == "validate" {
		return resultFromValidation(validation), nil
	}
	if !validation.Valid {
		return result{}, fmt.Errorf("plugin_package_invalid: validation found component errors; installation did not start")
	}
	installed, err := t.store.Install(ctx, plugins.InstallRequest{Source: source, Replace: request.Replace, ExpectedDigest: validation.Digest})
	if err != nil {
		return result{}, err
	}
	value := resultFromInstalled("install", installed.Plugin, true, installed.Replaced, installed.Message)
	value.PreviousTrashedPath = installed.PreviousTrashedPath
	return value, nil
}

func parseInput(raw string) (input, error) {
	var request input
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &request); err != nil {
		return request, fmt.Errorf("arguments must be a JSON object: %w", err)
	}
	request.Action = strings.ToLower(strings.TrimSpace(request.Action))
	switch request.Action {
	case "inspect":
		request.Name = strings.TrimSpace(request.Name)
		if request.Name == "" {
			return request, fmt.Errorf("name is required for inspect")
		}
	case "export":
		request.Name = strings.TrimSpace(request.Name)
		request.Destination = strings.TrimSpace(request.Destination)
		if request.Name == "" || request.Destination == "" {
			return request, fmt.Errorf("name and destination are required for export")
		}
	case "commit":
		request.Name = strings.TrimSpace(request.Name)
		request.ExpectedDigest = strings.TrimSpace(request.ExpectedDigest)
		if request.Name == "" || request.ExpectedDigest == "" {
			return request, fmt.Errorf("name and expected_digest are required for commit")
		}
	case "validate", "install":
		if request.Source == nil {
			return request, fmt.Errorf("source is required for %s", request.Action)
		}
		request.Source.Kind = strings.ToLower(strings.TrimSpace(request.Source.Kind))
		if request.Source.Kind == "" {
			return request, fmt.Errorf("source.kind is required")
		}
	default:
		return request, fmt.Errorf("unsupported action %q", request.Action)
	}
	return request, nil
}

func toSource(source *sourceInput) plugins.Source {
	if source == nil {
		return plugins.Source{}
	}
	return plugins.Source{
		Kind: plugins.SourceKind(source.Kind), Path: strings.TrimSpace(source.Path), URL: strings.TrimSpace(source.URL),
		Ref: strings.TrimSpace(source.Ref), Subpath: strings.TrimSpace(source.Subpath),
	}
}

func resultFromValidation(validation plugins.ValidationResult) result {
	message := "Plugin package is valid"
	if !validation.Valid {
		message = "Plugin package has validation errors"
	}
	return result{
		Action: "validate", Valid: validation.Valid, Installed: false, Exported: false, Replaced: false,
		Name: validation.Name, Version: validation.Version, Description: validation.Description,
		RootDir: validation.TargetRoot, Digest: validation.Digest, SkillCount: validation.SkillCount, ServerCount: validation.ServerCount,
		Diagnostics: nonNilDiagnostics(validation.Diagnostics), Message: message,
	}
}

func resultFromInstalled(action string, installed *plugins.InstalledPlugin, didInstall, replaced bool, message string) result {
	if installed == nil {
		return result{Action: action, Diagnostics: []plugins.Diagnostic{}, Message: message}
	}
	skillCount := 0
	if installed.SkillRoot != "" {
		for _, def := range skills.LoadDir(installed.SkillRoot, skills.SourcePlugin) {
			if def != nil && def.State == skills.StateLoaded {
				skillCount++
			}
		}
	}
	valid := true
	for _, diagnostic := range installed.Diagnostics {
		if diagnostic.Severity == "error" {
			valid = false
			break
		}
	}
	return result{
		Action: action, Valid: valid, Installed: didInstall || action == "inspect" || action == "export", Exported: false, Replaced: replaced,
		Name: installed.Name, Version: installed.Manifest.Version, Description: installed.Manifest.Description,
		RootDir: installed.Root, Digest: installed.Digest, SkillCount: skillCount, ServerCount: len(installed.MCPServers),
		Diagnostics: nonNilDiagnostics(installed.Diagnostics), Message: message,
	}
}

func nonNilDiagnostics(values []plugins.Diagnostic) []plugins.Diagnostic {
	if values == nil {
		return []plugins.Diagnostic{}
	}
	return values
}

func resultDetails(value result) map[string]any {
	return map[string]any{
		"action": value.Action, "valid": value.Valid, "installed": value.Installed, "exported": value.Exported, "replaced": value.Replaced,
		"name": value.Name, "version": value.Version, "description": value.Description,
		"root_dir": value.RootDir, "digest": value.Digest, "previous_digest": value.PreviousDigest, "changed": value.Changed, "skill_count": value.SkillCount,
		"server_count": value.ServerCount, "diagnostics": value.Diagnostics, "message": value.Message,
		"previous_trashed_path": value.PreviousTrashedPath,
	}
}

func classifyError(err error) (string, tool.ErrorCategory, bool) {
	message := err.Error()
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return "plugin_operation_cancelled", tool.ErrorCategoryCancelled, true
	case strings.Contains(message, "plugin_invalid_input"):
		return "plugin_invalid_input", tool.ErrorCategoryValidation, false
	case strings.Contains(message, "plugin_store_unavailable"), errors.Is(err, plugins.ErrStateCorrupt), errors.Is(err, plugins.ErrHomeRequired):
		return "plugin_store_unavailable", tool.ErrorCategoryRuntime, true
	case strings.Contains(message, "plugin_package_invalid"):
		return "plugin_package_invalid", tool.ErrorCategoryValidation, false
	case errors.Is(err, plugins.ErrSourceChanged):
		return "plugin_source_changed", tool.ErrorCategoryValidation, false
	case errors.Is(err, plugins.ErrDigestChanged):
		return "plugin_digest_changed", tool.ErrorCategoryDomain, false
	case errors.Is(err, plugins.ErrRecordedDigestChanged):
		return "plugin_recorded_digest_changed", tool.ErrorCategoryDomain, false
	case errors.Is(err, plugins.ErrNotFound):
		return "plugin_not_found", tool.ErrorCategoryDomain, false
	case errors.Is(err, plugins.ErrAlreadyExists):
		return "plugin_already_exists", tool.ErrorCategoryDomain, false
	case strings.Contains(message, "export destination already exists"):
		return "plugin_export_destination_exists", tool.ErrorCategoryDomain, false
	case errors.Is(err, plugins.ErrReplaceTrashUnavailable):
		return "plugin_trash_unavailable", tool.ErrorCategoryUnsupported, false
	case errors.Is(err, plugins.ErrUnsupportedSchema), errors.Is(err, plugins.ErrManifestInvalid), errors.Is(err, plugins.ErrMCPInvalid), errors.Is(err, plugins.ErrPathEscapes), strings.Contains(message, "plugin_source_invalid"):
		return "plugin_source_invalid", tool.ErrorCategoryValidation, false
	case strings.Contains(message, "download"), strings.Contains(message, "clone"), strings.Contains(message, "remote"):
		return "plugin_source_unavailable", tool.ErrorCategoryRuntime, true
	default:
		return "plugin_operation_failed", tool.ErrorCategoryTool, false
	}
}
