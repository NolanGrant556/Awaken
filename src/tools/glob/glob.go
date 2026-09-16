package glob

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"awaken/src/tool"
	"awaken/src/tools/internal/rgexec"
	"awaken/src/tools/internal/searchroots"
)

const (
	ToolName      = "Glob"
	defaultLimit  = 100
	maximumLimit  = 1000
	maximumOffset = 1_000_000
	cursorPrefix  = "glob-v1:"
)

type Tool struct{}
type params struct {
	Pattern   string `json:"pattern"`
	Path      string `json:"path,omitempty"`
	ScopeMode string `json:"scope_mode,omitempty"`
	Limit     int    `json:"limit,omitempty"`
	Cursor    string `json:"cursor,omitempty"`
	Offset    int    `json:"-"`
}

func (t *Tool) Name() string { return ToolName }
func (t *Tool) Description() string {
	return "List files with the run-pinned ripgrep executable. The default scope_mode=project keeps 'this project' on the exact Host-provided workspace roots; omit path to cover every root. Project mode rejects a common-parent or outside path without changing filesystem permissions. Use scope_mode=explicit_path only when the user explicitly names a path outside the project. Results are stably sorted and paginated with limit/cursor."
}
func (t *Tool) Parameters() []tool.ToolParameter {
	return []tool.ToolParameter{
		{Name: "pattern", Description: "ripgrep glob pattern, for example **/*.go.", Type: "string", Required: true},
		{Name: "path", Description: "Search directory. Omit for the exact project roots. A project-internal subdirectory is allowed in project mode.", Type: "string"},
		{Name: "scope_mode", Description: "project (default) or explicit_path. Use explicit_path only for a user-named path outside the project.", Type: "string"},
		{Name: "limit", Description: "Maximum returned paths, default 100 and maximum 1000.", Type: "integer"},
		{Name: "cursor", Description: "Continuation cursor returned by the previous Glob call.", Type: "string"},
	}
}
func (t *Tool) IsReadOnly() bool { return true }
func (t *Tool) Definition() tool.ToolDefinition {
	return tool.BuildBuiltinDefinition(t, tool.ToolDefinition{
		Name: ToolName, DisplayName: "Glob", Description: t.Description(), Purpose: "Discover files with the exact pinned rg runtime and stable bounded results.",
		WhenToUse:    []string{"Finding files by name or extension before Read or Grep", "Enumerating hidden or ignored task files intentionally"},
		WhenNotToUse: []string{"Use Grep for content search", "Use Read when the exact path is known"},
		SideEffects:  []tool.ToolSideEffect{{Kind: tool.SideEffectRead, Resources: []string{"directory entries"}, Description: "Enumerates paths only.", Reversibility: tool.ReversibilityNotApplicable}},
		Examples:     []tool.ToolExample{{Scenario: "Find project Go sources", Input: map[string]any{"pattern": "**/*.go"}, Outcome: "Returns the first sorted page across the exact project roots and a next_cursor when more paths exist."}},
		FailureModes: []tool.ToolFailureMode{{Code: tool.ErrorCodeNeedsRuntime, Cause: "The pinned run has no rg executable", Retryable: true}, {Code: searchroots.ErrorCodeProjectScopePathMismatch, Cause: "Project mode received a common-parent or outside path", Retryable: true}, {Code: "glob_failed", Cause: "rg rejected the pattern or could not scan the path", Retryable: true}},
		OutputSchema: tool.ToolParam{Type: "object", Description: "Matched paths, total count, truncation, and runtime snapshot details.", AdditionalProperties: tool.BoolPtr(true)},
		Annotations:  tool.ToolAnnotations{ReadOnly: true, CapabilityClass: tool.CapabilityArtifactRead, TrustedOrigin: tool.TrustedOriginBuiltin},
	})
}

func (t *Tool) CheckPermission(_ context.Context, input string, permissionContext tool.PermissionContext) tool.PermissionDecision {
	p, err := parseParams(input)
	if err != nil {
		return tool.DenyPermission(err.Error())
	}
	if strings.TrimSpace(p.Path) != "" {
		if _, err := permissionContext.ResolveReadablePathChecked(p.Path); err != nil {
			return tool.DenyPermission(err.Error())
		}
	}
	return tool.AllowPermission()
}

