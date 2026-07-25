package glob

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"awaken/src/tool"
	"awaken/src/tools/internal/rgexec"
	"awaken/src/tools/internal/searchroots"
)

const (
	ToolName       = "Glob"
	defaultMaximum = 100
)

type Tool struct{}
type params struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path,omitempty"`
}

func (t *Tool) Name() string { return ToolName }
func (t *Tool) Description() string {
	return "List files using the run-pinned ripgrep executable with --hidden and --no-ignore. Without an explicit path the search covers every readable root of the task and returns absolute paths; with one root the paths stay root-relative. An explicit path is listed wherever it points, including outside those roots. Results are stably path-sorted and capped at 100 while reporting the true total and truncation."
}
func (t *Tool) Parameters() []tool.ToolParameter {
	return []tool.ToolParameter{
		{Name: "pattern", Description: "ripgrep glob pattern, for example **/*.go.", Type: "string", Required: true},
		{Name: "path", Description: "Readable search directory. Omit to search every readable root of the task, starting at the thread cwd.", Type: "string"},
	}
}
func (t *Tool) IsReadOnly() bool { return true }
func (t *Tool) Definition() tool.ToolDefinition {
	return tool.BuildBuiltinDefinition(t, tool.ToolDefinition{
		Name: ToolName, DisplayName: "Glob", Description: t.Description(), Purpose: "Discover files with the exact pinned rg runtime and stable bounded results.",
		WhenToUse:    []string{"Finding files by name or extension before Read or Grep", "Enumerating hidden or ignored task files intentionally"},
		WhenNotToUse: []string{"Use Grep for content search", "Use Read when the exact path is known"},
		SideEffects:  []tool.ToolSideEffect{{Kind: tool.SideEffectRead, Resources: []string{"directory entries"}, Description: "Enumerates paths only.", Reversibility: tool.ReversibilityNotApplicable}},
		Examples:     []tool.ToolExample{{Scenario: "Find Go sources", Input: map[string]any{"pattern": "**/*.go", "path": "."}, Outcome: "Returns at most 100 sorted paths plus total_matches and truncated."}},
		FailureModes: []tool.ToolFailureMode{{Code: tool.ErrorCodeNeedsRuntime, Cause: "The pinned run has no rg executable", Retryable: true}, {Code: "glob_failed", Cause: "rg rejected the pattern or could not scan the path", Retryable: true}},
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
	roots, err := searchroots.Resolve(permissionContext, cwd, p.Path)
	if err != nil {
		return tool.DeniedOutcome(err.Error(), tool.ErrorCodeToolDenied).WithTiming(started, time.Now().UTC()), nil
	}
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
	kept := make([]string, 0, defaultMaximum)
	for _, root := range roots {
		rootTotal, rootKept, failure := globRoot(ctx, started, runtime, root, p.Pattern, multiRoot, kept)
		if failure != nil {
			return *failure, nil
		}
		total += rootTotal
		kept = rootKept
	}
	sort.Strings(kept)
	truncated := total > len(kept)
	content := strings.Join(kept, "\n")
	if content == "" {
		content = "No files matched."
	}
	outcome := tool.SucceededOutcome(content).WithTiming(started, time.Now().UTC())
	outcome.Truncated = truncated
	outcome.Summary = fmt.Sprintf("Glob found %d file(s) across %d root(s); returned %d.", total, len(roots), len(kept))
	pathStyle := "root_relative"
	if multiRoot {
		pathStyle = "absolute"
	}
	outcome.Details = map[string]any{"path": roots[0], "roots": roots, "path_style": pathStyle, "pattern": p.Pattern, "paths": kept, "returned": len(kept), "total": total, "truncated": truncated, "environment_snapshot_id": runtime.SnapshotID}
	return outcome, nil
}

func globRoot(ctx context.Context, started time.Time, runtime rgexec.Runtime, root, pattern string, multiRoot bool, kept []string) (int, []string, *tool.ExecutionOutcome) {
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
		kept = insertSmallest(kept, searchroots.DisplayPath(root, relative, multiRoot), defaultMaximum)
	}
	scanErr := scanner.Err()
	waitErr := command.Wait()
	if scanErr != nil {
		failure := tool.FailedOutcome(scanErr.Error(), "glob_failed", tool.ErrorCategoryProcess, true).WithTiming(started, time.Now().UTC())
		return 0, kept, &failure
	}
	if waitErr != nil {
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
	outcome.RecoveryOptions = []string{"install_ripgrep", "refresh_run_environment"}
	return outcome.WithTiming(started, time.Now().UTC())
}

func init() { tool.MustRegisterToolWithMode(&Tool{}, tool.DefinitionValidationStrictBuiltin) }
