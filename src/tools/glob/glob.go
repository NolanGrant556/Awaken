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
	return "List files using the run-pinned ripgrep executable with --hidden and --no-ignore. Results are stably path-sorted and capped at 100 while reporting the true total and truncation."
}
func (t *Tool) Parameters() []tool.ToolParameter {
	return []tool.ToolParameter{
		{Name: "pattern", Description: "ripgrep glob pattern, for example **/*.go.", Type: "string", Required: true},
		{Name: "path", Description: "Readable search directory. Defaults to the thread cwd.", Type: "string"},
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
	root := permissionContext.WorkspaceRootOrCWD()
	if state != nil {
		root = state.CWD(root)
	}
	if strings.TrimSpace(p.Path) != "" {
		root, err = permissionContext.ResolveReadablePathChecked(p.Path)
	} else {
		root, err = permissionContext.ResolveReadablePathChecked(root)
	}
	if err != nil {
		return tool.DeniedOutcome(err.Error(), tool.ErrorCodeToolDenied).WithTiming(started, time.Now().UTC()), nil
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return tool.FailedOutcome(fmt.Sprintf("Glob path is not a readable directory: %s", root), "glob_path_unavailable", tool.ErrorCategoryDomain, false).WithTiming(started, time.Now().UTC()), nil
	}
	runtime, err := rgexec.Resolve(ctx)
	if err != nil {
		return needsRuntime(started, err.Error()), nil
	}
	command := runtime.Command(ctx, root, "--files", "--hidden", "--no-ignore", "--glob", p.Pattern, ".")
	stdout, err := command.StdoutPipe()
	if err != nil {
		return tool.ExecutionOutcome{}, err
	}
	var stderr strings.Builder
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		return tool.FailedOutcome(err.Error(), "glob_failed", tool.ErrorCategoryProcess, true).WithTiming(started, time.Now().UTC()), nil
	}
	total := 0
	kept := make([]string, 0, defaultMaximum)
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		total++
		candidate := filepath.ToSlash(strings.TrimPrefix(scanner.Text(), "."+string(filepath.Separator)))
		kept = insertSmallest(kept, candidate, defaultMaximum)
	}
	scanErr := scanner.Err()
	waitErr := command.Wait()
	if scanErr != nil {
		return tool.FailedOutcome(scanErr.Error(), "glob_failed", tool.ErrorCategoryProcess, true).WithTiming(started, time.Now().UTC()), nil
	}
	if waitErr != nil {
		return tool.FailedOutcome(strings.TrimSpace(stderr.String()), "glob_failed", tool.ErrorCategoryProcess, true).WithTiming(started, time.Now().UTC()), nil
	}
	sort.Strings(kept)
	truncated := total > len(kept)
	content := strings.Join(kept, "\n")
	if content == "" {
		content = "No files matched."
	}
	outcome := tool.SucceededOutcome(content).WithTiming(started, time.Now().UTC())
	outcome.Truncated = truncated
	outcome.Summary = fmt.Sprintf("Glob found %d file(s); returned %d.", total, len(kept))
	outcome.Details = map[string]any{"path": root, "pattern": p.Pattern, "paths": kept, "returned": len(kept), "total": total, "truncated": truncated, "environment_snapshot_id": runtime.SnapshotID}
	return outcome, nil
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