func (t *Tool) Execute(ctx context.Context, input string) (string, error) {
	outcome, err := t.ExecuteOutcome(ctx, input, nil)
	if err != nil {
		return "", err
	}
	if outcome.IsError() {
		return outcome.Content, fmt.Errorf("%s", outcome.ErrorMessage)
	}
	return outcome.Content, nil
}

func (t *Tool) ExecuteOutcome(ctx context.Context, input string, _ tool.ToolObserver) (tool.ExecutionOutcome, error) {
	started := time.Now().UTC()
	p, err := parseParams(input)
	if err != nil {
		return tool.FailedOutcome(err.Error(), "glob_invalid_input", tool.ErrorCategoryValidation, false).WithTiming(started, time.Now().UTC()), nil
	}
	permissionContext := tool.PermissionContextFromContext(ctx)
	state, _ := tool.ToolSessionStateFromContext(ctx)
	cwd := permissionContext.WorkspaceRootOrCWD()
	if state != nil {
		cwd = state.CWD(cwd)
	}
	resolution, err := searchroots.Resolve(permissionContext, cwd, p.Path, p.ScopeMode)
	if err != nil {
		if scopeErr, ok := err.(*searchroots.ScopeError); ok {
			return tool.FailedOutcome(scopeErr.Message, scopeErr.Code, tool.ErrorCategoryValidation, true).WithTiming(started, time.Now().UTC()), nil
		}
		return tool.DeniedOutcome(err.Error(), tool.ErrorCodeToolDenied).WithTiming(started, time.Now().UTC()), nil
	}
	roots := resolution.Roots
	for _, root := range roots {
		info, statErr := os.Stat(root)
		if statErr != nil || !info.IsDir() {
			return tool.FailedOutcome(fmt.Sprintf("Glob path is not a readable directory: %s", root), "glob_path_unavailable", tool.ErrorCategoryDomain, false).WithTiming(started, time.Now().UTC()), nil
		}
	}
	runtime, err := rgexec.Resolve(ctx)
	if err != nil {
		return needsRuntime(started, err.Error()), nil
	}
	multiRoot := len(roots) > 1
	total := 0
	keepMaximum := p.Offset + p.Limit
	kept := make([]string, 0, keepMaximum)
	for _, root := range roots {
		rootTotal, rootKept, failure := globRoot(ctx, started, runtime, root, p.Pattern, multiRoot, kept, keepMaximum)
		if failure != nil {
			return *failure, nil
		}
		total += rootTotal
		kept = rootKept
	}
	sort.Strings(kept)
	pageStart := p.Offset
	if pageStart > len(kept) {
		pageStart = len(kept)
	}
	pageEnd := pageStart + p.Limit
	if pageEnd > len(kept) {
		pageEnd = len(kept)
	}
	selected := append([]string(nil), kept[pageStart:pageEnd]...)
	truncated := pageEnd < total
	nextCursor := ""
	if truncated {
		nextCursor = fmt.Sprintf("%s%d", cursorPrefix, pageEnd)
	}
	content := strings.Join(selected, "\n")
	if content == "" {
		content = "No files matched."
	}
	outcome := tool.SucceededOutcome(content).WithTiming(started, time.Now().UTC())
	outcome.Truncated = truncated
	outcome.Summary = fmt.Sprintf("Glob found %d file(s) across %d root(s); returned %d from offset %d.", total, len(roots), len(selected), p.Offset)
	pathStyle := "root_relative"
	if multiRoot {
		pathStyle = "absolute"
	}
	outcome.Details = map[string]any{"path": roots[0], "roots": roots, "path_style": pathStyle, "pattern": p.Pattern, "paths": selected, "returned": len(selected), "total": total, "limit": p.Limit, "cursor": p.Cursor, "next_cursor": nextCursor, "truncated": truncated, "scope_mode": resolution.ScopeMode, "scope_relation": resolution.ScopeRelation, "scope_source": resolution.ScopeSource, "environment_snapshot_id": runtime.SnapshotID}
	return outcome, nil
}

func globRoot(ctx context.Context, started time.Time, runtime rgexec.Runtime, root, pattern string, multiRoot bool, kept []string, maximum int) (int, []string, *tool.ExecutionOutcome) {
	command := runtime.Command(ctx, root, "--files", "--hidden", "--no-ignore", "--glob", pattern, ".")
	stdout, err := command.StdoutPipe()
	if err != nil {
		failure := tool.FailedOutcome(err.Error(), "glob_failed", tool.ErrorCategoryProcess, true).WithTiming(started, time.Now().UTC())
		return 0, kept, &failure
	}
	var stderr strings.Builder
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		failure := tool.FailedOutcome(err.Error(), "glob_failed", tool.ErrorCategoryProcess, true).WithTiming(started, time.Now().UTC())
		return 0, kept, &failure
	}
	total := 0
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		total++
		relative := strings.TrimPrefix(scanner.Text(), "."+string(filepath.Separator))
		kept = insertSmallest(kept, searchroots.DisplayPath(root, relative, multiRoot), maximum)
	}
	scanErr := scanner.Err()
	waitErr := command.Wait()
	if scanErr != nil {
		failure := tool.FailedOutcome(scanErr.Error(), "glob_failed", tool.ErrorCategoryProcess, true).WithTiming(started, time.Now().UTC())
		return 0, kept, &failure
	}
	// ripgrep uses exit code 1 to report a successful scan with no matches.
	// That is a normal result for one root, especially when a project contains
	// multiple folders, so only other non-zero exits are execution failures.
	if waitErr != nil && (command.ProcessState == nil || command.ProcessState.ExitCode() != 1) {
		failure := tool.FailedOutcome(strings.TrimSpace(stderr.String()), "glob_failed", tool.ErrorCategoryProcess, true).WithTiming(started, time.Now().UTC())
		return 0, kept, &failure
	}
	return total, kept, nil
}

func parseParams(input string) (params, error) {
	var p params
	decoder := json.NewDecoder(strings.NewReader(strings.TrimSpace(input)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&p); err != nil {
		return params{}, fmt.Errorf("invalid Glob input: %w", err)
	}
	p.Pattern = strings.TrimSpace(p.Pattern)
	if p.Pattern == "" {
		return params{}, fmt.Errorf("pattern is required")
	}
	if p.ScopeMode == "" {
		p.ScopeMode = searchroots.ScopeModeProject
	}
	if p.ScopeMode != searchroots.ScopeModeProject && p.ScopeMode != searchroots.ScopeModeExplicitPath {
		return params{}, fmt.Errorf("scope_mode must be project or explicit_path")
	}
	if p.Limit == 0 {
		p.Limit = defaultLimit
	}
	if p.Limit < 1 || p.Limit > maximumLimit {
		return params{}, fmt.Errorf("limit must be between 1 and %d", maximumLimit)
	}
	if p.Cursor != "" {
		if !strings.HasPrefix(p.Cursor, cursorPrefix) {
			return params{}, fmt.Errorf("cursor is invalid")
		}
		offset, cursorErr := strconv.Atoi(strings.TrimPrefix(p.Cursor, cursorPrefix))
		if cursorErr != nil || offset < 0 || offset > maximumOffset {
			return params{}, fmt.Errorf("cursor is invalid")
		}
		p.Offset = offset
	}
	if p.Offset > maximumOffset-p.Limit {
		return params{}, fmt.Errorf("cursor is out of range")
	}
	return p, nil
}

func insertSmallest(values []string, candidate string, maximum int) []string {
	index := sort.SearchStrings(values, candidate)
	if index < len(values) && values[index] == candidate {
		return values
	}
	if len(values) >= maximum && index >= maximum {
		return values
	}
	values = append(values, "")
	copy(values[index+1:], values[index:])
	values[index] = candidate
	if len(values) > maximum {
		values = values[:maximum]
	}
	return values
}

func needsRuntime(started time.Time, message string) tool.ExecutionOutcome {
	outcome := tool.FailedOutcome(message, tool.ErrorCodeNeedsRuntime, tool.ErrorCategoryRuntime, true)
	outcome.Status = tool.OutcomeNeedsRuntime
	outcome.RecoveryOptions = []string{"ensure_environment:ripgrep"}
	outcome.NextActions = []tool.NextAction{{ID: "prepare_environment", Description: "准备所需共享环境后重试原工具", Tool: "ensure_environment", InputHint: map[string]any{"capability": "ripgrep"}}}
	return outcome.WithTiming(started, time.Now().UTC())
}

func init() { tool.MustRegisterToolWithMode(&Tool{}, tool.DefinitionValidationStrictBuiltin) }
